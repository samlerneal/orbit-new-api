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
