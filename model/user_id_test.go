package model

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withTestUserIDSource(t *testing.T, source func() (int, error)) {
	t.Helper()
	userIDSourceMu.Lock()
	previous := userIDSource
	userIDSource = source
	userIDSourceMu.Unlock()
	t.Cleanup(func() {
		userIDSourceMu.Lock()
		userIDSource = previous
		userIDSourceMu.Unlock()
	})
}

func TestRandomUserIDContract(t *testing.T) {
	for range 200 {
		id, err := randomUserID()
		require.NoError(t, err)
		require.GreaterOrEqual(t, int64(id), UserIDMin)
		require.LessOrEqual(t, int64(id), UserIDMax)
	}
}

func TestRandomUserIDRejectsOutOfRangeSource(t *testing.T) {
	withTestUserIDSource(t, func() (int, error) { return 100000000000, nil })
	_, err := randomUserID()
	require.EqualError(t, err, "generated user id outside six-digit range: 100000000000")
}

func TestUserRejectsExplicitID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:o023-user-id-explicit?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	err = db.Create(&User{Id: 123456789012, Username: "explicit", Password: "password"}).Error
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrExplicitUserIDForbidden))
}

func TestRandomOpaqueBusinessIDDoesNotContainUserID(t *testing.T) {
	value, err := RandomOpaqueBusinessID("EPAY-")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(value, "EPAY-"))
	require.Len(t, value, len("EPAY-")+32)
	require.NotContains(t, value, "123456789012")
}

func TestCreateUserWithRetryExhaustsAfterThirtyTwoPrimaryKeyCollisions(t *testing.T) {
	originalDB, originalMain := DB, common.MainDatabaseType()
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:o023-collision-exhaustion?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	DB = db
	t.Cleanup(func() { DB = originalDB; common.SetMainDatabaseType(originalMain) })
	const candidate = 123456
	require.NoError(t, db.Exec(`INSERT INTO users (id, username, password) VALUES (?, ?, ?)`, candidate, "occupied", "password").Error)
	var calls atomic.Int32
	withTestUserIDSource(t, func() (int, error) { calls.Add(1); return candidate, nil })
	err = CreateUserWithRetry(&User{Username: "fresh", Password: "password"})
	require.ErrorIs(t, err, ErrUserIDCollisionExhausted)
	assert.Equal(t, int32(UserIDCreateMax), calls.Load())
	var count int64
	require.NoError(t, db.Model(&User{}).Where("username = ?", "fresh").Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateUserWithRetryDoesNotRetryOrdinaryUniqueConstraint(t *testing.T) {
	originalDB, originalMain := DB, common.MainDatabaseType()
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:o023-unique-no-retry?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	DB = db
	t.Cleanup(func() { DB = originalDB; common.SetMainDatabaseType(originalMain) })
	require.NoError(t, db.Exec(`INSERT INTO users (id, username, password) VALUES (?, ?, ?)`, 123456, "duplicate", "password").Error)
	var calls atomic.Int32
	withTestUserIDSource(t, func() (int, error) { calls.Add(1); return 123457, nil })
	err = CreateUserWithRetry(&User{Username: "duplicate", Password: "password"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrUserIDCollisionExhausted)
	assert.Equal(t, int32(1), calls.Load())
}
