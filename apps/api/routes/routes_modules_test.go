package routes_test

import (
	"testing"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/appmodules"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/routes"
	"github.com/gin-gonic/gin"
)

func TestSalesRoutesAreModuleContribution(t *testing.T) {
	productionOnly, err := modules.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name      string
		installed *modules.Registry
		hasSales  bool
	}{
		{"production only", productionOnly, false},
		{"sales installed", appmodules.Default(), true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			router := gin.New()
			routes.RegisterRoutesWithModules(router, nil, scenario.installed)
			found := map[string]bool{}
			for _, route := range router.Routes() {
				found[route.Method+" "+route.Path] = true
			}
			if !found["GET /print-jobs"] || !found["GET /features"] {
				t.Fatal("core routes missing")
			}
			for _, path := range []string{"GET /sales-orders", "GET /sales/summary", "GET /print-jobs/:id/allocations"} {
				if found[path] != scenario.hasSales {
					t.Fatalf("%s registration = %t, want %t", path, found[path], scenario.hasSales)
				}
			}
		})
	}
}
