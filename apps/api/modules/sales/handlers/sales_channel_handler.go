package saleshandlers

import (
	"errors"
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func salesChannelID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Sales Channel UUID"})
		return uuid.Nil, false
	}
	return id, true
}

func salesChannelError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sales Channel not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save or retrieve Sales Channel"})
}

func bindSalesChannel(c *gin.Context) (salesdto.SalesChannelRequest, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input salesdto.SalesChannelRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Sales Channel input"})
		return salesdto.SalesChannelRequest{}, false
	}
	if err := validateSalesChannelRequest(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return salesdto.SalesChannelRequest{}, false
	}
	return input, true
}

func GetSalesChannels(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := db.Model(&salesmodels.SalesChannel{})
		switch c.DefaultQuery("state", "active") {
		case "active":
			query = query.Where("is_archived = ?", false)
		case "archived":
			query = query.Where("is_archived = ?", true)
		case "all":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Sales Channel state filter"})
			return
		}

		channels := []salesmodels.SalesChannel{}
		if err := query.Order("name ASC, id ASC").Find(&channels).Error; err != nil {
			salesChannelError(c, err)
			return
		}
		result := make([]salesdto.SalesChannelDTO, 0, len(channels))
		for _, channel := range channels {
			result = append(result, salesChannelDTO(channel))
		}
		c.JSON(http.StatusOK, result)
	}
}

func CreateSalesChannel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindSalesChannel(c)
		if !ok {
			return
		}
		channel := salesmodels.SalesChannel{Name: input.Name, Notes: input.Notes}
		if err := db.Create(&channel).Error; err != nil {
			salesChannelError(c, err)
			return
		}
		c.JSON(http.StatusCreated, salesChannelDTO(channel))
	}
}

func UpdateSalesChannel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesChannelID(c)
		if !ok {
			return
		}
		input, ok := bindSalesChannel(c)
		if !ok {
			return
		}
		var channel salesmodels.SalesChannel
		if err := db.Where("id = ?", id).First(&channel).Error; err != nil {
			salesChannelError(c, err)
			return
		}
		channel.Name = input.Name
		channel.Notes = input.Notes
		if err := db.Save(&channel).Error; err != nil {
			salesChannelError(c, err)
			return
		}
		c.JSON(http.StatusOK, salesChannelDTO(channel))
	}
}

func ArchiveSalesChannel(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := salesChannelID(c)
		if !ok {
			return
		}
		var channel salesmodels.SalesChannel
		if err := db.Where("id = ?", id).First(&channel).Error; err != nil {
			salesChannelError(c, err)
			return
		}
		if !channel.IsArchived {
			channel.IsArchived = true
			if err := db.Save(&channel).Error; err != nil {
				salesChannelError(c, err)
				return
			}
		}
		c.JSON(http.StatusOK, salesChannelDTO(channel))
	}
}
