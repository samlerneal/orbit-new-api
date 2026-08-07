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
	UserIDMin       int64 = 100000
	UserIDMax       int64 = 999999
	UserIDCreateMax       = 32
)

var (
	ErrExplicitUserIDForbidden     = errors.New("EXPLICIT_USER_ID_FORBIDDEN")
	ErrExplicitInternalIDForbidden = errors.New("EXPLICIT_INTERNAL_USER_ID_FORBIDDEN")
	ErrInternalIDExhausted         = errors.New("INTERNAL_USER_ID_EXHAUSTED")
	ErrO031UserCreationNotReady    = errors.New("O031_USER_CREATION_NOT_READY")
	ErrUserIDCollisionExhausted    = errors.New("USER_ID_COLLISION_RETRY_EXHAUSTED")
)

var userIDSourceMu sync.RWMutex
var userIDSource = cryptoRandomUserID
var o023UserIDSource = cryptoRandomO023UserID

func randomUserID() (int, error) {
	userIDSourceMu.RLock()
	source := userIDSource
	userIDSourceMu.RUnlock()
	id, err := source()
	if err != nil {
		return 0, err
	}
	if int64(id) < UserIDMin || int64(id) > UserIDMax {
		return 0, fmt.Errorf("generated user id outside six-digit range: %d", id)
	}
	return id, nil
}

func cryptoRandomUserID() (int, error) {
	return cryptoRandomIDInRange(UserIDMin, UserIDMax)
}

func randomO023UserID() (int, error) {
	userIDSourceMu.RLock()
	source := o023UserIDSource
	userIDSourceMu.RUnlock()
	id, err := source()
	if err != nil {
		return 0, err
	}
	if int64(id) < O023UserIDMin || int64(id) > O023UserIDMax {
		return 0, fmt.Errorf("generated O-023 user id outside historical range: %d", id)
	}
	return id, nil
}

func cryptoRandomO023UserID() (int, error) {
	return cryptoRandomIDInRange(O023UserIDMin, O023UserIDMax)
}

func cryptoRandomIDInRange(minimum, maximum int64) (int, error) {
	span := big.NewInt(maximum - minimum + 1)
	n, err := cryptorand.Int(cryptorand.Reader, span)
	if err != nil {
		return 0, fmt.Errorf("generate random user id: %w", err)
	}
	return int(n.Int64() + minimum), nil
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

func ensureO031UserCreationReady(tx *gorm.DB, internalIDColumnExists bool) error {
	if !internalIDColumnExists {
		optionsExist, err := sqliteTableExists(tx, "options")
		if err != nil {
			return err
		}
		if !optionsExist {
			return nil
		}
		if version, present, err := o023OptionValue(tx, O023UserIDMigrationVersion); err != nil {
			return err
		} else if present && version == "1" {
			return ErrO031UserCreationNotReady
		}
		return nil
	}
	version, present, err := o023OptionValue(tx, O031MigrationVersion)
	if err != nil || !present || version != "1" {
		if err != nil {
			return err
		}
		return ErrO031UserCreationNotReady
	}
	cleanup, present, err := o023OptionValue(tx, O031CleanupState)
	if err != nil || !present || cleanup != O031CleanupClean {
		if err != nil {
			return err
		}
		return ErrO031UserCreationNotReady
	}
	indexReady, err := o031InternalIDIndexReady(tx)
	if err != nil || !indexReady {
		if err != nil {
			return err
		}
		return ErrO031UserCreationNotReady
	}
	return nil
}

// BeforeCreate is the last line of defence for every GORM Create path.
// Explicit IDs are never accepted by production model creation.
func (user *User) BeforeCreate(tx *gorm.DB) error {
	if user.Id != 0 {
		return ErrExplicitUserIDForbidden
	}
	if user.InternalId != 0 {
		return ErrExplicitInternalIDForbidden
	}
	internalIDColumnExists, err := sqliteColumnExists(tx, "users", "internal_id")
	if err != nil {
		return fmt.Errorf("inspect internal user id schema: %w", err)
	}
	if err := ensureO031UserCreationReady(tx, internalIDColumnExists); err != nil {
		return err
	}
	if internalIDColumnExists {
		var maxInternalID int64
		if err := tx.Unscoped().Model(&User{}).Select("coalesce(max(internal_id), 0)").Scan(&maxInternalID).Error; err != nil {
			return fmt.Errorf("allocate internal user id: %w", err)
		}
		if maxInternalID >= int64(^uint(0)>>1) {
			return ErrInternalIDExhausted
		}
		user.InternalId = int(maxInternalID + 1)
	} else {
		// O-023 fixtures and pre-O-031 databases do not have this explicitly
		// prepared column. The O-031 release is started only after its offline
		// migration, so historical creation remains byte-compatible here.
		user.InternalId = 0
	}
	id, err := randomUserID()
	if err != nil {
		return err
	}
	user.Id = id
	return nil
}

// AfterCreate writes the administrator-only sequence through an explicit SQL
// path. The field is deliberately excluded from GORM's ordinary model schema,
// which prevents accidental reads or JSON exposure outside the admin DTO.
func (user *User) AfterCreate(tx *gorm.DB) error {
	if user.InternalId == 0 {
		return nil
	}
	return tx.Exec("UPDATE users SET internal_id = ? WHERE id = ?", user.InternalId, user.Id).Error
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

func sqliteUniqueConstraint(err error) bool {
	if !isSQLiteMainDatabase() {
		return false
	}
	var sqliteErr *glebarezsqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	return sqliteErr.Code() == sqlitelib.SQLITE_CONSTRAINT_UNIQUE
}

func isSQLiteMainDatabase() bool {
	return common.UsingMainDatabase(common.DatabaseTypeSQLite)
}

// createUserWithRetry retries only identified SQLite collisions on the public
// six-digit ID or the administrator-only internal sequence. Other unique
// constraints and all non-SQLite errors are returned unchanged.
func createUserWithRetry(tx *gorm.DB, user *User) error {
	for attempt := 0; attempt < UserIDCreateMax; attempt++ {
		if err := tx.SavePoint("o023_user_id").Error; err != nil {
			return err
		}
		user.Id = 0
		user.InternalId = 0
		err := tx.Create(user).Error
		if err == nil {
			return nil
		}
		candidateID := user.Id
		candidateInternalID := user.InternalId
		var existing int64
		switch {
		case sqlitePrimaryKeyCollision(err):
			if countErr := tx.Unscoped().Model(&User{}).Where("id = ?", candidateID).Count(&existing).Error; countErr != nil {
				return countErr
			}
		case sqliteUniqueConstraint(err) && candidateInternalID > 0:
			if countErr := tx.Unscoped().Model(&User{}).Where("internal_id = ?", candidateInternalID).Count(&existing).Error; countErr != nil {
				return countErr
			}
		default:
			return err
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
