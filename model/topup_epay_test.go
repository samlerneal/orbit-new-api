package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupEpayTopupTestDB(t *testing.T) {
	t.Helper()

	previousDB, previousLogDB := DB, LOG_DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &Log{}))
	DB, LOG_DB = db, db

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		common.RedisEnabled = previousRedisEnabled
		_ = sqlDB.Close()
	})
}

func TestCompleteEpayTopUpCreditsPackageQuotaExactlyOnce(t *testing.T) {
	setupEpayTopupTestDB(t)

	user := User{Username: "epay-user", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	topUp := TopUp{
		UserId:          user.Id,
		Amount:          2,
		CreditQuota:     1_000,
		Money:           14,
		TradeNo:         "epay-package-success",
		PaymentMethod:   "wxpay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(&topUp).Error)

	require.NoError(t, CompleteEpayTopUp(topUp.TradeNo, "wxpay", decimal.NewFromInt(14), "127.0.0.1"))
	require.NoError(t, CompleteEpayTopUp(topUp.TradeNo, "wxpay", decimal.NewFromInt(14), "127.0.0.1"))

	var updatedUser User
	require.NoError(t, DB.First(&updatedUser, user.Id).Error)
	assert.Equal(t, 1_100, updatedUser.Quota)
	var updatedTopUp TopUp
	require.NoError(t, DB.First(&updatedTopUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, updatedTopUp.Status)
	assert.Positive(t, updatedTopUp.CompleteTime)
}

func TestCompleteEpayTopUpRejectsAmountOrPaymentMethodMismatch(t *testing.T) {
	setupEpayTopupTestDB(t)

	user := User{Username: "epay-mismatch-user", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	topUp := TopUp{
		UserId:          user.Id,
		Amount:          2,
		CreditQuota:     1_000,
		Money:           14,
		TradeNo:         "epay-package-mismatch",
		PaymentMethod:   "wxpay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(&topUp).Error)

	amountErr := CompleteEpayTopUp(topUp.TradeNo, "wxpay", decimal.NewFromInt(1), "127.0.0.1")
	assert.ErrorIs(t, amountErr, ErrPaymentAmountMismatch)
	methodErr := CompleteEpayTopUp(topUp.TradeNo, "alipay", decimal.NewFromInt(14), "127.0.0.1")
	assert.ErrorIs(t, methodErr, ErrPaymentMethodMismatch)

	var updatedUser User
	require.NoError(t, DB.First(&updatedUser, user.Id).Error)
	assert.Equal(t, 100, updatedUser.Quota)
	var updatedTopUp TopUp
	require.NoError(t, DB.First(&updatedTopUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, updatedTopUp.Status)
}
