package controller

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageEditChannelTestSendsOneSyntheticMultipartRequest(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.InitHttpClient()
	originalRatios := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		data, err := common.Marshal(originalRatios)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	})
	ratioMap := ratio_setting.GetModelRatioCopy()
	ratioMap["gpt-image-2"] = 1
	ratioData, err := common.Marshal(ratioMap)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(ratioData)))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/v1/images/edits", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(2<<20))
		require.Equal(t, "gpt-image-2", r.FormValue("model"))
		require.Equal(t, "Apply a subtle neutral edit.", r.FormValue("prompt"))
		require.Equal(t, "1", r.FormValue("n"))
		require.Equal(t, "1024x1024", r.FormValue("size"))
		require.Equal(t, "low", r.FormValue("quality"))
		files := r.MultipartForm.File["image"]
		require.Len(t, files, 1)
		file, err := files[0].Open()
		require.NoError(t, err)
		defer file.Close()
		decoded, err := png.Decode(file)
		require.NoError(t, err)
		require.Equal(t, 512, decoded.Bounds().Dx())
		require.Equal(t, 512, decoded.Bounds().Dy())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"url":"https://example.test/image.png"}],"usage":{}}`)
	}))
	defer server.Close()
	insertModelListUser(t, db, 901, "image-edit-tester", "default")
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "image edit", Key: "test-key", BaseURL: &baseURL, Models: "gpt-image-2", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-image-2", ChannelId: channel.Id, Enabled: true}).Error)
	result := testChannel(context.Background(), channel, 901, "gpt-image-2", string(constant.EndpointTypeImageEdit), false)
	require.NoError(t, result.localErr)
	require.Equal(t, 1, requests)
}

func TestImageEditTestChannelPreflightRejectsBeforeUpstream(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "image edit", Key: "test-key", BaseURL: &baseURL, Models: "gpt-image-2", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })

	tests := []string{
		"endpoint_type=image-edit&model=gpt-image-2",
		"endpoint_type=image-edit&confirm_paid_image_edit=true",
		"endpoint_type=image-edit&model=gpt-image-2&confirm_paid_image_edit=true&stream=true",
	}
	for _, query := range tests {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/"+fmt.Sprintf("%d", channel.Id)+"?"+query, nil)
		TestChannel(ctx)
		var response struct {
			Success   bool            `json:"success"`
			ErrorCode types.ErrorCode `json:"error_code"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
		assert.Equal(t, types.ErrorCodeInvalidRequest, response.ErrorCode)
		assert.Equal(t, 0, requests)
	}
}

func TestImageEditChannelTestRejectsOversizedErrorResponse(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.InitHttpClient()
	originalRatios := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		data, err := common.Marshal(originalRatios)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	})
	ratios := ratio_setting.GetModelRatioCopy()
	ratios["gpt-image-2"] = 1
	data, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/v1/images/edits", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(2<<20))
		require.Equal(t, "gpt-image-2", r.FormValue("model"))
		require.Equal(t, "Apply a subtle neutral edit.", r.FormValue("prompt"))
		file, err := r.MultipartForm.File["image"][0].Open()
		require.NoError(t, err)
		defer file.Close()
		decoded, err := png.Decode(file)
		require.NoError(t, err)
		require.Equal(t, 512, decoded.Bounds().Dx())
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "secret-error-body-"+strings.Repeat("x", (1<<20)+1))
	}))
	defer server.Close()
	insertModelListUser(t, db, 902, "image-edit-cap", "default")
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "image edit", Key: "test-key", BaseURL: &baseURL, Models: "gpt-image-2", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-image-2", ChannelId: channel.Id, Enabled: true}).Error)
	result := testChannel(context.Background(), channel, 902, "gpt-image-2", string(constant.EndpointTypeImageEdit), false)
	require.Equal(t, 1, requests)
	require.EqualError(t, result.localErr, "image edit response exceeds the allowed size")
	require.NotNil(t, result.newAPIError)
	require.Equal(t, types.ErrorCodeBadResponseBody, result.newAPIError.GetErrorCode())
	for _, secret := range []string{"secret-error-body", "Apply a subtle neutral edit.", "test-key", "b64"} {
		require.NotContains(t, result.localErr.Error(), secret)
	}
}

func TestImageEditChannelTestRejectsOversizedSuccessResponse(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.InitHttpClient()
	original := ratio_setting.GetModelRatioCopy()
	t.Cleanup(func() {
		data, err := common.Marshal(original)
		require.NoError(t, err)
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	})
	ratios := ratio_setting.GetModelRatioCopy()
	ratios["gpt-image-2"] = 1
	data, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(data)))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/v1/images/edits", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(2<<20))
		require.Equal(t, "gpt-image-2", r.FormValue("model"))
		file, err := r.MultipartForm.File["image"][0].Open()
		require.NoError(t, err)
		defer file.Close()
		decoded, err := png.Decode(file)
		require.NoError(t, err)
		require.Equal(t, 512, decoded.Bounds().Dx())
		_, _ = io.WriteString(w, "secret-success-body-"+strings.Repeat("x", (64<<20)+1))
	}))
	defer server.Close()
	insertModelListUser(t, db, 903, "image-edit-success-cap", "default")
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "image edit", Key: "test-key", BaseURL: &baseURL, Models: "gpt-image-2", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-image-2", ChannelId: channel.Id, Enabled: true}).Error)
	result := testChannel(context.Background(), channel, 903, "gpt-image-2", string(constant.EndpointTypeImageEdit), false)
	require.Equal(t, 1, requests)
	require.EqualError(t, result.localErr, "image edit response exceeds the allowed size")
	require.NotNil(t, result.newAPIError)
	require.Equal(t, types.ErrorCodeBadResponseBody, result.newAPIError.GetErrorCode())
	for _, secret := range []string{"secret-success-body", "Apply a subtle neutral edit.", "test-key", "b64"} {
		require.NotContains(t, result.localErr.Error(), secret)
	}
}

func TestValidateChannelProxy(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{name: "empty"},
		{name: "http", proxy: "http://proxy.example:8080"},
		{name: "https", proxy: "https://proxy.example:8443"},
		{name: "socks5", proxy: "socks5://proxy.example"},
		{name: "socks5h", proxy: "socks5h://proxy.example:1080/"},
		{name: "unsupported", proxy: "ftp://proxy.example", wantErr: true},
		{name: "path", proxy: "socks5://proxy.example:1080/path", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting, err := common.Marshal(dto.ChannelSettings{Proxy: test.proxy})
			require.NoError(t, err)
			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Setting: common.GetPointer(string(setting)),
			}

			err = validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "invalid channel proxy")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestImageEditChannelTestContract(t *testing.T) {
	endpoint, ok := common.GetDefaultEndpointInfo(constant.EndpointTypeImageEdit)
	require.True(t, ok)
	assert.Equal(t, "/v1/images/edits", endpoint.Path)

	request, ok := buildTestRequest("gpt-image-2", string(constant.EndpointTypeImageEdit), nil, false).(*dto.ImageRequest)
	require.True(t, ok)
	assert.Equal(t, "gpt-image-2", request.Model)
	assert.Equal(t, "Apply a subtle neutral edit.", request.Prompt)
	require.NotNil(t, request.N)
	assert.Equal(t, uint(1), *request.N)
	assert.Equal(t, "1024x1024", request.Size)
	assert.Equal(t, "low", request.Quality)
}

func TestImageEditResponseValidation(t *testing.T) {
	assert.True(t, isValidImageEditTestResponse([]byte(`{"data":[{"url":"https://example.test/image.png"}]}`)))
	assert.True(t, isValidImageEditTestResponse([]byte(`{"data":[{"b64_json":"encoded"}]}`)))
	assert.False(t, isValidImageEditTestResponse([]byte(`{"data":[]}`)))
	assert.False(t, isValidImageEditTestResponse([]byte(`{"data":[{}]}`)))
}

func TestCopyChannelRejectsInvalidLegacyProxySettings(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		Proxy: "socks5://proxy.example/legacy-path",
	})
	require.NoError(t, err)
	setting := string(settingBytes)
	origin := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Name:    "legacy proxy channel",
		Key:     "test-key",
		Models:  "gpt-test",
		Group:   "default",
		Setting: &setting,
	}
	require.NoError(t, db.Create(origin).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy", nil)

	CopyChannel(ctx)

	assert.Contains(t, recorder.Body.String(), "invalid channel settings")
	var channelCount int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)
}

func TestDeleteChannelResetsProxyCacheWhenPreReadFails(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.ResetProxyClientCache()
	t.Cleanup(service.ResetProxyClientCache)

	proxyURL := "http://proxy.example:8080"
	beforeDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "999999"}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/999999", nil)

	DeleteChannel(ctx)

	assert.Contains(t, recorder.Body.String(), `"success":true`)
	afterDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)
	assert.NotSame(t, beforeDelete, afterDelete)
}

func TestDeleteChannelBatchReportsAndAuditsActualDeletedCount(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{Name: "existing", Key: "test-key"}
	require.NoError(t, db.Create(channel).Error)

	requestBody, err := common.Marshal(ChannelBatch{Ids: []int{channel.Id, 999999}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/batch", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	DeleteChannelBatch(ctx)

	var response struct {
		Success bool  `json:"success"`
		Data    int64 `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, int64(1), response.Data)

	var auditLog model.Log
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	var auditData struct {
		Operation struct {
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	require.NoError(t, common.UnmarshalJsonStr(auditLog.Other, &auditData))
	assert.Equal(t, float64(1), auditData.Operation.Params["count"])
}

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryOnlyUsesAutoDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 1)
	require.Equal(t, 2, selected[0].Id)
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}
