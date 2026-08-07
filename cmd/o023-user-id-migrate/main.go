package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openDatabase(path string) (*gorm.DB, error) {
	if path == "" {
		return nil, errors.New("-db is required")
	}
	if strconv.IntSize != 64 {
		return nil, errors.New("O-023 requires a 64-bit build")
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

func main() {
	dbPath := flag.String("db", "", "isolated SQLite database path")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: o023-user-id-migrate -db PATH audit|prepare-schema|migrate|verify")
		os.Exit(2)
	}
	db, err := openDatabase(*dbPath)
	if err == nil {
		switch flag.Arg(0) {
		case "audit":
			err = model.AuditO023Schema(db)
		case "prepare-schema":
			err = model.PrepareO023Schema(db)
		case "migrate":
			err = model.MigrateO023UserIDs(db)
		case "verify":
			var status string
			status, err = model.O023MigrationStatus(db)
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
