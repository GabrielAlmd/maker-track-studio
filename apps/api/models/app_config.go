package models

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"time"
)

const (
	AppFeaturesConfigKey = "features"
)

type AppConfig struct {
	Key       string    `gorm:"type:varchar(50);primaryKey" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"value"`
	Type      string    `gorm:"type:varchar(20)" json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AppConfig) TableName() string {
	return "app_config"
}

func LoadFeatureConfig(db *gorm.DB) (map[string]bool, error) {
	var config AppConfig
	err := db.Where("key = ?", AppFeaturesConfigKey).First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var features map[string]bool
	if err := json.Unmarshal([]byte(config.Value), &features); err != nil {
		return nil, err
	}
	if features == nil {
		return map[string]bool{}, nil
	}
	return features, nil
}
