package controller

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func confirmPaymentComplianceForTest(t *testing.T) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
}

func TestStripeWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	originalPriceID := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
		setting.StripePriceId = originalPriceID
	})

	setting.StripeWebhookSecret = ""
	setting.StripeApiSecret = "sk_test_123"
	setting.StripePriceId = "price_123"
	require.False(t, isStripeWebhookEnabled())

	setting.StripeWebhookSecret = "whsec_test"
	require.True(t, isStripeWebhookEnabled())

	setting.StripePriceId = ""
	require.False(t, isStripeWebhookEnabled())
}

func TestCreemWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPIKey := setting.CreemApiKey
	originalProducts := setting.CreemProducts
	originalWebhookSecret := setting.CreemWebhookSecret
	t.Cleanup(func() {
		setting.CreemApiKey = originalAPIKey
		setting.CreemProducts = originalProducts
		setting.CreemWebhookSecret = originalWebhookSecret
	})

	setting.CreemWebhookSecret = ""
	setting.CreemApiKey = "creem_api_key"
	setting.CreemProducts = `[{"productId":"prod_123"}]`
	require.False(t, isCreemWebhookEnabled())

	setting.CreemWebhookSecret = "creem_secret"
	require.True(t, isCreemWebhookEnabled())

	setting.CreemProducts = "[]"
	require.False(t, isCreemWebhookEnabled())
}

func TestWaffoWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalEnabled := setting.WaffoEnabled
	originalSandbox := setting.WaffoSandbox
	originalAPIKey := setting.WaffoApiKey
	originalPrivateKey := setting.WaffoPrivateKey
	originalPublicCert := setting.WaffoPublicCert
	originalSandboxAPIKey := setting.WaffoSandboxApiKey
	originalSandboxPrivateKey := setting.WaffoSandboxPrivateKey
	originalSandboxPublicCert := setting.WaffoSandboxPublicCert
	t.Cleanup(func() {
		setting.WaffoEnabled = originalEnabled
		setting.WaffoSandbox = originalSandbox
		setting.WaffoApiKey = originalAPIKey
		setting.WaffoPrivateKey = originalPrivateKey
		setting.WaffoPublicCert = originalPublicCert
		setting.WaffoSandboxApiKey = originalSandboxAPIKey
		setting.WaffoSandboxPrivateKey = originalSandboxPrivateKey
		setting.WaffoSandboxPublicCert = originalSandboxPublicCert
	})

	setting.WaffoEnabled = true
	setting.WaffoSandbox = false
	setting.WaffoApiKey = ""
	setting.WaffoPrivateKey = "private"
	setting.WaffoPublicCert = "public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoApiKey = "api"
	require.True(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = false
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = true
	setting.WaffoSandbox = true
	setting.WaffoSandboxApiKey = ""
	setting.WaffoSandboxPrivateKey = "sandbox_private"
	setting.WaffoSandboxPublicCert = "sandbox_public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoSandboxApiKey = "sandbox_api"
	require.True(t, isWaffoWebhookEnabled())
}

func TestWaffoPancakeWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalProductID := setting.WaffoPancakeProductID
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeProductID = originalProductID
	})

	// Presence of all three credentials enables the gateway. Webhook public
	// keys are bundled in the SDK and there is no separate Enabled toggle —
	// clear any of the three fields to disable.
	setting.WaffoPancakeMerchantID = ""
	setting.WaffoPancakePrivateKey = "private"
	setting.WaffoPancakeProductID = "product"
	require.False(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeMerchantID = "merchant"
	require.True(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeProductID = ""
	require.False(t, isWaffoPancakeWebhookEnabled())

	setting.WaffoPancakeProductID = "product"
	setting.WaffoPancakePrivateKey = ""
	require.False(t, isWaffoPancakeWebhookEnabled())
}

func TestEpayWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
	})

	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = ""
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	require.False(t, isEpayWebhookEnabled())

	operation_setting.EpayKey = "epay_key"
	require.True(t, isEpayWebhookEnabled())

	operation_setting.PayMethods = nil
	require.False(t, isEpayWebhookEnabled())
}

func TestSupportedEpayMethodAllowsOnlyWechatAndAlipay(t *testing.T) {
	require.True(t, isSupportedEpayMethod("wxpay"))
	require.True(t, isSupportedEpayMethod("alipay"))
	require.False(t, isSupportedEpayMethod("custom1"))
	require.False(t, isSupportedEpayMethod(""))
}

func TestParseEpayNotifyFormAcceptsOnlyUnambiguousPostForm(t *testing.T) {
	validForm := url.Values{
		"pid":          {"orbit"},
		"out_trade_no": {"ORDER-1"},
		"trade_status": {"TRADE_SUCCESS"},
		"sign_type":    {"MD5"},
		"sign":         {"00000000000000000000000000000000"},
	}
	tests := []struct {
		name    string
		method  string
		target  string
		content string
		form    url.Values
		wantErr bool
	}{
		{name: "valid POST form", method: http.MethodPost, target: "/api/user/epay/notify", content: "application/x-www-form-urlencoded", form: validForm},
		{name: "GET callback", method: http.MethodGet, target: "/api/user/epay/notify", content: "application/x-www-form-urlencoded", form: validForm, wantErr: true},
		{name: "URL query", method: http.MethodPost, target: "/api/user/epay/notify?sign=leak", content: "application/x-www-form-urlencoded", form: validForm, wantErr: true},
		{name: "wrong content type", method: http.MethodPost, target: "/api/user/epay/notify", content: "text/plain", form: validForm, wantErr: true},
		{name: "duplicate field", method: http.MethodPost, target: "/api/user/epay/notify", content: "application/x-www-form-urlencoded", form: url.Values{
			"pid": {"orbit"}, "out_trade_no": {"ORDER-1", "ORDER-2"}, "sign": {"00000000000000000000000000000000"},
		}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(test.method, test.target, strings.NewReader(test.form.Encode()))
			require.NoError(t, err)
			request.Header.Set("Content-Type", test.content)

			fields, parseErr := parseEpayNotifyForm(request)
			if test.wantErr {
				require.Error(t, parseErr)
				return
			}
			require.NoError(t, parseErr)
			require.Equal(t, "ORDER-1", fields["out_trade_no"])
		})
	}
}

func TestValidEpayNotifyEnvelopeRequiresExpectedMerchantChannelAndSignature(t *testing.T) {
	const partnerID = "orbit"
	const key = "0123456789abcdef0123456789abcdef"
	valid := map[string]string{
		"pid":          partnerID,
		"trade_no":     "PROVIDER-1",
		"out_trade_no": "ORDER-1",
		"type":         "alipay",
		"name":         "Orbit API credit",
		"money":        "1.00",
		"trade_status": "TRADE_SUCCESS",
		"sign_type":    "MD5",
	}
	epay.GenerateParams(valid, key)
	require.True(t, validEpayNotifyEnvelope(valid, partnerID, key))

	tests := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{name: "wrong merchant", mutate: func(fields map[string]string) { fields["pid"] = "other" }},
		{name: "unsupported channel", mutate: func(fields map[string]string) { fields["type"] = "custom1" }},
		{name: "wrong sign type", mutate: func(fields map[string]string) { fields["sign_type"] = "SHA256" }},
		{name: "missing required field", mutate: func(fields map[string]string) { delete(fields, "trade_no") }},
		{name: "forged signature", mutate: func(fields map[string]string) { fields["sign"] = strings.Repeat("0", 32) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields := make(map[string]string, len(valid))
			for field, value := range valid {
				fields[field] = value
			}
			test.mutate(fields)
			require.False(t, validEpayNotifyEnvelope(fields, partnerID, key))
		})
	}
}
