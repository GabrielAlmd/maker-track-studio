package inventory

import (
	"math"
	"sort"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Milligrams rounds measured quantities once at the ledger boundary.
func Milligrams(value, scale float64) (int64, error) {
	scaled := value * scale
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) || scaled < 0 || scaled > float64(MaxQuantity) {
		return 0, ErrInvalid
	}
	return int64(math.Round(scaled)), nil
}

type SourceAmount struct {
	ItemID   uuid.UUID
	Quantity int64
}

// SetSource reconciles a source's physical contribution. The caller holds the
// SQLite writer lock and commits source and ledger together. A repeated target
// is a no-op; a changed target appends compensations without rewriting history.
func SetSource(tx *gorm.DB, key string, next SourceAmount, kind string, previous *SourceAmount) error {
	var rows []Movement
	if err := tx.Where("source_key = ?", key).Order("sequence").Find(&rows).Error; err != nil {
		return err
	}
	current := map[uuid.UUID]int64{}
	cutoffs := map[uuid.UUID]int64{}
	var last int64
	for _, row := range rows {
		cutoff, ok := cutoffs[row.ItemID]
		if !ok {
			var err error
			cutoff, err = postGapCount(tx, row.ItemID)
			if err != nil {
				return err
			}
			cutoffs[row.ItemID] = cutoff
		}
		if row.Sequence > cutoff {
			current[row.ItemID] += row.Quantity
		}
		last = row.Sequence
	}
	// Move only outstanding credit plus the edited quantity difference.
	// Settled or skipped quantities cannot reappear on a new filament.
	if previous != nil {
		next.Quantity = max(0, current[previous.ItemID]+(next.Quantity-previous.Quantity))
	}
	target := map[uuid.UUID]int64{next.ItemID: next.Quantity}
	for id := range current {
		if id != next.ItemID {
			target[id] = 0
		}
	}
	// Stable ordering makes event identities independent of Go map iteration.
	ids := make([]string, 0, len(target))
	for id := range target {
		ids = append(ids, id.String())
	}
	sort.Strings(ids)
	for _, raw := range ids {
		id := uuid.MustParse(raw)
		delta := target[id] - current[id]
		if delta == 0 {
			continue
		}
		balance, err := Availability(tx, "filament", id)
		if err != nil {
			return err
		}
		if balance.TrackingPaused && (kind != "purchase" || delta > 0) {
			continue
		}
		event := uuid.NewSHA1(uuid.NameSpaceOID, []byte(key+":"+strconv.FormatInt(last, 10)+":"+raw+":"+strconv.FormatInt(target[id], 10)))
		movementKind := kind
		if len(rows) > 0 {
			movementKind = "compensation"
		}
		_, err = Post(tx, Posting{OperationID: event, SourceID: event, SourceKey: key, ItemType: "filament", ItemID: id, Kind: movementKind, Quantity: delta, Reason: key, Actor: "local application"})
		if err != nil {
			return err
		}
	}
	return nil
}

func postGapCount(tx *gorm.DB, id uuid.UUID) (int64, error) {
	var cutoff int64
	err := tx.Raw(`SELECT COALESCE(MAX(sequence),0) FROM inventory_movements
        WHERE item_type='filament' AND item_id=? AND kind='count'
        AND sequence > (SELECT MIN(sequence) FROM inventory_movements
            WHERE item_type='filament' AND item_id=? AND kind='tracking_paused')`, id, id).Scan(&cutoff).Error
	return cutoff, err
}
