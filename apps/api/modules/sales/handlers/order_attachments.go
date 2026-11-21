package saleshandlers

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var attachmentUploadSlots = make(chan struct{}, 2)

func ListOrderAttachments(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		items := []storage.OrderAttachment{}
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Select("id").Take(&storage.SalesOrder{}, "id = ?", id).Error; err != nil {
				return err
			}
			return tx.Omit("content").Where("sales_order_id = ?", id).Order("created_at, id").Find(&items).Error
		})
		if err != nil {
			orderProductionError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func CreateOrderAttachment(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		name := c.Query("name")
		operation, err := uuid.Parse(c.Query("operation_id"))
		if err != nil || operation == uuid.Nil || !storage.ValidAttachmentName(name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Provide a valid file name and operation ID"})
			return
		}
		if c.ContentType() != "application/octet-stream" {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Send the file as application/octet-stream"})
			return
		}
		select {
		case attachmentUploadSlots <- struct{}{}:
			defer func() { <-attachmentUploadSlots }()
		default:
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Try uploading again shortly"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, storage.MaxOrderAttachmentBytes)
		content, err := io.ReadAll(c.Request.Body)
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Attachments must be at most 5 MiB"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "Could not read attachment"})
			return
		}
		mediaType := storage.AttachmentMediaType(name, content)
		if mediaType == "" {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Choose a valid PDF, PNG or JPEG file"})
			return
		}

		item := storage.OrderAttachment{
			SalesOrderID: id, OperationID: operation, Name: name, MediaType: mediaType,
			Size: len(content), Checksum: storage.AttachmentChecksum(content), Content: content,
		}
		created, err := saveOrderAttachment(db, &item)
		if err != nil {
			orderProductionError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.JSON(status, item)
	}
}

func saveOrderAttachment(db *gorm.DB, item *storage.OrderAttachment) (bool, error) {
	created := false
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE sales_orders SET updated_at = updated_at WHERE id = ?", item.SalesOrderID).Error; err != nil {
			return err
		}
		if err := tx.Select("id").Take(&storage.SalesOrder{}, "id = ?", item.SalesOrderID).Error; err != nil {
			return err
		}
		features, err := models.LoadFeatureConfig(tx)
		if err != nil {
			return err
		}
		if !features["sales_orders"] {
			return productionConflict("Enable Business Operations before adding attachments")
		}
		var previous storage.OrderAttachment
		err = tx.Omit("content").Take(&previous, "operation_id = ?", item.OperationID).Error
		if err == nil {
			if previous.SalesOrderID != item.SalesOrderID || previous.Name != item.Name || previous.Checksum != item.Checksum {
				return productionConflict("This attachment operation was already used for a different file or order")
			}
			*item = previous
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		query := tx.Model(&storage.OrderAttachment{})
		var count, size int64
		if err := query.Where("sales_order_id = ?", item.SalesOrderID).Count(&count).Error; err != nil {
			return err
		}
		if err := tx.Model(&storage.OrderAttachment{}).Select("COALESCE(SUM(size), 0)").Scan(&size).Error; err != nil {
			return err
		}
		if count >= storage.MaxOrderAttachments || size+int64(item.Size) > storage.MaxWorkspaceAttachmentBytes {
			return productionConflict("The order or workspace attachment limit has been reached")
		}
		created = true
		return tx.Create(item).Error
	})
	return created, err
}

func DownloadOrderAttachment(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesOrderID(c)
		if !ok {
			return
		}
		attachmentID, err := uuid.Parse(c.Param("attachmentID"))
		if err != nil || attachmentID == uuid.Nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid attachment ID"})
			return
		}
		var item storage.OrderAttachment
		if err := db.Take(&item, "sales_order_id = ? AND id = ?", id, attachmentID).Error; err != nil {
			orderProductionError(c, err)
			return
		}
		if !storage.ValidAttachmentName(item.Name) || item.Size != len(item.Content) ||
			item.Checksum != storage.AttachmentChecksum(item.Content) {
			orderProductionError(c, errors.New("stored attachment failed its integrity check"))
			return
		}
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": item.Name}))
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, "application/octet-stream", item.Content)
	}
}
