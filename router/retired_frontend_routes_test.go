package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRetiredFrontendAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	_, hasAsyncCleanup := routes[http.MethodPost+" /api/system-task/log-cleanup"]
	_, hasDirectDelete := routes[http.MethodDelete+" /api/log/"]
	_, hasConsoleMigration := routes[http.MethodPost+" /api/option/migrate_console_setting"]
	_, hasStripeWebhook := routes[http.MethodPost+" /api/stripe/webhook"]
	_, hasStripeTopUp := routes[http.MethodPost+" /api/user/stripe/pay"]
	_, hasStripeAmount := routes[http.MethodPost+" /api/user/stripe/amount"]
	_, hasStripeSubscription := routes[http.MethodPost+" /api/subscription/stripe/pay"]
	assert.True(t, hasAsyncCleanup)
	assert.False(t, hasDirectDelete)
	assert.False(t, hasConsoleMigration)
	assert.False(t, hasStripeWebhook)
	assert.False(t, hasStripeTopUp)
	assert.False(t, hasStripeAmount)
	assert.False(t, hasStripeSubscription)
}
