package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTopupPackageUsesPayAmountWhenPromotionIsDisabled(t *testing.T) {
	originalPackages := GetTopupPackages()
	originalPromotionEnabled := paymentSetting.PromotionEnabled
	t.Cleanup(func() {
		paymentSetting.TopupPackages = originalPackages
		paymentSetting.PromotionEnabled = originalPromotionEnabled
	})

	paymentSetting.TopupPackages = []TopupPackage{
		{ID: "advanced", PayAmount: 98, CreditAmount: 108},
	}
	paymentSetting.PromotionEnabled = false

	packageOption, ok := ResolveTopupPackage("advanced")
	require.True(t, ok)
	assert.Equal(t, 98.0, packageOption.PayAmount)
	assert.Equal(t, 98.0, packageOption.CreditAmount)
}

func TestResolveTopupPackageAppliesConfiguredCreditWhenPromotionIsEnabled(t *testing.T) {
	originalPackages := GetTopupPackages()
	originalPromotionEnabled := paymentSetting.PromotionEnabled
	t.Cleanup(func() {
		paymentSetting.TopupPackages = originalPackages
		paymentSetting.PromotionEnabled = originalPromotionEnabled
	})

	paymentSetting.TopupPackages = []TopupPackage{
		{ID: "advanced", PayAmount: 98, CreditAmount: 108},
	}
	paymentSetting.PromotionEnabled = true

	packageOption, ok := ResolveTopupPackage("advanced")
	require.True(t, ok)
	assert.Equal(t, 108.0, packageOption.CreditAmount)
}

func TestResolveTopupPackageRejectsUnknownOrInvalidPackages(t *testing.T) {
	originalPackages := GetTopupPackages()
	t.Cleanup(func() {
		paymentSetting.TopupPackages = originalPackages
	})

	paymentSetting.TopupPackages = []TopupPackage{
		{ID: "invalid", PayAmount: 0, CreditAmount: 0},
	}

	_, foundInvalid := ResolveTopupPackage("invalid")
	_, foundUnknown := ResolveTopupPackage("missing")
	assert.False(t, foundInvalid)
	assert.False(t, foundUnknown)
}
