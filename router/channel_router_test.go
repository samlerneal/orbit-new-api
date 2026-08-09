package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelStatusRoutesUseOperatePermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodPost, "/:id/status", authz.ChannelOperate, controller.UpdateChannelStatus)
	assertChannelRoutePermission(t, http.MethodPost, "/status/batch", authz.ChannelOperate, controller.BatchUpdateChannelStatus)
	assertChannelRoutePermission(t, http.MethodPut, "/", authz.ChannelWrite, controller.UpdateChannel)
}

func TestChannelDeleteRoutesUseSensitiveWritePermission(t *testing.T) {
	assertChannelRoutePermission(t, http.MethodDelete, "/:id", authz.ChannelSensitiveWrite, controller.DeleteChannel)
	assertChannelRoutePermission(t, http.MethodPost, "/batch", authz.ChannelSensitiveWrite, controller.DeleteChannelBatch)
	assertChannelRoutePermission(t, http.MethodDelete, "/disabled", authz.ChannelSensitiveWrite, controller.DeleteDisabledChannel)
	assertChannelRoutePermission(t, http.MethodPut, "/", authz.ChannelWrite, controller.UpdateChannel)
	assertChannelRoutePermission(t, http.MethodPut, "/tag", authz.ChannelWrite, controller.EditTagChannels)
	assertChannelRoutePermission(t, http.MethodPost, "/batch/tag", authz.ChannelWrite, controller.BatchSetChannelTag)
}

func TestChannelStatusRoutesRegisterWithoutConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerChannelRoutes(api)
	})
}

func TestImageCapabilityRoutesRegisterWithDistinctMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")
	registerChannelRoutes(api)
	routes := engine.Routes()
	seen := map[string]bool{}
	for _, route := range routes {
		seen[route.Method+" "+route.Path] = true
	}
	assert.True(t, seen[http.MethodGet+" /api/channel/test/:id/image-capability"])
	assert.True(t, seen[http.MethodPost+" /api/channel/test/:id/image-capability/run"])
}

func TestImageCapabilityRouteSpecsRequireOperateAndProtectPaidPost(t *testing.T) {
	require.Len(t, imageCapabilityRouteSpecs, 2)
	get, post := imageCapabilityRouteSpecs[0], imageCapabilityRouteSpecs[1]
	assert.Equal(t, http.MethodGet, get.method)
	assert.Equal(t, authz.ChannelOperate, get.permission)
	assert.False(t, get.requiresCriticalRateLimit)
	assert.Equal(t, http.MethodPost, post.method)
	assert.Equal(t, authz.ChannelOperate, post.permission)
	assert.True(t, post.requiresCriticalRateLimit)
	assert.True(t, post.requiresDisableCache)
	assert.True(t, post.requiresSecureVerification)
	assert.Equal(t, reflect.ValueOf(controller.RunImageCapability).Pointer(), reflect.ValueOf(post.handler).Pointer())
}

func TestImageCapabilityRoutesRejectUnauthenticatedRequestsBeforeHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")
	registerChannelRoutes(api)
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/channel/test/1/image-capability", nil),
		httptest.NewRequest(http.MethodPost, "/api/channel/test/1/image-capability/run", bytes.NewBufferString(`{}`)),
	} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	}
}

func assertChannelRoutePermission(t *testing.T, method string, path string, permission authz.Permission, handler any) {
	t.Helper()
	for _, route := range channelPermissionRoutes {
		if route.method == method && route.path == path {
			assert.Equal(t, permission, route.permission)
			assert.Equal(t, reflect.ValueOf(handler).Pointer(), reflect.ValueOf(route.handler).Pointer())
			return
		}
	}
	t.Fatalf("route %s %s not found", method, path)
}
