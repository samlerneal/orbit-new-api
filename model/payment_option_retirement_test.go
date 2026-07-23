package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func usePaymentOptionRetirementDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}, &User{}, &SubscriptionPlan{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
	})
	return db
}

func TestRetireStripePaymentConfigurationRemovesSecretsAndReferences(t *testing.T) {
	db := usePaymentOptionRetirementDB(t)
	for _, key := range retiredStripeOptionKeys {
		require.NoError(t, db.Create(&Option{Key: key, Value: "sensitive"}).Error)
	}
	require.NoError(t, db.Create(&Option{Key: "EpayId", Value: "preserved"}).Error)
	require.NoError(t, db.Create(&User{
		Username:       "stripe-user",
		StripeCustomer: "cus_123",
	}).Error)
	require.NoError(t, db.Create(&SubscriptionPlan{
		Title:         "Legacy plan",
		StripePriceId: "price_123",
	}).Error)

	require.NoError(t, RetireStripePaymentConfiguration())

	for _, key := range retiredStripeOptionKeys {
		var count int64
		require.NoError(t, db.Model(&Option{}).Where("key = ?", key).Count(&count).Error)
		assert.Zero(t, count)
	}
	assert.Equal(t, "preserved", requireOptionValue(t, db, "EpayId"))

	var user User
	require.NoError(t, db.Where("username = ?", "stripe-user").First(&user).Error)
	assert.Empty(t, user.StripeCustomer)

	var plan SubscriptionPlan
	require.NoError(t, db.Where("title = ?", "Legacy plan").First(&plan).Error)
	assert.Empty(t, plan.StripePriceId)

	require.NoError(t, RetireStripePaymentConfiguration())
}

func TestRetiredStripeOptionsCannotBeReintroduced(t *testing.T) {
	db := usePaymentOptionRetirementDB(t)

	require.Error(t, UpdateOption("StripeApiSecret", "sk_test_rejected"))
	require.Error(t, UpdateOptionsBulk(map[string]string{
		"EpayId":        "must-not-partially-save",
		"StripePriceId": "price_rejected",
	}))

	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	assert.Zero(t, count)
}
