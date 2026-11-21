package desktopruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/budget"
	"github.com/google/uuid"
)

func TestProposalSQLiteRuntimeUpgrade(t *testing.T) {
	for _, mode := range []string{"desktop", "self-hosted"} {
		t.Run(mode, func(t *testing.T) {
			config := Config{SQLitePath: filepath.Join(t.TempDir(), "workspace.db")}
			const origin = "https://studio.home.arpa"
			if mode == "self-hosted" {
				config.SameOriginProxy = true
				config.TrustedProxies = []string{"127.0.0.1"}
				config.LANAccess = &LANAccess{Origin: origin, ProxySecret: strings.Repeat("a", 64)}
			}
			open := func() *Runtime {
				runtime, err := New(config)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := runtime.Shutdown(context.Background()); err != nil {
						t.Error(err)
					}
				})
				return runtime
			}
			runtime := open()
			request := func(method, path string, input, result any, status int) {
				t.Helper()
				data, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest(method, path, bytes.NewReader(data))
				req.Header.Set("Content-Type", "application/json")
				if config.LANAccess != nil {
					req.Host, req.RemoteAddr = "studio.home.arpa", "127.0.0.1:1234"
					req.Header.Set("X-Maker-Proxy-Key", config.LANAccess.ProxySecret)
					req.Header.Set("X-Forwarded-Proto", "https")
					req.Header.Set("Origin", origin)
				}
				rec := httptest.NewRecorder()
				runtime.Server.Handler.ServeHTTP(rec, req)
				if rec.Code != status {
					t.Fatalf("%s %s: got %d, want %d: %s", method, path, rec.Code, status, rec.Body.String())
				}
				if result != nil {
					if err := json.Unmarshal(rec.Body.Bytes(), result); err != nil {
						t.Fatal(err)
					}
				}
			}
			var customer budget.Customer
			request("POST", "/v2/business/customers", budget.CustomerCommand{
				Command: budget.Command{OperationID: uuid.New()}, CustomerInput: budget.CustomerInput{Name: "Sample customer"},
			}, &customer, 201)
			var estimate budget.Record
			request("POST", "/v2/business/budgets", budget.BudgetCommand{
				Command: budget.Command{OperationID: uuid.New()}, Header: budget.Header{Title: "Sample", CustomerID: &customer.ID},
				Items: []budget.ItemInput{{Name: "Sample item", Quantity: 2, Assumptions: budget.Assumptions{OtherCents: 250}}},
			}, &estimate, 201)
			if err := runtime.db.Migrator().DropTable(&budget.Proposal{}); err != nil {
				t.Fatal(err)
			}
			if err := runtime.db.Exec("UPDATE module_versions SET version = 2 WHERE id = ?", "sales_orders").Error; err != nil {
				t.Fatal(err)
			}
			if err := runtime.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}

			runtime = open()
			var proposal budget.ProposalView
			request("POST", "/v2/business/proposals", budget.ProposalCommand{
				Command: budget.Command{OperationID: uuid.New()}, BudgetID: estimate.ID, BudgetRevision: 1, CustomerID: customer.ID,
				Terms: budget.ProposalTerms{Title: "Sample quote", ValidUntil: time.Now().UTC().AddDate(0, 0, 10).Format(time.DateOnly)},
				Lines: []budget.ProposalLineInput{{
					BudgetLineID: estimate.Items[0].ID, Description: "Sample item",
					Price: budget.PriceSelection{Mode: "manual", Value: new(int64(500))},
				}},
			}, &proposal, 201)
			path := "/v2/business/proposals/" + proposal.ID.String()
			request("POST", path+"/issue", budget.Command{OperationID: uuid.New(), ExpectedRevision: 1}, nil, 200)
			if err := runtime.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}

			runtime = open()
			var document budget.ProposalDocument
			request("GET", path+"/document?revision=2", nil, &document, 200)
			if document.ID != proposal.ID || document.Revision != 2 || document.NetCents != 1000 {
				t.Fatalf("proposal changed after reopening the workspace: %+v", document)
			}
			request("GET", "/desktop-data/export", nil, nil, 200)
		})
	}
}
