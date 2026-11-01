package diagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// Directory selects private local state, independently of SQLITE_PATH.
func Directory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("diagnostic directory unavailable")
	}
	return directoryFor(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_STATE_HOME"))
}

func directoryFor(platform, home, local, state string) (string, error) {
	var base string
	if platform == "windows" {
		base = local
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		base = filepath.Join(base, "3d-printing-cost-tracker", "diagnostics")
	} else {
		// XDG relative paths must be ignored, not interpreted against the CWD.
		if !filepath.IsAbs(state) {
			state = filepath.Join(home, ".local", "state")
		}
		base = filepath.Join(state, "3d-printing-cost-tracker")
	}
	if !filepath.IsAbs(base) {
		return "", errors.New("diagnostic directory unavailable")
	}
	return filepath.Join(base, "logs"), nil
}

// EnvironmentConfig never returns or logs invalid input values. DEBUG is a
// temporary 30-minute override; the logger reverts to INFO automatically.
func EnvironmentConfig() (Config, bool) {
	c := DefaultConfig()
	if value := os.Getenv("DIAGNOSTICS_LEVEL"); value != "" {
		c.MinimumLevel = Level(value)
	}
	if value := os.Getenv("DIAGNOSTICS_MAX_FILE_MIB"); value != "" {
		size, err := strconv.ParseInt(value, 10, 32)
		if err != nil || size < 1 || size > 100 {
			return DefaultConfig(), true
		}
		c.MaxFileBytes = size * MiB
	}
	return ResolveConfig(c)
}
