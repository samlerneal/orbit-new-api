package model

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withTestO023UserIDSource(t *testing.T, source func() (int, error)) {
	t.Helper()
	userIDSourceMu.Lock()
	previous := o023UserIDSource
	o023UserIDSource = source
	userIDSourceMu.Unlock()
	t.Cleanup(func() {
		userIDSourceMu.Lock()
		o023UserIDSource = previous
		userIDSourceMu.Unlock()
	})
}

func nextIDSource(t *testing.T, values ...int) func() (int, error) {
	t.Helper()
	var index atomic.Int32
	return func() (int, error) {
		position := int(index.Add(1)) - 1
		require.Less(t, position, len(values), "test ID source exhausted")
		return values[position], nil
	}
}

func prepareO031SourceFixture(t *testing.T, name string) (*gorm.DB, []O031LegacyUser) {
	t.Helper()
	db := newO023CanonicalSchemaFixture(t, name)
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB })

	legacy := []O031LegacyUser{
		{InternalID: 1, Username: "owner", CreatedAt: 100},
		{InternalID: 2, Username: "partner", CreatedAt: 200},
		{InternalID: 3, Username: "member", CreatedAt: 300},
	}
	for _, user := range legacy {
		require.NoError(t, db.Exec(
			`INSERT INTO users (id, username, password, created_at, quota, used_quota, request_count) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			user.InternalID, user.Username, "password", user.CreatedAt, 100, 10, 1,
		).Error)
	}
	require.NoError(t, db.Exec(`INSERT INTO tokens (user_id, key, status, remain_quota, used_quota) VALUES (?, ?, ?, ?, ?)`, 1, "o031-stable-api-key", 1, 90, 10).Error)
	require.NoError(t, db.Exec(`INSERT INTO user_sessions (user_id, sid, user_auth_version, status, refresh_hash, login_method, last_active_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, 2, "o031-session", 1, "active", "refresh", "password", 100, 9999999999).Error)

	require.NoError(t, AuditO023Schema(db))
	require.NoError(t, PrepareO023Schema(db))
	withTestO023UserIDSource(t, nextIDSource(t, 111111111111, 222222222222, 333333333333))
	require.NoError(t, MigrateO023UserIDs(db))
	require.NoError(t, VerifyO023Invariants(db))
	// O-031 must independently revoke sessions during its own cutover.
	require.NoError(t, db.Exec(`UPDATE user_sessions SET status = 'active', revoked_at = 0`).Error)
	require.NoError(t, PrepareO031Schema(db))
	return db, legacy
}

func TestO031RekeysUsersAndRestoresInternalIDs(t *testing.T) {
	db, legacy := prepareO031SourceFixture(t, "o031-complete-rehearsal")
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	withTestUserIDSource(t, nextIDSource(t, 654321, 765432, 876543, 456789))
	// Schema preparation is safe to retry after a lost client response.
	require.NoError(t, PrepareO031Schema(db))
	blockedUser := &User{Username: "blocked-before-migration", Password: "password"}
	require.ErrorIs(t, CreateUserWithRetry(blockedUser), ErrO031UserCreationNotReady)
	require.NoError(t, MigrateO031UserIDs(db, legacy))
	require.NoError(t, VerifyO031Invariants(db))
	status, err := O031MigrationStatus(db)
	require.NoError(t, err)
	assert.Equal(t, "ALREADY_MIGRATED", status)

	var users []struct {
		ID         int    `gorm:"column:id"`
		InternalID int    `gorm:"column:internal_id"`
		Username   string `gorm:"column:username"`
	}
	require.NoError(t, db.Table("users").Select("id", "internal_id", "username").Order("internal_id ASC").Scan(&users).Error)
	require.Len(t, users, 3)
	for index, user := range users {
		assert.Equal(t, index+1, user.InternalID)
		assert.Equal(t, legacy[index].Username, user.Username)
		assert.GreaterOrEqual(t, int64(user.ID), UserIDMin)
		assert.LessOrEqual(t, int64(user.ID), UserIDMax)
	}
	assert.Equal(t, []int{654321, 765432, 876543}, []int{users[0].ID, users[1].ID, users[2].ID})

	var tokenKey string
	require.NoError(t, db.Table("tokens").Pluck("key", &tokenKey).Error)
	assert.Equal(t, "o031-stable-api-key", tokenKey)
	var tokenUserID int
	require.NoError(t, db.Table("tokens").Pluck("user_id", &tokenUserID).Error)
	assert.Equal(t, 654321, tokenUserID)
	var sessionStatus string
	require.NoError(t, db.Table("user_sessions").Where("sid = ?", "o031-session").Pluck("status", &sessionStatus).Error)
	assert.Equal(t, "revoked", sessionStatus)

	// The migration is idempotent and does not require the historical backup
	// after its version marker and invariants have been committed.
	require.NoError(t, MigrateO031UserIDs(db, nil))

	newUser := &User{Username: "new-user", Password: "password"}
	require.NoError(t, CreateUserWithRetry(newUser))
	assert.Equal(t, 456789, newUser.Id)
	assert.Equal(t, 4, newUser.InternalId)
}

func TestO031PostCommitCleanupResumesIdempotently(t *testing.T) {
	db, legacy := prepareO031SourceFixture(t, "o031-cache-cleanup-resume")
	withTestUserIDSource(t, nextIDSource(t, 654321, 765432, 876543))

	previousInvalidate := o031InvalidateCaches
	previousResiduals := o031VerifyRedisResiduals
	previousSummary := o031ReadAPISummary
	t.Cleanup(func() {
		o031InvalidateCaches = previousInvalidate
		o031VerifyRedisResiduals = previousResiduals
		o031ReadAPISummary = previousSummary
	})

	o031InvalidateCaches = func(context.Context, []string) error {
		return errors.New("injected cache delete failure")
	}
	require.EqualError(t, MigrateO031UserIDs(db, legacy), "injected cache delete failure")
	status, err := O031MigrationStatus(db)
	require.NoError(t, err)
	assert.Equal(t, "CACHE_CLEANUP_PENDING", status)

	o031InvalidateCaches = previousInvalidate
	o031VerifyRedisResiduals = func(context.Context, []int) error {
		return errors.New("injected residual scan failure")
	}
	require.EqualError(t, MigrateO031UserIDs(db, nil), "injected residual scan failure")

	o031VerifyRedisResiduals = previousResiduals
	o031ReadAPISummary = func(*gorm.DB) (string, error) {
		return "", errors.New("injected API summary failure")
	}
	require.EqualError(t, MigrateO031UserIDs(db, nil), "injected API summary failure")

	o031ReadAPISummary = previousSummary
	require.NoError(t, MigrateO031UserIDs(db, nil))
	status, err = O031MigrationStatus(db)
	require.NoError(t, err)
	assert.Equal(t, "ALREADY_MIGRATED", status)
	_, planPresent, err := o023OptionValue(db, O031CleanupPlan)
	require.NoError(t, err)
	assert.False(t, planPresent)
}

func TestO031RejectsAmbiguousLegacyIdentityWithoutMutation(t *testing.T) {
	db, legacy := prepareO031SourceFixture(t, "o031-ambiguous-legacy")
	legacy = append(legacy, O031LegacyUser{InternalID: 4, Username: legacy[0].Username, CreatedAt: legacy[0].CreatedAt})
	withTestUserIDSource(t, nextIDSource(t, 654321, 765432, 876543))

	var before []int
	require.NoError(t, db.Order("id").Table("users").Pluck("id", &before).Error)
	err := MigrateO031UserIDs(db, legacy)
	require.EqualError(t, err, "ambiguous O-031 legacy identity")
	var after []int
	require.NoError(t, db.Order("id").Table("users").Pluck("id", &after).Error)
	assert.Equal(t, before, after)
	status, statusErr := O031MigrationStatus(db)
	require.NoError(t, statusErr)
	assert.Equal(t, "READY", status)
}

func TestUserJSONNeverExposesInternalID(t *testing.T) {
	payload, err := json.Marshal(User{Id: 654321, InternalId: 1, Username: "owner"})
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	assert.EqualValues(t, 654321, decoded["id"])
	_, exposed := decoded["internal_id"]
	assert.False(t, exposed)
}
