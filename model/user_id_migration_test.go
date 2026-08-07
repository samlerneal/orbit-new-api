package model

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newO023CanonicalSchemaFixture(t *testing.T, name string) *gorm.DB {
	t.Helper()
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB := DB
	DB = db
	require.NoError(t, migrateDB())
	DB = previousDB
	return db
}

func TestWaffoSnapshotColumnsRequirePrepareSchema(t *testing.T) {
	db := newO023CanonicalSchemaFixture(t, "o023-waffo-schema")
	for _, table := range []string{"top_ups", "subscription_orders"} {
		var count int64
		require.NoError(t, db.Raw("SELECT count(*) FROM pragma_table_xinfo(?) WHERE name = ?", table, "waffo_buyer_identity").Scan(&count).Error)
		require.Zero(t, count, table)
	}
	require.NoError(t, PrepareO023Schema(db))
	for _, table := range []string{"top_ups", "subscription_orders"} {
		var count int64
		require.NoError(t, db.Raw("SELECT count(*) FROM pragma_table_xinfo(?) WHERE name = ?", table, "waffo_buyer_identity").Scan(&count).Error)
		require.Equal(t, int64(1), count, table)
	}
}

func TestMigrateDBDoesNotCreateWaffoSnapshotColumns(t *testing.T) {
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:o023-real-startup-automigrate?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous })
	require.NoError(t, migrateDB())
	for _, table := range []string{"top_ups", "subscription_orders"} {
		var count int64
		require.NoError(t, db.Raw("SELECT count(*) FROM pragma_table_xinfo(?) WHERE name = ?", table, "waffo_buyer_identity").Scan(&count).Error)
		require.Zero(t, count, table)
	}
}

func TestO023InvariantQueryErrorsFailClosed(t *testing.T) {
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:o023-invariant-query-error?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Exec(`CREATE TABLE checkins (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, checkin_date TEXT NOT NULL)`).Error)
	var count int64
	legacyErr := db.Table("checkins").Where("quota < 0").Count(&count).Error
	assert.Error(t, legacyErr, "the old quota query is invalid for the real Checkin schema")
	assert.NoError(t, func() error {
		if legacyErr != nil {
			return nil
		}
		return legacyErr
	}(), "the old implementation would swallow this error")
	exists, existsErr := sqliteTableExists(db, "checkins")
	require.NoError(t, existsErr)
	require.True(t, exists)
	quotaColumn, quotaColumnErr := sqliteColumnExists(db, "checkins", "quota")
	require.NoError(t, quotaColumnErr)
	require.False(t, quotaColumn)
	require.ErrorIs(t, o023RequireNonNegativeColumn(db, "checkins", "quota"), ErrMigrationStateCorrupt)
	assert.ErrorIs(t, verifyO023BusinessAggregates(db), ErrMigrationStateCorrupt)
}

func TestO023StructuredScanErrorsFailClosed(t *testing.T) {
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:o023-structured-query-error?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Exec(`INSERT INTO users (id, username, password, setting) VALUES (?, ?, ?, ?)`, 7, "structured-user", "password", "not-json").Error)
	require.NoError(t, db.Exec(`CREATE TEMP TABLE temp_o023_user_ids (old_id INTEGER PRIMARY KEY, new_id INTEGER NOT NULL UNIQUE)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO temp_o023_user_ids(old_id, new_id) VALUES (?, ?)`, 7, UserIDMin).Error)
	_, _, err = rewriteO023JSON("not-json", map[int]int{7: int(UserIDMin)})
	assert.Error(t, err)
	assert.ErrorIs(t, migrateO023StructuredReferences(db), ErrUnknownUserReference)
}

func TestO023RawResidualScanIsMappingFree(t *testing.T) {
	db := newO023MigrationFixture(t, "o023-raw-residuals")
	require.NoError(t, db.Exec("UPDATE users SET id = ?, setting = ?", UserIDMin, `{"user_id":7}`).Error)
	require.ErrorIs(t, verifyO023RawResiduals(db, []int{7}), ErrUnknownUserReference)
}

func TestAuditO023RejectsUnknownColumnAndIndexDefinition(t *testing.T) {
	db := newO023CanonicalSchemaFixture(t, "o023-schema-contract-negative-column")
	require.NoError(t, db.Exec("ALTER TABLE users ADD COLUMN o023_unknown_column TEXT").Error)
	require.ErrorIs(t, AuditO023Schema(db), ErrUnknownUserReference)
	db = newO023CanonicalSchemaFixture(t, "o023-schema-contract-negative-index")
	require.NoError(t, db.Exec("CREATE INDEX idx_o023_unknown ON users(username)").Error)
	require.ErrorIs(t, AuditO023Schema(db), ErrUnknownUserReference)
}

func TestAuditO023SchemaRejectsUnknownUserReference(t *testing.T) {
	db := newO023CanonicalSchemaFixture(t, "o023-unknown-reference")
	require.NoError(t, db.Exec("CREATE VIEW rogue_users AS SELECT id FROM users").Error)
	require.ErrorIs(t, AuditO023Schema(db), ErrUnknownUserReference)
}

func TestAuditO023SchemaRejectsPartialFixtureWithoutBypass(t *testing.T) {
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:o023-partial-schema-rejected?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Option{}))
	require.ErrorIs(t, AuditO023Schema(db), ErrUnknownUserReference)
}

func TestAuditO023SchemaRejectsCanonicalObjectDrift(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{name: "missing-index", sql: "DROP INDEX idx_users_email"},
		{name: "trigger", sql: "CREATE TRIGGER o023_rogue_trigger AFTER UPDATE ON users BEGIN UPDATE users SET remark = remark WHERE id = NEW.id; END"},
		{name: "foreign-key", sql: "CREATE TABLE o023_rogue_fk (id INTEGER PRIMARY KEY, user_id INTEGER REFERENCES users(id))"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newO023CanonicalSchemaFixture(t, "o023-schema-object-drift-"+test.name)
			require.NoError(t, db.Exec(test.sql).Error)
			require.ErrorIs(t, AuditO023Schema(db), ErrUnknownUserReference)
		})
	}
}

func TestAuditO023SchemaHashDetectsDrift(t *testing.T) {
	db := newO023CanonicalSchemaFixture(t, "o023-schema-hash")
	require.NoError(t, PrepareO023Schema(db))
	require.NoError(t, AuditO023Schema(db))
	require.NoError(t, db.Model(&Option{}).Where("key = ?", O023SchemaHash).Update("value", O023SchemaContractVersion+"|drifted").Error)
	require.ErrorIs(t, AuditO023Schema(db), ErrMigrationVersionConflict)
}

func TestO023StructuredReferencesUseExactGrammar(t *testing.T) {
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:o023-structured-refs?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&CasbinRule{}, &Option{}))
	require.NoError(t, db.Exec("CREATE TEMP TABLE temp_o023_user_ids (old_id INTEGER PRIMARY KEY, new_id INTEGER NOT NULL UNIQUE)").Error)
	require.NoError(t, db.Exec("INSERT INTO temp_o023_user_ids(old_id,new_id) VALUES (?,?)", 7, 123456789012).Error)
	require.NoError(t, db.Create(&CasbinRule{Ptype: "p", V0: "user:7", V1: "channel:read"}).Error)
	require.NoError(t, db.Create(&CasbinRule{Ptype: "p", V0: "role:70", V1: "channel:read"}).Error)
	require.NoError(t, migrateO023StructuredReferences(db))
	var rules []CasbinRule
	require.NoError(t, db.Order("id").Find(&rules).Error)
	assert.Equal(t, "user:123456789012", rules[0].V0)
	assert.Equal(t, "role:70", rules[1].V0)
}

func TestO023MigrationPreservesHistoricalWaffoSnapshots(t *testing.T) {
	db := newO023CanonicalSchemaFixture(t, "o023-waffo-history")
	require.NoError(t, PrepareO023Schema(db))
	require.NoError(t, db.Exec("INSERT INTO top_ups (user_id, trade_no, payment_provider, status, waffo_buyer_identity) VALUES (?, ?, ?, ?, ?)", 7, "waffo-history-topup", PaymentProviderWaffoPancake, common.TopUpStatusSuccess, "new-api-user-7").Error)
	require.NoError(t, db.Exec("INSERT INTO subscription_orders (user_id, trade_no, payment_provider, status, waffo_buyer_identity) VALUES (?, ?, ?, ?, ?)", 7, "waffo-history-subscription", PaymentProviderWaffoPancake, common.TopUpStatusSuccess, "new-api-user-7").Error)
	require.NoError(t, db.Exec("CREATE TEMP TABLE temp_o023_user_ids (old_id INTEGER PRIMARY KEY, new_id INTEGER NOT NULL UNIQUE)").Error)
	require.NoError(t, db.Exec("INSERT INTO temp_o023_user_ids(old_id, new_id) VALUES (?, ?)", 7, 123456789012).Error)

	require.NoError(t, migrateO023StructuredReferences(db))
	var topup, order struct{ WaffoBuyerIdentity string }
	require.NoError(t, db.Raw("SELECT waffo_buyer_identity FROM top_ups WHERE trade_no = ?", "waffo-history-topup").Scan(&topup).Error)
	require.NoError(t, db.Raw("SELECT waffo_buyer_identity FROM subscription_orders WHERE trade_no = ?", "waffo-history-subscription").Scan(&order).Error)
	assert.Equal(t, "new-api-user-7", topup.WaffoBuyerIdentity)
	assert.Equal(t, "new-api-user-7", order.WaffoBuyerIdentity)
}

func TestO023MigrationStatusRejectsPartialAndUnknownVersions(t *testing.T) {
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:o023-status-machine?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Option{}))
	status, err := O023MigrationStatus(db)
	require.NoError(t, err)
	assert.Equal(t, "READY", status)
	require.NoError(t, db.Exec(`INSERT INTO users (id, username, password) VALUES (?, ?, ?)`, 7, "partial", "password").Error)
	_, err = O023MigrationStatus(db)
	require.NoError(t, err)
	require.NoError(t, db.Exec(`INSERT INTO users (id, username, password) VALUES (?, ?, ?)`, UserIDMin, "partial-new", "password").Error)
	_, err = O023MigrationStatus(db)
	require.ErrorIs(t, err, ErrUnmarkedPartialMigration)
	require.NoError(t, db.Create(&Option{Key: O023UserIDMigrationVersion, Value: "2"}).Error)
	_, err = O023MigrationStatus(db)
	require.ErrorIs(t, err, ErrMigrationVersionConflict)
}

func newO023MigrationFixture(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db := newO023CanonicalSchemaFixture(t, name)
	require.NoError(t, PrepareO023Schema(db))
	require.NoError(t, db.Exec(`INSERT INTO users (id, username, password, quota, used_quota, request_count) VALUES (?, ?, ?, ?, ?, ?)`, 7, "fixture-user", "password", 100, 20, 3).Error)
	return db
}

func TestMigrateO023SuccessRollbackRetryAndAlreadyMigrated(t *testing.T) {
	db := newO023MigrationFixture(t, "o023-success-retry")
	var originalForeignKeys, originalDeferred int
	require.NoError(t, db.Raw("PRAGMA foreign_keys").Scan(&originalForeignKeys).Error)
	require.NoError(t, db.Raw("PRAGMA defer_foreign_keys").Scan(&originalDeferred).Error)
	o023MigrationFailureHook = func(stage string) error {
		if stage == "after-temp" {
			return errors.New("synthetic interruption")
		}
		return nil
	}
	t.Cleanup(func() { o023MigrationFailureHook = nil })
	err := MigrateO023UserIDs(db)
	require.EqualError(t, err, "synthetic interruption")
	o023MigrationFailureHook = nil
	var oldCount int64
	require.NoError(t, db.Table("users").Where("id = ?", 7).Count(&oldCount).Error)
	assert.EqualValues(t, 1, oldCount)
	_, _, err = o023OptionValue(db, O023UserIDMigrationVersion)
	require.NoError(t, err)
	var tempCount int64
	require.Error(t, db.Raw("SELECT count(*) FROM temp_o023_user_ids").Scan(&tempCount).Error)
	var foreignKeys, deferred int
	require.NoError(t, db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error)
	require.NoError(t, db.Raw("PRAGMA defer_foreign_keys").Scan(&deferred).Error)
	assert.Equal(t, originalForeignKeys, foreignKeys)
	assert.Equal(t, originalDeferred, deferred)

	require.NoError(t, MigrateO023UserIDs(db))
	status, err := O023MigrationStatus(db)
	require.NoError(t, err)
	assert.Equal(t, "ALREADY_MIGRATED", status)
	var migratedCount int64
	require.NoError(t, db.Table("users").Where("id >= ? AND id <= ?", UserIDMin, UserIDMax).Count(&migratedCount).Error)
	assert.EqualValues(t, 1, migratedCount)
	require.NoError(t, MigrateO023UserIDs(db))
	var leaked int64
	require.Error(t, db.Raw("SELECT count(*) FROM temp_o023_user_ids").Scan(&leaked).Error)
}

func TestMigrateO023RejectsUnverifiedRedisBeforeDatabaseWrite(t *testing.T) {
	db := newO023MigrationFixture(t, "o023-unverified-redis")
	previousEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousEnabled, previousRDB })
	err, foreignKeysBefore, deferredBefore, foreignKeysAfter, deferredAfter := o023MigrateWithNonDefaultPragmas(t, db)
	require.ErrorIs(t, err, ErrRedisTopologyUnverified)
	assert.Equal(t, foreignKeysBefore, foreignKeysAfter)
	assert.Equal(t, deferredBefore, deferredAfter)
	var userID int
	require.NoError(t, db.Table("users").Pluck("id", &userID).Error)
	require.Equal(t, 7, userID)
	_, present, err := o023OptionValue(db, O023UserIDMigrationVersion)
	require.NoError(t, err)
	require.False(t, present)
}

func TestMigrateO023AlreadyMigratedRejectsUnverifiedRedisWithoutWrites(t *testing.T) {
	db := newO023MigrationFixture(t, "o023-already-unverified-redis")
	previousEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousEnabled, previousRDB })
	require.NoError(t, MigrateO023UserIDs(db))
	var userIDBefore int
	require.NoError(t, db.Table("users").Pluck("id", &userIDBefore).Error)
	invariantBefore, present, err := o023OptionValue(db, O023InvariantHash)
	require.NoError(t, err)
	require.True(t, present)
	common.RedisEnabled = true
	err, foreignKeysBefore, deferredBefore, foreignKeysAfter, deferredAfter := o023MigrateWithNonDefaultPragmas(t, db)
	require.ErrorIs(t, err, ErrRedisTopologyUnverified)
	assert.Equal(t, foreignKeysBefore, foreignKeysAfter)
	assert.Equal(t, deferredBefore, deferredAfter)
	var userIDAfter int
	require.NoError(t, db.Table("users").Pluck("id", &userIDAfter).Error)
	require.Equal(t, userIDBefore, userIDAfter)
	invariantAfter, present, err := o023OptionValue(db, O023InvariantHash)
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, invariantBefore, invariantAfter)
}

func o023Pragmas(t *testing.T, db *gorm.DB) (foreignKeys, deferred int) {
	t.Helper()
	require.NoError(t, db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error)
	require.NoError(t, db.Raw("PRAGMA defer_foreign_keys").Scan(&deferred).Error)
	return foreignKeys, deferred
}

func o023MigrateWithNonDefaultPragmas(t *testing.T, db *gorm.DB) (migrationErr error, foreignKeysBefore, deferredBefore, foreignKeysAfter, deferredAfter int) {
	t.Helper()
	require.NoError(t, db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
			return err
		}
		if err := conn.Exec("PRAGMA defer_foreign_keys = ON").Error; err != nil {
			return err
		}
		foreignKeysBefore, deferredBefore = o023Pragmas(t, conn)
		migrationErr = migrateO023UserIDsOnConn(conn)
		foreignKeysAfter, deferredAfter = o023Pragmas(t, conn)
		return nil
	}))
	return migrationErr, foreignKeysBefore, deferredBefore, foreignKeysAfter, deferredAfter
}

func requireO023WaffoMutationRejected(t *testing.T, db *gorm.DB, persisted o023PersistedInvariant, savepoint, query string, args ...any) {
	t.Helper()
	require.NoError(t, db.Exec("SAVEPOINT "+savepoint).Error)
	require.NoError(t, db.Exec(query, args...).Error)
	boundaries := map[string]int64{}
	for table, historical := range persisted.HistoricalWaffo {
		boundaries[table] = historical.Boundary
	}
	actual, err := o023HistoricalWaffoDigests(db, boundaries)
	if err == nil {
		err = o023CompareHistoricalWaffo(persisted.HistoricalWaffo, actual)
	}
	require.Error(t, err)
	require.NoError(t, db.Exec("ROLLBACK TO "+savepoint).Error)
	require.NoError(t, db.Exec("RELEASE "+savepoint).Error)
}

func uniqueO023Keys(keys []string) []string {
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	return unique
}

func runO023CompleteSyntheticRehearsal(t *testing.T, name string) {
	t.Helper()
	db := newO023CanonicalSchemaFixture(t, name)
	redisServer := miniredis.RunT(t)
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB })
	userIDSourceMu.Lock()
	previousUserIDSource := userIDSource
	nextUserIDs := []int{113456789012, 213456789012, 123456789012, 223456789012}
	userIDSource = func() (int, error) {
		id := nextUserIDs[0]
		nextUserIDs = nextUserIDs[1:]
		return id, nil
	}
	userIDSourceMu.Unlock()
	t.Cleanup(func() {
		userIDSourceMu.Lock()
		userIDSource = previousUserIDSource
		userIDSourceMu.Unlock()
	})
	require.NoError(t, db.Exec(`INSERT INTO users (id, username, password, quota, used_quota, request_count, inviter_id, setting) VALUES (?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?)`, 7, "synthetic-a", "password", 100, 20, 3, 0, `{"user_id":7}`, 8, "synthetic-b", "password", 200, 30, 4, 7, `{"inviter_id":7}`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tokens (user_id, key, status, remain_quota, used_quota) VALUES (?, ?, ?, ?, ?)`, 7, "synthetic-token", 1, 90, 10).Error)
	require.NoError(t, db.Exec(`INSERT INTO user_sessions (user_id, sid, user_auth_version, status, refresh_hash, login_method, last_active_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, 7, "synthetic-session", 1, "active", "refresh-hash", "password", 100, 200).Error)
	require.NoError(t, db.Exec(`INSERT INTO top_ups (user_id, trade_no, payment_provider, status, money, credit_quota) VALUES (?, ?, ?, ?, ?, ?)`, 7, "synthetic-topup", "epay", common.TopUpStatusSuccess, 12, 100).Error)
	require.NoError(t, db.Exec(`INSERT INTO subscription_orders (user_id, trade_no, payment_provider, status, money) VALUES (?, ?, ?, ?, ?)`, 8, "synthetic-subscription", "epay", common.TopUpStatusSuccess, 20).Error)
	campaignID := "launch-first-topup-30"
	claimKey := campaignID + ":user:7:package:experience:slot:1"
	emailClaimKey := campaignID + ":email:synthetic-email:package:experience:slot:1"
	campaignSlotKey := campaignID + ":campaign-slot:1"
	require.NoError(t, db.Create(&PaymentCampaignClaim{CampaignId: campaignID, UserId: 7, PackageId: "experience", TopUpId: 1, Status: CampaignClaimStatusAwarded, EmailHash: "synthetic-email", ClaimKey: &claimKey, EmailClaimKey: &emailClaimKey, CampaignSlotKey: &campaignSlotKey}).Error)
	participantKey := campaignID + ":participant:user:7"
	emailParticipantKey := campaignID + ":email:synthetic-email:participant"
	require.NoError(t, db.Create(&PaymentCampaignParticipant{CampaignId: campaignID, UserId: 7, EmailHash: "synthetic-email", Status: CampaignParticipantStatusAdmitted, ParticipantKey: &participantKey, EmailParticipantKey: &emailParticipantKey, CampaignSlotKey: &campaignSlotKey}).Error)
	require.NoError(t, db.Create(&BonusBalance{UserId: 7, CampaignId: campaignID, TopUpId: 1, AmountTotal: 50, AmountUsed: 10, Status: BonusBalanceStatusActive}).Error)
	require.NoError(t, db.Create(&WalletConsumeRecord{UserId: 7, RequestId: "synthetic-wallet", BonusQuota: 5, WalletQuota: 15, Status: WalletConsumeStatusConsumed}).Error)
	require.NoError(t, db.Create(&UserSubscription{UserId: 7, PlanId: 1, AmountTotal: 100, AmountUsed: 20, Status: "active"}).Error)
	require.NoError(t, db.Create(&SubscriptionPreConsumeRecord{UserId: 7, UserSubscriptionId: 1, PreConsumed: 12, Status: "consumed"}).Error)
	require.NoError(t, db.Create(&ExternalIdentityClaim{Provider: "synthetic", Subject: "subject-7", UserId: 7}).Error)
	require.NoError(t, db.Create(&UserOAuthBinding{UserId: 7, ProviderId: 1, ProviderUserId: "oauth-7"}).Error)
	require.NoError(t, db.Create(&TwoFA{UserId: 7, Secret: "synthetic-secret", IsEnabled: true}).Error)
	require.NoError(t, db.Create(&Checkin{UserId: 7, CheckinDate: "2026-08-01", QuotaAwarded: 9}).Error)
	require.NoError(t, db.Create(&Task{TaskID: "synthetic-task", UserId: 7, Quota: 4, Status: TaskStatusSuccess}).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, Quota: 4, Content: "synthetic-log"}).Error)
	require.NoError(t, db.Exec(`UPDATE tasks SET data = ?, private_data = ?, properties = ? WHERE task_id = ?`, `{"user_id":7}`, `{"confirmed_by":7}`, `{"user_id":7}`, "synthetic-task").Error)
	require.NoError(t, db.Exec(`UPDATE logs SET other = ? WHERE content = ?`, `{"user_id":7}`, "synthetic-log").Error)
	require.NoError(t, db.Create(&CasbinRule{Ptype: "p", V0: "user:7", V1: "channel:read"}).Error)
	require.NoError(t, db.Create(&Option{Key: "payment_setting.compliance_confirmed_by", Value: "7"}).Error)
	for _, key := range []string{
		"auth:user:device:7", "notify_limit:7:email:2026080514",
		getUserCacheKey(123456789012), getUserAuthFenceKey(123456789012), getUserAuthVersionKey(123456789012),
		"auth:user:device:123456789012", "notify_limit:123456789012:email:2026080514",
		getUserCacheKey(223456789012), getUserAuthFenceKey(223456789012), getUserAuthVersionKey(223456789012),
		"auth:user:device:223456789012", "notify_limit:223456789012:email:2026080514",
	} {
		require.NoError(t, common.RDB.Set(context.Background(), key, "o023-candidate", 0).Err())
	}
	oldCacheKeys, err := collectO023CacheKeys(context.Background(), db, []int{7, 8})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		getUserCacheKey(7), getUserAuthFenceKey(7), getUserAuthVersionKey(7),
		getUserCacheKey(8), getUserAuthFenceKey(8), getUserAuthVersionKey(8),
		"token:" + common.GenerateHMAC("synthetic-token"), userSessionCacheKey("synthetic-session"),
		"auth:user:device:7", "notify_limit:7:email:2026080514",
	}, uniqueO023Keys(oldCacheKeys))
	for _, key := range oldCacheKeys {
		require.NoError(t, common.RDB.Set(context.Background(), key, "o023-old", 0).Err())
	}
	require.NoError(t, common.RDB.Set(context.Background(), "unrelated:o023-rehearsal", "keep", 0).Err())
	require.NoError(t, AuditO023Schema(db))
	require.NoError(t, PrepareO023Schema(db))
	require.NoError(t, AuditO023Schema(db))
	require.NoError(t, db.Exec(`INSERT INTO top_ups (user_id, trade_no, payment_provider, status, money, credit_quota, waffo_buyer_identity) VALUES (?, ?, ?, ?, ?, ?, ?)`, 7, "synthetic-waffo-topup", PaymentProviderWaffoPancake, common.TopUpStatusSuccess, 15, 120, "new-api-user-7").Error)
	require.NoError(t, db.Exec(`INSERT INTO subscription_orders (user_id, trade_no, payment_provider, status, money, waffo_buyer_identity) VALUES (?, ?, ?, ?, ?, ?)`, 7, "synthetic-waffo-subscription", PaymentProviderWaffoPancake, common.TopUpStatusSuccess, 25, "new-api-user-7").Error)
	preWaffo, err := o023HistoricalWaffoDigests(db, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), preWaffo["top_ups"].Count)
	require.Equal(t, int64(1), preWaffo["subscription_orders"].Count)
	var preMigrationUsers int64
	require.NoError(t, db.Table("users").Unscoped().Count(&preMigrationUsers).Error)
	require.EqualValues(t, 2, preMigrationUsers)
	o023MigrationFailureHook = func(stage string) error {
		if stage == "after-structured" {
			return errors.New("synthetic complete fixture interruption")
		}
		return nil
	}
	require.EqualError(t, MigrateO023UserIDs(db), "synthetic complete fixture interruption")
	o023MigrationFailureHook = nil
	var rolledBackSettings []string
	require.NoError(t, db.Table("users").Order("id").Pluck("setting", &rolledBackSettings).Error)
	assert.Contains(t, strings.Join(rolledBackSettings, "\n"), `"user_id":7`)
	require.NoError(t, MigrateO023UserIDs(db))
	require.NoError(t, VerifyO023Invariants(db))
	var migratedUsers []User
	require.NoError(t, db.Where("username IN ?", []string{"synthetic-a", "synthetic-b"}).Find(&migratedUsers).Error)
	require.Len(t, migratedUsers, 2)
	newIDs := []int{migratedUsers[0].Id, migratedUsers[1].Id}
	newCacheKeys, err := collectO023CacheKeys(context.Background(), db, newIDs)
	require.NoError(t, err)
	expectedNewCacheKeys := []string{
		getUserCacheKey(123456789012), getUserAuthFenceKey(123456789012), getUserAuthVersionKey(123456789012),
		getUserCacheKey(223456789012), getUserAuthFenceKey(223456789012), getUserAuthVersionKey(223456789012),
		"token:" + common.GenerateHMAC("synthetic-token"), userSessionCacheKey("synthetic-session"),
		"auth:user:device:123456789012", "notify_limit:123456789012:email:2026080514",
		"auth:user:device:223456789012", "notify_limit:223456789012:email:2026080514",
	}
	for _, key := range expectedNewCacheKeys {
		require.False(t, redisServer.Exists(key), "new O-023 cache key should be removed: %s", key)
	}
	for _, key := range oldCacheKeys {
		require.False(t, redisServer.Exists(key), "old O-023 cache key should be removed: %s", key)
	}
	require.True(t, redisServer.Exists("unrelated:o023-rehearsal"))
	assert.True(t, strings.Contains(strings.Join(oldCacheKeys, "\n"), getUserCacheKey(7)))
	assert.True(t, strings.Contains(strings.Join(newCacheKeys, "\n"), getUserCacheKey(migratedUsers[0].Id)) || strings.Contains(strings.Join(newCacheKeys, "\n"), getUserCacheKey(migratedUsers[1].Id)))
	var userSettings []string
	require.NoError(t, db.Table("users").Order("id").Pluck("setting", &userSettings).Error)
	for _, setting := range userSettings {
		assert.NotRegexp(t, `(?:^|[^0-9])7(?:$|[^0-9])`, setting)
		assert.NotRegexp(t, `(?:^|[^0-9])7(?:$|[^0-9])`, setting)
	}
	var taskData, taskPrivate, taskProperties string
	require.NoError(t, db.Table("tasks").Select("data, private_data, properties").Row().Scan(&taskData, &taskPrivate, &taskProperties))
	assert.NotRegexp(t, `(?:^|[^0-9])7(?:$|[^0-9])`, taskData)
	assert.NotRegexp(t, `(?:^|[^0-9])7(?:$|[^0-9])`, taskPrivate)
	assert.NotRegexp(t, `(?:^|[^0-9])7(?:$|[^0-9])`, taskProperties)
	var logOther string
	require.NoError(t, db.Table("logs").Where("content = ?", "synthetic-log").Pluck("other", &logOther).Error)
	assert.Equal(t, `{"user_id":`+strconv.Itoa(123456789012)+`}`, logOther)
	var complianceValue string
	require.NoError(t, db.Table("options").Where("key = ?", "payment_setting.compliance_confirmed_by").Pluck("value", &complianceValue).Error)
	var migratedA int
	require.NoError(t, db.Table("users").Where("username = ?", "synthetic-a").Pluck("id", &migratedA).Error)
	assert.Equal(t, strconv.Itoa(migratedA), complianceValue)
	var topupSnapshot, subscriptionSnapshot string
	require.NoError(t, db.Raw("SELECT waffo_buyer_identity FROM top_ups WHERE trade_no = ?", "synthetic-waffo-topup").Scan(&topupSnapshot).Error)
	require.NoError(t, db.Raw("SELECT waffo_buyer_identity FROM subscription_orders WHERE trade_no = ?", "synthetic-waffo-subscription").Scan(&subscriptionSnapshot).Error)
	assert.Equal(t, "new-api-user-7", topupSnapshot)
	assert.Equal(t, "new-api-user-7", subscriptionSnapshot)
	var inviterID int
	require.NoError(t, db.Table("users").Where("username = ?", "synthetic-b").Pluck("inviter_id", &inviterID).Error)
	assert.NotZero(t, inviterID)
	assert.Equal(t, migratedUsers[0].Id == inviterID || migratedUsers[1].Id == inviterID, true)
	expectedCounts := map[string]int64{
		"tokens": 1, "user_sessions": 1, "top_ups": 2, "subscription_orders": 2,
		"payment_campaign_claims": 1, "payment_campaign_participants": 1, "bonus_balances": 1,
		"wallet_consume_records": 1, "user_subscriptions": 1, "subscription_pre_consume_records": 1,
		"external_identity_claims": 1, "user_oauth_bindings": 1, "two_fas": 1, "checkins": 1,
		"tasks": 1, "logs": 1, "casbin_rule": 1,
	}
	for table, expected := range expectedCounts {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error)
		assert.EqualValues(t, expected, count, table)
	}
	var tokenRow struct {
		Key         string
		Status      int
		RemainQuota int
		UsedQuota   int
	}
	require.NoError(t, db.Table("tokens").Select("key, status, remain_quota, used_quota").Scan(&tokenRow).Error)
	assert.Equal(t, "synthetic-token", tokenRow.Key)
	assert.Equal(t, common.TokenStatusEnabled, tokenRow.Status)
	assert.Equal(t, 90, tokenRow.RemainQuota)
	assert.Equal(t, 10, tokenRow.UsedQuota)
	var sessionStatus string
	require.NoError(t, db.Table("user_sessions").Pluck("status", &sessionStatus).Error)
	assert.Equal(t, "revoked", sessionStatus)
	var claim, participant struct{ ClaimKey, EmailClaimKey, CampaignSlotKey, ParticipantKey, EmailParticipantKey string }
	require.NoError(t, db.Table("payment_campaign_claims").Select("claim_key, email_claim_key, campaign_slot_key").Scan(&claim).Error)
	require.NoError(t, db.Table("payment_campaign_participants").Select("participant_key, email_participant_key, campaign_slot_key").Scan(&participant).Error)
	assert.Equal(t, campaignID+":user:"+strconv.Itoa(migratedA)+":package:experience:slot:1", claim.ClaimKey)
	assert.Equal(t, emailClaimKey, claim.EmailClaimKey)
	assert.Equal(t, campaignSlotKey, claim.CampaignSlotKey)
	assert.Equal(t, campaignID+":participant:user:"+strconv.Itoa(migratedA), participant.ParticipantKey)
	assert.Equal(t, emailParticipantKey, participant.EmailParticipantKey)
	assert.Equal(t, campaignSlotKey, participant.CampaignSlotKey)
	var casbinV0 string
	require.NoError(t, db.Table("casbin_rule").Pluck("v0", &casbinV0).Error)
	assert.NotEqual(t, "user:7", casbinV0)
	assert.Regexp(t, `^user:[1-9][0-9]{11}$`, casbinV0)
	assert.NoError(t, verifyO023PostMigrationStructuredReferences(db))
	status, err := O023MigrationStatus(db)
	require.NoError(t, err)
	require.Equal(t, "ALREADY_MIGRATED", status)
	storedInvariant, present, err := o023OptionValue(db, O023InvariantHash)
	require.NoError(t, err)
	require.True(t, present)
	persisted, err := o023DecodePersistedInvariant(storedInvariant)
	require.NoError(t, err)
	boundaries := map[string]int64{}
	for table, historical := range persisted.HistoricalWaffo {
		boundaries[table] = historical.Boundary
	}
	currentSnapshot, err := o023InvariantSnapshotForBoundaries(db, nil, boundaries)
	require.NoError(t, err)
	require.Equal(t, map[string]int64{
		"ALLOWED_HISTORICAL_WAFFO_SNAPSHOT:top_ups":             1,
		"ALLOWED_HISTORICAL_WAFFO_SNAPSHOT:subscription_orders": 1,
	}, o023AllowedHistoricalWaffoReport(currentSnapshot))

	for _, table := range []string{"top_ups", "subscription_orders"} {
		requireO023WaffoMutationRejected(t, db, persisted, "o023_delete_"+table, "DELETE FROM "+table+" WHERE payment_provider = ?", PaymentProviderWaffoPancake)
		requireO023WaffoMutationRejected(t, db, persisted, "o023_modify_"+table, "UPDATE "+table+" SET waffo_buyer_identity = ? WHERE payment_provider = ?", "new-api-user-999", PaymentProviderWaffoPancake)
		requireO023WaffoMutationRejected(t, db, persisted, "o023_provider_"+table, "UPDATE "+table+" SET payment_provider = ? WHERE payment_provider = ?", "epay", PaymentProviderWaffoPancake)
		requireO023WaffoMutationRejected(t, db, persisted, "o023_empty_"+table, "UPDATE "+table+" SET waffo_buyer_identity = NULL WHERE payment_provider = ?", PaymentProviderWaffoPancake)
		requireO023WaffoMutationRejected(t, db, persisted, "o023_add_"+table, "INSERT INTO "+table+" (id, user_id, trade_no, payment_provider, status, waffo_buyer_identity) VALUES (?, ?, ?, ?, ?, ?)", 0, migratedA, "synthetic-added-history-"+table, PaymentProviderWaffoPancake, common.TopUpStatusSuccess, "new-api-user-7")
		requireO023WaffoMutationRejected(t, db, persisted, "o023_old_new_"+table, "INSERT INTO "+table+" (user_id, trade_no, payment_provider, status, waffo_buyer_identity) VALUES (?, ?, ?, ?, ?)", migratedA, "synthetic-old-format-new-"+table, PaymentProviderWaffoPancake, common.TopUpStatusSuccess, "new-api-user-7")
	}
	require.NoError(t, db.Exec(`INSERT INTO top_ups (user_id, trade_no, payment_provider, status, waffo_buyer_identity) VALUES (?, ?, ?, ?, ?)`, migratedA, "synthetic-new-waffo-topup", PaymentProviderWaffoPancake, common.TopUpStatusSuccess, "wb_top_0123456789abcdef").Error)
	require.NoError(t, db.Exec(`INSERT INTO subscription_orders (user_id, trade_no, payment_provider, status, waffo_buyer_identity) VALUES (?, ?, ?, ?, ?)`, migratedA, "synthetic-new-waffo-subscription", PaymentProviderWaffoPancake, common.TopUpStatusSuccess, "wb_sub_0123456789abcdef").Error)
	for _, key := range expectedNewCacheKeys {
		require.NoError(t, common.RDB.Set(context.Background(), key, "o023-new", 0).Err())
	}
	require.NoError(t, MigrateO023UserIDs(db))
	for _, key := range expectedNewCacheKeys {
		require.True(t, redisServer.Exists(key), "ALREADY_MIGRATED must preserve current key: %s", key)
		value, err := redisServer.Get(key)
		require.NoError(t, err)
		require.Equal(t, "o023-new", value)
	}
	require.NoError(t, VerifyO023Invariants(db))
}

func TestO023CompleteSyntheticRehearsals(t *testing.T) {
	runO023CompleteSyntheticRehearsal(t, "o023-complete-rehearsal-one")
	runO023CompleteSyntheticRehearsal(t, "o023-complete-rehearsal-two")
}
