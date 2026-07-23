package operation_setting

import (
	"errors"
	"fmt"
	"net/url"
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
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Tag          string  `json:"tag"`
	PayAmount    float64 `json:"pay_amount"`
	CreditAmount float64 `json:"credit_amount"`
	Enabled      bool    `json:"enabled"`
	SortOrder    int     `json:"sort_order"`
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

	SupportContactQQ     = "qq"
	SupportContactWeChat = "wechat"
	SupportContactPhone  = "phone"
	SupportContactQRCode = "qrcode"
)

// PaymentCampaign is a controlled, data-only promotion rule. Administrators
// can combine supported conditions without executing custom code.
type PaymentCampaign struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	BannerTitle        string             `json:"banner_title"`
	BannerText         string             `json:"banner_text"`
	BadgeText          string             `json:"badge_text"`
	Enabled            bool               `json:"enabled"`
	StartsAt           int64              `json:"starts_at"`
	EndsAt             int64              `json:"ends_at"`
	PackageIDs         []string           `json:"package_ids"`
	Eligibility        string             `json:"eligibility"`
	MaxClaimsPerUser   int                `json:"max_claims_per_user"`
	MaxClaimsPerEmail  int                `json:"max_claims_per_email"`
	MaxClaimsTotal     int                `json:"max_claims_total"`
	ReservationMinutes int                `json:"reservation_minutes"`
	RewardMode         string             `json:"reward_mode"`
	RewardPercent      float64            `json:"reward_percent"`
	FixedBonus         map[string]float64 `json:"fixed_bonus"`
	RoundingMode       string             `json:"rounding_mode"`
	ValidDays          int                `json:"valid_days"`
	Stackable          bool               `json:"stackable"`
	Priority           int                `json:"priority"`
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
		{ID: "experience", Name: "Experience", Description: "Best for daily conversations", PayAmount: 14, CreditAmount: 14, Enabled: true, SortOrder: 10},
		{ID: "standard", Name: "Standard", Description: "Best for solving complex problems", Tag: "Daily choice", PayAmount: 49, CreditAmount: 50, Enabled: true, SortOrder: 20},
		{ID: "advanced", Name: "Advanced package", Description: "Best for frequent use", Tag: "Most popular", PayAmount: 98, CreditAmount: 100, Enabled: true, SortOrder: 30},
		{ID: "professional", Name: "Professional", Description: "Built for professional developers", Tag: "High-volume value", PayAmount: 490, CreditAmount: 500, Enabled: true, SortOrder: 40},
	},
	Campaigns: []PaymentCampaign{
		{
			ID:                 "launch-first-topup-30",
			Name:               "Launch first top-up",
			BannerTitle:        "First top-up bonus: 30%",
			BannerText:         "Complete your first top-up in this campaign to receive time-limited bonus balance.",
			BadgeText:          "First top-up +30%",
			Enabled:            false,
			PackageIDs:         []string{"experience", "standard", "advanced", "professional"},
			Eligibility:        CampaignEligibilityPerCampaign,
			MaxClaimsPerUser:   1,
			MaxClaimsPerEmail:  1,
			MaxClaimsTotal:     100,
			ReservationMinutes: 3,
			RewardMode:         CampaignRewardTargetTotalPercent,
			RewardPercent:      30,
			RoundingMode:       CampaignRoundingCeilYuan,
			ValidDays:          45,
			Stackable:          false,
			Priority:           100,
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
	config.GlobalConfig.Read("payment_setting", func(raw interface{}) {
		setting := raw.(*PaymentSetting)
		campaigns = make([]PaymentCampaign, len(setting.Campaigns))
		for i, campaign := range setting.Campaigns {
			copied := normalizeLegacyCampaignSafeguards(campaign)
			copied.PackageIDs = append([]string(nil), campaign.PackageIDs...)
			if campaign.FixedBonus != nil {
				copied.FixedBonus = make(map[string]float64, len(campaign.FixedBonus))
				for packageId, amount := range campaign.FixedBonus {
					copied.FixedBonus[packageId] = amount
				}
			}
			campaigns[i] = copied
		}
	})
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
	if campaign.ReservationMinutes != 0 {
		return campaign
	}
	campaign.ReservationMinutes = DefaultCampaignReservationMinutes
	if campaign.ID == "launch-first-topup-30" &&
		campaign.MaxClaimsPerEmail == 0 &&
		campaign.MaxClaimsTotal == 0 {
		campaign.MaxClaimsPerEmail = 1
		campaign.MaxClaimsTotal = 100
	}
	return campaign
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
		if packageOption.PayAmount <= 0 || packageOption.CreditAmount < packageOption.PayAmount {
			return fmt.Errorf("topup package %s has invalid amounts", id)
		}
		if packageOption.PayAmount > 100000 || packageOption.CreditAmount > 100000 {
			return fmt.Errorf("topup package %s exceeds the amount limit", id)
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

func ValidatePaymentCampaigns(campaigns []PaymentCampaign, packages []TopupPackage) error {
	if len(campaigns) > 50 {
		return errors.New("payment campaigns cannot exceed 50 items")
	}
	packageIDs := make(map[string]struct{}, len(packages))
	for _, packageOption := range packages {
		packageIDs[packageOption.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(campaigns))
	for _, campaign := range campaigns {
		id := strings.TrimSpace(campaign.ID)
		if id == "" || len(id) > 64 {
			return errors.New("campaign id is required and must be at most 64 characters")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate campaign id: %s", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(campaign.Name) == "" || len(campaign.Name) > 100 || len(campaign.BannerTitle) > 120 || len(campaign.BannerText) > 300 || len(campaign.BadgeText) > 50 {
			return fmt.Errorf("campaign %s has invalid text", id)
		}
		if campaign.StartsAt > 0 && campaign.EndsAt > 0 && campaign.EndsAt <= campaign.StartsAt {
			return fmt.Errorf("campaign %s end time must be after start time", id)
		}
		if campaign.ValidDays < 1 || campaign.ValidDays > 3650 {
			return fmt.Errorf("campaign %s valid days must be between 1 and 3650", id)
		}
		if campaign.MaxClaimsPerUser < 0 || campaign.MaxClaimsPerUser > 1000 {
			return fmt.Errorf("campaign %s has an invalid per-user claim limit", id)
		}
		if campaign.MaxClaimsPerEmail < 0 || campaign.MaxClaimsPerEmail > 1000 {
			return fmt.Errorf("campaign %s has an invalid per-email claim limit", id)
		}
		if campaign.MaxClaimsTotal < 0 || campaign.MaxClaimsTotal > 1000000 {
			return fmt.Errorf("campaign %s has an invalid total claim limit", id)
		}
		if campaign.ReservationMinutes < 1 || campaign.ReservationMinutes > 1440 {
			return fmt.Errorf("campaign %s reservation minutes must be between 1 and 1440", id)
		}
		switch campaign.Eligibility {
		case CampaignEligibilityPerCampaign, CampaignEligibilityPerPackage, CampaignEligibilityUnlimited:
		default:
			return fmt.Errorf("campaign %s has an unsupported eligibility rule", id)
		}
		if campaign.Eligibility == CampaignEligibilityUnlimited && campaign.MaxClaimsPerUser != 0 {
			return fmt.Errorf("campaign %s must use zero claim limit when eligibility is unlimited", id)
		}
		switch campaign.RewardMode {
		case CampaignRewardTargetTotalPercent:
			if campaign.RewardPercent <= 0 || campaign.RewardPercent > 1000 {
				return fmt.Errorf("campaign %s reward percent must be between 0 and 1000", id)
			}
		case CampaignRewardFixedBonus:
			if len(campaign.FixedBonus) == 0 {
				return fmt.Errorf("campaign %s fixed bonus is empty", id)
			}
		default:
			return fmt.Errorf("campaign %s has an unsupported reward mode", id)
		}
		if campaign.RoundingMode != CampaignRoundingCeilYuan {
			return fmt.Errorf("campaign %s has an unsupported rounding mode", id)
		}
		for _, packageID := range campaign.PackageIDs {
			if _, exists := packageIDs[packageID]; !exists {
				return fmt.Errorf("campaign %s references unknown package %s", id, packageID)
			}
		}
		for packageID, amount := range campaign.FixedBonus {
			if _, exists := packageIDs[packageID]; !exists || amount < 0 || amount > 100000 {
				return fmt.Errorf("campaign %s has an invalid fixed bonus", id)
			}
		}
	}
	return nil
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}
