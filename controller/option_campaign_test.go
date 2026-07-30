package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePaymentCampaignValidationErrorReturnsFieldMap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	validationErrors := &operation_setting.PaymentCampaignValidationErrors{
		Errors: []operation_setting.PaymentCampaignFieldError{
			{
				Index:      0,
				CampaignID: "launch-first-topup-30",
				Field:      "max_participants_total",
				Message:    "participant limit is invalid",
			},
		},
	}

	writePaymentCampaignValidationError(context, validationErrors)

	var response struct {
		Success     bool              `json:"success"`
		Message     string            `json:"message"`
		FieldErrors map[string]string `json:"field_errors"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "充值活动配置无效")
	assert.Equal(
		t,
		"participant limit is invalid",
		response.FieldErrors["campaigns.0.max_participants_total"],
	)
}
