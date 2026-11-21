package sales

import (
	"errors"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var errDocumentsBackup = errors.New("backup contains missing or invalid order documents or activity")

func (data *backupData) validateDocuments(db *gorm.DB, schemaVersion int) error {
	hasAttachments := db.Migrator().HasTable(&storage.OrderAttachment{})
	hasEvents := db.Migrator().HasTable(&storage.OrderEvent{})
	if schemaVersion >= 16 && (!hasAttachments || !hasEvents) {
		return errDocumentsBackup
	}
	orders := make(map[uuid.UUID]storage.SalesOrder, len(data.orders))
	for _, order := range data.orders {
		orders[order.ID] = order
	}
	if hasAttachments {
		// Check the stored byte lengths before materializing BLOBs from an
		// untrusted SQLite archive. The per-order quota is checked below.
		var bounds struct{ Total, Largest int64 }
		if err := db.Model(&storage.OrderAttachment{}).
			Select("COALESCE(SUM(length(CAST(content AS BLOB))), 0) AS total, COALESCE(MAX(length(CAST(content AS BLOB))), 0) AS largest").
			Scan(&bounds).Error; err != nil || bounds.Total > storage.MaxWorkspaceAttachmentBytes || bounds.Largest > storage.MaxOrderAttachmentBytes {
			return errDocumentsBackup
		}
		if err := db.Find(&data.attachments).Error; err != nil {
			return errDocumentsBackup
		}
	}
	if hasEvents {
		if err := db.Find(&data.events).Error; err != nil {
			return errDocumentsBackup
		}
	}
	if schemaVersion < 16 && (len(data.attachments) > 0 || len(data.events) > 0) {
		return errDocumentsBackup
	}
	counts := make(map[uuid.UUID]int)
	ids := make(map[uuid.UUID]bool)
	operations := make(map[uuid.UUID]bool)
	for _, a := range data.attachments {
		mediaType := storage.AttachmentMediaType(a.Name, a.Content)
		_, orderExists := orders[a.SalesOrderID]
		counts[a.SalesOrderID]++
		if !orderExists || counts[a.SalesOrderID] > storage.MaxOrderAttachments ||
			a.ID == uuid.Nil || ids[a.ID] || a.OperationID == uuid.Nil || operations[a.OperationID] ||
			a.CreatedAt.IsZero() || a.Size != len(a.Content) || a.Checksum != storage.AttachmentChecksum(a.Content) ||
			mediaType == "" || mediaType != a.MediaType {
			return errDocumentsBackup
		}
		ids[a.ID], operations[a.OperationID] = true, true
	}

	type eventKey struct {
		order, run uuid.UUID
		kind       string
		revision   int64
	}
	seen := make(map[eventKey]bool)
	clear(ids)
	for _, event := range data.events {
		order := orders[event.SalesOrderID]
		if order.AcceptedProposalID == nil || event.ID == uuid.Nil || ids[event.ID] || event.CreatedAt.IsZero() {
			return errDocumentsBackup
		}
		key := eventKey{order: event.SalesOrderID, kind: event.Kind, revision: event.Revision}
		switch event.Kind {
		case storage.OrderProductionRevised:
			if event.RunID != nil || event.Revision < 1 || event.Revision > order.ProductionRevision {
				return errDocumentsBackup
			}
		case storage.OrderRemainderCancelled:
			if event.RunID != nil || event.Revision != 0 || order.FulfilmentStatus != storage.SalesOrderFulfilmentCancelled {
				return errDocumentsBackup
			}
		case "run_in_progress", "run_completed", "run_failed", "run_cancelled":
			// A historical run reference survives deletion of a released job.
			if event.RunID == nil || *event.RunID == uuid.Nil || event.Revision < 2 {
				return errDocumentsBackup
			}
			key.run = *event.RunID
		default:
			return errDocumentsBackup
		}
		if seen[key] {
			return errDocumentsBackup
		}
		seen[key], ids[event.ID] = true, true
	}
	return nil
}
