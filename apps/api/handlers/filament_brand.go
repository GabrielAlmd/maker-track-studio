package handlers

import (
	"sort"
	"strings"

	"gorm.io/gorm"
)

// canonicalFilamentBrand trims new input and reuses the spelling of an
// existing brand that differs only by case or surrounding whitespace.
func canonicalFilamentBrand(db *gorm.DB, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}

	var existing string
	err := db.Model(&struct{ Brand string }{}).
		Table("filaments").
		Select("brand").
		Where("LOWER(TRIM(brand)) = ?", strings.ToLower(trimmed)).
		Order("created_at ASC, id ASC").
		Limit(1).
		Scan(&existing).Error
	if err != nil {
		return "", err
	}
	if existing != "" {
		return strings.TrimSpace(existing), nil
	}
	return trimmed, nil
}

func listFilamentBrands(db *gorm.DB) ([]string, error) {
	var stored []string
	if err := db.Model(&struct{ Brand string }{}).Table("filaments").Order("created_at ASC, id ASC").Pluck("brand", &stored).Error; err != nil {
		return nil, err
	}

	brands := make([]string, 0, len(stored))
	seen := make(map[string]struct{}, len(stored))
	for _, value := range stored {
		brand := strings.TrimSpace(value)
		key := strings.ToLower(brand)
		if brand == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		brands = append(brands, brand)
	}
	sort.SliceStable(brands, func(i, j int) bool {
		return strings.ToLower(brands[i]) < strings.ToLower(brands[j])
	})
	return brands, nil
}
