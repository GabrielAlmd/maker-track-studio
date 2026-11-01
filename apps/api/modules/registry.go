// Package modules defines the composition contract for domains installed in the
// desktop application. Feature preferences control presentation, not schema or
// data retention: installed modules are always migrated and registered.
package modules

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const CoreVersion = 1

// Module owns its HTTP surface and additive SQLite migration. Migrate receives
// a transaction; returning an error rolls back all changes made by that module.
type Module interface {
	ID() string
	Version() int
	RequiresCoreVersion() int
	EnabledByDefault() bool
	BackupTables() []string
	Migrate(*gorm.DB) error
	RegisterRoutes(*gin.Engine, *gorm.DB)
	ValidateBackup(*gorm.DB, int) (any, error)
	DeleteBackup(*gorm.DB) error
	RestoreBackup(*gorm.DB, any) error
	CheckPrintJobDeletion(*gorm.DB, string) error
}

var ErrPrintJobDeleteConflict = errors.New("print job is used by a module")

type PrintJobDeleteConflict struct{ Message string }

func (err PrintJobDeleteConflict) Error() string    { return err.Message }
func (PrintJobDeleteConflict) Is(target error) bool { return target == ErrPrintJobDeleteConflict }

type Registry struct {
	modules []Module
}

type moduleVersion struct {
	ID      string `gorm:"primaryKey;size:100"`
	Version int    `gorm:"not null"`
}

func (moduleVersion) TableName() string { return "module_versions" }

func (registry *Registry) DefaultFeatures() map[string]bool {
	defaults := make(map[string]bool, len(registry.modules))
	for _, module := range registry.modules {
		defaults[module.ID()] = module.EnabledByDefault()
	}
	return defaults
}

func New(installed ...Module) (*Registry, error) {
	seen := make(map[string]bool, len(installed))
	ownedTables := map[string]string{}
	for _, module := range installed {
		if module == nil {
			return nil, fmt.Errorf("nil module")
		}
		id := module.ID()
		if id == "" || seen[id] || module.Version() < 1 || module.RequiresCoreVersion() != CoreVersion {
			return nil, fmt.Errorf("invalid or incompatible module %q", id)
		}
		seen[id] = true
		for _, table := range module.BackupTables() {
			if table == "" || ownedTables[table] != "" {
				return nil, fmt.Errorf("module %s has invalid or duplicate table %q", id, table)
			}
			ownedTables[table] = id
		}
	}
	return &Registry{modules: append([]Module(nil), installed...)}, nil
}

func (registry *Registry) Migrate(db *gorm.DB) error {
	if err := registry.CheckBackupTables(db); err != nil {
		return fmt.Errorf("initialize modules: %w", err)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&moduleVersion{}); err != nil {
			return fmt.Errorf("initialize module version registry: %w", err)
		}
		for _, module := range registry.modules {
			var recorded moduleVersion
			err := tx.Where("id = ?", module.ID()).First(&recorded).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("initialize module %s v%d: %w", module.ID(), module.Version(), err)
			}
			if err == nil && recorded.Version > module.Version() {
				return fmt.Errorf("initialize module %s v%d: database requires module version %d, build supports %d", module.ID(), module.Version(), recorded.Version, module.Version())
			}
			if err := module.Migrate(tx); err != nil {
				return fmt.Errorf("initialize module %s v%d: %w", module.ID(), module.Version(), err)
			}
			if err := tx.Save(&moduleVersion{ID: module.ID(), Version: module.Version()}).Error; err != nil {
				return fmt.Errorf("record module %s v%d: %w", module.ID(), module.Version(), err)
			}
		}
		return nil
	})
}

// Legacy module tables predate module_versions. This compatibility map is only
// used to recognize backups and databases written by those older builds.
var legacyModuleTables = map[string]string{
	"sales_channels": "sales_orders", "sales_orders": "sales_orders",
	"sales_order_items": "sales_orders", "sales_order_allocations": "sales_orders",
}

// CheckBackupTables checks legacy module tables and recorded module versions
// before an import can replace core rows. Other legacy tables are left alone.
func (registry *Registry) CheckBackupTables(db *gorm.DB) error {
	installed := make(map[string]bool, len(registry.modules))
	for _, module := range registry.modules {
		installed[module.ID()] = true
	}
	var tables []string
	if err := db.Raw("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").Scan(&tables).Error; err != nil {
		return err
	}
	for _, name := range tables {
		if owner := legacyModuleTables[name]; owner != "" && !installed[owner] {
			return fmt.Errorf("database contains unavailable module table %s", name)
		}
	}
	if db.Migrator().HasTable(&moduleVersion{}) {
		var recorded []moduleVersion
		if err := db.Find(&recorded).Error; err != nil {
			return err
		}
		for _, row := range recorded {
			found := false
			for _, module := range registry.modules {
				if row.ID == module.ID() {
					found = true
					if row.Version > module.Version() {
						return fmt.Errorf("database requires module %s v%d, build supports v%d", row.ID, row.Version, module.Version())
					}
				}
			}
			if strings.TrimSpace(row.ID) == "" {
				return errors.New("database has invalid module version metadata")
			}
			if !found {
				return fmt.Errorf("database requires unavailable module %s", row.ID)
			}
		}
	}
	return nil
}

func (registry *Registry) RegisterRoutes(router *gin.Engine, db *gorm.DB) {
	for _, module := range registry.modules {
		module.RegisterRoutes(router, db)
	}
}

// ValidateBackups checks every module before the target database is changed.
func (registry *Registry) ValidateBackups(source *gorm.DB, schemaVersion int) (map[string]any, error) {
	data := make(map[string]any, len(registry.modules))
	for _, module := range registry.modules {
		value, err := module.ValidateBackup(source, schemaVersion)
		if err != nil {
			return nil, fmt.Errorf("module %s backup: %w", module.ID(), err)
		}
		data[module.ID()] = value
	}
	return data, nil
}

// DeleteBackups runs in reverse dependency order inside the restore transaction.
func (registry *Registry) DeleteBackups(tx *gorm.DB) error {
	for i := len(registry.modules) - 1; i >= 0; i-- {
		if err := registry.modules[i].DeleteBackup(tx); err != nil {
			return fmt.Errorf("delete module %s data: %w", registry.modules[i].ID(), err)
		}
	}
	return nil
}

// RestoreBackups runs after core rows exist, inside the same transaction.
func (registry *Registry) RestoreBackups(tx *gorm.DB, data map[string]any) error {
	for _, module := range registry.modules {
		if err := module.RestoreBackup(tx, data[module.ID()]); err != nil {
			return fmt.Errorf("restore module %s data: %w", module.ID(), err)
		}
	}
	return nil
}

// CheckPrintJobDeletion runs while core holds SQLite's writer lock.
func (registry *Registry) CheckPrintJobDeletion(tx *gorm.DB, id string) error {
	for _, module := range registry.modules {
		if err := module.CheckPrintJobDeletion(tx, id); err != nil {
			return err
		}
	}
	return nil
}

// WithRequestDB keeps request correlation context on database operations.
func WithRequestDB(db *gorm.DB, factory func(*gorm.DB) gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) { factory(db.WithContext(c.Request.Context()))(c) }
}
