package models

import (
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
