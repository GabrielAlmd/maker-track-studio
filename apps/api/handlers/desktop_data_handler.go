package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	desktopDataFormatVersion = 1
	// RecoveryBackupDirectoryKey is supplied only by the owning runtime.
	RecoveryBackupDirectoryKey = "maker-track-recovery-backup-directory"
	desktopDataSchemaVersion   = 4
	desktopDataMaxImportSize   = 64 << 20
)

type desktopDataManifest struct {
	FormatVersion    int       `json:"format_version"`
	SchemaVersion    int       `json:"schema_version"`
	ExportedAt       time.Time `json:"exported_at"`
	Driver           string    `json:"driver"`
	MigrationEntries []string  `json:"migration_entries,omitempty"`
}

func GetDesktopDataExport(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !desktopDataAvailable(db) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Desktop data export is only available in desktop SQLite mode"})
			return
		}

		archiveBytes, filename, err := buildDesktopDataArchive(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to export desktop data"})
			return
		}

		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		c.Data(http.StatusOK, "application/zip", archiveBytes)
	}
}

func ImportDesktopData(db *gorm.DB, installed *modules.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !desktopDataAvailable(db) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Desktop data import is only available in desktop SQLite mode"})
			return
		}

		fileHeader, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Backup file is required"})
			return
		}

		if fileHeader.Size > desktopDataMaxImportSize {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Backup file is too large"})
			return
		}

		file, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read backup file"})
			return
		}
		defer file.Close()

		payload, err := io.ReadAll(io.LimitReader(file, desktopDataMaxImportSize+1))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read backup file"})
			return
		}
		if int64(len(payload)) > desktopDataMaxImportSize {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Backup file is too large"})
			return
		}

		manifest, snapshotBytes, err := readDesktopDataArchive(payload)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := validateDesktopDataManifest(manifest); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		snapshotPath, err := writeImportedSnapshot(snapshotBytes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to stage imported backup"})
			return
		}
		defer os.Remove(snapshotPath)

		sourceDB, err := gorm.Open(sqlite.Open(snapshotPath), &gorm.Config{Logger: db.Logger})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Backup file contains an invalid database"})
			return
		}
		sourceDB = sourceDB.WithContext(c.Request.Context())
		sourceSQLDB, err := sourceDB.DB()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open backup database"})
			return
		}
		// Close before removing the staged file, including on Windows.
		defer sourceSQLDB.Close()
		var integrity string
		if err := sourceDB.Raw("PRAGMA quick_check").Scan(&integrity).Error; err != nil || integrity != "ok" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Backup database is invalid (integrity check failed)"})
			return
		}
		if err := installed.CheckBackupTables(sourceDB); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := installed.CheckBackupTables(db); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}

		if manifest.SchemaVersion >= 2 && !sourceDB.Migrator().HasTable(&models.Printer{}) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Backup is missing printer data"})
			return
		}
		if manifest.SchemaVersion >= 3 && (!sourceDB.Migrator().HasColumn(&models.Printer{}, "cost_config") ||
			!sourceDB.Migrator().HasColumn(&models.PrintJob{}, "printer_id") ||
			!sourceDB.Migrator().HasColumn(&models.PrintJob{}, "printer_cost")) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Backup is missing printer cost data"})
			return
		}
		backupData, err := installed.ValidateBackups(sourceDB, manifest.SchemaVersion)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var recoveryErr error
		beforeReplace := func() error {
			if directory, ok := c.Get(RecoveryBackupDirectoryKey); ok {
				_, recoveryErr = SaveDesktopDataBackup(db, directory.(string), 5)
			}
			return recoveryErr
		}
		if err := importDesktopDataSnapshot(db, sourceDB, installed, backupData, beforeReplace); err != nil {
			message := "Failed to import desktop data"
			if recoveryErr != nil {
				message = "Could not save recovery backup; restore cancelled"
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": message})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":          "Desktop data imported successfully",
			"schema_version":   manifest.SchemaVersion,
			"migration_needed": false,
		})
	}
}

func desktopDataAvailable(db *gorm.DB) bool {
	return db.Dialector.Name() == "sqlite"
}

func buildDesktopDataArchive(db *gorm.DB) ([]byte, string, error) {
	snapshotPath, err := createSQLiteSnapshot(db)
	if err != nil {
		return nil, "", err
	}
	defer os.Remove(snapshotPath)

	snapshotDB, err := gorm.Open(sqlite.Open(snapshotPath), &gorm.Config{Logger: db.Logger})
	if err != nil {
		return nil, "", err
	}

	snapshotSQLDB, err := snapshotDB.DB()
	if err != nil {
		return nil, "", err
	}
	defer snapshotSQLDB.Close()
	if err := removeUnmanagedAppConfig(snapshotDB); err != nil {
		return nil, "", err
	}
	if err := snapshotSQLDB.Close(); err != nil {
		return nil, "", err
	}

	snapshotBytes, err := os.ReadFile(snapshotPath)
	if err != nil {
		return nil, "", err
	}

	manifest, err := buildDesktopDataManifest(db)
	if err != nil {
		return nil, "", err
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, "", err
	}

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	if err := writeZipEntry(writer, "manifest.json", manifestBytes); err != nil {
		return nil, "", err
	}
	if err := writeZipEntry(writer, "app.db", snapshotBytes); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("3d-printing-cost-tracker-export-%s.zip", time.Now().UTC().Format("20060102-150405"))
	return archive.Bytes(), filename, nil
}

func writeZipEntry(writer *zip.Writer, name string, data []byte) error {
	entry, err := writer.Create(name)
	if err != nil {
		return err
	}
	_, err = entry.Write(data)
	return err
}

func createSQLiteSnapshot(db *gorm.DB) (string, error) {
	tempFile, err := os.CreateTemp("", "desktop-export-*.db")
	if err != nil {
		return "", err
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	// VACUUM INTO accepts an existing empty file. Keep its private 0600 mode.

	escapedPath := strings.ReplaceAll(tempPath, "'", "''")
	if err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escapedPath)).Error; err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}

	return tempPath, nil
}

func buildDesktopDataManifest(db *gorm.DB) (*desktopDataManifest, error) {
	migrationEntries, err := loadMigrationEntries(db)
	if err != nil {
		return nil, err
	}

	return &desktopDataManifest{
		FormatVersion:    desktopDataFormatVersion,
		SchemaVersion:    desktopDataSchemaVersion,
		ExportedAt:       time.Now().UTC(),
		Driver:           db.Dialector.Name(),
		MigrationEntries: migrationEntries,
	}, nil
}

func loadMigrationEntries(db *gorm.DB) ([]string, error) {
	if !db.Migrator().HasTable("schema_migrations") {
		return nil, nil
	}

	var entries []string
	if err := db.Table("schema_migrations").Order("version asc").Pluck("version", &entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

func readDesktopDataArchive(payload []byte) (*desktopDataManifest, []byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, nil, errors.New("Backup file must be a valid ZIP archive")
	}

	var (
		manifest      desktopDataManifest
		manifestFound bool
		snapshotBytes []byte
	)

	seen := map[string]bool{}
	for _, file := range reader.File {
		if seen[file.Name] {
			return nil, nil, errors.New("Backup archive contains duplicate files")
		}
		seen[file.Name] = true
		switch file.Name {
		case "manifest.json":
			body, err := readZipFile(file)
			if err != nil {
				return nil, nil, errors.New("Failed to read backup manifest")
			}
			if err := json.Unmarshal(body, &manifest); err != nil {
				return nil, nil, errors.New("Backup manifest is invalid")
			}
			manifestFound = true
		case "app.db":
			body, err := readZipFile(file)
			if err != nil {
				return nil, nil, errors.New("Failed to read backup database")
			}
			snapshotBytes = body
		default:
			return nil, nil, errors.New("Backup archive contains unsupported files")
		}
	}

	if !manifestFound {
		return nil, nil, errors.New("Backup manifest is missing")
	}
	if len(snapshotBytes) == 0 {
		return nil, nil, errors.New("Backup database is missing")
	}

	return &manifest, snapshotBytes, nil
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	body, err := io.ReadAll(io.LimitReader(rc, desktopDataMaxImportSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > desktopDataMaxImportSize {
		return nil, errors.New("Backup entry is too large")
	}
	return body, nil
}

func validateDesktopDataManifest(manifest *desktopDataManifest) error {
	if manifest.FormatVersion != desktopDataFormatVersion {
		return fmt.Errorf("Unsupported backup format version: %d", manifest.FormatVersion)
	}
	if manifest.Driver != "sqlite" {
		return errors.New("Backup was not generated from a SQLite desktop database")
	}
	if manifest.SchemaVersion < 1 || manifest.SchemaVersion > desktopDataSchemaVersion {
		return fmt.Errorf("Backup schema version %d is not compatible with this build", manifest.SchemaVersion)
	}
	return nil
}

func writeImportedSnapshot(snapshotBytes []byte) (string, error) {
	tempFile, err := os.CreateTemp("", "desktop-import-*.db")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	if _, err := tempFile.Write(snapshotBytes); err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}

func importDesktopDataSnapshot(targetDB, sourceDB *gorm.DB, installed *modules.Registry, backupData map[string]any, beforeReplace func() error) error {
	var (
		appConfigs        []models.AppConfig
		filaments         []models.Filament
		modelsList        []models.Model
		filamentPurchases []models.FilamentPurchase
		printJobs         []models.PrintJob
		printJobFilaments []models.PrintJobFilament
		printers          []models.Printer
	)

	// Version 1 snapshots have no printers table. They are upgraded inside the
	// target transaction with one migrated printer while historical jobs remain
	// unassigned and continue to use their imported global cost configuration.
	legacySnapshot := !sourceDB.Migrator().HasTable(&models.Printer{})
	if !legacySnapshot {
		if err := sourceDB.Order("created_at asc").Find(&printers).Error; err != nil {
			return err
		}
	}
	activePrinters, defaultPrinters := 0, 0
	for _, printer := range printers {
		if err := models.ValidatePrinter(printer); err != nil {
			return err
		}
		if !printer.IsArchived {
			activePrinters++
		}
		if printer.IsDefault {
			defaultPrinters++
		}
	}
	if (activePrinters > 0 && defaultPrinters != 1) || (activePrinters == 0 && defaultPrinters != 0) {
		return errors.New("Backup has an invalid default printer configuration")
	}

	if err := sourceDB.Order("key asc").Find(&appConfigs).Error; err != nil {
		return err
	}
	managedConfigs := appConfigs[:0]
	for _, config := range appConfigs {
		if isManagedAppConfigKey(config.Key) {
			managedConfigs = append(managedConfigs, config)
		}
	}
	appConfigs = managedConfigs
	if err := sourceDB.Order("created_at asc").Find(&filaments).Error; err != nil {
		return err
	}
	if err := sourceDB.Order("created_at asc").Find(&modelsList).Error; err != nil {
		return err
	}
	if err := sourceDB.Order("created_at asc").Find(&filamentPurchases).Error; err != nil {
		return err
	}
	if err := sourceDB.Order("created_at asc").Find(&printJobs).Error; err != nil {
		return err
	}
	printerIDs := make(map[string]bool, len(printers))
	for _, printer := range printers {
		printerIDs[printer.ID.String()] = true
	}
	for _, job := range printJobs {
		if job.PrinterID == nil {
			if job.PrinterCost != nil {
				return errors.New("Backup contains an unassigned printer cost snapshot")
			}
			continue
		}
		if !printerIDs[job.PrinterID.String()] {
			return errors.New("Backup references a missing printer")
		}
		if err := models.ValidatePrinterCostSnapshot(job.PrinterCost); err != nil {
			return err
		}
	}
	if err := sourceDB.Order("created_at asc").Find(&printJobFilaments).Error; err != nil {
		return err
	}

	// All source rows have been read and validated; take the recovery snapshot
	// immediately before the first destructive operation.
	if err := beforeReplace(); err != nil {
		return err
	}

	// Bound the number of SQL parameters for large backups.
	tx := targetDB.Session(&gorm.Session{CreateBatchSize: 50}).Begin()
	if tx.Error != nil {
		return tx.Error
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	deleteSession := tx.Session(&gorm.Session{AllowGlobalUpdate: true})
	if err := installed.DeleteBackups(tx); err != nil {
		tx.Rollback()
		return err
	}
	for _, model := range []any{
		&models.PrintJobFilament{},
		&models.FilamentPurchase{},
		&models.PrintJob{},
		&models.Printer{},
		&models.Model{},
		&models.Filament{},
		&models.AppConfig{},
	} {
		if err := deleteSession.Delete(model).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := createBatch(tx, &filaments); err != nil {
		tx.Rollback()
		return err
	}
	if err := createBatch(tx, &printers); err != nil {
		tx.Rollback()
		return err
	}
	if err := createBatch(tx, &modelsList); err != nil {
		tx.Rollback()
		return err
	}
	if err := createBatch(tx, &filamentPurchases); err != nil {
		tx.Rollback()
		return err
	}
	// SQLite rejects DEFAULT inside a multi-row VALUES list. GORM emits it
	// when nullable print_time_minutes values are mixed with explicit times.
	if err := createBatch(tx.Session(&gorm.Session{CreateBatchSize: 1}), &printJobs); err != nil {
		tx.Rollback()
		return err
	}
	if err := installed.RestoreBackups(tx, backupData); err != nil {
		tx.Rollback()
		return err
	}
	if err := createBatch(tx, &printJobFilaments); err != nil {
		tx.Rollback()
		return err
	}
	if err := createBatch(tx.Clauses(clause.OnConflict{UpdateAll: true}), &appConfigs); err != nil {
		tx.Rollback()
		return err
	}
	needsPrinterMigration := legacySnapshot
	for _, job := range printJobs {
		if job.PrinterID == nil || job.PrinterCost == nil {
			needsPrinterMigration = true
			break
		}
	}
	if needsPrinterMigration {
		if err := models.MigrateLegacyPrintJobCosts(tx); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

func createBatch[T any](tx *gorm.DB, values *[]T) error {
	if len(*values) == 0 {
		return nil
	}
	return tx.Create(values).Error
}

func removeUnmanagedAppConfig(db *gorm.DB) error {
	var configs []models.AppConfig
	if err := db.Find(&configs).Error; err != nil {
		return err
	}
	for _, config := range configs {
		if !isManagedAppConfigKey(config.Key) {
			if err := db.Delete(&config).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
