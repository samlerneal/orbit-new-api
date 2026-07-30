package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStatusPublicBrandUsesTheCanonicalJSONField(t *testing.T) {
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

	GetStatus(context)

	var payload struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.Equal(t, common.PublicBrandZhCN, payload.Data["public_brand_zh_cn"])
	assert.NotContains(t, payload.Data, "publicBrandZhCN")
	assert.NotContains(t, payload.Data, "brand")
	_, ok := payload.Data["public_brand_zh_cn"].(string)
	assert.True(t, ok)
}

func TestVerificationEmailBrandAndPasswordResetBrand(t *testing.T) {
	verificationSubject, verificationContent := buildVerificationEmail("123456")
	resetSubject, resetContent := buildPasswordResetEmail("https://api.mydaily.info/user/reset")

	for _, value := range []string{
		verificationSubject,
		verificationContent,
		resetSubject,
		resetContent,
	} {
		assert.True(t, strings.Contains(value, common.PublicBrandZhCN))
	}
	assert.Contains(t, resetContent, "https://api.mydaily.info/user/reset")
}
