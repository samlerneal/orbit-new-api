package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCampaignRewardUsesPaymentAmountAndRoundsUp(t *testing.T) {
	campaign := operation_setting.PaymentCampaign{
		RewardMode:    operation_setting.CampaignRewardTargetTotalPercent,
		RewardPercent: 30,
		RoundingMode:  operation_setting.CampaignRoundingCeilYuan,
	}
	tests := []struct {
		pay     float64
		regular float64
		total   string
		bonus   string
	}{
		{pay: 14, regular: 14, total: "19", bonus: "5"},
		{pay: 49, regular: 50, total: "64", bonus: "14"},
		{pay: 98, regular: 100, total: "128", bonus: "28"},
		{pay: 490, regular: 500, total: "637", bonus: "137"},
	}

	for _, test := range tests {
		t.Run(decimal.NewFromFloat(test.pay).String(), func(t *testing.T) {
			total, bonus, err := resolveCampaignReward(campaign, operation_setting.TopupPackage{
				PayAmount: test.pay, CreditAmount: test.regular,
			})
			require.NoError(t, err)
			assert.True(t, total.Equal(decimal.RequireFromString(test.total)))
			assert.True(t, bonus.Equal(decimal.RequireFromString(test.bonus)))
		})
	}
}

func TestCompleteEpayTopUpCreatesIndependentExpiringBonus(t *testing.T) {
	setupEpayTopupTestDB(t)
	user := User{Username: "campaign-user"}
	require.NoError(t, DB.Create(&user).Error)

	snapshot := CampaignAwardSnapshot{
		CampaignId:     "launch-first-topup-30",
		CampaignName:   "Launch first top-up",
		Eligibility:    operation_setting.CampaignEligibilityPerCampaign,
		MaxClaims:      1,
		PackageId:      "advanced",
		BonusQuota:     280,
		BonusAmountCNY: "28.00",
		ValidDays:      45,
	}
	topup := TopUp{
		UserId: user.Id, PackageId: "advanced", CreditQuota: 1_000,
		Money: 98, TradeNo: "campaign-topup", PaymentMethod: "wxpay",
		PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending,
		CampaignSnapshot: EncodeCampaignAwardSnapshots([]CampaignAwardSnapshot{snapshot}),
	}
	require.NoError(t, DB.Create(&topup).Error)
	require.NoError(t, CompleteEpayTopUp(topup.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1"))

	var updated TopUp
	require.NoError(t, DB.First(&updated, topup.Id).Error)
	assert.EqualValues(t, 280, updated.BonusCreditQuota)
	var bonus BonusBalance
	require.NoError(t, DB.Where("topup_id = ?", topup.Id).First(&bonus).Error)
	assert.EqualValues(t, 280, bonus.AmountTotal)
	assert.InDelta(t, time.Now().Add(45*24*time.Hour).Unix(), bonus.ExpiresAt, 3)

	// Repeated callbacks cannot credit either regular or bonus balance twice.
	require.NoError(t, CompleteEpayTopUp(topup.TradeNo, "wxpay", decimal.NewFromInt(98), "127.0.0.1"))
	var count int64
	require.NoError(t, DB.Model(&BonusBalance{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestWalletConsumesEarliestBonusThenRegularAndRestoresSources(t *testing.T) {
	setupEpayTopupTestDB(t)
	user := User{Username: "wallet-user", Quota: 1_000}
	require.NoError(t, DB.Create(&user).Error)
	now := common.GetTimestamp()
	later := BonusBalance{UserId: user.Id, CampaignId: "later", TopUpId: 1, AmountTotal: 100, ExpiresAt: now + 200}
	sooner := BonusBalance{UserId: user.Id, CampaignId: "sooner", TopUpId: 2, AmountTotal: 100, ExpiresAt: now + 100}
	require.NoError(t, DB.Create(&later).Error)
	require.NoError(t, DB.Create(&sooner).Error)

	result, err := PreConsumeWalletFunds("wallet-order", user.Id, 250)
	require.NoError(t, err)
	assert.EqualValues(t, 200, result.BonusQuota)
	assert.EqualValues(t, 50, result.WalletQuota)

	require.NoError(t, DB.First(&sooner, sooner.Id).Error)
	require.NoError(t, DB.First(&later, later.Id).Error)
	assert.EqualValues(t, 100, sooner.AmountUsed)
	assert.EqualValues(t, 100, later.AmountUsed)
	var updatedUser User
	require.NoError(t, DB.First(&updatedUser, user.Id).Error)
	assert.Equal(t, 950, updatedUser.Quota)

	require.NoError(t, AdjustWalletConsumption("wallet-order", user.Id, -70))
	require.NoError(t, DB.First(&later, later.Id).Error)
	require.NoError(t, DB.First(&updatedUser, user.Id).Error)
	assert.EqualValues(t, 80, later.AmountUsed)
	assert.Equal(t, 1_000, updatedUser.Quota)

	require.NoError(t, RefundWalletConsumption("wallet-order", user.Id))
	require.NoError(t, DB.First(&sooner, sooner.Id).Error)
	require.NoError(t, DB.First(&later, later.Id).Error)
	assert.Zero(t, sooner.AmountUsed)
	assert.Zero(t, later.AmountUsed)
}

func TestExpiredBonusIsNotSpendable(t *testing.T) {
	setupEpayTopupTestDB(t)
	user := User{Username: "expired-wallet-user", Quota: 50}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&BonusBalance{
		UserId: user.Id, CampaignId: "expired", TopUpId: 3,
		AmountTotal: 100, ExpiresAt: common.GetTimestamp() - 1,
	}).Error)

	spendable, err := GetUserSpendableQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 50, spendable)
	_, err = PreConsumeWalletFunds("expired-order", user.Id, 51)
	assert.Error(t, err)
}
