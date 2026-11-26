package desktopruntime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFilamentSupplierSchemaUpgrade(t *testing.T) {
	for _, lan := range []bool{false, true} {
		name := "desktop"
		if lan {
			name = "self-hosted"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.db")
			legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := legacy.AutoMigrate(&models.Filament{}, &models.FilamentPurchase{}); err != nil {
				t.Fatal(err)
			}
			filament := models.Filament{Brand: "Fixture", Type: "PLA", Color: "Blue", AvgPrice: 10}
			if err := legacy.Create(&filament).Error; err != nil {
				t.Fatal(err)
			}
			purchase := models.FilamentPurchase{FilamentID: filament.ID, Price: 20, Amount: 2, PurchaseDate: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)}
			if err := legacy.Create(&purchase).Error; err != nil {
				t.Fatal(err)
			}
			for _, ddl := range []string{
				"ALTER TABLE filaments DROP COLUMN product_url",
				"ALTER TABLE filament_purchases DROP COLUMN supplier",
				"ALTER TABLE filament_purchases DROP COLUMN supplier_reference",
				"ALTER TABLE filament_purchases DROP COLUMN purchase_url",
			} {
				if err := legacy.Exec(ddl).Error; err != nil {
					t.Fatal(err)
				}
			}
			conn, err := legacy.DB()
			if err != nil {
				t.Fatal(err)
			}
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}

			config := Config{SQLitePath: path}
			if lan {
				config.SameOriginProxy = true
				config.TrustedProxies = []string{"127.0.0.1"}
				config.LANAccess = &LANAccess{Origin: "https://studio.home.arpa", ProxySecret: strings.Repeat("a", 64)}
			}
			for _, expectedSupplier := range []string{"", "Fixture supply"} {
				func() {
					runtime, err := New(config)
					if err != nil {
						t.Fatal(err)
					}
					defer func() {
						if err := runtime.Shutdown(context.Background()); err != nil {
							t.Error(err)
						}
					}()
					var saved models.FilamentPurchase
					if err := runtime.db.Preload("Filament").First(&saved, "id = ?", purchase.ID).Error; err != nil {
						t.Fatal(err)
					}
					if saved.Supplier != expectedSupplier || saved.PurchaseURL != "" || saved.SupplierReference != "" ||
						saved.Filament.ID != filament.ID || saved.Filament.ProductURL != "" || saved.Filament.AvgPrice != filament.AvgPrice || saved.Price != purchase.Price ||
						saved.Amount != purchase.Amount || !saved.PurchaseDate.Equal(purchase.PurchaseDate) {
						t.Fatalf("startup changed purchase or filament: %+v", saved)
					}
					if err := runtime.db.Model(&saved).Update("supplier", "Fixture supply").Error; err != nil {
						t.Fatal(err)
					}
				}()
			}
		})
	}
}
