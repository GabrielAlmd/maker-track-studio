package handlers

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ListParams is deliberately opt-in: requests without list parameters retain
// the legacy array response used by existing desktop clients.
type ListParams struct {
	Enabled   bool
	Page      int
	PageSize  int
	Sort      string
	Direction string
}

type PageResponse[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func parseListParams(c *gin.Context, allowedSort map[string]string, defaultSort string) (ListParams, bool) {
	q := c.Request.URL.Query()
	_, hasPage := q["page"]
	_, hasSize := q["pageSize"]
	_, hasSort := q["sort"]
	_, hasDirection := q["direction"]
	_, hasSearch := q["search"]
	if !(hasPage || hasSize || hasSort || hasDirection || hasSearch || len(q) > 0) {
		return ListParams{}, true
	}
	p := ListParams{Enabled: true, Page: 1, PageSize: 25, Sort: defaultSort, Direction: "asc"}
	var err error
	if hasPage {
		p.Page, err = strconv.Atoi(q.Get("page"))
		if err != nil || p.Page < 1 {
			return p, false
		}
	}
	if hasSize {
		p.PageSize, err = strconv.Atoi(q.Get("pageSize"))
		if err != nil || p.PageSize < 1 || p.PageSize > 100 {
			return p, false
		}
	}
	if sort := q.Get("sort"); sort != "" {
		mapped, ok := allowedSort[sort]
		if !ok {
			return p, false
		}
		p.Sort = mapped
	}
	if direction := strings.ToLower(q.Get("direction")); direction != "" {
		if direction != "asc" && direction != "desc" {
			return p, false
		}
		p.Direction = direction
	}
	return p, true
}

// ParseListParams is the shared HTTP list contract available to modules.
func ParseListParams(c *gin.Context, allowedSort map[string]string, defaultSort string) (ListParams, bool) {
	return parseListParams(c, allowedSort, defaultSort)
}
