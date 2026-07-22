package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTopupPackageKeepsPermanentCreditWhenCampaignsAreDisabled(t *testing.T) {
	originalPackages := GetTopupPackages()
	originalPromotionEnabled := paymentSetting.PromotionEnabled
	t.Cleanup(func() {
		paymentSetting.TopupPackages = originalPackages
		paymentSetting.PromotionEnabled = originalPromotionEnabled
	})

	paymentSetting.TopupPackages = []TopupPackage{
		{ID: "advanced", PayAmount: 98, CreditAmount: 100, Enabled: true},
	}
	paymentSetting.PromotionEnabled = false

	packageOption, ok := ResolveTopupPackage("advanced")
	require.True(t, ok)
	assert.Equal(t, 98.0, packageOption.PayAmount)
	assert.Equal(t, 100.0, packageOption.CreditAmount)
}

func TestResolveTopupPackageIsIndependentFromLegacyPromotionSwitch(t *testing.T) {
	originalPackages := GetTopupPackages()
	originalPromotionEnabled := paymentSetting.PromotionEnabled
	t.Cleanup(func() {
		paymentSetting.TopupPackages = originalPackages
		paymentSetting.PromotionEnabled = originalPromotionEnabled
	})

	paymentSetting.TopupPackages = []TopupPackage{
		{ID: "advanced", PayAmount: 98, CreditAmount: 100, Enabled: true},
	}
	paymentSetting.PromotionEnabled = true

	packageOption, ok := ResolveTopupPackage("advanced")
	require.True(t, ok)
	assert.Equal(t, 100.0, packageOption.CreditAmount)
}

func TestResolveTopupPackageRejectsUnknownOrInvalidPackages(t *testing.T) {
	originalPackages := GetTopupPackages()
	t.Cleanup(func() {
		paymentSetting.TopupPackages = originalPackages
	})

	paymentSetting.TopupPackages = []TopupPackage{
		{ID: "invalid", PayAmount: 0, CreditAmount: 0, Enabled: true},
		{ID: "disabled", PayAmount: 14, CreditAmount: 14, Enabled: false},
	}

	_, foundInvalid := ResolveTopupPackage("invalid")
	_, foundDisabled := ResolveTopupPackage("disabled")
	_, foundUnknown := ResolveTopupPackage("missing")
	assert.False(t, foundInvalid)
	assert.False(t, foundDisabled)
	assert.False(t, foundUnknown)
}
