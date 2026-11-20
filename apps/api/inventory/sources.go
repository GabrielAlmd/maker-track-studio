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

// SetSource reconciles a source's physical contribution. The caller holds the
// SQLite writer lock and commits source and ledger together. A repeated target
// is a no-op; a changed target appends compensations without rewriting history.
func SetSource(tx *gorm.DB, key string, target map[uuid.UUID]int64, kind string) error {
	var rows []Movement
	if err := tx.Where("source_key = ?", key).Order("sequence").Find(&rows).Error; err != nil {
		return err
	}
	current := map[uuid.UUID]int64{}
	var last int64
	for _, row := range rows {
		current[row.ItemID] += row.Quantity
		last = row.Sequence
	}
	for id := range current {
		if _, ok := target[id]; !ok {
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
		event := uuid.NewSHA1(uuid.NameSpaceOID, []byte(key+":"+strconv.FormatInt(last, 10)+":"+raw+":"+strconv.FormatInt(target[id], 10)))
		movementKind := kind
		if len(rows) > 0 {
			movementKind = "compensation"
		}
		_, err := Post(tx, Posting{OperationID: event, SourceID: event, SourceKey: key, ItemType: "filament", ItemID: id, Kind: movementKind, Quantity: delta, Reason: key, Actor: "local application"})
		if err != nil {
			return err
		}
	}
	return nil
}
