package migration

import (
	"fmt"
	"os"
	"strings"

	"github.com/felixbennettio/s-ui-next/config"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func MigrateDb() error {
	// void running on first install
	path := config.GetDBPath()
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		println("Database not found")
		return nil
	}
	if err != nil {
		return err
	}

	db, err := gorm.Open(sqlite.Open(path))
	if err != nil {
		return err
	}
	defer func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}()
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback().Error
		}
	}()
	currentVersion := config.GetVersion()
	dbVersion := ""
	if err := tx.Raw("SELECT value FROM settings WHERE key = ?", "version").Scan(&dbVersion).Error; err != nil {
		return fmt.Errorf("read database version: %w", err)
	}
	fmt.Println("Current version:", currentVersion, "\nDatabase version:", dbVersion)

	if currentVersion == dbVersion {
		fmt.Println("Database is up to date, no need to migrate")
		return nil
	}

	fmt.Println("Start migrating database...")
	if dbVersion != "" &&
		!isVersionSeries(dbVersion, "1.1") &&
		!isVersionSeries(dbVersion, "1.2") &&
		!isVersionSeries(dbVersion, "1.3") &&
		!isVersionSeries(dbVersion, "1.4") {
		return fmt.Errorf("unsupported database version %q", dbVersion)
	}

	// Before 1.2
	if dbVersion == "" {
		if err := runMigrationStep("1.1", tx, to1_1); err != nil {
			return err
		}
		if err := runMigrationStep("1.2", tx, to1_2); err != nil {
			return err
		}
		dbVersion = "1.2"
	} else if isVersionSeries(dbVersion, "1.1") {
		if err := runMigrationStep("1.2", tx, to1_2); err != nil {
			return err
		}
		dbVersion = "1.2"
	}

	// Before 1.3
	if isVersionSeries(dbVersion, "1.2") {
		if err := runMigrationStep("1.3", tx, to1_3); err != nil {
			return err
		}
	}

	// Set version
	result := tx.Exec("UPDATE settings SET value = ? WHERE key = ?", currentVersion, "version")
	if result.Error != nil {
		return fmt.Errorf("update database version: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		if err := tx.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "version", currentVersion).Error; err != nil {
			return fmt.Errorf("insert database version: %w", err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("commit database migration: %w", err)
	}
	committed = true
	fmt.Println("Migration done!")
	return nil
}

func isVersionSeries(version, series string) bool {
	version = strings.TrimSpace(version)
	return version == series || strings.HasPrefix(version, series+".")
}

func runMigrationStep(name string, tx *gorm.DB, step func(*gorm.DB) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("migration to %s failed: %v", name, recovered)
		}
	}()
	if err := step(tx); err != nil {
		return fmt.Errorf("migration to %s failed: %w", name, err)
	}
	return nil
}
