package handlers_test

import (
	salesdto "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/api"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestSalesChannelLifecycle(t *testing.T) {
	router, _ := setupTestRouter(t)

	invalid := performRequest(t, router, http.MethodPost, "/sales-channels", map[string]any{"name": "   "})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid create: %d %s", invalid.Code, invalid.Body.String())
	}

	createdResponse := performRequest(t, router, http.MethodPost, "/sales-channels", map[string]any{
		"name": "  Direct sales  ", "notes": "  Local orders  ",
	})
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created salesdto.SalesChannelDTO
	decodeBody(t, createdResponse, &created)
	if created.ID == uuid.Nil || created.Name != "Direct sales" || created.Notes != "Local orders" || created.IsArchived {
		t.Fatalf("unexpected created channel: %#v", created)
	}

	activeResponse := performRequest(t, router, http.MethodGet, "/sales-channels", nil)
	if activeResponse.Code != http.StatusOK {
		t.Fatalf("list active: %d %s", activeResponse.Code, activeResponse.Body.String())
	}
	var active []salesdto.SalesChannelDTO
	decodeBody(t, activeResponse, &active)
	if len(active) != 1 || active[0].ID != created.ID {
		t.Fatalf("unexpected active channels: %#v", active)
	}

	updatedResponse := performRequest(t, router, http.MethodPut, "/sales-channels/"+created.ID.String(), map[string]any{
		"name": "Craft fair", "notes": "In person",
	})
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("update: %d %s", updatedResponse.Code, updatedResponse.Body.String())
	}
	var updated salesdto.SalesChannelDTO
	decodeBody(t, updatedResponse, &updated)
	if updated.ID != created.ID || updated.Name != "Craft fair" || updated.IsArchived {
		t.Fatalf("unexpected updated channel: %#v", updated)
	}

	archivedResponse := performRequest(t, router, http.MethodPut, "/sales-channels/"+created.ID.String()+"/archive", nil)
	if archivedResponse.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", archivedResponse.Code, archivedResponse.Body.String())
	}
	var archived salesdto.SalesChannelDTO
	decodeBody(t, archivedResponse, &archived)
	if !archived.IsArchived {
		t.Fatalf("channel was not archived: %#v", archived)
	}

	activeResponse = performRequest(t, router, http.MethodGet, "/sales-channels", nil)
	decodeBody(t, activeResponse, &active)
	if len(active) != 0 {
		t.Fatalf("archived channel returned as active: %#v", active)
	}
	for _, state := range []string{"archived", "all"} {
		response := performRequest(t, router, http.MethodGet, "/sales-channels?state="+state, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("list %s: %d %s", state, response.Code, response.Body.String())
		}
		var channels []salesdto.SalesChannelDTO
		decodeBody(t, response, &channels)
		if len(channels) != 1 || channels[0].ID != created.ID || !channels[0].IsArchived {
			t.Fatalf("unexpected %s channels: %#v", state, channels)
		}
	}
}

func TestSalesChannelErrorsArePredictable(t *testing.T) {
	router, _ := setupTestRouter(t)

	tests := []struct {
		method string
		path   string
		body   any
		status int
	}{
		{http.MethodGet, "/sales-channels?state=unknown", nil, http.StatusBadRequest},
		{http.MethodPut, "/sales-channels/not-a-uuid", map[string]any{"name": "Direct"}, http.StatusBadRequest},
		{http.MethodPut, "/sales-channels/" + uuid.New().String(), map[string]any{"name": "Direct"}, http.StatusNotFound},
		{http.MethodPut, "/sales-channels/" + uuid.New().String() + "/archive", nil, http.StatusNotFound},
	}
	for _, test := range tests {
		response := performRequest(t, router, test.method, test.path, test.body)
		if response.Code != test.status {
			t.Errorf("%s %s: got %d, want %d: %s", test.method, test.path, response.Code, test.status, response.Body.String())
		}
	}
}
