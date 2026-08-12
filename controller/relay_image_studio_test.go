package controller

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageStudioRelaySendsRetryableUpstreamFailureOnlyOnce(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.UserSubscription{}, &model.BonusBalance{}))
	service.InitHttpClient()

	originalRetryTimes := common.RetryTimes
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalErrorLogEnabled := constant.ErrorLogEnabled
	originalRatios := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		common.RetryTimes = originalRetryTimes
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		constant.ErrorLogEnabled = originalErrorLogEnabled
		ratioData, err := common.Marshal(originalRatios)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratioData)))
	})
	common.RetryTimes = 2
	common.MemoryCacheEnabled = false
	constant.ErrorLogEnabled = false
	ratios := ratio_setting.GetModelRatioCopy()
	ratios["gpt-image-2"] = 0
	ratioData, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratioData)))

	upstreamRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamRequests++
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		require.JSONEq(t, `{"model":"gpt-image-2","prompt":"safe prompt","n":1,"size":"1024x1024","quality":"low","response_format":"b64_json","background":"opaque","output_format":"png","stream":false}`, string(body))
		writer.Header().Set("Content-Encoding", "gzip")
		writer.Header().Set("Content-Length", "999")
		writer.Header().Set("Retry-After", "60")
		writer.Header().Set("X-Upstream-Trace", "upstream-trace")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"message":"upstream detail must not escape","type":"rate_limit"}}`))
	}))
	defer server.Close()

	insertModelListUser(t, db, 945, "image-studio-relay", "default")
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 945).Update("quota", 1).Error)
	autoBan := 0
	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Name:    "image-studio-fake",
		Key:     "test-key",
		BaseURL: &baseURL,
		Models:  "gpt-image-2",
		Group:   "default",
		Status:  common.ChannelStatusEnabled,
		AutoBan: &autoBan,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-image-2", ChannelId: channel.Id, Enabled: true}).Error)

	var usedChannels []string
	engine := gin.New()
	engine.POST("/pg/images/generations", func(context *gin.Context) {
		context.Set("id", 945)
		context.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		common.SetContextKey(context, constant.ContextKeyTokenGroup, "default")
		common.SetContextKey(context, constant.ContextKeyUsingGroup, "default")
	}, ImageStudioErrorProjection(), middleware.Distribute(), ImageStudio, func(context *gin.Context) {
		usedChannels = context.GetStringSlice("use_channel")
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/pg/images/generations", bytes.NewBufferString(`{"prompt":"safe prompt"}`))
	request.Header.Set("Content-Type", gin.MIMEJSON)
	engine.ServeHTTP(recorder, request)

	require.Equal(t, []string{"1"}, usedChannels)
	require.Equal(t, 1, upstreamRequests)
	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, gin.MIMEJSON, recorder.Header().Get("Content-Type"))
	require.Empty(t, recorder.Header().Get("Content-Encoding"))
	require.Empty(t, recorder.Header().Get("Content-Length"))
	require.Empty(t, recorder.Header().Get("Retry-After"))
	require.Empty(t, recorder.Header().Get("X-Upstream-Trace"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	for _, forbidden := range []string{"safe prompt", "test-key", "upstream detail must not escape", server.URL} {
		require.NotContains(t, recorder.Body.String(), forbidden)
	}

	invalidRecorder := httptest.NewRecorder()
	invalidRequest := httptest.NewRequest(http.MethodPost, "/pg/images/generations", bytes.NewBufferString(`{"prompt":"safe prompt","model":"untrusted"}`))
	invalidRequest.Header.Set("Content-Type", gin.MIMEJSON)
	engine.ServeHTTP(invalidRecorder, invalidRequest)
	require.Equal(t, http.StatusBadRequest, invalidRecorder.Code)
	require.Equal(t, 1, upstreamRequests)
	for _, forbidden := range []string{"safe prompt", "untrusted", "test-key", server.URL} {
		require.NotContains(t, invalidRecorder.Body.String(), forbidden)
	}
}
