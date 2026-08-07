package model

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	O031MigrationVersion = "o031_user_id_migration_version"
	O031InvariantHash    = "o031_invariant_hash"
	O031SchemaState      = "o031_schema_state"
	O031SchemaPrepared   = "prepared-v1"
	O031CleanupPlan      = "o031_cache_cleanup_plan"
	O031CleanupState     = "o031_cache_cleanup_state"
	O031CleanupPending   = "pending-v1"
	O031CleanupClean     = "clean-v1"
)

type O031LegacyUser struct {
	InternalID int
	Username   string
	CreatedAt  int64
}

type o031UserMapping struct {
	OldID      int
	NewID      int
	InternalID int
}

type o031Invariant struct {
	Users          int64  `json:"users"`
	InternalIDs    int64  `json:"internal_ids"`
	APIKeyDigest   string `json:"api_key_digest"`
	BusinessDigest string `json:"business_digest"`
}

type o031CacheCleanupPlan struct {
	OldIDs     []int    `json:"old_ids"`
	NewIDs     []int    `json:"new_ids"`
	CacheKeys  []string `json:"cache_keys"`
	APISummary string   `json:"api_summary"`
}

var o031InvalidateCaches = invalidateO023Caches
var o031VerifyRedisResiduals = verifyO023RawRedisResiduals
var o031ReadAPISummary = o023APISummary

func PrepareO031Schema(db *gorm.DB) error {
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return fmt.Errorf("O-031 only supports SQLite: %w", ErrUnknownUserReference)
	}
	internalIDColumnExists, err := sqliteColumnExists(db, "users", "internal_id")
	if err != nil {
		return err
	}
	if internalIDColumnExists {
		return AuditO031PreparedSchema(db)
	}
	if err := AuditO023Schema(db); err != nil {
		return fmt.Errorf("O-031 source schema is not the accepted O-023 lineage: %w", err)
	}
	status, err := O023MigrationStatus(db)
	if err != nil || status != "ALREADY_MIGRATED" {
		if err != nil {
			return err
		}
		return fmt.Errorf("O-031 requires completed O-023 source state")
	}
	return db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("BEGIN IMMEDIATE").Error; err != nil {
			return err
		}
		conn = conn.Session(&gorm.Session{SkipDefaultTransaction: true})
		committed := false
		defer func() {
			if !committed {
				_ = conn.Exec("ROLLBACK").Error
			}
		}()
		exists, err := sqliteColumnExists(conn, "users", "internal_id")
		if err != nil {
			return err
		}
		if !exists {
			if err := conn.Exec("ALTER TABLE users ADD COLUMN internal_id INTEGER NULL").Error; err != nil {
				return err
			}
		}
		if err := setO023Option(conn, O031SchemaState, O031SchemaPrepared); err != nil {
			return err
		}
		if err := conn.Exec("COMMIT").Error; err != nil {
			return err
		}
		committed = true
		return AuditO031PreparedSchema(conn)
	})
}

func AuditO031PreparedSchema(db *gorm.DB) error {
	type columnContract struct {
		Name         string         `gorm:"column:name"`
		Type         string         `gorm:"column:type"`
		NotNull      int            `gorm:"column:notnull"`
		DefaultValue sql.NullString `gorm:"column:dflt_value"`
		PK           int            `gorm:"column:pk"`
		Hidden       int            `gorm:"column:hidden"`
	}
	var columns []columnContract
	if err := db.Raw(`SELECT name, type, "notnull", dflt_value, pk, hidden FROM pragma_table_xinfo('users') WHERE name = 'internal_id'`).Scan(&columns).Error; err != nil {
		return err
	}
	if len(columns) != 1 || !strings.EqualFold(strings.TrimSpace(columns[0].Type), "INTEGER") || columns[0].NotNull != 0 || columns[0].DefaultValue.Valid || columns[0].PK != 0 || columns[0].Hidden != 0 {
		return fmt.Errorf("O-031 internal_id column contract mismatch")
	}
	state, present, err := o023OptionValue(db, O031SchemaState)
	if err != nil {
		return err
	}
	if !present || state != O031SchemaPrepared {
		return fmt.Errorf("O-031 schema marker is missing or invalid")
	}
	return nil
}

func O031MigrationStatus(db *gorm.DB) (string, error) {
	version, present, err := o023OptionValue(db, O031MigrationVersion)
	if err != nil {
		return "", err
	}
	if present {
		if version != "1" {
			return "", ErrMigrationVersionConflict
		}
		if err := VerifyO031Invariants(db); err != nil {
			return "", err
		}
		cleanupState, cleanupPresent, err := o023OptionValue(db, O031CleanupState)
		if err != nil {
			return "", err
		}
		if cleanupPresent && cleanupState == O031CleanupClean {
			return "ALREADY_MIGRATED", nil
		}
		if cleanupPresent && cleanupState == O031CleanupPending {
			if _, planPresent, err := o023OptionValue(db, O031CleanupPlan); err != nil {
				return "", err
			} else if planPresent {
				return "CACHE_CLEANUP_PENDING", nil
			}
		}
		return "", fmt.Errorf("O-031 cache cleanup state is missing or invalid")
	}
	if err := AuditO031PreparedSchema(db); err != nil {
		return "", err
	}
	var total, source int64
	if err := db.Unscoped().Model(&User{}).Count(&total).Error; err != nil {
		return "", err
	}
	if err := db.Unscoped().Model(&User{}).Where("id >= ? AND id <= ?", O023UserIDMin, O023UserIDMax).Count(&source).Error; err != nil {
		return "", err
	}
	if source != total {
		return "", fmt.Errorf("O-031 source IDs are not entirely O-023 12-digit IDs")
	}
	var assigned int64
	if err := db.Unscoped().Model(&User{}).Where("internal_id IS NOT NULL AND internal_id != 0").Count(&assigned).Error; err != nil {
		return "", err
	}
	if assigned != 0 {
		return "", ErrUnmarkedPartialMigration
	}
	indexReady, err := o031InternalIDIndexReady(db)
	if err != nil {
		return "", err
	}
	if indexReady {
		return "", ErrUnmarkedPartialMigration
	}
	return "READY", nil
}

func o031InternalIDIndexReady(db *gorm.DB) (bool, error) {
	type indexContract struct {
		Name    string `gorm:"column:name"`
		Unique  int    `gorm:"column:unique"`
		Partial int    `gorm:"column:partial"`
	}
	var indexes []indexContract
	if err := db.Raw(`SELECT name, "unique", partial FROM pragma_index_list('users') WHERE name = 'idx_users_internal_id'`).Scan(&indexes).Error; err != nil {
		return false, err
	}
	if len(indexes) == 0 {
		return false, nil
	}
	if len(indexes) != 1 || indexes[0].Unique != 1 || indexes[0].Partial != 0 {
		return false, fmt.Errorf("O-031 internal ID index contract mismatch")
	}
	var columns []string
	if err := db.Raw(`SELECT name FROM pragma_index_info('idx_users_internal_id') ORDER BY seqno`).Pluck("name", &columns).Error; err != nil {
		return false, err
	}
	if len(columns) != 1 || columns[0] != "internal_id" {
		return false, fmt.Errorf("O-031 internal ID index column mismatch")
	}
	return true, nil
}

func o031LegacyKey(username string, createdAt int64) string {
	return strings.TrimSpace(username) + "\x00" + strconv.FormatInt(createdAt, 10)
}

func buildO031Mappings(users []User, legacy []O031LegacyUser) ([]o031UserMapping, error) {
	legacyByKey := make(map[string]int, len(legacy))
	seenInternal := make(map[int]struct{}, len(legacy))
	maxInternal := 0
	for _, item := range legacy {
		if item.InternalID <= 0 || strings.TrimSpace(item.Username) == "" {
			return nil, fmt.Errorf("invalid O-031 legacy identity")
		}
		key := o031LegacyKey(item.Username, item.CreatedAt)
		if _, duplicate := legacyByKey[key]; duplicate {
			return nil, fmt.Errorf("ambiguous O-031 legacy identity")
		}
		if _, duplicate := seenInternal[item.InternalID]; duplicate {
			return nil, fmt.Errorf("duplicate O-031 legacy internal ID")
		}
		legacyByKey[key] = item.InternalID
		seenInternal[item.InternalID] = struct{}{}
		if item.InternalID > maxInternal {
			maxInternal = item.InternalID
		}
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].CreatedAt != users[j].CreatedAt {
			return users[i].CreatedAt < users[j].CreatedAt
		}
		return users[i].Id < users[j].Id
	})
	mappings := make([]o031UserMapping, 0, len(users))
	usedNewIDs := make(map[int]struct{}, len(users))
	matchedLegacy := make(map[int]struct{}, len(legacy))
	for _, user := range users {
		if int64(user.Id) < O023UserIDMin || int64(user.Id) > O023UserIDMax {
			return nil, fmt.Errorf("unexpected O-031 source user ID")
		}
		internalID, matched := legacyByKey[o031LegacyKey(user.Username, user.CreatedAt)]
		if matched {
			if _, duplicate := matchedLegacy[internalID]; duplicate {
				return nil, fmt.Errorf("O-031 legacy identity matched more than once")
			}
			matchedLegacy[internalID] = struct{}{}
		} else {
			maxInternal++
			internalID = maxInternal
		}
		var newID int
		for attempt := 0; attempt < UserIDCreateMax; attempt++ {
			candidate, err := randomUserID()
			if err != nil {
				return nil, err
			}
			if _, duplicate := usedNewIDs[candidate]; duplicate {
				continue
			}
			usedNewIDs[candidate] = struct{}{}
			newID = candidate
			break
		}
		if newID == 0 {
			return nil, ErrUserIDCollisionExhausted
		}
		mappings = append(mappings, o031UserMapping{OldID: user.Id, NewID: newID, InternalID: internalID})
	}
	if len(matchedLegacy) != len(legacy) {
		return nil, fmt.Errorf("O-031 legacy backup contains unmatched users")
	}
	return mappings, nil
}

func buildO031CacheCleanupPlan(db *gorm.DB, oldIDs, newIDs []int, apiSummary string) (o031CacheCleanupPlan, error) {
	oldKeys, err := collectO023CacheKeys(context.Background(), db, oldIDs)
	if err != nil {
		return o031CacheCleanupPlan{}, err
	}
	newKeys, err := collectO023CacheKeys(context.Background(), db, newIDs)
	if err != nil {
		return o031CacheCleanupPlan{}, err
	}
	uniqueKeys := make(map[string]struct{}, len(oldKeys)+len(newKeys))
	for _, key := range append(oldKeys, newKeys...) {
		if key != "" {
			uniqueKeys[key] = struct{}{}
		}
	}
	cacheKeys := make([]string, 0, len(uniqueKeys))
	for key := range uniqueKeys {
		cacheKeys = append(cacheKeys, key)
	}
	sort.Strings(cacheKeys)
	return o031CacheCleanupPlan{
		OldIDs:     append([]int(nil), oldIDs...),
		NewIDs:     append([]int(nil), newIDs...),
		CacheKeys:  cacheKeys,
		APISummary: apiSummary,
	}, nil
}

func validateO031CacheCleanupPlan(plan o031CacheCleanupPlan) error {
	if len(plan.OldIDs) != len(plan.NewIDs) || strings.TrimSpace(plan.APISummary) == "" {
		return fmt.Errorf("invalid O-031 cache cleanup plan")
	}
	oldSeen := make(map[int]struct{}, len(plan.OldIDs))
	newSeen := make(map[int]struct{}, len(plan.NewIDs))
	for index, oldID := range plan.OldIDs {
		newID := plan.NewIDs[index]
		if int64(oldID) < O023UserIDMin || int64(oldID) > O023UserIDMax || int64(newID) < UserIDMin || int64(newID) > UserIDMax {
			return fmt.Errorf("invalid O-031 cache cleanup identity")
		}
		if _, duplicate := oldSeen[oldID]; duplicate {
			return fmt.Errorf("duplicate O-031 cleanup old ID")
		}
		if _, duplicate := newSeen[newID]; duplicate {
			return fmt.Errorf("duplicate O-031 cleanup new ID")
		}
		oldSeen[oldID] = struct{}{}
		newSeen[newID] = struct{}{}
	}
	for _, key := range plan.CacheKeys {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid empty O-031 cleanup cache key")
		}
	}
	return nil
}

func finalizeO031CacheCleanup(db *gorm.DB) error {
	state, present, err := o023OptionValue(db, O031CleanupState)
	if err != nil {
		return err
	}
	if present && state == O031CleanupClean {
		return nil
	}
	if !present || state != O031CleanupPending {
		return fmt.Errorf("O-031 cache cleanup is not pending")
	}
	encoded, present, err := o023OptionValue(db, O031CleanupPlan)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("O-031 cache cleanup plan is missing")
	}
	var plan o031CacheCleanupPlan
	if err := json.Unmarshal([]byte(encoded), &plan); err != nil {
		return fmt.Errorf("decode O-031 cache cleanup plan: %w", err)
	}
	if err := validateO031CacheCleanupPlan(plan); err != nil {
		return err
	}
	if err := verifyO023RedisTopology(context.Background()); err != nil {
		return err
	}
	if err := o031InvalidateCaches(context.Background(), plan.CacheKeys); err != nil {
		return err
	}
	if err := o031VerifyRedisResiduals(context.Background(), plan.OldIDs); err != nil {
		return err
	}
	apiSummary, err := o031ReadAPISummary(db)
	if err != nil {
		return err
	}
	if apiSummary != plan.APISummary {
		return fmt.Errorf("API key summary changed during O-031 migration")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("key = ?", O031CleanupPlan).Delete(&Option{}).Error; err != nil {
			return err
		}
		return setO023Option(tx, O031CleanupState, O031CleanupClean)
	})
}

func MigrateO031UserIDs(db *gorm.DB, legacy []O031LegacyUser) error {
	var callbackErr error
	connectionErr := db.Connection(func(conn *gorm.DB) error {
		callbackErr = migrateO031UserIDsOnConn(conn, legacy)
		return callbackErr
	})
	if callbackErr != nil {
		return callbackErr
	}
	return connectionErr
}

func migrateO031UserIDsOnConn(db *gorm.DB, legacy []O031LegacyUser) (err error) {
	connPool := db.Statement.ConnPool
	db = db.Session(&gorm.Session{NewDB: true})
	db.Statement.ConnPool = connPool
	if err := verifyO023RedisTopology(context.Background()); err != nil {
		return err
	}
	status, err := O031MigrationStatus(db)
	if err != nil {
		return err
	}
	if status == "ALREADY_MIGRATED" {
		return nil
	}
	if status == "CACHE_CLEANUP_PENDING" {
		return finalizeO031CacheCleanup(db)
	}
	var users []User
	if err := db.Unscoped().Select("id", "username", "created_at").Find(&users).Error; err != nil {
		return err
	}
	mappings, err := buildO031Mappings(users, legacy)
	if err != nil {
		return err
	}
	oldIDs := make([]int, 0, len(mappings))
	newIDs := make([]int, 0, len(mappings))
	mapping := make(map[int]int, len(mappings))
	for _, item := range mappings {
		oldIDs = append(oldIDs, item.OldID)
		newIDs = append(newIDs, item.NewID)
		mapping[item.OldID] = item.NewID
	}
	apiBefore, err := o023APISummary(db)
	if err != nil {
		return err
	}
	cleanupPlan, err := buildO031CacheCleanupPlan(db, oldIDs, newIDs, apiBefore)
	if err != nil {
		return err
	}
	persistedO023Value, present, err := o023OptionValue(db, O023InvariantHash)
	if err != nil || !present {
		if err != nil {
			return err
		}
		return fmt.Errorf("O-023 invariant is missing")
	}
	persistedO023, err := o023DecodePersistedInvariant(persistedO023Value)
	if err != nil {
		return err
	}
	boundaries := make(map[string]int64, len(persistedO023.HistoricalWaffo))
	for table, snapshot := range persistedO023.HistoricalWaffo {
		boundaries[table] = snapshot.Boundary
	}
	before, err := o023InvariantSnapshotForBoundaries(db, mapping, boundaries)
	if err != nil {
		return err
	}
	var originalForeignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&originalForeignKeys).Error; err != nil {
		return err
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		return err
	}
	defer func() {
		value := "OFF"
		if originalForeignKeys == 1 {
			value = "ON"
		}
		if restoreErr := db.Exec("PRAGMA foreign_keys = " + value).Error; err == nil && restoreErr != nil {
			err = restoreErr
		}
	}()
	if err := db.Exec("BEGIN IMMEDIATE").Error; err != nil {
		return err
	}
	tx := db.Session(&gorm.Session{SkipDefaultTransaction: true})
	committed := false
	defer func() {
		if !committed {
			_ = tx.Exec("ROLLBACK").Error
		}
	}()
	if err := tx.Exec("PRAGMA defer_foreign_keys = ON").Error; err != nil {
		return err
	}
	if err := tx.Exec("CREATE TEMP TABLE temp_o023_user_ids (old_id INTEGER PRIMARY KEY, new_id INTEGER NOT NULL UNIQUE, internal_id INTEGER NOT NULL UNIQUE)").Error; err != nil {
		return err
	}
	for _, item := range mappings {
		if err := tx.Exec("INSERT INTO temp_o023_user_ids(old_id, new_id, internal_id) VALUES (?, ?, ?)", item.OldID, item.NewID, item.InternalID).Error; err != nil {
			return err
		}
	}
	if err := migrateO023StructuredReferences(tx); err != nil {
		return err
	}
	for table, columns := range o023DirectUserReferences {
		for _, column := range columns {
			if table == "users" && column == "inviter_id" {
				continue
			}
			if err := migrateO023References(tx, table, column); err != nil {
				return err
			}
		}
	}
	if err := tx.Exec("UPDATE users SET id = (SELECT new_id FROM temp_o023_user_ids WHERE old_id = users.id), internal_id = (SELECT internal_id FROM temp_o023_user_ids WHERE old_id = users.id)").Error; err != nil {
		return err
	}
	if err := tx.Exec("UPDATE users AS target SET inviter_id = (SELECT map.new_id FROM temp_o023_user_ids AS map WHERE map.old_id = target.inviter_id) WHERE target.inviter_id != 0").Error; err != nil {
		return err
	}
	if err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_internal_id ON users(internal_id)").Error; err != nil {
		return err
	}
	if exists, err := sqliteTableExists(tx, "user_sessions"); err != nil {
		return err
	} else if exists {
		if err := tx.Exec("UPDATE user_sessions SET status = 'revoked', revoked_at = strftime('%s','now') WHERE status != 'revoked'").Error; err != nil {
			return err
		}
	}
	after, err := o023InvariantSnapshotForBoundaries(tx, nil, boundaries)
	if err != nil {
		return err
	}
	if err := compareO023InvariantSnapshots(before, after); err != nil {
		return err
	}
	if err := VerifyO031Invariants(tx); err != nil {
		return err
	}
	invariant, err := o031CurrentInvariant(tx)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(invariant)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	if err := setO023Option(tx, O031InvariantHash, hex.EncodeToString(digest[:])); err != nil {
		return err
	}
	if err := setO023Option(tx, O031MigrationVersion, "1"); err != nil {
		return err
	}
	encodedCleanupPlan, err := json.Marshal(cleanupPlan)
	if err != nil {
		return err
	}
	if err := setO023Option(tx, O031CleanupPlan, string(encodedCleanupPlan)); err != nil {
		return err
	}
	if err := setO023Option(tx, O031CleanupState, O031CleanupPending); err != nil {
		return err
	}
	if err := tx.Exec("DROP TABLE temp_o023_user_ids").Error; err != nil {
		return err
	}
	if err := tx.Exec("COMMIT").Error; err != nil {
		return err
	}
	committed = true
	return finalizeO031CacheCleanup(db)
}

func VerifyO031Invariants(db *gorm.DB) error {
	if err := AuditO031PreparedSchema(db); err != nil {
		return err
	}
	var total, valid, internal, distinctInternal int64
	if err := db.Unscoped().Model(&User{}).Count(&total).Error; err != nil {
		return err
	}
	if err := db.Unscoped().Model(&User{}).Where("id >= ? AND id <= ?", UserIDMin, UserIDMax).Count(&valid).Error; err != nil {
		return err
	}
	if valid != total {
		return fmt.Errorf("O-031 invalid six-digit user IDs: %d", total-valid)
	}
	if err := db.Unscoped().Model(&User{}).Where("internal_id > 0").Count(&internal).Error; err != nil {
		return err
	}
	if err := db.Unscoped().Model(&User{}).Distinct("internal_id").Where("internal_id > 0").Count(&distinctInternal).Error; err != nil {
		return err
	}
	if internal != total || distinctInternal != total {
		return fmt.Errorf("O-031 internal ID invariant failed")
	}
	indexReady, err := o031InternalIDIndexReady(db)
	if err != nil {
		return err
	}
	if !indexReady {
		return fmt.Errorf("O-031 internal ID unique index is missing")
	}
	var orphan int64
	for table, columns := range o023DirectUserReferences {
		exists, err := sqliteTableExists(db, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		for _, column := range columns {
			ok, err := sqliteColumnExists(db, table, column)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			query := "SELECT count(*) FROM \"" + table + "\" AS ref WHERE ref.\"" + column + "\" != 0 AND ref.\"" + column + "\" IS NOT NULL AND NOT EXISTS (SELECT 1 FROM users AS target WHERE target.id = ref.\"" + column + "\")"
			if err := db.Raw(query).Scan(&orphan).Error; err != nil {
				return err
			}
			if orphan != 0 {
				return fmt.Errorf("O-031 orphan reference %s.%s", table, column)
			}
		}
	}
	if err := verifyO031StructuredReferences(db); err != nil {
		return err
	}
	if err := verifyO023BusinessAggregates(db); err != nil {
		return err
	}
	return verifyO023SQLiteIntegrity(db)
}

func verifyO031StructuredReferences(db *gorm.DB) error {
	checkID := func(raw string) error {
		id, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || int64(id) < UserIDMin || int64(id) > UserIDMax {
			return ErrUnknownUserReference
		}
		var count int64
		if err := db.Unscoped().Model(&User{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrUnknownUserReference
		}
		return nil
	}
	if exists, err := sqliteTableExists(db, "casbin_rule"); err != nil {
		return err
	} else if exists {
		var values []string
		if err := db.Table("casbin_rule").Where("v0 LIKE ?", "user:%").Pluck("v0", &values).Error; err != nil {
			return err
		}
		for _, value := range values {
			if err := checkID(strings.TrimPrefix(value, "user:")); err != nil {
				return fmt.Errorf("invalid O-031 Casbin user reference")
			}
		}
	}
	if exists, err := sqliteTableExists(db, "options"); err != nil {
		return err
	} else if exists {
		var option Option
		err := db.Where("key = ?", "payment_setting.compliance_confirmed_by").First(&option).Error
		if err == nil && strings.TrimSpace(option.Value) != "" {
			if err := checkID(option.Value); err != nil {
				return fmt.Errorf("invalid O-031 compliance user reference")
			}
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	for _, table := range []string{"payment_campaign_claims", "payment_campaign_participants"} {
		for _, column := range []string{"claim_key", "participant_key", "email_participant_key"} {
			ok, err := sqliteColumnExists(db, table, column)
			if err != nil || !ok {
				if err != nil {
					return err
				}
				continue
			}
			var values []string
			if err := db.Table(table).Where(column+" IS NOT NULL AND "+column+" <> ''").Pluck(column, &values).Error; err != nil {
				return err
			}
			for _, value := range values {
				parts := strings.Split(value, ":")
				for index := 0; index+1 < len(parts); index++ {
					if parts[index] == "user" {
						if err := checkID(parts[index+1]); err != nil {
							return fmt.Errorf("invalid O-031 activity user reference")
						}
					}
				}
			}
		}
	}
	return nil
}

func o031CurrentInvariant(db *gorm.DB) (o031Invariant, error) {
	var result o031Invariant
	if err := db.Unscoped().Model(&User{}).Count(&result.Users).Error; err != nil {
		return result, err
	}
	if err := db.Unscoped().Model(&User{}).Where("internal_id > 0").Count(&result.InternalIDs).Error; err != nil {
		return result, err
	}
	apiDigest, err := o023APISummary(db)
	if err != nil {
		return result, err
	}
	result.APIKeyDigest = apiDigest
	snapshot, err := o023InvariantSnapshotFor(db, nil)
	if err != nil {
		return result, err
	}
	businessDigest, err := o023InvariantDigest(snapshot)
	if err != nil {
		return result, err
	}
	result.BusinessDigest = businessDigest
	return result, nil
}
