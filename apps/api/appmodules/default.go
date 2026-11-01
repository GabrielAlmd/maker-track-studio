// Package appmodules lists the modules shipped with this desktop application.
package appmodules

import (
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales"
)

func Default() *modules.Registry {
	registry, err := modules.New(sales.Module{})
	if err != nil {
		panic(err) // The built-in module declaration is a programming error.
	}
	return registry
}
