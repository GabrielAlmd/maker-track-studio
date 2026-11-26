// Package inventory owns the physical stock ledger shared by both runtimes.
package inventory

import (
	"errors"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Quantity and balance must round-trip exactly through browser JSON numbers.
const MaxQuantity int64 = 1<<53 - 1

var (
	ErrInvalid  = errors.New("invalid inventory movement")
	ErrConflict = errors.New("inventory event conflicts with existing history")
)

type Posting struct {
	Units            int64     `json:"units,omitempty" gorm:"not null;default:0"`
	Reference        string    `json:"reference,omitempty" gorm:"not null;default:''"`
	CountedQuantity  int64     `json:"counted_quantity,omitempty" gorm:"not null;default:0"`
	ExpectedRevision int64     `json:"expected_revision,omitempty" gorm:"not null;default:0"`
	SourceKey        string    `json:"source_key,omitempty" gorm:"not null;default:'';index"`
	OperationID      uuid.UUID `json:"operation_id" gorm:"type:uuid;not null;uniqueIndex"`
	SourceID         uuid.UUID `json:"source_id" gorm:"type:uuid;not null;uniqueIndex"`
	ItemType         string    `json:"item_type" gorm:"not null;index:idx_inventory_item"`
	ItemID           uuid.UUID `json:"item_id" gorm:"type:uuid;not null;index:idx_inventory_item"`
	Kind             string    `json:"kind" gorm:"not null"`
	Quantity         int64     `json:"quantity" gorm:"not null"`
	Reason           string    `json:"reason" gorm:"not null"`
	Actor            string    `json:"actor" gorm:"not null"`
}

type Movement struct {
	Sequence  int64 `json:"sequence" gorm:"primaryKey;autoIncrement"`
	Posting   `gorm:"embedded"`
	CreatedAt time.Time `json:"created_at" gorm:"not null"`
}

func (Movement) TableName() string { return "inventory_movements" }

func (p Posting) Validate() error {
	if p.OperationID == uuid.Nil || p.SourceID == uuid.Nil || p.ItemID == uuid.Nil ||
		(p.ItemType != "filament" && p.ItemType != "model") ||
		strings.TrimSpace(p.Reason) == "" || len(p.Reason) > 512 || strings.TrimSpace(p.Actor) == "" || len(p.Actor) > 128 ||
		p.Quantity < -MaxQuantity || p.Quantity > MaxQuantity {
		return ErrInvalid
	}
	if strings.HasPrefix(p.SourceKey, "output:") {
		return validateOutputPosting(p)
	}
	if p.Units != 0 || p.Reference != "" {
		return ErrInvalid
	}
	if p.Kind != "count" && (p.CountedQuantity != 0 || p.ExpectedRevision != 0) {
		return ErrInvalid
	}
	switch p.Kind {
	case "count":
		if p.SourceKey != "" || p.CountedQuantity < 0 || p.CountedQuantity > MaxQuantity || p.ExpectedRevision < 0 || p.ExpectedRevision > MaxQuantity {
			return ErrInvalid
		}
	case "tracking_paused", "tracking_resumed":
		if p.ItemType != "filament" || p.SourceKey != "" || p.Quantity != 0 {
			return ErrInvalid
		}
	case "opening":
		if p.SourceKey != "" || p.Quantity < 0 {
			return ErrInvalid
		}
	case "purchase", "consumption", "compensation":
		sourceType, sourceID, ok := strings.Cut(p.SourceKey, ":")
		ref, err := uuid.Parse(sourceID)
		if !ok || err != nil || ref == uuid.Nil || (sourceType != "purchase" && sourceType != "material") || (p.Kind == "purchase" && sourceType != "purchase") || (p.Kind == "consumption" && sourceType != "material") {
			return ErrInvalid
		}
		if p.ItemType != "filament" || strings.TrimSpace(p.SourceKey) == "" || len(p.SourceKey) > 128 || p.Quantity == 0 {
			return ErrInvalid
		}
		if p.Kind == "purchase" && p.Quantity < 0 || p.Kind == "consumption" && p.Quantity > 0 {
			return ErrInvalid
		}
	case "adjustment":
		if p.SourceKey != "" || p.Quantity == 0 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

type Balance struct {
	TrackingPaused bool  `json:"tracking_paused,omitempty"`
	TrackingGap    bool  `json:"tracking_gap,omitempty"`
	OnHand         int64 `json:"on_hand"`
	Reserved       int64 `json:"reserved"`
	Available      int64 `json:"available"`
	Verified       bool  `json:"verified"`
	Negative       bool  `json:"negative"`
	Revision       int64 `json:"revision"`
}

func itemExists(db *gorm.DB, typ string, id uuid.UUID) error {
	var record any
	switch typ {
	case "filament":
		record = &models.Filament{}
	case "model":
		record = &models.Model{}
	default:
		return ErrInvalid
	}
	return db.Select("id").Take(record, "id = ?", id).Error
}

// VerifiedStockSQL is shared by the projection and the paginated low-stock filter.
const VerifiedStockSQL = "MAX(CASE WHEN kind IN ('opening','count') THEN sequence ELSE 0 END) > MAX(CASE WHEN kind = 'tracking_paused' THEN sequence ELSE 0 END)"

const balanceColumns = `
	COALESCE(SUM(quantity), 0) AS on_hand,
	COALESCE(SUM(CASE
		WHEN kind = 'reserve' THEN units
		WHEN kind IN ('release', 'dispatch') THEN -units
		ELSE 0 END), 0) AS reserved,
	COALESCE(` + VerifiedStockSQL + `, 0) AS verified,
    COALESCE(MAX(CASE WHEN kind = 'tracking_paused' THEN sequence ELSE 0 END) >
             MAX(CASE WHEN kind = 'tracking_resumed' THEN sequence ELSE 0 END), 0) AS tracking_paused,
    COALESCE(MAX(kind = 'tracking_paused'), 0) AS tracking_gap,
	COALESCE(MAX(sequence), 0) AS revision`

// MaterialBalances projects the same ledger state as Availability for catalogue reads.
func MaterialBalances(db *gorm.DB) *gorm.DB {
	return db.Model(&Movement{}).Select("item_id, "+balanceColumns).
		Where("item_type = ?", "filament").Group("item_id")
}

// SetAvailability derives display fields from the projected physical and reserved stock.
func (b *Balance) SetAvailability() {
	b.Available = b.OnHand - b.Reserved
	b.Negative = b.OnHand < 0
}

func projection(db *gorm.DB, typ string, id uuid.UUID, through int64) (Balance, error) {
	var b Balance
	q := db.Model(&Movement{}).Where("item_type = ? AND item_id = ?", typ, id)
	if through > 0 {
		q = q.Where("sequence <= ?", through)
	}
	err := q.Select(balanceColumns).Scan(&b).Error
	b.SetAvailability()
	return b, err
}

func Availability(db *gorm.DB, typ string, id uuid.UUID) (Balance, error) {
	if err := itemExists(db, typ, id); err != nil {
		return Balance{}, err
	}
	return projection(db, typ, id, 0)
}

// History returns up to 100 movements after a stable ledger sequence.
func History(db *gorm.DB, typ string, id uuid.UUID, after int64) ([]Movement, error) {
	if err := itemExists(db, typ, id); err != nil {
		return nil, err
	}
	var rows []Movement
	err := db.Where("item_type = ? AND item_id = ? AND sequence > ?", typ, id, after).Order("sequence").Limit(100).Find(&rows).Error
	return rows, err
}

type Receipt struct {
	Movement
	Balance Balance `json:"balance"`
}

// Post runs inside the caller's SQLite transaction so a source write can roll
// back together with its movement. Acquire the writer lock before reading.
func Post(tx *gorm.DB, p Posting) (Receipt, error) {
	var r Receipt
	if err := p.Validate(); err != nil {
		return r, err
	}
	if err := tx.Exec("UPDATE inventory_movements SET sequence = sequence WHERE 0").Error; err != nil {
		return r, err
	}
	err := tx.Where("operation_id = ? OR source_id = ?", p.OperationID, p.SourceID).Take(&r.Movement).Error
	if err == nil {
		if p.Kind == "count" {
			// The quantity on the receipt is the server-computed ledger delta.
			p.Quantity = r.Quantity
		}
		if r.Posting != p {
			return Receipt{}, ErrConflict
		}
		r.Balance, err = projection(tx, p.ItemType, p.ItemID, r.Sequence)
		return r, err
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return r, err
	}
	b, err := Availability(tx, p.ItemType, p.ItemID)
	if err != nil {
		return r, err
	}
	if p.Kind == "count" {
		p.Quantity = p.CountedQuantity - b.OnHand
		if p.Quantity > MaxQuantity || p.Quantity < -MaxQuantity {
			return r, ErrInvalid
		}
	}
	if strings.HasPrefix(p.SourceKey, "output:") {
		lot, err := loadLot(tx, p.SourceKey)
		if err != nil {
			return r, err
		}
		if err := lot.apply(p); err != nil {
			return r, err
		}
	}
	if err = b.apply(p); err != nil {
		return r, err
	}
	r.Movement = Movement{Posting: p}
	if err = tx.Create(&r.Movement).Error; err != nil {
		return r, err
	}
	if r.Sequence > MaxQuantity {
		return Receipt{}, ErrConflict
	}
	b.Revision = r.Sequence
	r.Balance = b
	return r, nil
}

func (b *Balance) apply(p Posting) error {
	switch p.Kind {
	case "tracking_paused":
		if b.TrackingPaused {
			return ErrConflict
		}
		b.TrackingPaused, b.TrackingGap, b.Verified = true, true, false
	case "tracking_resumed":
		if !b.TrackingPaused {
			return ErrConflict
		}
		b.TrackingPaused = false
	case "opening":
		if b.TrackingGap {
			return ErrConflict
		}
	default:
		if b.TrackingPaused && !(p.Kind == "compensation" && p.Quantity < 0 && strings.HasPrefix(p.SourceKey, "purchase:")) {
			return ErrConflict
		}
	}
	next := b.OnHand + p.Quantity
	if p.Kind == "count" && (p.ExpectedRevision != b.Revision || next != p.CountedQuantity) {
		return ErrConflict
	}
	if next < -MaxQuantity || next > MaxQuantity || (p.Kind == "opening" && b.Verified) {
		return ErrConflict
	}
	switch p.Kind {
	case "reserve":
		b.Reserved += p.Units
	case "release", "dispatch":
		b.Reserved -= p.Units
	}
	if b.Reserved < 0 || b.Reserved > 0 && next < b.Reserved || (p.Kind == "dispatch" && next < 0) {
		return ErrConflict
	}
	b.OnHand = next
	b.Available = next - b.Reserved
	b.Negative = next < 0
	b.Verified = b.Verified || p.Kind == "opening" || p.Kind == "count"
	return nil
}

// ValidateHistory checks imported rows in original posting order.
func ValidateHistory(rows []Movement, filaments []models.Filament, products []models.Model) error {
	type key struct {
		typ string
		id  uuid.UUID
	}
	balances := map[key]Balance{}
	for _, f := range filaments {
		balances[key{"filament", f.ID}] = Balance{}
	}
	for _, m := range products {
		balances[key{"model", m.ID}] = Balance{}
	}
	ops, sources := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	lots := map[string]*outputLot{}
	var last int64
	for _, m := range rows {
		k := key{m.ItemType, m.ItemID}
		b, ok := balances[k]
		if !ok || m.Validate() != nil || m.Sequence <= last || m.Sequence > MaxQuantity || m.CreatedAt.IsZero() || ops[m.OperationID] || sources[m.SourceID] || b.apply(m.Posting) != nil {
			return ErrInvalid
		}
		if strings.HasPrefix(m.SourceKey, "output:") {
			lot := lots[m.SourceKey]
			if lot == nil {
				lot = newLot()
				lots[m.SourceKey] = lot
			}
			if err := lot.apply(m.Posting); err != nil {
				return ErrInvalid
			}
			lot.Revision = m.Sequence
		}
		ops[m.OperationID], sources[m.SourceID] = true, true
		last = m.Sequence
		b.Revision = m.Sequence
		balances[k] = b
	}
	return nil
}
