package model

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"sync"

	"github.com/QuantumNous/new-api/common"
	glebarezsqlite "github.com/glebarez/go-sqlite"
	"gorm.io/gorm"
	sqlitelib "modernc.org/sqlite/lib"
)

const (
	UserIDMin       int64 = 100000000000
	UserIDMax       int64 = 999999999999
	UserIDCreateMax       = 32
)

var (
	ErrExplicitUserIDForbidden  = errors.New("EXPLICIT_USER_ID_FORBIDDEN")
	ErrUserIDCollisionExhausted = errors.New("USER_ID_COLLISION_RETRY_EXHAUSTED")
)

var userIDSourceMu sync.RWMutex
var userIDSource = cryptoRandomUserID

// Keep the application-wide int user ID safe for the 12-digit contract.
var _ [strconv.IntSize - 64]byte

func randomUserID() (int, error) {
	userIDSourceMu.RLock()
	source := userIDSource
	userIDSourceMu.RUnlock()
	return source()
}

func cryptoRandomUserID() (int, error) {
	span := big.NewInt(UserIDMax - UserIDMin + 1)
	n, err := cryptorand.Int(cryptorand.Reader, span)
	if err != nil {
		return 0, fmt.Errorf("generate random user id: %w", err)
	}
	return int(n.Int64() + UserIDMin), nil
}

func RandomUserNameSuffix() (string, error) {
	id, err := randomUserID()
	if err != nil {
		return "", err
	}
	return strconv.Itoa(id), nil
}

// RandomOpaqueBusinessID creates a collision-resistant identifier which does
// not encode a user identity or any other business key.
func RandomOpaqueBusinessID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := cryptorand.Read(b); err != nil {
		return "", fmt.Errorf("generate opaque business id: %w", err)
	}
	return prefix + hex.EncodeToString(b), nil
}

func CreateUserWithRetry(user *User) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return createUserWithRetry(tx, user)
	})
}

// BeforeCreate is the last line of defence for every GORM Create path.
// Explicit IDs are never accepted by production model creation.
func (user *User) BeforeCreate(_ *gorm.DB) error {
	if user.Id != 0 {
		return ErrExplicitUserIDForbidden
	}
	id, err := randomUserID()
	if err != nil {
		return err
	}
	user.Id = id
	return nil
}

func sqlitePrimaryKeyCollision(err error) bool {
	if !isSQLiteMainDatabase() {
		return false
	}
	var sqliteErr *glebarezsqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code()
	return code == sqlitelib.SQLITE_CONSTRAINT_PRIMARYKEY || code == sqlitelib.SQLITE_CONSTRAINT_ROWID
}

func isSQLiteMainDatabase() bool {
	return common.UsingMainDatabase(common.DatabaseTypeSQLite)
}

// createUserWithRetry retries only an identified SQLite users primary-key
// collision. Other unique constraints and all non-SQLite errors are returned.
func createUserWithRetry(tx *gorm.DB, user *User) error {
	for attempt := 0; attempt < UserIDCreateMax; attempt++ {
		if err := tx.SavePoint("o023_user_id").Error; err != nil {
			return err
		}
		user.Id = 0
		err := tx.Create(user).Error
		if err == nil {
			return nil
		}
		if !sqlitePrimaryKeyCollision(err) {
			return err
		}
		candidateID := user.Id
		var existing int64
		if countErr := tx.Unscoped().Model(&User{}).Where("id = ?", candidateID).Count(&existing).Error; countErr != nil {
			return countErr
		}
		if existing != 1 {
			return err
		}
		if rollbackErr := tx.RollbackTo("o023_user_id").Error; rollbackErr != nil {
			return rollbackErr
		}
	}
	return ErrUserIDCollisionExhausted
}
