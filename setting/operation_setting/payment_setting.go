package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type PaymentSetting struct {
	AmountOptions    []int           `json:"amount_options"`
	AmountDiscount   map[int]float64 `json:"amount_discount"` // 充值金额对应的折扣，例如 100 元 0.9 表示 100 元充值享受 9 折优惠
	TopupPackages    []TopupPackage  `json:"topup_packages"`
	PromotionEnabled bool            `json:"promotion_enabled"`

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
	PayAmount    float64 `json:"pay_amount"`
	CreditAmount float64 `json:"credit_amount"`
}

const CurrentComplianceTermsVersion = "v1"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:  []int{10, 20, 50, 100, 200, 500},
	AmountDiscount: map[int]float64{},
	TopupPackages: []TopupPackage{
		{ID: "experience", Name: "Experience", Description: "Best for trying the service", PayAmount: 14, CreditAmount: 14},
		{ID: "standard", Name: "Standard", Description: "Best for light everyday use", PayAmount: 49, CreditAmount: 49},
		{ID: "advanced", Name: "Advanced package", Description: "Best for frequent use", PayAmount: 98, CreditAmount: 98},
		{ID: "high-frequency", Name: "High Frequency", Description: "Best for long-term heavy use", PayAmount: 490, CreditAmount: 490},
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
	packages := make([]TopupPackage, len(paymentSetting.TopupPackages))
	copy(packages, paymentSetting.TopupPackages)
	return packages
}

func ResolveTopupPackage(packageID string) (TopupPackage, bool) {
	for _, packageOption := range paymentSetting.TopupPackages {
		if packageOption.ID != packageID || packageOption.PayAmount <= 0 {
			continue
		}

		if !paymentSetting.PromotionEnabled || packageOption.CreditAmount <= 0 {
			packageOption.CreditAmount = packageOption.PayAmount
		}
		return packageOption, true
	}

	return TopupPackage{}, false
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}
