package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Empty list contracts must remain iterable in both the legacy lookup response
// and the paginated response used by current pages. Assert JSON, not Go len(nil).
func TestEmptyListsReturnIterableCollections(t *testing.T) {
	for _, path := range []string{"/models", "/filaments", "/filament-purchases", "/print-jobs", "/printers"} {
		for _, paginated := range []bool{false, true} {
			url := path
			if paginated && path != "/printers" {
				url += "?page=1&pageSize=25"
			}
			if paginated && path == "/printers" {
				continue
			}
			t.Run(url, func(t *testing.T) {
				r, _ := setupTestRouter(t)
				response := performRequest(t, r, http.MethodGet, url, nil)
				if response.Code != http.StatusOK {
					t.Fatalf("GET %s: %d %s", url, response.Code, response.Body.String())
				}
				var body any
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				items := body
				if paginated {
					page, ok := body.(map[string]any)
					if !ok {
						t.Fatalf("expected page envelope, got %s", response.Body.String())
					}
					assertFloat(t, page["total"], 0)
					assertFloat(t, page["page"], 1)
					assertFloat(t, page["page_size"], 25)
					items = page["items"]
				}
				rows, ok := items.([]any)
				if !ok || len(rows) != 0 {
					t.Fatalf("expected empty JSON array, got %s", response.Body.String())
				}
			})
		}
	}
}
