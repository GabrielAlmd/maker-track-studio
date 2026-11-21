package saleshandlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type orderActivity struct {
	ID               string     `json:"id"`
	Kind             string     `json:"kind"`
	CreatedAt        time.Time  `json:"created_at"`
	Revision         int64      `json:"revision,omitempty"`
	ReferenceID      *uuid.UUID `json:"reference_id,omitempty"`
	ProposalRevision int64      `json:"proposal_revision,omitempty"`
	Detail           string     `json:"detail,omitempty"`
	Units            int64      `json:"units,omitempty"`
}

// Follow the ledger's bounded reference chain from an order allocation through
// a reservation, dispatch, and inspected return. These rows are the original
// stock evidence and remain owned and validated by Inventory.
const orderActivitySQL = `
WITH reservations AS (
	SELECT m.* FROM sales_order_items AS line
	JOIN sales_order_allocations AS a ON a.sales_order_item_id = line.id
	JOIN inventory_movements AS m ON m.reference = a.id
		AND m.source_key = 'output:' || a.print_run_item_id
	WHERE line.sales_order_id = @id AND m.kind = 'reserve' AND m.actor = @actor
), movements AS (
	SELECT operation_id, kind, created_at, reference, reason, units FROM reservations
	UNION ALL
	SELECT m.operation_id, m.kind, m.created_at, m.reference, m.reason, m.units
	FROM reservations AS r JOIN inventory_movements AS m ON m.reference = r.operation_id
		AND m.source_key = r.source_key
	WHERE m.kind IN ('release', 'dispatch') AND m.actor = @actor
	UNION ALL
	SELECT m.operation_id, m.kind, m.created_at, m.reference, m.reason, m.units
	FROM reservations AS r JOIN inventory_movements AS d ON d.reference = r.operation_id
		AND d.source_key = r.source_key AND d.kind = 'dispatch' AND d.actor = @actor
	JOIN inventory_movements AS m ON m.reference = d.operation_id AND m.source_key = d.source_key
	WHERE m.kind IN ('restock', 'reject_return') AND m.actor = @actor
)
SELECT id, kind, created_at, revision, reference_id, proposal_revision, detail, units FROM (
	SELECT 'order:' || revision AS id,
		CASE revision WHEN 1 THEN 'proposal_accepted' ELSE 'order_amended' END AS kind,
		created_at, revision, proposal_id AS reference_id, proposal_revision, reason AS detail, 0 AS units
	FROM business_order_revisions WHERE id = @id
	UNION ALL
	SELECT 'event:' || id, kind, created_at, revision, run_id, 0, '', 0
	FROM business_order_events WHERE sales_order_id = @id
	UNION ALL
	SELECT 'attachment:' || id, 'attachment_added', created_at, 0, id, 0, name, 0
	FROM business_order_attachments WHERE sales_order_id = @id
	UNION ALL
	SELECT 'fulfilment:' || operation_id, kind, created_at, 0, reference, 0, reason, units
	FROM movements
)
ORDER BY julianday(created_at) DESC, created_at DESC, kind ASC, revision DESC, id ASC
LIMIT @limit OFFSET @offset`

func GetOrderActivity(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		const size = 25
		page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
		if err != nil || page < 1 || page > 100000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Provide a valid activity page"})
			return
		}
		items := []orderActivity{}
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Select("id").Take(&storage.SalesOrder{}, "id = ?", id).Error; err != nil {
				return err
			}
			return tx.Raw(orderActivitySQL, map[string]any{
				"id": id, "actor": storage.FulfilmentActor, "limit": size + 1, "offset": (page - 1) * size,
			}).Scan(&items).Error
		})
		if err != nil {
			orderProductionError(c, err)
			return
		}
		hasMore := len(items) > size
		if hasMore {
			items = items[:size]
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "page": page, "has_more": hasMore})
	}
}
