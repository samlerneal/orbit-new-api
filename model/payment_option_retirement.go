package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var retiredStripeOptionKeys = []string{
	"StripeApiSecret",
	"StripeWebhookSecret",
	"StripePriceId",
	"StripeUnitPrice",
	"StripeMinTopUp",
	"StripePromotionCodesEnabled",
}

var retiredStripeOptionKeySet = map[string]struct{}{
	"StripeApiSecret":             {},
	"StripeWebhookSecret":         {},
	"StripePriceId":               {},
	"StripeUnitPrice":             {},
	"StripeMinTopUp":              {},
	"StripePromotionCodesEnabled": {},
}

func isRetiredStripeOptionKey(key string) bool {
	_, retired := retiredStripeOptionKeySet[key]
	return retired
}

// RetireStripePaymentConfiguration removes persisted Stripe credentials and
// external customer references while preserving historical payment records.
func RetireStripePaymentConfiguration() error {
	if DB == nil {
		return errors.New("database is not initialized")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		for _, key := range retiredStripeOptionKeys {
			if err := tx.Where(&Option{Key: key}).Delete(&Option{}).Error; err != nil {
				return fmt.Errorf("delete retired option %s: %w", key, err)
			}
		}
		if err := tx.Model(&User{}).
			Where("stripe_customer <> ?", "").
			Update("stripe_customer", "").Error; err != nil {
			return fmt.Errorf("clear Stripe customer references: %w", err)
		}
		if err := tx.Model(&SubscriptionPlan{}).
			Where("stripe_price_id <> ?", "").
			Update("stripe_price_id", "").Error; err != nil {
			return fmt.Errorf("clear Stripe subscription price references: %w", err)
		}
		return nil
	})
}
