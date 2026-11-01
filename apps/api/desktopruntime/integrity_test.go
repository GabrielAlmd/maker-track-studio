package desktopruntime

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	salesmodels "github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testWorkspace(t *testing.T) (*Runtime, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "workspace.db")
	r, err := New(Config{SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return r, path
}

func TestDatabaseOwnershipAcrossProcessesAndRestart(t *testing.T) {
	if path := os.Getenv("MAKER_TEST_OWNED_DB"); path != "" {
		if r, err := New(Config{SQLitePath: path}); err == nil {
			_ = r.Shutdown(context.Background())
			os.Exit(3)
		}
		return
	}
	r, path := testWorkspace(t)
	if err := r.db.Create(&models.Model{Name: "survives restart"}).Error; err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestDatabaseOwnershipAcrossProcessesAndRestart$")
	cmd.Env = append(os.Environ(), "MAKER_TEST_OWNED_DB="+path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("second process bypassed ownership: %v %s", err, output)
	}
	if _, err := New(Config{SQLitePath: path}); err == nil {
		t.Fatal("second runtime opened owned database")
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	next, err := New(Config{SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Shutdown(context.Background())
	var count int64
	if err := next.db.Model(&models.Model{}).Where("name = ?", "survives restart").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("data lost on restart: %d %v", count, err)
	}
}

func TestSQLitePolicyAndLockedDatabase(t *testing.T) {
	r, path := testWorkspace(t)
	for pragma, want := range map[string]string{"journal_mode": "delete", "synchronous": "2", "busy_timeout": "5000"} {
		var got string
		if err := r.db.Raw("PRAGMA " + pragma).Scan(&got).Error; err != nil || got != want {
			t.Fatalf("%s = %q, want %q (%v)", pragma, got, want, err)
		}
	}
	pool, _ := r.db.DB()
	if pool.Stats().MaxOpenConnections != 1 {
		t.Fatal("unbounded pool")
	}
	other, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	otherSQL, _ := other.DB()
	defer otherSQL.Close()
	if err := other.Exec("BEGIN EXCLUSIVE").Error; err != nil {
		t.Fatal(err)
	}
	defer other.Exec("ROLLBACK")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := r.db.WithContext(ctx).Exec("INSERT INTO models (id,name) VALUES ('locked','blocked')").Error; err == nil {
		t.Fatal("external lock bypassed")
	}
	if err := other.Exec("ROLLBACK").Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := r.db.Model(&models.Model{}).Where("id = ?", "locked").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("interrupted write persisted: %d %v", count, err)
	}
}

func TestConcurrentWorkspaceCRUDReportsAndBackups(t *testing.T) {
	r, _ := testWorkspace(t)
	backups := t.TempDir()
	const clients = 12
	var wg sync.WaitGroup
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			perform := func(method, path, body string, want int) []byte {
				req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
				req.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				r.Server.Handler.ServeHTTP(response, req)
				if response.Code != want {
					t.Errorf("%s %s: %d %s", method, path, response.Code, response.Body.String())
				}
				return response.Body.Bytes()
			}
			body := perform("POST", "/models", fmt.Sprintf(`{"name":"Model %d","material_weight":10,"print_time":30}`, i), 201)
			var created struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(body, &created); err != nil || created.ID == "" {
				t.Errorf("create: %s", body)
				return
			}
			perform("PUT", "/models/"+created.ID, fmt.Sprintf(`{"name":"Edited %d","material_weight":20,"print_time":60}`, i), 200)
			perform("GET", "/models/"+created.ID, "", 200)
			perform("GET", "/reports/export", "", 200)
			if i%3 == 0 {
				if err := r.Backup(context.Background(), backups, 3); err != nil {
					t.Error(err)
				}
			}
			if i%2 == 0 {
				perform("DELETE", "/models/"+created.ID, "", 204)
			}
		}(i)
	}
	wg.Wait()
	var count int64
	if err := r.db.Model(&models.Model{}).Count(&count).Error; err != nil || count != clients/2 {
		t.Fatalf("lost updates: %d %v", count, err)
	}
	var integrity string
	if err := r.db.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil || integrity != "ok" {
		t.Fatalf("integrity: %q %v", integrity, err)
	}
}

func importArchive(r *Runtime, data []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "backup.zip")
	_, _ = part.Write(data)
	_ = writer.Close()
	req := httptest.NewRequest("POST", "/desktop-data/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	r.Server.Handler.ServeHTTP(response, req)
	return response
}

func TestRestoreRecoverySnapshotAndRollback(t *testing.T) {
	r, _ := testWorkspace(t)
	original := models.Model{Name: "original"}
	if err := r.db.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	backup := httptest.NewRecorder()
	r.Server.Handler.ServeHTTP(backup, httptest.NewRequest("GET", "/desktop-data/export", nil))
	if backup.Code != 200 {
		t.Fatal(backup.Body.String())
	}
	if err := r.db.Model(&original).Update("name", "newer").Error; err != nil {
		t.Fatal(err)
	}
	// A partial replacement must roll back even after DELETE has run.
	if err := r.db.Exec(`CREATE TRIGGER reject_restore BEFORE INSERT ON models BEGIN SELECT RAISE(ABORT, 'test restore failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if response := importArchive(r, backup.Body.Bytes()); response.Code != 500 {
		t.Fatalf("restore: %d %s", response.Code, response.Body.String())
	}
	var saved models.Model
	if err := r.db.First(&saved, "id = ?", original.ID).Error; err != nil || saved.Name != "newer" {
		t.Fatalf("partial import persisted: %#v %v", saved, err)
	}
	files, err := filepath.Glob(filepath.Join(r.recoveryDir, "*.zip"))
	if err != nil || len(files) != 1 {
		t.Fatalf("no recovery snapshot: %v %v", files, err)
	}
	if err := r.db.Exec("DROP TRIGGER reject_restore").Error; err != nil {
		t.Fatal(err)
	}
	if response := importArchive(r, backup.Body.Bytes()); response.Code != 200 {
		t.Fatalf("restore: %d %s", response.Code, response.Body.String())
	}
	if err := r.db.First(&saved, "id = ?", original.ID).Error; err != nil || saved.Name != "original" {
		t.Fatal("restore failed")
	}
	// The presnapshot is itself a portable archive that restores the newer data.
	recovery, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if response := importArchive(r, recovery); response.Code != 200 {
		t.Fatalf("recovery: %d %s", response.Code, response.Body.String())
	}
	if err := r.db.First(&saved, "id = ?", original.ID).Error; err != nil || saved.Name != "newer" {
		t.Fatal("recovery lost newer data")
	}
	if response := importArchive(r, []byte("corrupt")); response.Code != 400 {
		t.Fatal("corrupt archive accepted")
	}
}

func TestRestoreFailsClosedWithoutRecoveryStorage(t *testing.T) {
	r, _ := testWorkspace(t)
	response := httptest.NewRecorder()
	r.Server.Handler.ServeHTTP(response, httptest.NewRequest("GET", "/desktop-data/export", nil))
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if err := os.WriteFile(r.recoveryDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result := importArchive(r, response.Body.Bytes()); result.Code != 500 {
		t.Fatalf("restore without backup: %d %s", result.Code, result.Body.String())
	}
}

func TestBackupRetentionCancellationAndDestinationFailure(t *testing.T) {
	r, _ := testWorkspace(t)
	directory := t.TempDir()
	unrelated := filepath.Join(directory, "keep.zip")
	_ = os.WriteFile(unrelated, []byte("keep"), 0o600)
	interrupted := filepath.Join(directory, ".maker-track-backup-interrupted.tmp")
	_ = os.WriteFile(interrupted, []byte("partial"), 0o600)
	for i := 0; i < 4; i++ {
		if err := r.Backup(context.Background(), directory, 2); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(directory, "maker-track-backup-*.zip"))
	if len(files) != 2 {
		t.Fatalf("retention: %v", files)
	}
	for _, path := range files {
		archive, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(archive.File) != 2 {
			t.Fatal("partial archive published")
		}
		archive.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Backup(ctx, directory, 1); err == nil {
		t.Fatal("cancelled backup succeeded")
	}
	if err := r.Backup(context.Background(), unrelated, 1); err == nil {
		t.Fatal("invalid destination accepted")
	}
	after, _ := filepath.Glob(filepath.Join(directory, "maker-track-backup-*.zip"))
	if len(after) != 2 {
		t.Fatal("failure pruned backups")
	}
	for _, path := range []string{unrelated, interrupted} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("unmanaged file removed")
		}
	}
}

func TestNextBackupUsesUTCAndNextDay(t *testing.T) {
	now := time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)
	if next := nextBackup(now, "03:00"); !next.Equal(now.Add(24 * time.Hour)) {
		t.Fatal(next)
	}
	if next := nextBackup(now.Add(-time.Second), "03:00"); !next.Equal(now) {
		t.Fatal(next)
	}
}

func workspaceJSON(r *Runtime, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.Server.Handler.ServeHTTP(response, req)
	return response
}

func TestConcurrentPurchasesPrintJobsAndSalesCapacity(t *testing.T) {
	r, _ := testWorkspace(t)
	filament := models.Filament{Brand: "Fixture", Type: "PLA", Color: "Black"}
	product := models.Model{Name: "Fixture model", MaterialWeight: 20, PrintTime: 30}
	for _, record := range []any{&filament, &product} {
		if err := r.db.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	response := workspaceJSON(r, "POST", "/printers", `{"name":"Fixture printer","manufacturer":"Test","model":"Test"}`)
	if response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	// Shared production has just one available unit, contested by two orders.
	job := models.PrintJob{ModelID: &product.ID, PrintDate: time.Now(), UnitsPrinted: 1}
	if err := r.db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	orders := make([]salesmodels.SalesOrder, 2)
	for i := range orders {
		orders[i] = salesmodels.SalesOrder{Reference: fmt.Sprintf("Fixture-%d", i), OrderDate: time.Now(), Items: []salesmodels.SalesOrderItem{{ModelID: &product.ID, ModelNameSnapshot: product.Name, Quantity: 1, UnitPriceCents: 1000}}}
		if err := r.db.Create(&orders[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for _, order := range orders {
		wg.Add(1)
		go func(order salesmodels.SalesOrder) {
			defer wg.Done()
			body := fmt.Sprintf(`{"allocations":[{"sales_order_item_id":%q,"print_job_id":%q,"units_allocated":1}]}`, order.Items[0].ID, job.ID)
			statuses <- workspaceJSON(r, "PUT", "/sales-orders/"+order.ID.String()+"/allocations", body).Code
		}(order)
	}
	const sessions = 8
	for i := 0; i < sessions; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			purchase := fmt.Sprintf(`{"purchase_date":"2026-10-30T00:00:00Z","items":[{"filament_id":%q,"price":20,"amount":1}]}`, filament.ID)
			if result := workspaceJSON(r, "POST", "/filament-purchases/batch", purchase); result.Code != 201 {
				t.Errorf("purchase: %d %s", result.Code, result.Body.String())
			}
			print := fmt.Sprintf(`{"model_id":%q,"print_date":"2026-10-30T00:00:00Z","units_printed":2,"labor_hours":0}`, product.ID)
			if result := workspaceJSON(r, "POST", "/print-jobs", print); result.Code != 201 {
				t.Errorf("print: %d %s", result.Code, result.Body.String())
			}
		}()
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("capacity not enforced: %v", counts)
	}
	var allocated int64
	if err := r.db.Model(&salesmodels.SalesOrderAllocation{}).Select("COALESCE(SUM(units_allocated),0)").Scan(&allocated).Error; err != nil || allocated != 1 {
		t.Fatalf("over-allocation: %d %v", allocated, err)
	}
	for _, check := range []struct {
		model any
		want  int64
	}{{&models.FilamentPurchase{}, sessions}, {&models.PrintJob{}, sessions + 1}} {
		var count int64
		if err := r.db.Model(check.model).Count(&count).Error; err != nil || count != check.want {
			t.Fatalf("lost writes for %T: %d %v", check.model, count, err)
		}
	}
}

func TestConcurrentRestoreAndReportReads(t *testing.T) {
	r, _ := testWorkspace(t)
	if err := r.db.Create(&models.Model{Name: "retained"}).Error; err != nil {
		t.Fatal(err)
	}
	backup := workspaceJSON(r, "GET", "/desktop-data/export", "")
	if backup.Code != 200 {
		t.Fatal(backup.Body.String())
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if response := importArchive(r, backup.Body.Bytes()); response.Code != 200 {
				t.Errorf("import: %d %s", response.Code, response.Body.String())
			}
		}()
		go func() {
			defer wg.Done()
			if response := workspaceJSON(r, "GET", "/reports/export", ""); response.Code != 200 {
				t.Errorf("report: %d %s", response.Code, response.Body.String())
			}
		}()
	}
	wg.Wait()
	var count int64
	if err := r.db.Model(&models.Model{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("partial import visible: %d %v", count, err)
	}
}

func TestMalformedArchiveDoesNotRotateRecovery(t *testing.T) {
	r, _ := testWorkspace(t)
	for _, duplicate := range []bool{false, true} {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		manifest := []byte(`{"format_version":1,"schema_version":4,"driver":"sqlite"}`)
		entry, _ := writer.Create("manifest.json")
		_, _ = entry.Write(manifest)
		entry, _ = writer.Create("app.db")
		_, _ = entry.Write([]byte("not SQLite"))
		if duplicate {
			entry, _ = writer.Create("app.db")
			_, _ = entry.Write([]byte("duplicate"))
		}
		_ = writer.Close()
		if response := importArchive(r, buffer.Bytes()); response.Code != 400 {
			t.Fatalf("bad archive: %d %s", response.Code, response.Body.String())
		}
	}
	if _, err := os.Stat(r.recoveryDir); !os.IsNotExist(err) {
		t.Fatal("invalid archive created recovery backup")
	}
}

func TestCrashReleasesOwnership(t *testing.T) {
	if path := os.Getenv("MAKER_TEST_CRASH_DB"); path != "" {
		r, err := New(Config{SQLitePath: path})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.db.Create(&models.Model{Name: "committed before crash"}).Error; err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(os.Stdout, "workspace ready")
		for {
			time.Sleep(time.Hour)
		}
	}
	path := filepath.Join(t.TempDir(), "crash.db")
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashReleasesOwnership$")
	cmd.Env = append(os.Environ(), "MAKER_TEST_CRASH_DB="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if scanner.Text() == "workspace ready" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("child startup failed")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("child startup timed out")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	r, err := New(Config{SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Shutdown(context.Background())
	var count int64
	if err := r.db.Model(&models.Model{}).Where("name = ?", "committed before crash").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("committed data lost: %d %v", count, err)
	}
}

func TestSchedulerShutdownReleasesOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheduled.db")
	r, err := New(Config{SQLitePath: path, Backups: &BackupConfig{Directory: t.TempDir(), TimeUTC: "03:00", Retention: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(Config{SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Shutdown(context.Background())
}

func TestArchiveDecompressionLimit(t *testing.T) {
	r, _ := testWorkspace(t)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, _ := writer.Create("manifest.json")
	_, _ = entry.Write([]byte(`{"format_version":1,"schema_version":4,"driver":"sqlite"}`))
	entry, _ = writer.Create("app.db")
	chunk := make([]byte, 1<<20)
	for i := 0; i < 65; i++ {
		if _, err := entry.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if response := importArchive(r, buffer.Bytes()); response.Code != 400 {
		t.Fatalf("oversized expanded archive accepted: %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(r.recoveryDir); !os.IsNotExist(err) {
		t.Fatal("oversized archive rotated backups")
	}
}
