package controller

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageStudioErrorProjectionWriterReportsCapturedFailureStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	writer := &imageStudioErrorProjectionWriter{ResponseWriter: context.Writer}

	writer.WriteHeader(http.StatusTooManyRequests)

	require.Equal(t, http.StatusTooManyRequests, writer.Status())
	require.True(t, writer.Written())
}

func TestImageStudioRetryMarkerDisablesRetry(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/pg/images/generations", nil)
	c.Set("specific_channel_id", "42")

	require.False(t, shouldRetry(c, types.NewErrorWithStatusCode(errors.New("retryable upstream response"), types.ErrorCodeInvalidRequest, http.StatusTooManyRequests), common.RetryTimes+1))
}

func TestImageStudioRejectsAccessTokensBeforeItsRelayHandler(t *testing.T) {
	recorder := httptest.NewRecorder()
	relayHandlerCalls := 0
	engine := gin.New()
	engine.POST("/pg/images/generations", func(c *gin.Context) {
		c.Set("use_access_token", true)
	}, ImageStudio, func(c *gin.Context) {
		relayHandlerCalls++
	})

	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pg/images/generations", nil))

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.JSONEq(t, `{"error":{"code":"IMAGE_STUDIO_SESSION_REQUIRED","message":"Please sign in to generate images."}}`, recorder.Body.String())
	require.Zero(t, relayHandlerCalls)
}

func TestImageStudioAnonymousRequestStopsBeforeAnyHandler(t *testing.T) {
	recorder := httptest.NewRecorder()
	nextHandlerCalls := 0
	engine := gin.New()
	engine.POST("/pg/images/generations", middleware.UserAuth(), func(c *gin.Context) {
		nextHandlerCalls++
	})

	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pg/images/generations", nil))

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Zero(t, nextHandlerCalls)
}

func TestImageStudioNoChannelStopsBeforeTheRelay(t *testing.T) {
	require.NoError(t, i18n.Init())
	db := setupModelListControllerTestDB(t)
	insertModelListUser(t, db, 946, "image-studio-no-channel", "default")
	upstreamCalls := 0
	engine := gin.New()
	engine.POST("/pg/images/generations", func(c *gin.Context) {
		c.Set("id", 946)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	}, middleware.Distribute(), ImageStudio, func(c *gin.Context) {
		upstreamCalls++
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/pg/images/generations", bytes.NewBufferString(`{"prompt":"safe"}`))
	request.Header.Set("Content-Type", gin.MIMEJSON)

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Zero(t, upstreamCalls)
}
