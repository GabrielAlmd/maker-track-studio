package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeLocale(t *testing.T) {
	tests := map[string]string{
		"en": "en", "en-US": "en", "English": "en",
		"pt": "pt-PT", "pt_PT.UTF-8": "pt-PT", "pt-PT": "pt-PT",
		"fr-FR": "", "": "",
	}
	for input, expected := range tests {
		if actual := normalizeLocale(input); actual != expected {
			t.Errorf("normalizeLocale(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestLocalePreferenceCleanInstallAndUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "locale")
	if err := writeLocalePreferenceAt(path, "pt-PT", true); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalePreferenceAt(path, "en", true); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(value)) != "pt-PT" {
		t.Fatalf("upgrade replaced preference: %q", value)
	}
	if err := writeLocalePreferenceAt(path, "en", false); err != nil {
		t.Fatal(err)
	}
	value, err = os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(value)) != "en" {
		t.Fatalf("settings change was not persisted: %q, %v", value, err)
	}
}

func TestWindowsInstallerLocaleContract(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("build", "windows", "installer", "project.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	installer := string(contents)
	for _, expected := range []string{"MUI_LANGDLL_DISPLAY", "MUI_LANGUAGE \"Portuguese\"", "/LOCALE=", "--set-locale-if-unset"} {
		if !strings.Contains(installer, expected) {
			t.Errorf("installer is missing %q", expected)
		}
	}
}
