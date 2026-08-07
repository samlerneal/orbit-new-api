package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInvalidateUserIdentityCachesIsNoopWithoutRedis(t *testing.T) {
	originalEnabled, originalRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = originalEnabled, originalRDB })
	require.NoError(t, InvalidateUserIdentityCaches(context.Background(), []int{7, 8}))
}

func TestInvalidateUserIdentityCachesDoesNotFlushDatabase(t *testing.T) {
	// The implementation uses DEL with constructed exact keys; this regression
	// test keeps the disabled-Redis path deterministic in restricted CI.
	originalEnabled, originalRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = originalEnabled, originalRDB })
	require.NoError(t, InvalidateUserIdentityCaches(context.Background(), []int{42}))
}

func TestInvalidateUserIdentityCachesRejectsUnverifiedTopology(t *testing.T) {
	previousEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousEnabled, previousRDB })
	require.ErrorIs(t, InvalidateUserIdentityCaches(context.Background(), []int{42}), ErrRedisTopologyUnverified)
}

func TestInvalidateUserIdentityCachesDeletesOnlyExactNamespaces(t *testing.T) {
	server := miniredis.RunT(t)
	previousEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousEnabled, previousRDB })
	ctx := context.Background()
	server.Set("user:7", "user")
	server.Set("auth:user:fence:7", "fence")
	server.Set("auth:user:version:7", "version")
	server.Set("user:8", "unrelated")
	require.NoError(t, InvalidateUserIdentityCaches(ctx, []int{7}))
	require.False(t, server.Exists("user:7"))
	require.False(t, server.Exists("auth:user:fence:7"))
	require.False(t, server.Exists("auth:user:version:7"))
	require.True(t, server.Exists("user:8"))
}

func setupWaffoHistoricalIdentityFixture(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousMain := common.MainDatabaseType()
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.SubscriptionOrder{}))
	require.NoError(t, db.Exec("ALTER TABLE top_ups ADD COLUMN waffo_buyer_identity varchar(128) NULL").Error)
	require.NoError(t, db.Exec("ALTER TABLE subscription_orders ADD COLUMN waffo_buyer_identity varchar(128) NULL").Error)
	model.DB = db
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{model.O023WaffoSnapshotVersion: "1"}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		model.DB = previousDB
		common.SetMainDatabaseType(previousMain)
	})
}

func TestWaffoTopUpHistoricalSnapshotRemainsTheWebhookIdentity(t *testing.T) {
	setupWaffoHistoricalIdentityFixture(t)
	require.NoError(t, model.DB.Exec("INSERT INTO top_ups (user_id, trade_no, payment_provider, status, waffo_buyer_identity) VALUES (?, ?, ?, ?, ?)", 123456789012, "waffo-history-topup", model.PaymentProviderWaffoPancake, common.TopUpStatusPending, "new-api-user-7").Error)
	event := &WaffoPancakeWebhookEvent{Data: WaffoPancakeWebhookData{OrderMerchantExternalID: "waffo-history-topup", MerchantProvidedBuyerIdentity: "new-api-user-7"}}
	tradeNo, err := ResolveWaffoPancakeTradeNo(event)
	require.NoError(t, err)
	require.Equal(t, "waffo-history-topup", tradeNo)
	_, err = ResolveWaffoPancakeTradeNo(&WaffoPancakeWebhookEvent{Data: WaffoPancakeWebhookData{OrderMerchantExternalID: "waffo-history-topup", MerchantProvidedBuyerIdentity: "new-api-user-123456789012"}})
	require.Error(t, err)
}

func TestWaffoSubscriptionHistoricalSnapshotRemainsTheWebhookIdentity(t *testing.T) {
	setupWaffoHistoricalIdentityFixture(t)
	require.NoError(t, model.DB.Exec("INSERT INTO subscription_orders (user_id, trade_no, payment_provider, status, waffo_buyer_identity) VALUES (?, ?, ?, ?, ?)", 123456789012, "waffo-history-subscription", model.PaymentProviderWaffoPancake, common.TopUpStatusPending, "new-api-user-7").Error)
	event := &WaffoPancakeWebhookEvent{Data: WaffoPancakeWebhookData{OrderMerchantExternalID: "waffo-history-subscription", MerchantProvidedBuyerIdentity: "new-api-user-7"}}
	tradeNo, err := ResolveWaffoPancakeSubscriptionTradeNo(event)
	require.NoError(t, err)
	require.Equal(t, "waffo-history-subscription", tradeNo)
	_, err = ResolveWaffoPancakeSubscriptionTradeNo(&WaffoPancakeWebhookEvent{Data: WaffoPancakeWebhookData{OrderMerchantExternalID: "waffo-history-subscription", MerchantProvidedBuyerIdentity: "new-api-user-123456789012"}})
	require.Error(t, err)
}
