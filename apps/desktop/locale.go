package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultLocale  = "en"
	localeDirName  = "3d-printing-cost-tracker"
	localeFileName = "locale"
)

// LocalePreference is intentionally stored outside SQLite so an installer can
// choose it before the database exists. IsSet distinguishes an OS-derived
// suggestion from a preference the user or installer has confirmed.
type LocalePreference struct {
	Locale string `json:"locale"`
	IsSet  bool   `json:"is_set"`
}

func normalizeLocale(value string) string {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	if value == "pt" || strings.HasPrefix(value, "pt-pt") || value == "portuguese" {
		return "pt-PT"
	}
	if value == "en" || strings.HasPrefix(value, "en-") || value == "english" {
		return "en"
	}
	return ""
}

func localePreferencePath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, localeDirName, localeFileName), nil
}

func readLocalePreference() (LocalePreference, error) {
	path, err := localePreferencePath()
	if err != nil {
		return LocalePreference{Locale: defaultLocale}, err
	}
	value, err := os.ReadFile(path)
	if err == nil {
		locale := normalizeLocale(string(value))
		if locale == "" {
			return LocalePreference{Locale: defaultLocale}, fmt.Errorf("invalid saved locale")
		}
		return LocalePreference{Locale: locale, IsSet: true}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return LocalePreference{Locale: defaultLocale}, err
	}
	locale := normalizeLocale(systemLocale())
	if locale == "" {
		locale = defaultLocale
	}
	return LocalePreference{Locale: locale}, nil
}

func writeLocalePreference(value string, onlyIfUnset bool) error {
	locale := normalizeLocale(value)
	if locale == "" {
		return fmt.Errorf("unsupported locale")
	}
	path, err := localePreferencePath()
	if err != nil {
		return err
	}
	return writeLocalePreferenceAt(path, locale, onlyIfUnset)
}

func writeLocalePreferenceAt(path, locale string, onlyIfUnset bool) error {
	if onlyIfUnset {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "locale-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(locale + "\n"); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func (a *App) GetLocalePreference() LocalePreference {
	preference, err := readLocalePreference()
	if err != nil {
		return LocalePreference{Locale: defaultLocale}
	}
	return preference
}

func (a *App) SetLocalePreference(locale string) error {
	return writeLocalePreference(locale, false)
}
