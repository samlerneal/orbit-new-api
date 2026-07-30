package operation_setting

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validCampaignForSafeguardTest() PaymentCampaign {
	return PaymentCampaign{
		ID:                   "safeguard-test",
		Name:                 "Safeguard test",
		Eligibility:          CampaignEligibilityPerCampaign,
		MaxClaimsPerUser:     1,
		MaxClaimsPerEmail:    1,
		MaxClaimsTotal:       100,
		MaxParticipantsTotal: 0,
		ReservationMinutes:   3,
		RewardMode:           CampaignRewardTargetTotalPercent,
		RewardPercent:        30,
		RoundingMode:         CampaignRoundingCeilYuan,
		ValidDays:            45,
	}
}

func TestNormalizeLegacyLaunchCampaignAddsSafeDefaults(t *testing.T) {
	campaign := normalizeLegacyCampaignSafeguards(PaymentCampaign{
		ID:                   "launch-first-topup-30",
		MaxClaimsPerEmail:    0,
		MaxClaimsTotal:       100,
		MaxParticipantsTotal: 0,
		ReservationMinutes:   0,
		Eligibility:          CampaignEligibilityPerCampaign,
	})

	assert.Equal(t, 1, campaign.MaxClaimsPerEmail)
	assert.Equal(t, 100, campaign.MaxParticipantsTotal)
	assert.Equal(t, 0, campaign.MaxClaimsTotal)
	assert.Equal(t, 3, campaign.ReservationMinutes)
	assert.Equal(t, CampaignEligibilityPerPackage, campaign.Eligibility)
}

func TestDefaultTopupPackagesUseCurrentChineseCopy(t *testing.T) {
	packages := GetTopupPackages()
	require.Len(t, packages, 4)

	expected := []TopupPackage{
		{ID: "experience", Name: "体验", Description: "适合日常对话", PayAmount: 14, CreditAmount: 14, Enabled: true, SortOrder: 10},
		{ID: "standard", Name: "标准", Description: "适合解决复杂问题", Tag: "人气之选", PayAmount: 49, CreditAmount: 50, Enabled: true, SortOrder: 20},
		{ID: "advanced", Name: "进阶", Description: "适合频繁使用", Tag: "最受欢迎", PayAmount: 98, CreditAmount: 100, Enabled: true, SortOrder: 30},
		{ID: "professional", Name: "专业", Description: "为专业开发者打造", Tag: "高量超值", PayAmount: 490, CreditAmount: 500, Enabled: true, SortOrder: 40},
	}

	for index, want := range expected {
		got := packages[index]
		assert.Equal(t, want.ID, got.ID)
		assert.Equal(t, want.Name, got.Name)
		assert.Equal(t, want.Description, got.Description)
		assert.Equal(t, want.Tag, got.Tag)
		assert.Equal(t, want.PayAmount, got.PayAmount)
		assert.Equal(t, want.CreditAmount, got.CreditAmount)
		assert.Equal(t, want.Enabled, got.Enabled)
		assert.Equal(t, want.SortOrder, got.SortOrder)
		assert.NotContains(t, got.Name, "档")
	}
}

func TestNormalizeLegacyCampaignWithReservationMinutesConvertsEligibility(t *testing.T) {
	campaign := normalizeLegacyCampaignSafeguards(PaymentCampaign{
		ID:                   "launch-first-topup-30",
		MaxClaimsPerEmail:    0,
		MaxClaimsTotal:       100,
		MaxParticipantsTotal: 0,
		ReservationMinutes:   3,
		Eligibility:          CampaignEligibilityPerCampaign,
	})

	assert.Equal(t, 1, campaign.MaxClaimsPerEmail)
	assert.Equal(t, 100, campaign.MaxParticipantsTotal)
	assert.Equal(t, 0, campaign.MaxClaimsTotal)
	assert.Equal(t, 3, campaign.ReservationMinutes)
	assert.Equal(t, CampaignEligibilityPerPackage, campaign.Eligibility)
}

func TestNormalizePaymentCampaignsForSavePreservesExplicitParticipantLimitAndDerivesClaims(t *testing.T) {
	packages := []TopupPackage{
		{ID: "experience", Enabled: true},
		{ID: "standard", Enabled: true},
		{ID: "advanced", Enabled: true},
		{ID: "professional", Enabled: true},
	}
	campaign := PaymentCampaign{
		ID:                   "launch-first-topup-30",
		Name:                 "Launch campaign",
		Eligibility:          CampaignEligibilityPerCampaign,
		MaxClaimsPerUser:     1,
		MaxClaimsPerEmail:    1,
		MaxClaimsTotal:       100,
		MaxParticipantsTotal: 30,
		ReservationMinutes:   3,
		RewardMode:           CampaignRewardTargetTotalPercent,
		RewardPercent:        30,
		RoundingMode:         CampaignRoundingCeilYuan,
		ValidDays:            45,
		PackageIDs:           []string{"experience", "standard", "advanced", "professional"},
	}

	normalized, err := NormalizePaymentCampaignsForSave([]PaymentCampaign{campaign}, packages)

	require.NoError(t, err)
	require.Len(t, normalized, 1)
	assert.Equal(t, CampaignEligibilityPerPackage, normalized[0].Eligibility)
	assert.Equal(t, 30, normalized[0].MaxParticipantsTotal)
	assert.Equal(t, 120, normalized[0].MaxClaimsTotal)
}

func TestNormalizePaymentCampaignsForSaveForcesPerPackageClaimCountsToOne(t *testing.T) {
	packages := []TopupPackage{{ID: "advanced", Enabled: true}}
	campaign := validCampaignForSafeguardTest()
	campaign.ID = "four-package"
	campaign.Eligibility = CampaignEligibilityPerPackage
	campaign.MaxClaimsPerUser = 4
	campaign.MaxClaimsPerEmail = 3
	campaign.MaxParticipantsTotal = 30
	campaign.MaxClaimsTotal = 99
	campaign.PackageIDs = []string{"advanced"}

	normalized, err := NormalizePaymentCampaignsForSave([]PaymentCampaign{campaign}, packages)

	require.NoError(t, err)
	require.Len(t, normalized, 1)
	assert.Equal(t, 1, normalized[0].MaxClaimsPerUser)
	assert.Equal(t, 1, normalized[0].MaxClaimsPerEmail)
}

func TestNormalizePaymentCampaignsForSaveTreatsUnlimitedParticipantsAsUnlimitedClaims(t *testing.T) {
	packages := []TopupPackage{{ID: "advanced", Enabled: true}}
	campaign := validCampaignForSafeguardTest()
	campaign.ID = "unlimited-participants"
	campaign.Eligibility = CampaignEligibilityPerPackage
	campaign.MaxParticipantsTotal = 0
	campaign.MaxClaimsTotal = 100
	campaign.PackageIDs = []string{"advanced"}

	normalized, err := NormalizePaymentCampaignsForSave([]PaymentCampaign{campaign}, packages)

	require.NoError(t, err)
	require.Len(t, normalized, 1)
	assert.Zero(t, normalized[0].MaxParticipantsTotal)
	assert.Zero(t, normalized[0].MaxClaimsTotal)
}

func TestNormalizeCampaignAuthorityReadbackAppliesControlledLimits(t *testing.T) {
	packages := []TopupPackage{{ID: "advanced", Enabled: true}}
	campaign := validCampaignForSafeguardTest()
	campaign.ID = "readback-authority"
	campaign.Eligibility = CampaignEligibilityPerPackage
	campaign.MaxClaimsPerUser = 9
	campaign.MaxClaimsPerEmail = 8
	campaign.MaxParticipantsTotal = 0
	campaign.MaxClaimsTotal = 100
	campaign.PackageIDs = []string{"advanced"}

	authoritative := normalizeCampaignAuthority(campaign, packages)

	assert.Equal(t, 1, authoritative.MaxClaimsPerUser)
	assert.Equal(t, 1, authoritative.MaxClaimsPerEmail)
	assert.Zero(t, authoritative.MaxParticipantsTotal)
	assert.Zero(t, authoritative.MaxClaimsTotal)
}

func TestNormalizePaymentCampaignsForSaveDerivesPerCampaignClaimsByAccountLimit(t *testing.T) {
	packages := []TopupPackage{
		{ID: "advanced", Enabled: true},
		{ID: "professional", Enabled: true},
	}
	campaign := validCampaignForSafeguardTest()
	campaign.ID = "per-campaign-two-claims"
	campaign.Eligibility = CampaignEligibilityPerCampaign
	campaign.MaxClaimsPerUser = 2
	campaign.MaxClaimsPerEmail = 2
	campaign.MaxParticipantsTotal = 30
	campaign.MaxClaimsTotal = 100
	campaign.PackageIDs = []string{"advanced", "professional"}

	normalized, err := NormalizePaymentCampaignsForSave([]PaymentCampaign{campaign}, packages)

	require.NoError(t, err)
	require.Len(t, normalized, 1)
	assert.Equal(t, CampaignEligibilityPerCampaign, normalized[0].Eligibility)
	assert.Equal(t, 60, normalized[0].MaxClaimsTotal)
}

func TestLaunchCampaignLegacyMigrationIsOneTimeAcrossSaveReadback(t *testing.T) {
	packages := []TopupPackage{{ID: "advanced", Enabled: true}, {ID: "professional", Enabled: true}}
	legacy := validCampaignForSafeguardTest()
	legacy.ID = "launch-first-topup-30"
	legacy.Eligibility = CampaignEligibilityPerCampaign
	legacy.MaxClaimsTotal = 100
	legacy.MaxParticipantsTotal = 0

	migrated, err := NormalizePaymentCampaignsForSave([]PaymentCampaign{legacy}, packages)
	require.NoError(t, err)
	require.Len(t, migrated, 1)
	assert.Equal(t, CampaignEligibilityPerPackage, migrated[0].Eligibility)
	assert.Equal(t, CampaignLegacyMigrationVersionOne, migrated[0].LegacyMigrationVersion)

	adminEdited := migrated[0]
	adminEdited.Eligibility = CampaignEligibilityPerCampaign
	adminEdited.MaxClaimsPerUser = 2
	adminEdited.MaxClaimsPerEmail = 2
	adminEdited.MaxParticipantsTotal = 30
	adminEdited.MaxClaimsTotal = 0
	readback, err := NormalizePaymentCampaignsForSave([]PaymentCampaign{adminEdited}, packages)

	require.NoError(t, err)
	require.Len(t, readback, 1)
	assert.Equal(t, CampaignEligibilityPerCampaign, readback[0].Eligibility)
	assert.Equal(t, 2, readback[0].MaxClaimsPerUser)
	assert.Equal(t, 60, readback[0].MaxClaimsTotal)
	assert.Equal(t, CampaignLegacyMigrationVersionOne, readback[0].LegacyMigrationVersion)
}

func TestValidatePaymentCampaignsReturnsFieldErrors(t *testing.T) {
	packages := []TopupPackage{{ID: "advanced", Enabled: true}}
	campaign := validCampaignForSafeguardTest()
	campaign.PackageIDs = []string{"advanced"}
	campaign.StartsAt = -1
	campaign.EndsAt = -2
	campaign.ReservationMinutes = 0
	campaign.RewardPercent = 0

	err := ValidatePaymentCampaigns([]PaymentCampaign{campaign}, packages)

	require.Error(t, err)
	var validationErrors *PaymentCampaignValidationErrors
	require.ErrorAs(t, err, &validationErrors)
	assert.Contains(t, validationErrors.FieldErrors(), "campaigns.0.starts_at")
	assert.Contains(t, validationErrors.FieldErrors(), "campaigns.0.ends_at")
	assert.Contains(t, validationErrors.FieldErrors(), "campaigns.0.reservation_minutes")
	assert.Contains(t, validationErrors.FieldErrors(), "campaigns.0.reward_percent")
}

func TestValidatePaymentCampaignsRejectsAmbiguousStackingPriority(t *testing.T) {
	packages := []TopupPackage{{ID: "advanced", Enabled: true}}
	first := validCampaignForSafeguardTest()
	first.ID = "first"
	first.Enabled = true
	first.Priority = 10
	first.PackageIDs = []string{"advanced"}
	second := first
	second.ID = "second"

	err := ValidatePaymentCampaigns([]PaymentCampaign{first, second}, packages)

	require.Error(t, err)
	var validationErrors *PaymentCampaignValidationErrors
	require.ErrorAs(t, err, &validationErrors)
	assert.Contains(t, validationErrors.FieldErrors(), "campaigns.1.priority")
}

func TestGetPaymentCampaignsReturnsDeepCopy(t *testing.T) {
	campaigns := GetPaymentCampaigns()
	require.NotEmpty(t, campaigns)
	require.NotEmpty(t, campaigns[0].PackageIDs)
	originalPackageId := campaigns[0].PackageIDs[0]
	campaigns[0].PackageIDs[0] = "mutated"
	if campaigns[0].FixedBonus == nil {
		campaigns[0].FixedBonus = map[string]float64{}
	}
	campaigns[0].FixedBonus["mutated"] = 999

	fresh := GetPaymentCampaigns()
	require.NotEmpty(t, fresh)
	assert.Equal(t, originalPackageId, fresh[0].PackageIDs[0])
	_, leaked := fresh[0].FixedBonus["mutated"]
	assert.False(t, leaked)
}

func TestSupportContactsAllowEmptyAndValidateSupportedTypes(t *testing.T) {
	require.NoError(t, ValidateSupportContacts(nil))
	require.NoError(t, ValidateSupportContacts([]SupportContact{
		{ID: "qq", Type: SupportContactQQ, Value: "3184917639"},
		{ID: "wechat", Type: SupportContactWeChat, Value: "orbit-support"},
		{ID: "phone", Type: SupportContactPhone, Value: "+86 138 0000 0000"},
		{ID: "qrcode", Type: SupportContactQRCode, Value: "https://example.com/support.png"},
	}))

	assert.Error(t, ValidateSupportContacts([]SupportContact{
		{ID: "email", Type: "email", Value: "support@example.com"},
	}))
	assert.Error(t, ValidateSupportContacts([]SupportContact{
		{ID: "unsafe", Type: SupportContactQRCode, Value: "javascript:alert(1)"},
	}))
	assert.Error(t, ValidateSupportContacts([]SupportContact{
		{ID: "http", Type: SupportContactQRCode, Value: "http://example.com/support.png"},
	}))
}

func TestGetSupportContactsReturnsCopy(t *testing.T) {
	contacts := GetSupportContacts()
	require.NotEmpty(t, contacts)
	originalValue := contacts[0].Value
	contacts[0].Value = "mutated"

	fresh := GetSupportContacts()
	require.NotEmpty(t, fresh)
	assert.Equal(t, originalValue, fresh[0].Value)
}

func TestValidatePaymentCampaignSafeguards(t *testing.T) {
	packages := []TopupPackage{{
		ID: "advanced", Name: "Advanced", PayAmount: 98, CreditAmount: 100,
	}}
	campaign := validCampaignForSafeguardTest()
	campaign.PackageIDs = []string{"advanced"}
	require.NoError(t, ValidatePaymentCampaigns([]PaymentCampaign{campaign}, packages))

	campaign.MaxClaimsTotal = -1
	assert.Error(t, ValidatePaymentCampaigns([]PaymentCampaign{campaign}, packages))
	campaign = validCampaignForSafeguardTest()
	campaign.PackageIDs = []string{"advanced"}
	campaign.MaxClaimsPerEmail = -1
	assert.Error(t, ValidatePaymentCampaigns([]PaymentCampaign{campaign}, packages))
	campaign = validCampaignForSafeguardTest()
	campaign.PackageIDs = []string{"advanced"}
	campaign.ReservationMinutes = 1441
	assert.Error(t, ValidatePaymentCampaigns([]PaymentCampaign{campaign}, packages))
	campaign.ReservationMinutes = 0
	assert.Error(t, ValidatePaymentCampaigns([]PaymentCampaign{campaign}, packages))
}

func TestValidateTopupPackagesSellingPointsAndFooterNoteAndVisualStyle(t *testing.T) {
	packages := []TopupPackage{{
		ID: "valid", Name: "Valid", PayAmount: 98, CreditAmount: 100,
		SellingPoints: []string{"卖点1", "卖点2"},
		FooterNote:    "底部说明文字",
		VisualStyle:   "recommended",
	}}

	// valid
	require.NoError(t, ValidateTopupPackages(packages))

	// too many selling points
	bad := []TopupPackage{{
		ID: "too-many", Name: "Bad", PayAmount: 98, CreditAmount: 100,
		SellingPoints: []string{"a", "b", "c", "d"},
	}}
	assert.Error(t, ValidateTopupPackages(bad))

	// selling point too long
	bad2 := []TopupPackage{{
		ID: "too-long", Name: "Bad", PayAmount: 98, CreditAmount: 100,
		SellingPoints: []string{strings.Repeat("x", 81)},
	}}
	assert.Error(t, ValidateTopupPackages(bad2))

	// footer note too long
	bad3 := []TopupPackage{{
		ID: "footer-long", Name: "Bad", PayAmount: 98, CreditAmount: 100,
		FooterNote: strings.Repeat("x", 121),
	}}
	assert.Error(t, ValidateTopupPackages(bad3))

	// invalid visual style
	bad4 := []TopupPackage{{
		ID: "bad-style", Name: "Bad", PayAmount: 98, CreditAmount: 100,
		VisualStyle: "gold",
	}}
	assert.Error(t, ValidateTopupPackages(bad4))

	// empty visual style = default (valid)
	okEmpty := []TopupPackage{{
		ID: "empty-style", Name: "OK", PayAmount: 98, CreditAmount: 100,
		VisualStyle: "",
	}}
	require.NoError(t, ValidateTopupPackages(okEmpty))
}
func TestValidateTopupPackagesRejectsForbiddenContent(t *testing.T) {
	for name, content := range map[string]string{
		"html":             "<script>alert(1)</script>",
		"markdown-link":    "[click](https://evil.com)",
		"markdown-bold":    "**bold**",
		"javascript":       "javascript:alert(1)",
		"event-handler":    "onmouseover=alert(1)",
		"css-attribute":    "class=red large",
		"css-selector":     ".text-red-500",
		"tailwind-class":   "bg-red-500",
		"hex-color":        "#fff",
		"functional-color": "rgb(255, 0, 0)",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, ValidateTopupPackages([]TopupPackage{{
				ID: "forbidden", Name: "Bad", PayAmount: 98, CreditAmount: 100,
				SellingPoints: []string{content},
			}}))
		})
	}

	assert.Error(t, ValidateTopupPackages([]TopupPackage{{
		ID: "name", Name: "**Bad**", PayAmount: 98, CreditAmount: 100,
	}}))
	assert.Error(t, ValidateTopupPackages([]TopupPackage{{
		ID: "duplicate", Name: "Bad", PayAmount: 98, CreditAmount: 100,
		SellingPoints: []string{"支持主流模型", " 支持主流模型 "},
	}}))
	require.NoError(t, ValidateTopupPackages([]TopupPackage{{
		ID: "clean", Name: "Clean", PayAmount: 98, CreditAmount: 100,
		SellingPoints: []string{"支持主流模型", "赠送余额"},
		FooterNote:    "活动最终解释权归平台所有",
	}}))
}

func TestValidatePaymentCampaignsRejectsForbiddenDisplayText(t *testing.T) {
	packages := []TopupPackage{{
		ID: "advanced", Name: "Advanced", PayAmount: 98, CreditAmount: 100,
	}}
	campaign := validCampaignForSafeguardTest()
	campaign.PackageIDs = []string{"advanced"}
	campaign.BannerText = "onmouseover=alert(1)"

	assert.Error(t, ValidatePaymentCampaigns([]PaymentCampaign{campaign}, packages))
}

func TestPaymentCampaignReadsAndUpdatesShareConfigLock(t *testing.T) {
	var original []PaymentCampaign
	var originalPackages []TopupPackage
	require.True(t, config.GlobalConfig.Read("payment_setting", func(raw interface{}) {
		setting := raw.(*PaymentSetting)
		original = append([]PaymentCampaign(nil), setting.Campaigns...)
		originalPackages = append([]TopupPackage(nil), setting.TopupPackages...)
	}))
	originalJSON, err := json.Marshal(original)
	require.NoError(t, err)
	originalPackagesJSON, err := json.Marshal(originalPackages)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, restoreErr := config.GlobalConfig.Update(
			"payment_setting",
			map[string]string{
				"campaigns":      string(originalJSON),
				"topup_packages": string(originalPackagesJSON),
			},
		)
		require.NoError(t, restoreErr)
	})

	campaign := validCampaignForSafeguardTest()
	campaign.Enabled = true
	campaign.PackageIDs = []string{"advanced"}
	campaign.FixedBonus = map[string]float64{"advanced": 28}
	var waitGroup sync.WaitGroup
	start := make(chan struct{})
	for reader := 0; reader < 4; reader++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			for iteration := 0; iteration < 500; iteration++ {
				for _, snapshot := range GetPaymentCampaigns() {
					for _, packageId := range snapshot.PackageIDs {
						_ = packageId
					}
					for packageId, amount := range snapshot.FixedBonus {
						_, _ = packageId, amount
					}
				}
				_, _ = ResolveTopupPackage("advanced")
			}
		}()
	}
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		<-start
		for iteration := 0; iteration < 500; iteration++ {
			campaign.Enabled = iteration%2 == 0
			if campaign.Enabled {
				campaign.PackageIDs = []string{"advanced"}
				campaign.FixedBonus = map[string]float64{"advanced": 28}
			} else {
				campaign.PackageIDs = []string{"experience", "professional"}
				campaign.FixedBonus = map[string]float64{"experience": 5, "professional": 137}
			}
			encoded, marshalErr := json.Marshal([]PaymentCampaign{campaign})
			assert.NoError(t, marshalErr)
			packages := []TopupPackage{{
				ID: "advanced", Name: "Advanced", PayAmount: 98,
				CreditAmount: 100, Enabled: campaign.Enabled,
			}}
			packagesJSON, packagesMarshalErr := json.Marshal(packages)
			assert.NoError(t, packagesMarshalErr)
			updated, updateErr := config.GlobalConfig.Update(
				"payment_setting",
				map[string]string{
					"campaigns":      string(encoded),
					"topup_packages": string(packagesJSON),
				},
			)
			assert.True(t, updated)
			assert.NoError(t, updateErr)
		}
	}()
	close(start)
	waitGroup.Wait()
}

func TestNormalizeLegacyCampaignTextExactMatchAllThreeFields(t *testing.T) {
	campaign := normalizeLegacyCampaignSafeguards(PaymentCampaign{
		ID:                   "launch-first-topup-30",
		BannerTitle:          "First top-up bonus: 30%",
		BannerText:           "Complete your first top-up in this campaign to receive time-limited bonus balance.",
		BadgeText:            "First top-up +30%",
		Eligibility:          CampaignEligibilityPerPackage,
		MaxClaimsPerEmail:    1,
		MaxParticipantsTotal: 100,
		ReservationMinutes:   3,
	})

	assert.Equal(t, "Limited four-package bonus: 30%", campaign.BannerTitle)
	assert.Equal(t, "Each account can receive the campaign bonus once per eligible package.", campaign.BannerText)
	assert.Equal(t, "Limited bonus +30%", campaign.BadgeText)
}

func TestNormalizeLegacyCampaignTextPreservesCustomizedBannerTitle(t *testing.T) {
	campaign := normalizeLegacyCampaignSafeguards(PaymentCampaign{
		ID:                   "launch-first-topup-30",
		BannerTitle:          "Custom banner title",
		BannerText:           "Complete your first top-up in this campaign to receive time-limited bonus balance.",
		BadgeText:            "First top-up +30%",
		Eligibility:          CampaignEligibilityPerPackage,
		MaxClaimsPerEmail:    1,
		MaxParticipantsTotal: 100,
		ReservationMinutes:   3,
	})

	assert.Equal(t, "Custom banner title", campaign.BannerTitle)
	assert.Equal(t, "Complete your first top-up in this campaign to receive time-limited bonus balance.", campaign.BannerText)
	assert.Equal(t, "First top-up +30%", campaign.BadgeText)
}

func TestNormalizeLegacyCampaignTextPreservesCustomizedBadgeText(t *testing.T) {
	campaign := normalizeLegacyCampaignSafeguards(PaymentCampaign{
		ID:                   "launch-first-topup-30",
		BannerTitle:          "First top-up bonus: 30%",
		BannerText:           "Complete your first top-up in this campaign to receive time-limited bonus balance.",
		BadgeText:            "Custom badge",
		Eligibility:          CampaignEligibilityPerPackage,
		MaxClaimsPerEmail:    1,
		MaxParticipantsTotal: 100,
		ReservationMinutes:   3,
	})

	assert.Equal(t, "First top-up bonus: 30%", campaign.BannerTitle)
	assert.Equal(t, "Complete your first top-up in this campaign to receive time-limited bonus balance.", campaign.BannerText)
	assert.Equal(t, "Custom badge", campaign.BadgeText)
}

func TestNormalizeLegacyCampaignTextSkipsNonLaunchCampaign(t *testing.T) {
	campaign := normalizeLegacyCampaignSafeguards(PaymentCampaign{
		ID:                   "other-campaign",
		BannerTitle:          "First top-up bonus: 30%",
		BannerText:           "Complete your first top-up in this campaign to receive time-limited bonus balance.",
		BadgeText:            "First top-up +30%",
		Eligibility:          CampaignEligibilityPerPackage,
		MaxClaimsPerEmail:    1,
		MaxParticipantsTotal: 100,
		ReservationMinutes:   3,
	})

	assert.Equal(t, "First top-up bonus: 30%", campaign.BannerTitle)
	assert.Equal(t, "Complete your first top-up in this campaign to receive time-limited bonus balance.", campaign.BannerText)
	assert.Equal(t, "First top-up +30%", campaign.BadgeText)
}

func TestNormalizeLegacyCampaignTextAlreadyNewValuesIsNoop(t *testing.T) {
	campaign := normalizeLegacyCampaignSafeguards(PaymentCampaign{
		ID:                   "launch-first-topup-30",
		BannerTitle:          "Limited four-package bonus: 30%",
		BannerText:           "Each account can receive the campaign bonus once per eligible package.",
		BadgeText:            "Limited bonus +30%",
		Eligibility:          CampaignEligibilityPerPackage,
		MaxClaimsPerEmail:    1,
		MaxParticipantsTotal: 100,
		ReservationMinutes:   3,
	})

	assert.Equal(t, "Limited four-package bonus: 30%", campaign.BannerTitle)
	assert.Equal(t, "Each account can receive the campaign bonus once per eligible package.", campaign.BannerText)
	assert.Equal(t, "Limited bonus +30%", campaign.BadgeText)
}

func TestNormalizeLegacyCampaignTextWithOldEligibilityMigratesBoth(t *testing.T) {
	// Regression: old per_campaign eligibility with max_claims_total > 0
	// used to return early before the text normalization ran.
	// This test asserts both the eligibility migration AND text
	// normalization are applied when all legacy fields match.
	campaign := normalizeLegacyCampaignSafeguards(PaymentCampaign{
		ID:                   "launch-first-topup-30",
		BannerTitle:          "First top-up bonus: 30%",
		BannerText:           "Complete your first top-up in this campaign to receive time-limited bonus balance.",
		BadgeText:            "First top-up +30%",
		Eligibility:          CampaignEligibilityPerCampaign,
		MaxClaimsPerEmail:    0,
		MaxClaimsTotal:       100,
		MaxParticipantsTotal: 0,
		ReservationMinutes:   0,
	})

	assert.Equal(t, "Limited four-package bonus: 30%", campaign.BannerTitle)
	assert.Equal(t, "Each account can receive the campaign bonus once per eligible package.", campaign.BannerText)
	assert.Equal(t, "Limited bonus +30%", campaign.BadgeText)
	assert.Equal(t, 1, campaign.MaxClaimsPerEmail)
	assert.Equal(t, 100, campaign.MaxParticipantsTotal)
	assert.Equal(t, 0, campaign.MaxClaimsTotal)
	assert.Equal(t, 3, campaign.ReservationMinutes)
	assert.Equal(t, CampaignEligibilityPerPackage, campaign.Eligibility)
}
