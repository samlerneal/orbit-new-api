package operation_setting

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type PaymentSetting struct {
	AmountOptions    []int             `json:"amount_options"`
	AmountDiscount   map[int]float64   `json:"amount_discount"` // 充值金额对应的折扣，例如 100 元 0.9 表示 100 元充值享受 9 折优惠
	TopupPackages    []TopupPackage    `json:"topup_packages"`
	Campaigns        []PaymentCampaign `json:"campaigns"`
	SupportContacts  []SupportContact  `json:"support_contacts"`
	PromotionEnabled bool              `json:"promotion_enabled"`

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`
}

type TopupPackage struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Tag           string   `json:"tag"`
	PayAmount     float64  `json:"pay_amount"`
	CreditAmount  float64  `json:"credit_amount"`
	Enabled       bool     `json:"enabled"`
	SortOrder     int      `json:"sort_order"`
	SellingPoints []string `json:"selling_points"`
	FooterNote    string   `json:"footer_note"`
	VisualStyle   string   `json:"visual_style"`
}

type SupportContact struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

const (
	CampaignEligibilityPerCampaign = "per_campaign"
	CampaignEligibilityPerPackage  = "per_package"
	CampaignEligibilityUnlimited   = "unlimited"

	CampaignRewardTargetTotalPercent = "target_total_percent"
	CampaignRewardFixedBonus         = "fixed_bonus"

	CampaignRoundingCeilYuan = "ceil_yuan"

	DefaultCampaignReservationMinutes = 3
	CampaignLegacyMigrationVersionOne = 1

	SupportContactQQ     = "qq"
	SupportContactWeChat = "wechat"
	SupportContactPhone  = "phone"
	SupportContactQRCode = "qrcode"
)

// PaymentCampaign is a controlled, data-only promotion rule. Administrators
// can combine supported conditions without executing custom code.
type PaymentCampaign struct {
	ID                     string             `json:"id"`
	Name                   string             `json:"name"`
	BannerTitle            string             `json:"banner_title"`
	BannerText             string             `json:"banner_text"`
	BadgeText              string             `json:"badge_text"`
	Enabled                bool               `json:"enabled"`
	StartsAt               int64              `json:"starts_at"`
	EndsAt                 int64              `json:"ends_at"`
	PackageIDs             []string           `json:"package_ids"`
	Eligibility            string             `json:"eligibility"`
	MaxClaimsPerUser       int                `json:"max_claims_per_user"`
	MaxClaimsPerEmail      int                `json:"max_claims_per_email"`
	MaxClaimsTotal         int                `json:"max_claims_total"`
	MaxParticipantsTotal   int                `json:"max_participants_total"`
	ReservationMinutes     int                `json:"reservation_minutes"`
	RewardMode             string             `json:"reward_mode"`
	RewardPercent          float64            `json:"reward_percent"`
	FixedBonus             map[string]float64 `json:"fixed_bonus"`
	RoundingMode           string             `json:"rounding_mode"`
	ValidDays              int                `json:"valid_days"`
	Stackable              bool               `json:"stackable"`
	Priority               int                `json:"priority"`
	LegacyMigrationVersion int                `json:"legacy_migration_version,omitempty"`
}

type PaymentCampaignFieldError struct {
	Index      int    `json:"index"`
	CampaignID string `json:"campaign_id"`
	Field      string `json:"field"`
	Message    string `json:"message"`
}

type PaymentCampaignValidationErrors struct {
	Errors []PaymentCampaignFieldError `json:"errors"`
}

func (validationErrors *PaymentCampaignValidationErrors) Error() string {
	if validationErrors == nil || len(validationErrors.Errors) == 0 {
		return "payment campaign validation failed"
	}
	return validationErrors.Errors[0].Message
}

func (validationErrors *PaymentCampaignValidationErrors) FieldErrors() map[string]string {
	fieldErrors := make(map[string]string, len(validationErrors.Errors))
	for _, validationError := range validationErrors.Errors {
		key := fmt.Sprintf("campaigns.%d.%s", validationError.Index, validationError.Field)
		if _, exists := fieldErrors[key]; !exists {
			fieldErrors[key] = validationError.Message
		}
	}
	return fieldErrors
}

const (
	CurrentComplianceTermsVersion = "v1"
	CurrentRefundNoticeVersion    = "refund-notice-v1"
)

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:  []int{10, 20, 50, 100, 200, 500},
	AmountDiscount: map[int]float64{},
	TopupPackages: []TopupPackage{
		{ID: "experience", Name: "体验", Description: "适合日常对话", PayAmount: 14, CreditAmount: 14, Enabled: true, SortOrder: 10},
		{ID: "standard", Name: "标准", Description: "适合解决复杂问题", Tag: "人气之选", PayAmount: 49, CreditAmount: 50, Enabled: true, SortOrder: 20},
		{ID: "advanced", Name: "进阶", Description: "适合频繁使用", Tag: "最受欢迎", PayAmount: 98, CreditAmount: 100, Enabled: true, SortOrder: 30},
		{ID: "professional", Name: "专业", Description: "为专业开发者打造", Tag: "高量超值", PayAmount: 490, CreditAmount: 500, Enabled: true, SortOrder: 40},
	},
	Campaigns: []PaymentCampaign{
		{
			ID:                   "launch-first-topup-30",
			Name:                 "Launch first top-up",
			BannerTitle:          "Limited four-package bonus: 30%",
			BannerText:           "Each account can receive the campaign bonus once per eligible package.",
			BadgeText:            "Limited bonus +30%",
			Enabled:              false,
			PackageIDs:           []string{"experience", "standard", "advanced", "professional"},
			Eligibility:          CampaignEligibilityPerPackage,
			MaxClaimsPerUser:     1,
			MaxClaimsPerEmail:    1,
			MaxClaimsTotal:       0,
			MaxParticipantsTotal: 100,
			ReservationMinutes:   3,
			RewardMode:           CampaignRewardTargetTotalPercent,
			RewardPercent:        30,
			RoundingMode:         CampaignRoundingCeilYuan,
			ValidDays:            45,
			Stackable:            false,
			Priority:             100,
		},
	},
	SupportContacts: []SupportContact{
		{ID: "support-qq", Type: SupportContactQQ, Value: "3184917639"},
	},
	PromotionEnabled: false,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

func GetTopupPackages() []TopupPackage {
	var packages []TopupPackage
	config.GlobalConfig.Read("payment_setting", func(raw interface{}) {
		setting := raw.(*PaymentSetting)
		packages = make([]TopupPackage, len(setting.TopupPackages))
		copy(packages, setting.TopupPackages)
	})
	return packages
}

func GetPaymentCampaigns() []PaymentCampaign {
	var campaigns []PaymentCampaign
	var packages []TopupPackage
	config.GlobalConfig.Read("payment_setting", func(raw interface{}) {
		setting := raw.(*PaymentSetting)
		campaigns = make([]PaymentCampaign, len(setting.Campaigns))
		for i, campaign := range setting.Campaigns {
			copied := campaign
			copied.PackageIDs = append([]string(nil), campaign.PackageIDs...)
			if campaign.FixedBonus != nil {
				copied.FixedBonus = make(map[string]float64, len(campaign.FixedBonus))
				for packageId, amount := range campaign.FixedBonus {
					copied.FixedBonus[packageId] = amount
				}
			}
			campaigns[i] = copied
		}
		packages = make([]TopupPackage, len(setting.TopupPackages))
		copy(packages, setting.TopupPackages)
	})
	for index := range campaigns {
		campaigns[index] = normalizeCampaignAuthority(campaigns[index], packages)
	}
	return campaigns
}

func GetSupportContacts() []SupportContact {
	var contacts []SupportContact
	config.GlobalConfig.Read("payment_setting", func(raw interface{}) {
		setting := raw.(*PaymentSetting)
		contacts = make([]SupportContact, 0, len(setting.SupportContacts))
		for _, contact := range setting.SupportContacts {
			contact.ID = strings.TrimSpace(contact.ID)
			contact.Type = strings.ToLower(strings.TrimSpace(contact.Type))
			contact.Value = strings.TrimSpace(contact.Value)
			if contact.ID != "" && contact.Type != "" && contact.Value != "" {
				contacts = append(contacts, contact)
			}
		}
	})
	return contacts
}

func normalizeLegacyCampaignSafeguards(campaign PaymentCampaign) PaymentCampaign {
	if campaign.ReservationMinutes <= 0 {
		campaign.ReservationMinutes = DefaultCampaignReservationMinutes
	}
	// Compatibility normalization: exact-match legacy campaign text
	// is mapped to the current four-package wording at read time.
	// This must run before the per-campaign migration block below
	// because that block returns early, and the text mapping must
	// apply regardless of which eligibility branch is taken.
	// Admins who have customized any of these fields are never
	// affected; the normalized values will be persisted the next
	// time the admin saves the campaign.
	if campaign.ID == "launch-first-topup-30" &&
		campaign.BannerTitle == "First top-up bonus: 30%" &&
		campaign.BannerText == "Complete your first top-up in this campaign to receive time-limited bonus balance." &&
		campaign.BadgeText == "First top-up +30%" {
		campaign.BannerTitle = "Limited four-package bonus: 30%"
		campaign.BannerText = "Each account can receive the campaign bonus once per eligible package."
		campaign.BadgeText = "Limited bonus +30%"
	}
	// Compatibility migration: per_campaign activity configs with
	// max_claims_total become per_package with participant capacity.
	// This is an expected business change approved by Owner, not a
	// no-op normalization.
	if campaign.ID == "launch-first-topup-30" &&
		campaign.LegacyMigrationVersion < CampaignLegacyMigrationVersionOne &&
		(campaign.Eligibility == "" || campaign.Eligibility == CampaignEligibilityPerCampaign) &&
		campaign.MaxClaimsTotal > 0 {
		if campaign.MaxClaimsPerEmail == 0 {
			campaign.MaxClaimsPerEmail = 1
		}
		campaign.Eligibility = CampaignEligibilityPerPackage
		if campaign.MaxParticipantsTotal == 0 {
			campaign.MaxParticipantsTotal = campaign.MaxClaimsTotal
		}
		campaign.MaxClaimsTotal = 0
		campaign.LegacyMigrationVersion = CampaignLegacyMigrationVersionOne
		return campaign
	}
	if campaign.ID == "launch-first-topup-30" &&
		campaign.LegacyMigrationVersion < CampaignLegacyMigrationVersionOne &&
		campaign.MaxClaimsPerEmail == 0 &&
		campaign.MaxClaimsTotal == 0 &&
		campaign.MaxParticipantsTotal == 0 {
		campaign.MaxClaimsPerEmail = 1
		campaign.MaxParticipantsTotal = 100
	}
	return campaign
}

func applicableCampaignPackageCount(campaign PaymentCampaign, packages []TopupPackage) int {
	eligiblePackages := make(map[string]struct{}, len(packages))
	for _, packageOption := range packages {
		if packageOption.Enabled {
			eligiblePackages[packageOption.ID] = struct{}{}
		}
	}
	if len(campaign.PackageIDs) == 0 {
		return len(eligiblePackages)
	}
	applicable := make(map[string]struct{}, len(campaign.PackageIDs))
	for _, packageID := range campaign.PackageIDs {
		if _, exists := eligiblePackages[packageID]; exists {
			applicable[packageID] = struct{}{}
		}
	}
	return len(applicable)
}

func derivedCampaignClaimLimit(campaign PaymentCampaign, packages []TopupPackage) int {
	if campaign.MaxParticipantsTotal <= 0 {
		return 0
	}
	claimsPerParticipant := campaign.MaxClaimsPerUser
	if claimsPerParticipant <= 0 {
		claimsPerParticipant = 1
	}
	packageCount := applicableCampaignPackageCount(campaign, packages)
	if packageCount == 0 {
		return 0
	}
	if campaign.Eligibility != CampaignEligibilityPerPackage {
		packageCount = 1
	}
	return campaign.MaxParticipantsTotal * claimsPerParticipant * packageCount
}

func normalizeCampaignAuthority(campaign PaymentCampaign, packages []TopupPackage) PaymentCampaign {
	normalized := normalizeLegacyCampaignSafeguards(campaign)
	if normalized.Eligibility == CampaignEligibilityPerPackage {
		// The controlled four-package contract is one claim per account and
		// verified email for each package. Do not trust legacy/custom values
		// that would allow a second claim for the same account and package.
		normalized.MaxClaimsPerUser = 1
		normalized.MaxClaimsPerEmail = 1
	}
	if normalized.MaxParticipantsTotal > 0 {
		normalized.MaxClaimsTotal = derivedCampaignClaimLimit(normalized, packages)
	} else {
		// Zero participant capacity means unlimited participants; a non-zero
		// total claim cap would be ambiguous and must not preserve legacy 100.
		normalized.MaxClaimsTotal = 0
	}
	return normalized
}

func NormalizePaymentCampaignsForSave(campaigns []PaymentCampaign, packages []TopupPackage) ([]PaymentCampaign, error) {
	normalized := make([]PaymentCampaign, len(campaigns))
	for index, campaign := range campaigns {
		copied := campaign
		copied.PackageIDs = append([]string(nil), campaign.PackageIDs...)
		if campaign.FixedBonus != nil {
			copied.FixedBonus = make(map[string]float64, len(campaign.FixedBonus))
			for packageID, amount := range campaign.FixedBonus {
				copied.FixedBonus[packageID] = amount
			}
		}
		normalized[index] = normalizeCampaignAuthority(copied, packages)
	}
	if err := ValidatePaymentCampaigns(normalized, packages); err != nil {
		return nil, err
	}
	return normalized, nil
}

// containsForbiddenContent rejects HTML tags, Markdown syntax,
// JavaScript fragments, CSS class names and raw colour values in
// admin-controlled free-text fields. The frontend renders these
// as plain text, but the contract forbids submission of such
// content so it is rejected at the server boundary.
var forbiddenControlledTextPatterns = []*regexp.Regexp{
	regexp.MustCompile(`!?\[[^]]*]\([^)]*\)`),
	regexp.MustCompile(`(?m)(^|[\r\n])[[:space:]]{0,3}(#{1,6}|>|[-+*])[[:space:]]+`),
	regexp.MustCompile(`(^|[[:space:](])[*_][^*_\r\n]+[*_]([[:space:]).,!?:;]|$)`),
	regexp.MustCompile(`(?i)javascript[[:space:]]*:`),
	regexp.MustCompile(`(?i)\bon[a-z]+[[:space:]]*=`),
	regexp.MustCompile(`(?i)\b(class|classname|style)[[:space:]]*=`),
	regexp.MustCompile(`(?i)(^|[[:space:];{])\.[a-z_-][a-z0-9_-]*`),
	regexp.MustCompile(`(?i)\b(bg|text|border|ring|from|via|to)-([a-z]+|\[[^]]+])(-[0-9]{1,3})?\b`),
	regexp.MustCompile(`(?i)#[0-9a-f]{3,8}\b`),
	regexp.MustCompile(`(?i)\b(rgb|rgba|hsl|hsla|oklch)[[:space:]]*\(`),
	regexp.MustCompile(`(?i)\b(color|background(-color)?|border-color)[[:space:]]*:`),
}

func containsForbiddenContent(value string) bool {
	if value == "" {
		return false
	}
	if strings.ContainsAny(value, "<>`") ||
		strings.Contains(value, "**") ||
		strings.Contains(value, "__") ||
		strings.Contains(value, "~~") {
		return true
	}
	for _, pattern := range forbiddenControlledTextPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func ResolveTopupPackage(packageID string) (TopupPackage, bool) {
	for _, packageOption := range GetTopupPackages() {
		if packageOption.ID != packageID || packageOption.PayAmount <= 0 || !packageOption.Enabled {
			continue
		}
		if packageOption.CreditAmount <= 0 {
			packageOption.CreditAmount = packageOption.PayAmount
		}
		return packageOption, true
	}

	return TopupPackage{}, false
}

func ValidateTopupPackages(packages []TopupPackage) error {
	if len(packages) == 0 || len(packages) > 20 {
		return errors.New("topup packages must contain between 1 and 20 items")
	}
	seen := make(map[string]struct{}, len(packages))
	for _, packageOption := range packages {
		id := strings.TrimSpace(packageOption.ID)
		if id == "" || len(id) > 64 {
			return errors.New("topup package id is required and must be at most 64 characters")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate topup package id: %s", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(packageOption.Name) == "" || len(packageOption.Name) > 80 {
			return fmt.Errorf("invalid name for topup package %s", id)
		}
		if len(packageOption.Description) > 160 || len(packageOption.Tag) > 40 {
			return fmt.Errorf("topup package %s text is too long", id)
		}
		for field, value := range map[string]string{
			"name":        packageOption.Name,
			"description": packageOption.Description,
			"tag":         packageOption.Tag,
		} {
			if containsForbiddenContent(value) {
				return fmt.Errorf("topup package %s %s contains forbidden content", id, field)
			}
		}
		if packageOption.PayAmount <= 0 || packageOption.CreditAmount < packageOption.PayAmount {
			return fmt.Errorf("topup package %s has invalid amounts", id)
		}
		if packageOption.PayAmount > 100000 || packageOption.CreditAmount > 100000 {
			return fmt.Errorf("topup package %s exceeds the amount limit", id)
		}
		if len(packageOption.SellingPoints) > 3 {
			return fmt.Errorf("topup package %s has more than 3 selling points", id)
		}
		seenSellingPoints := make(map[string]struct{}, len(packageOption.SellingPoints))
		for idx, point := range packageOption.SellingPoints {
			if len(point) > 80 {
				return fmt.Errorf("topup package %s selling point %d exceeds 80 characters", id, idx+1)
			}
			if containsForbiddenContent(point) {
				return fmt.Errorf("topup package %s selling point %d contains forbidden content", id, idx+1)
			}
			normalizedPoint := strings.ToLower(strings.TrimSpace(point))
			if normalizedPoint != "" {
				if _, exists := seenSellingPoints[normalizedPoint]; exists {
					return fmt.Errorf("topup package %s has duplicate selling points", id)
				}
				seenSellingPoints[normalizedPoint] = struct{}{}
			}
		}
		if len(packageOption.FooterNote) > 120 {
			return fmt.Errorf("topup package %s footer note exceeds 120 characters", id)
		}
		if containsForbiddenContent(packageOption.FooterNote) {
			return fmt.Errorf("topup package %s footer note contains forbidden content", id)
		}
		switch packageOption.VisualStyle {
		case "", "default", "recommended", "popular", "value":
		default:
			return fmt.Errorf("topup package %s has an unsupported visual style: %s", id, packageOption.VisualStyle)
		}
	}
	return nil
}

func ValidateSupportContacts(contacts []SupportContact) error {
	if len(contacts) > 8 {
		return errors.New("support contacts cannot exceed 8 items")
	}
	seen := make(map[string]struct{}, len(contacts))
	for _, contact := range contacts {
		id := strings.TrimSpace(contact.ID)
		contactType := strings.ToLower(strings.TrimSpace(contact.Type))
		value := strings.TrimSpace(contact.Value)
		if id == "" || len(id) > 64 {
			return errors.New("support contact id is required and must be at most 64 characters")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate support contact id: %s", id)
		}
		seen[id] = struct{}{}
		if value == "" || len(value) > 512 {
			return errors.New("support contact value is required and must be at most 512 characters")
		}
		switch contactType {
		case SupportContactQQ, SupportContactWeChat, SupportContactPhone:
		case SupportContactQRCode:
			parsed, err := url.Parse(value)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
				return errors.New("support QR code must use a valid HTTPS image URL")
			}
		default:
			return fmt.Errorf("unsupported support contact type: %s", contactType)
		}
	}
	return nil
}

func appendCampaignValidationError(
	validationErrors *PaymentCampaignValidationErrors,
	index int,
	campaignID string,
	field string,
	message string,
) {
	validationErrors.Errors = append(validationErrors.Errors, PaymentCampaignFieldError{
		Index: index, CampaignID: campaignID, Field: field, Message: message,
	})
}

func campaignPackagesOverlap(first PaymentCampaign, second PaymentCampaign) bool {
	if len(first.PackageIDs) == 0 || len(second.PackageIDs) == 0 {
		return true
	}
	firstPackages := make(map[string]struct{}, len(first.PackageIDs))
	for _, packageID := range first.PackageIDs {
		firstPackages[packageID] = struct{}{}
	}
	for _, packageID := range second.PackageIDs {
		if _, exists := firstPackages[packageID]; exists {
			return true
		}
	}
	return false
}

func ValidatePaymentCampaigns(campaigns []PaymentCampaign, packages []TopupPackage) error {
	validationErrors := &PaymentCampaignValidationErrors{}
	if len(campaigns) > 50 {
		appendCampaignValidationError(validationErrors, 0, "", "id", "payment campaigns cannot exceed 50 items")
		return validationErrors
	}
	packageIDs := make(map[string]TopupPackage, len(packages))
	for _, packageOption := range packages {
		packageIDs[packageOption.ID] = packageOption
	}
	seen := make(map[string]struct{}, len(campaigns))
	for index, campaign := range campaigns {
		id := strings.TrimSpace(campaign.ID)
		if id == "" || len(id) > 64 {
			appendCampaignValidationError(validationErrors, index, id, "id", "campaign id is required and must be at most 64 characters")
		} else if _, exists := seen[id]; exists {
			appendCampaignValidationError(validationErrors, index, id, "id", fmt.Sprintf("duplicate campaign id: %s", id))
		}
		seen[id] = struct{}{}

		textFields := []struct {
			field    string
			value    string
			maxLen   int
			required bool
		}{
			{field: "name", value: campaign.Name, maxLen: 100, required: true},
			{field: "banner_title", value: campaign.BannerTitle, maxLen: 120},
			{field: "banner_text", value: campaign.BannerText, maxLen: 300},
			{field: "badge_text", value: campaign.BadgeText, maxLen: 50},
		}
		for _, textField := range textFields {
			if (textField.required && strings.TrimSpace(textField.value) == "") || len(textField.value) > textField.maxLen {
				appendCampaignValidationError(validationErrors, index, id, textField.field, fmt.Sprintf("campaign %s has invalid %s", id, textField.field))
			} else if containsForbiddenContent(textField.value) {
				appendCampaignValidationError(validationErrors, index, id, textField.field, fmt.Sprintf("campaign %s %s contains forbidden content", id, textField.field))
			}
		}

		if campaign.StartsAt < 0 {
			appendCampaignValidationError(validationErrors, index, id, "starts_at", fmt.Sprintf("campaign %s start time cannot be negative", id))
		}
		if campaign.EndsAt < 0 {
			appendCampaignValidationError(validationErrors, index, id, "ends_at", fmt.Sprintf("campaign %s end time cannot be negative", id))
		}
		if campaign.StartsAt > 0 && campaign.EndsAt > 0 && campaign.EndsAt <= campaign.StartsAt {
			appendCampaignValidationError(validationErrors, index, id, "ends_at", fmt.Sprintf("campaign %s end time must be after start time", id))
		}
		if campaign.ValidDays < 1 || campaign.ValidDays > 3650 {
			appendCampaignValidationError(validationErrors, index, id, "valid_days", fmt.Sprintf("campaign %s valid days must be between 1 and 3650", id))
		}
		if campaign.MaxClaimsPerUser < 0 || campaign.MaxClaimsPerUser > 1000 {
			appendCampaignValidationError(validationErrors, index, id, "max_claims_per_user", fmt.Sprintf("campaign %s has an invalid per-user claim limit", id))
		}
		if campaign.MaxClaimsPerEmail < 0 || campaign.MaxClaimsPerEmail > 1000 {
			appendCampaignValidationError(validationErrors, index, id, "max_claims_per_email", fmt.Sprintf("campaign %s has an invalid per-email claim limit", id))
		}
		if campaign.MaxClaimsTotal < 0 || campaign.MaxClaimsTotal > 1000000 {
			appendCampaignValidationError(validationErrors, index, id, "max_claims_total", fmt.Sprintf("campaign %s has an invalid total claim limit", id))
		}
		if campaign.MaxParticipantsTotal < 0 || campaign.MaxParticipantsTotal > 1000000 {
			appendCampaignValidationError(validationErrors, index, id, "max_participants_total", fmt.Sprintf("campaign %s has an invalid total participant limit", id))
		}
		if campaign.ReservationMinutes < 1 || campaign.ReservationMinutes > 1440 {
			appendCampaignValidationError(validationErrors, index, id, "reservation_minutes", fmt.Sprintf("campaign %s reservation minutes must be between 1 and 1440", id))
		}
		switch campaign.Eligibility {
		case CampaignEligibilityPerCampaign, CampaignEligibilityPerPackage:
		case CampaignEligibilityUnlimited:
			if campaign.MaxClaimsPerUser != 0 {
				appendCampaignValidationError(validationErrors, index, id, "max_claims_per_user", fmt.Sprintf("campaign %s must use zero claim limit when eligibility is unlimited", id))
			}
			if campaign.MaxParticipantsTotal > 0 {
				appendCampaignValidationError(validationErrors, index, id, "max_participants_total", fmt.Sprintf("campaign %s cannot combine unlimited eligibility with a participant limit", id))
			}
		default:
			appendCampaignValidationError(validationErrors, index, id, "eligibility", fmt.Sprintf("campaign %s has an unsupported eligibility rule", id))
		}
		switch campaign.RewardMode {
		case CampaignRewardTargetTotalPercent:
			if campaign.RewardPercent <= 0 || campaign.RewardPercent > 1000 {
				appendCampaignValidationError(validationErrors, index, id, "reward_percent", fmt.Sprintf("campaign %s reward percent must be between 0 and 1000", id))
			}
		case CampaignRewardFixedBonus:
			if len(campaign.FixedBonus) == 0 {
				appendCampaignValidationError(validationErrors, index, id, "fixed_bonus", fmt.Sprintf("campaign %s fixed bonus is empty", id))
			}
		default:
			appendCampaignValidationError(validationErrors, index, id, "reward_mode", fmt.Sprintf("campaign %s has an unsupported reward mode", id))
		}
		if campaign.RoundingMode != CampaignRoundingCeilYuan {
			appendCampaignValidationError(validationErrors, index, id, "rounding_mode", fmt.Sprintf("campaign %s has an unsupported rounding mode", id))
		}
		if campaign.Priority < -1000000 || campaign.Priority > 1000000 {
			appendCampaignValidationError(validationErrors, index, id, "priority", fmt.Sprintf("campaign %s priority is outside the supported range", id))
		}

		seenPackageIDs := make(map[string]struct{}, len(campaign.PackageIDs))
		for _, packageID := range campaign.PackageIDs {
			if _, duplicate := seenPackageIDs[packageID]; duplicate {
				appendCampaignValidationError(validationErrors, index, id, "package_ids", fmt.Sprintf("campaign %s references package %s more than once", id, packageID))
			}
			seenPackageIDs[packageID] = struct{}{}
			if _, exists := packageIDs[packageID]; !exists {
				appendCampaignValidationError(validationErrors, index, id, "package_ids", fmt.Sprintf("campaign %s references unknown package %s", id, packageID))
			}
		}
		for packageID, amount := range campaign.FixedBonus {
			if _, exists := packageIDs[packageID]; !exists || amount < 0 || amount > 100000 {
				appendCampaignValidationError(validationErrors, index, id, "fixed_bonus", fmt.Sprintf("campaign %s has an invalid fixed bonus for package %s", id, packageID))
			}
		}
		if campaign.RewardMode == CampaignRewardFixedBonus {
			requiredPackages := campaign.PackageIDs
			if len(requiredPackages) == 0 {
				requiredPackages = make([]string, 0, len(packages))
				for _, packageOption := range packages {
					if packageOption.Enabled {
						requiredPackages = append(requiredPackages, packageOption.ID)
					}
				}
			}
			for _, packageID := range requiredPackages {
				if campaign.FixedBonus[packageID] <= 0 {
					appendCampaignValidationError(validationErrors, index, id, "fixed_bonus", fmt.Sprintf("campaign %s requires a positive fixed bonus for package %s", id, packageID))
				}
			}
		}
	}

	for index, campaign := range campaigns {
		if !campaign.Enabled {
			continue
		}
		for previousIndex := 0; previousIndex < index; previousIndex++ {
			previous := campaigns[previousIndex]
			if !previous.Enabled || previous.Priority != campaign.Priority || !campaignPackagesOverlap(previous, campaign) {
				continue
			}
			if previous.Stackable && campaign.Stackable {
				continue
			}
			appendCampaignValidationError(
				validationErrors,
				index,
				strings.TrimSpace(campaign.ID),
				"priority",
				fmt.Sprintf("campaign %s has ambiguous priority with campaign %s for overlapping packages", campaign.ID, previous.ID),
			)
		}
	}
	if len(validationErrors.Errors) > 0 {
		return validationErrors
	}
	return nil
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}
