package inventory

import (
	"strings"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func outputKey(item uuid.UUID) string { return "output:" + item.String() }

func outputPosting(item models.PrintRunItem) Posting {
	key := outputKey(item.ID)
	event := uuid.NewSHA1(uuid.NameSpaceOID, []byte(key))
	return Posting{
		Kind: "output", SourceKey: key, OperationID: event, SourceID: item.ID,
		ItemType: "model", ItemID: *item.ModelID, Quantity: item.Accepted,
		Actor: "local application", Reason: "Accepted production",
	}
}

// RecordOutput joins the transaction that completes the verified run.
func RecordOutput(tx *gorm.DB, item models.PrintRunItem) error {
	if item.ModelID == nil {
		return ErrInvalid
	}
	_, err := Post(tx, outputPosting(item))
	return err
}

// ValidateOutputSources verifies every credited item against its immutable run.
// Older production may predate posting, so validation is deliberately one-way.
func ValidateOutputSources(rows []Movement, runs []models.PrintRun, items []models.PrintRunItem) error {
	completed := map[uuid.UUID]bool{}
	for _, run := range runs {
		completed[run.ID] = run.Status == "completed" && run.Provenance == "verified"
	}
	expected := map[uuid.UUID]Posting{}
	for _, item := range items {
		if completed[item.PrintRunID] && item.Accepted > 0 && item.ModelID != nil {
			expected[item.ID] = outputPosting(item)
		}
	}
	for _, row := range rows {
		if row.Kind == "output" && expected[row.SourceID] != row.Posting {
			return ErrInvalid
		}
	}
	return nil
}

func validateOutputPosting(p Posting) error {
	id, err := uuid.Parse(strings.TrimPrefix(p.SourceKey, "output:"))
	if err != nil || id == uuid.Nil || p.SourceKey != outputKey(id) || p.ItemType != "model" || p.CountedQuantity != 0 {
		return ErrInvalid
	}
	if p.Kind == "output" {
		if p.Quantity <= 0 || p.SourceID != id || p.ExpectedRevision != 0 || p.Units != 0 || p.Reference != "" {
			return ErrInvalid
		}
		return nil
	}
	ref, err := uuid.Parse(p.Reference)
	if err != nil || ref == uuid.Nil || p.Reference != ref.String() || p.ExpectedRevision <= 0 || p.Units <= 0 || p.Units > MaxQuantity {
		return ErrInvalid
	}
	var quantity int64
	switch p.Kind {
	case "reserve", "release", "reject_return":
	case "dispatch":
		quantity = -p.Units
	case "restock":
		quantity = p.Units
	default:
		return ErrInvalid
	}
	if p.Quantity != quantity {
		return ErrInvalid
	}
	return nil
}

type outputLot struct {
	Balance
	product    uuid.UUID
	holds      map[string]int64
	dispatches map[string]int64
}

func newLot() *outputLot {
	return &outputLot{holds: map[string]int64{}, dispatches: map[string]int64{}}
}

func (lot *outputLot) apply(p Posting) error {
	if p.ExpectedRevision != lot.Revision {
		return ErrConflict
	}
	if p.Kind == "output" {
		lot.product = p.ItemID
		lot.Verified = true
	} else if lot.product != p.ItemID {
		return ErrConflict
	}
	switch p.Kind {
	case "reserve":
		if p.Units > lot.Available {
			return ErrConflict
		}
		lot.holds[p.OperationID.String()] = p.Units
	case "dispatch", "release":
		if p.Units > lot.holds[p.Reference] {
			return ErrConflict
		}
		lot.holds[p.Reference] -= p.Units
		if p.Kind == "dispatch" {
			lot.dispatches[p.OperationID.String()] = p.Units
		}
	case "restock", "reject_return":
		if p.Units > lot.dispatches[p.Reference] {
			return ErrConflict
		}
		lot.dispatches[p.Reference] -= p.Units
	}
	return lot.Balance.apply(p)
}

func loadLot(db *gorm.DB, key string) (*outputLot, error) {
	var rows []Movement
	if err := db.Where("source_key = ?", key).Order("sequence").Find(&rows).Error; err != nil {
		return nil, err
	}
	lot := newLot()
	for _, row := range rows {
		if err := lot.apply(row.Posting); err != nil {
			return nil, err
		}
		lot.Revision = row.Sequence
	}
	return lot, nil
}

// OutputAvailability returns verified quantities and the revision for one run item.
func OutputAvailability(db *gorm.DB, product, item uuid.UUID) (Balance, error) {
	lot, err := loadLot(db, outputKey(item))
	if err != nil {
		return Balance{}, err
	}
	if lot.product != product || !lot.Verified {
		return Balance{}, gorm.ErrRecordNotFound
	}
	return lot.Balance, nil
}

// FulfilmentCommand is one line event in the caller's transaction. Reference
// identifies the owner on reserve, the hold on release/dispatch, and the
// dispatch on return. SourceID identifies the unique external line event.
type FulfilmentCommand struct {
	OperationID, SourceID, ProductID, RunItemID, Reference uuid.UUID
	Kind, Reason, Actor                                    string
	Units, ExpectedRevision                                int64
}

func ApplyFulfilment(tx *gorm.DB, cmd FulfilmentCommand) (Receipt, error) {
	p := Posting{
		Kind: cmd.Kind, SourceKey: outputKey(cmd.RunItemID),
		ItemType: "model", ItemID: cmd.ProductID,
		OperationID: cmd.OperationID, SourceID: cmd.SourceID, Reference: cmd.Reference.String(),
		Units: cmd.Units, ExpectedRevision: cmd.ExpectedRevision,
		Reason: cmd.Reason, Actor: cmd.Actor,
	}
	switch cmd.Kind {
	case "reserve", "release", "reject_return":
	case "dispatch":
		p.Quantity = -cmd.Units
	case "restock":
		p.Quantity = cmd.Units
	default:
		return Receipt{}, ErrInvalid
	}
	return Post(tx, p)
}

// FinishedProduct combines model identity with its aggregate ledger balance.
type FinishedProduct struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Balance `json:"balance" gorm:"embedded"`
}

func FinishedProducts(db *gorm.DB) ([]FinishedProduct, error) {
	rows := []FinishedProduct{}
	totals := db.Model(&Movement{}).Where("item_type = ?", "model").Select("item_id, " + balanceColumns).Group("item_id")
	err := db.Table("models").Select("models.id, models.name, totals.on_hand, totals.reserved, totals.verified, totals.revision").Joins("JOIN (?) AS totals ON totals.item_id = models.id", totals).Order("models.name, models.id").Scan(&rows).Error
	for i := range rows {
		b := &rows[i].Balance
		b.Available = b.OnHand - b.Reserved
		b.Negative = b.OnHand < 0
	}
	return rows, err
}

// VerifiedOutputBalances projects physical availability by lot for one model.
func VerifiedOutputBalances(db *gorm.DB, model uuid.UUID) *gorm.DB {
	return db.Model(&Movement{}).Select("source_key, "+balanceColumns).
		Where("item_type = ? AND item_id = ?", "model", model).
		Group("source_key").Having("MAX(kind = 'output') = 1")
}

// OutputLot carries the current physical availability and supporting receipts.
type OutputLot struct {
	Balance
	Movements []Movement
}

// OutputLots reads all requested lots in one query. Older production without
// an output credit has no verified lot and cannot be reserved.
func OutputLots(db *gorm.DB, items []uuid.UUID) (map[uuid.UUID]OutputLot, error) {
	keys := make([]string, 0, len(items))
	for _, item := range items {
		keys = append(keys, outputKey(item))
	}
	var rows []Movement
	if len(keys) > 0 {
		if err := db.Where("source_key IN ?", keys).Order("sequence").Find(&rows).Error; err != nil {
			return nil, err
		}
	}
	lots := make(map[uuid.UUID]OutputLot)
	for _, row := range rows {
		id, err := uuid.Parse(strings.TrimPrefix(row.SourceKey, "output:"))
		if err != nil {
			return nil, ErrInvalid
		}
		lot := lots[id]
		if err := lot.Balance.apply(row.Posting); err != nil {
			return nil, err
		}
		lot.Verified = lot.Verified || row.Kind == "output"
		lot.Revision = row.Sequence
		lot.Movements = append(lot.Movements, row)
		lots[id] = lot
	}
	return lots, nil
}
