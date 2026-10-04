package database

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/felixbennettio/s-ui-next/cmd/migration"
	"github.com/felixbennettio/s-ui-next/config"
	"github.com/felixbennettio/s-ui-next/database/model"
	"github.com/felixbennettio/s-ui-next/logger"
	"github.com/felixbennettio/s-ui-next/util/common"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var importDBMu sync.Mutex

func GetDb(exclude string) ([]byte, error) {
	exclude_changes, exclude_stats := false, false
	for _, table := range strings.Split(exclude, ",") {
		if table == "changes" {
			exclude_changes = true
		} else if table == "stats" {
			exclude_stats = true
		}
	}

	dir := filepath.Dir(config.GetDBPath())
	tempFile, err := os.CreateTemp(dir, ".s-ui-next-backup-*.db")
	if err != nil {
		return nil, err
	}
	dbPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		return nil, err
	}
	defer os.Remove(dbPath)
	defer removeSQLiteSidecars(dbPath)

	backupDb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	defer func() {
		if sqlDB, e := backupDb.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}()
	err = backupDb.AutoMigrate(
		&model.Setting{},
		&model.Tls{},
		&model.Inbound{},
		&model.Outbound{},
		&model.Endpoint{},
		&model.ManagedRouteRule{},
		&model.Service{},
		&model.User{},
		&model.PasskeyCredential{},
		&model.Tokens{},
		&model.Stats{},
		&model.Client{},
		&model.Changes{},
	)
	if err != nil {
		return nil, err
	}

	var settings []model.Setting
	var tls []model.Tls
	var inbound []model.Inbound
	var outbound []model.Outbound
	var endpoint []model.Endpoint
	var managedRoutes []model.ManagedRouteRule
	var services []model.Service
	var users []model.User
	var passkeys []model.PasskeyCredential
	var tokens []model.Tokens
	var clients []model.Client
	var stats []model.Stats
	var changes []model.Changes

	// Perform scans and handle errors
	if err := db.Model(&model.Setting{}).Scan(&settings).Error; err != nil {
		return nil, err
	} else if len(settings) > 0 {
		if err := backupDb.Save(settings).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.Tls{}).Scan(&tls).Error; err != nil {
		return nil, err
	} else if len(tls) > 0 {
		if err := backupDb.Save(tls).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.Inbound{}).Scan(&inbound).Error; err != nil {
		return nil, err
	} else if len(inbound) > 0 {
		if err := backupDb.Save(inbound).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.Outbound{}).Scan(&outbound).Error; err != nil {
		return nil, err
	} else if len(outbound) > 0 {
		if err := backupDb.Save(outbound).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.Endpoint{}).Scan(&endpoint).Error; err != nil {
		return nil, err
	} else if len(endpoint) > 0 {
		if err := backupDb.Save(endpoint).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.ManagedRouteRule{}).Scan(&managedRoutes).Error; err != nil {
		return nil, err
	} else if len(managedRoutes) > 0 {
		if err := backupDb.Save(managedRoutes).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.Service{}).Scan(&services).Error; err != nil {
		return nil, err
	} else if len(services) > 0 {
		if err := backupDb.Save(services).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.User{}).Scan(&users).Error; err != nil {
		return nil, err
	} else if len(users) > 0 {
		if err := backupDb.Save(users).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.PasskeyCredential{}).Scan(&passkeys).Error; err != nil {
		return nil, err
	} else if len(passkeys) > 0 {
		if err := backupDb.Save(passkeys).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.Tokens{}).Scan(&tokens).Error; err != nil {
		return nil, err
	} else if len(tokens) > 0 {
		if err := backupDb.Save(tokens).Error; err != nil {
			return nil, err
		}
	}
	if err := db.Model(&model.Client{}).Scan(&clients).Error; err != nil {
		return nil, err
	} else if len(clients) > 0 {
		if err := backupDb.Save(clients).Error; err != nil {
			return nil, err
		}
	}

	if !exclude_stats {
		if err := db.Model(&model.Stats{}).Scan(&stats).Error; err != nil {
			return nil, err
		}
		if len(stats) > 0 {
			if err := backupDb.Save(stats).Error; err != nil {
				return nil, err
			}
		}
	}
	if !exclude_changes {
		if err := db.Model(&model.Changes{}).Scan(&changes).Error; err != nil {
			return nil, err
		}
		if len(changes) > 0 {
			if err := backupDb.Save(changes).Error; err != nil {
				return nil, err
			}
		}
	}

	// Update WAL
	err = backupDb.Exec("PRAGMA wal_checkpoint;").Error
	if err != nil {
		return nil, err
	}

	bdb, _ := backupDb.DB()
	bdb.Close()

	// Open the file for reading
	file, err := os.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the file contents
	fileContents, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return fileContents, nil
}

func ImportDB(file multipart.File) error {
	importDBMu.Lock()
	defer importDBMu.Unlock()

	// Check if the file is a SQLite database
	isValidDb, err := IsSQLiteDB(file)
	if err != nil {
		return common.NewErrorf("Error checking db file format: %v", err)
	}
	if !isValidDb {
		return common.NewError("Invalid db file format")
	}

	// Reset the file reader to the beginning
	_, err = file.Seek(0, 0)
	if err != nil {
		return common.NewErrorf("Error resetting file reader: %v", err)
	}

	dbPath := config.GetDBPath()
	tempFile, err := os.CreateTemp(filepath.Dir(dbPath), ".s-ui-next-import-*.db")
	if err != nil {
		return common.NewErrorf("Error creating temporary db file: %v", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	defer removeSQLiteSidecars(tempPath)
	if err := tempFile.Chmod(0o600); err != nil {
		tempFile.Close()
		return common.NewErrorf("Error securing temporary db file: %v", err)
	}

	if _, err = io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		return common.NewErrorf("Error saving db: %v", err)
	}
	if err = tempFile.Sync(); err != nil {
		tempFile.Close()
		return common.NewErrorf("Error syncing db: %v", err)
	}
	if err = tempFile.Close(); err != nil {
		return common.NewErrorf("Error closing temporary db: %v", err)
	}
	if err = validateSQLiteDatabase(tempPath); err != nil {
		return common.NewErrorf("Error checking db: %v", err)
	}

	fallbackPath := fmt.Sprintf("%s.backup", dbPath)
	if err := os.Remove(fallbackPath); err != nil && !os.IsNotExist(err) {
		return common.NewErrorf("Error removing existing fallback db file: %v", err)
	}
	if err := removeSQLiteSidecars(fallbackPath); err != nil {
		return common.NewErrorf("Error removing existing fallback db files: %v", err)
	}
	if db != nil {
		if err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
			return common.NewErrorf("Error checkpointing current db: %v", err)
		}
	}
	if err := closeCurrentDatabase(); err != nil {
		_ = InitDB(dbPath)
		return common.NewErrorf("Error closing current db: %v", err)
	}
	if err := removeSQLiteSidecars(dbPath); err != nil {
		_ = InitDB(dbPath)
		return common.NewErrorf("Error removing current db sidecar files: %v", err)
	}

	err = os.Rename(dbPath, fallbackPath)
	if err != nil {
		_ = InitDB(dbPath)
		return common.NewErrorf("Error backing up temporary db file: %v", err)
	}
	err = os.Rename(tempPath, dbPath)
	if err != nil {
		return restoreDatabaseBackup(dbPath, fallbackPath, common.NewErrorf("Error moving db file: %v", err))
	}

	if err = migration.MigrateDb(); err != nil {
		return restoreDatabaseBackup(dbPath, fallbackPath, common.NewErrorf("Error migrating db: %v", err))
	}
	if err = InitDB(dbPath); err != nil {
		return restoreDatabaseBackup(dbPath, fallbackPath, common.NewErrorf("Error opening imported db: %v", err))
	}
	if err := os.Remove(fallbackPath); err != nil && !os.IsNotExist(err) {
		logger.Warning("unable to remove database import backup: ", err)
	}
	_ = removeSQLiteSidecars(fallbackPath)

	if err = SendSighup(); err != nil {
		return common.NewErrorf("Error restarting app: %v", err)
	}

	return nil
}

func validateSQLiteDatabase(path string) error {
	dsn := (&url.URL{Scheme: "file", Path: path}).String() + "?mode=ro"
	validationDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, err := validationDB.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	var checks []string
	if err := validationDB.Raw("PRAGMA quick_check").Scan(&checks).Error; err != nil {
		return err
	}
	if len(checks) == 0 {
		return common.NewError("SQLite integrity check returned no result")
	}
	for _, check := range checks {
		if !strings.EqualFold(strings.TrimSpace(check), "ok") {
			return common.NewError("SQLite integrity check failed: ", check)
		}
	}
	var settingsTableCount int64
	if err := validationDB.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'settings'").Scan(&settingsTableCount).Error; err != nil {
		return err
	}
	if settingsTableCount != 1 {
		return common.NewError("database does not contain an S-UI settings table")
	}
	return nil
}

func closeCurrentDatabase() error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func removeSQLiteSidecars(path string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func restoreDatabaseBackup(dbPath, fallbackPath string, cause error) error {
	_ = closeCurrentDatabase()
	_ = os.Remove(dbPath)
	_ = removeSQLiteSidecars(dbPath)
	if err := os.Rename(fallbackPath, dbPath); err != nil {
		return common.NewErrorf("%v; restoring the previous database failed: %v", cause, err)
	}
	if err := InitDB(dbPath); err != nil {
		return common.NewErrorf("%v; reopening the previous database failed: %v", cause, err)
	}
	return cause
}

func IsSQLiteDB(file io.Reader) (bool, error) {
	signature := []byte("SQLite format 3\x00")
	buf := make([]byte, len(signature))
	_, err := io.ReadFull(file, buf)
	if err != nil {
		return false, err
	}
	return bytes.Equal(buf, signature), nil
}

func SendSighup() error {
	// Get the current process
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}

	// Send SIGHUP to the current process
	go func() {
		time.Sleep(3 * time.Second)
		var signalErr error
		if runtime.GOOS == "windows" {
			signalErr = process.Kill()
		} else {
			signalErr = process.Signal(syscall.SIGHUP)
		}
		if signalErr != nil {
			logger.Error("send signal SIGHUP failed:", signalErr)
		}
	}()
	return nil
}
