package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openDatabase(path string) (*gorm.DB, error) {
	if path == "" {
		return nil, errors.New("database path is required")
	}
	if os.Getenv("LOG_SQL_DSN") != "" {
		return nil, errors.New("LOG_SQL_DSN must be empty")
	}
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{PrepareStmt: false})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	return db, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func loadLegacyUsers(path, expectedSHA256 string) ([]model.O031LegacyUser, error) {
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	decoded, err := hex.DecodeString(expectedSHA256)
	if err != nil || len(decoded) != sha256.Size {
		return nil, errors.New("valid -legacy-sha256 is required")
	}
	before, err := fileSHA256(path)
	if err != nil {
		return nil, err
	}
	if before != expectedSHA256 {
		return nil, errors.New("legacy database SHA-256 mismatch before read")
	}
	db, err := gorm.Open(sqlite.Open("file:"+path+"?mode=ro"), &gorm.Config{PrepareStmt: false})
	if err != nil {
		return nil, err
	}
	var users []model.O031LegacyUser
	if err := db.Raw("SELECT id AS internal_id, username, created_at FROM users ORDER BY id").Scan(&users).Error; err != nil {
		return nil, err
	}
	after, err := fileSHA256(path)
	if err != nil {
		return nil, err
	}
	if after != expectedSHA256 {
		return nil, errors.New("legacy database SHA-256 mismatch after read")
	}
	return users, nil
}

func runMigration(db *gorm.DB, legacyPath, legacySHA256 string) error {
	status, err := model.O031MigrationStatus(db)
	if err != nil {
		return err
	}
	var legacy []model.O031LegacyUser
	if status == "READY" {
		if legacyPath == "" {
			return errors.New("-legacy-db is required for the first O-031 migration")
		}
		legacy, err = loadLegacyUsers(legacyPath, legacySHA256)
		if err != nil {
			return err
		}
	}
	if err := common.InitRedisClient(); err != nil {
		return err
	}
	if common.RDB != nil {
		defer common.RDB.Close()
	}
	return model.MigrateO031UserIDs(db, legacy)
}

func main() {
	dbPath := flag.String("db", "", "current isolated SQLite database path")
	legacyPath := flag.String("legacy-db", "", "read-only O-023 pre-migration SQLite backup")
	legacySHA256 := flag.String("legacy-sha256", "", "expected SHA-256 of the O-023 pre-migration backup")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: o031-user-id-migrate -db PATH [-legacy-db PATH -legacy-sha256 HEX] audit|prepare-schema|migrate|verify")
		os.Exit(2)
	}
	db, err := openDatabase(*dbPath)
	if err == nil {
		switch flag.Arg(0) {
		case "audit":
			err = model.AuditO031PreparedSchema(db)
		case "prepare-schema":
			err = model.PrepareO031Schema(db)
		case "migrate":
			err = runMigration(db, *legacyPath, *legacySHA256)
		case "verify":
			var status string
			status, err = model.O031MigrationStatus(db)
			if err == nil {
				fmt.Println(status)
			}
		default:
			err = fmt.Errorf("unknown mode %q", flag.Arg(0))
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
