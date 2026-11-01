package modules_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testModule struct {
	id       string
	requires int
	version  int
	tables   []string
	migrate  func(*gorm.DB) error
}

func (m testModule) ID() string { return m.id }
func (m testModule) Version() int {
	if m.version > 0 {
		return m.version
	}
	return 1
}
func (m testModule) RequiresCoreVersion() int                   { return m.requires }
func (testModule) EnabledByDefault() bool                       { return false }
func (m testModule) BackupTables() []string                     { return m.tables }
func (m testModule) Migrate(db *gorm.DB) error                  { return m.migrate(db) }
func (testModule) RegisterRoutes(*gin.Engine, *gorm.DB)         {}
func (testModule) ValidateBackup(*gorm.DB, int) (any, error)    { return nil, nil }
func (testModule) DeleteBackup(*gorm.DB) error                  { return nil }
func (testModule) RestoreBackup(*gorm.DB, any) error            { return nil }
func (testModule) CheckPrintJobDeletion(*gorm.DB, string) error { return nil }

func TestRegistryRejectsInvalidDeclarations(t *testing.T) {
	valid := testModule{id: "sales_orders", requires: modules.CoreVersion}
	for _, installed := range [][]modules.Module{
		{nil},
		{testModule{}},
		{valid, valid},
		{testModule{id: "future", requires: modules.CoreVersion + 1}},
	} {
		if _, err := modules.New(installed...); err == nil {
			t.Fatalf("accepted invalid modules: %#v", installed)
		}
	}
}

func TestModuleMigrationRollsBackOnFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/modules.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE markers (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	registry, err := modules.New(testModule{
		id: "broken", requires: modules.CoreVersion,
		migrate: func(tx *gorm.DB) error {
			if err := tx.Exec("INSERT INTO markers (id) VALUES (1)").Error; err != nil {
				return err
			}
			return errors.New("migration failed")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Migrate(db); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("expected named module error, got %v", err)
	}
	var count int64
	if err := db.Table("markers").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("migration left partial data: count=%d err=%v", count, err)
	}
}

func TestRegistryRejectsNewerPersistedModuleVersion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/versions.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	registry, err := modules.New(testModule{id: "sales_orders", requires: modules.CoreVersion, migrate: func(*gorm.DB) error { called++; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE module_versions SET version = 2 WHERE id = ?", "sales_orders").Error; err != nil {
		t.Fatal(err)
	}
	if err := registry.Migrate(db); err == nil || !strings.Contains(err.Error(), "requires module sales_orders v2") {
		t.Fatalf("expected version compatibility error, got %v", err)
	}
	if called != 1 {
		t.Fatalf("incompatible module migration ran %d times", called)
	}
}

func TestRegistryRejectsUnavailableModuleTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/backup.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE sales_orders (id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	empty, err := modules.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.CheckBackupTables(db); err == nil {
		t.Fatal("accepted Sales data without its module")
	}
	installed, err := modules.New(testModule{id: "sales_orders", requires: modules.CoreVersion, tables: []string{"sales_orders"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := installed.CheckBackupTables(db); err != nil {
		t.Fatal(err)
	}
}

func TestModuleInitializationRollsBackEarlierModules(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/atomic.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modules.New(
		testModule{id: "first", requires: modules.CoreVersion, migrate: func(tx *gorm.DB) error { return tx.Exec("CREATE TABLE first_module_data (id INTEGER)").Error }},
		testModule{id: "second", requires: modules.CoreVersion, migrate: func(*gorm.DB) error { return errors.New("failed") }},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Migrate(db); err == nil {
		t.Fatal("expected module initialization failure")
	}
	if db.Migrator().HasTable("first_module_data") || db.Migrator().HasTable("module_versions") {
		t.Fatal("failed initialization left partial module schema")
	}
}
