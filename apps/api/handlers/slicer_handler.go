package handlers

import (
	"io"
	"net/http"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/slicer"
	"github.com/gin-gonic/gin"
)

// InspectSlicerLayout performs no database writes and does not extract files.
func InspectSlicerLayout(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (32<<20)+(1<<20))
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
		c.JSON(400, gin.H{"error": "Invalid or oversized slicer upload"})
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	f, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "Select a 3MF layout"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	if err != nil || len(data) > 32<<20 {
		c.JSON(400, gin.H{"error": "Invalid or oversized slicer upload"})
		return
	}
	plates, err := slicer.Parse3MF(data)
	if err != nil {
		c.JSON(400, gin.H{"error": "Unsupported or invalid Bambu 3MF layout; export plate G-code for weight and time estimates"})
		return
	}
	c.JSON(200, gin.H{"plates": plates})
}
