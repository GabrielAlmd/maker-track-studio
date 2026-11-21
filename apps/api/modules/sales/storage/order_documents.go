package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	MaxOrderAttachmentBytes     = 5 << 20
	MaxWorkspaceAttachmentBytes = 32 << 20
	MaxOrderAttachments         = 50
)

type OrderAttachment struct {
	ID           uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	SalesOrderID uuid.UUID `json:"-" gorm:"type:uuid;not null;index"`
	OperationID  uuid.UUID `json:"-" gorm:"type:uuid;not null;uniqueIndex"`
	Name         string    `json:"name" gorm:"not null"`
	MediaType    string    `json:"media_type" gorm:"not null"`
	Size         int       `json:"size" gorm:"not null"`
	Checksum     string    `json:"sha256" gorm:"not null"`
	Content      []byte    `json:"-" gorm:"not null"`
	CreatedAt    time.Time `json:"created_at" gorm:"not null"`
}

func (OrderAttachment) TableName() string { return "business_order_attachments" }
func (a *OrderAttachment) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&a.ID)
	return nil
}

func ValidAttachmentName(name string) bool {
	if name != strings.TrimSpace(name) || len(name) == 0 || len(name) > 180 ||
		!utf8.ValidString(name) || strings.HasPrefix(name, ".") ||
		strings.ContainsAny(name, `/\:<>"|?*`) || strings.IndexFunc(name, func(r rune) bool {
		return unicode.IsControl(r) || unicode.In(r, unicode.Cf)
	}) != -1 {
		return false
	}
	stem, _, _ := strings.Cut(name, ".")
	switch strings.ToUpper(strings.TrimSpace(stem)) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
		"COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
		return false
	default:
		return true
	}
}

func AttachmentChecksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// AttachmentMediaType validates the name, byte limit and allowed file format.
// PDFs are opaque documents and are always offered as downloads.
func AttachmentMediaType(name string, content []byte) string {
	if !ValidAttachmentName(name) || len(content) == 0 || len(content) > MaxOrderAttachmentBytes {
		return ""
	}
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".pdf":
		if (bytes.HasPrefix(content, []byte("%PDF-1.")) || bytes.HasPrefix(content, []byte("%PDF-2."))) &&
			bytes.HasSuffix(bytes.TrimSpace(content), []byte("%%EOF")) {
			return "application/pdf"
		}
	case ".png", ".jpg", ".jpeg":
		cfg, format, err := image.DecodeConfig(bytes.NewReader(content))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 16_000_000/cfg.Height {
			return ""
		}
		if format == "png" && ext == ".png" || format == "jpeg" && (ext == ".jpg" || ext == ".jpeg") {
			if _, _, err := image.Decode(bytes.NewReader(content)); err == nil {
				return "image/" + format
			}
		}
	}
	return ""
}

const (
	OrderProductionRevised  = "production_revised"
	OrderRemainderCancelled = "remainder_cancelled"
)

type OrderEvent struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey"`
	SalesOrderID uuid.UUID  `gorm:"type:uuid;not null;index"`
	Kind         string     `gorm:"not null"`
	Revision     int64      `gorm:"not null"`
	RunID        *uuid.UUID `gorm:"type:uuid"`
	CreatedAt    time.Time  `gorm:"not null"`
}

func (OrderEvent) TableName() string { return "business_order_events" }
func (e *OrderEvent) BeforeCreate(_ *gorm.DB) error {
	ensureUUID(&e.ID)
	return nil
}

func OrderRunEventKind(status string) string {
	switch status {
	case "in_progress", "completed", "failed", "cancelled":
		return "run_" + status
	default:
		return ""
	}
}
