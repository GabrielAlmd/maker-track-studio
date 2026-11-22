package handlers_test

import (
	"archive/zip"
	"bytes"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"net/http"
	"testing"
)

func TestSlicerInspectionIsReadOnlyAndRejectsMalformedUploads(t *testing.T) {
	r, db, _ := setupTestRouterWithDefaultPrinter(t)
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, _ := z.Create("Metadata/model_settings.config")
	_, _ = f.Write([]byte(`<config><object id="1"><metadata key="name" value="Synthetic cap"/></object><plate><metadata key="plater_id" value="1"/><model_instance><metadata key="object_id" value="1"/><metadata key="instance_id" value="0"/></model_instance></plate></config>`))
	_ = z.Close()
	response := performMultipartFileRequest(t, r, http.MethodPost, "/v2/production/slicer/inspect", "file", "layout.3mf", b.Bytes())
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var count int64
	db.Model(&models.PrintJob{}).Count(&count)
	if count != 0 {
		t.Fatal("inspection wrote production")
	}
	response = performMultipartFileRequest(t, r, http.MethodPost, "/v2/production/slicer/inspect", "file", "broken.3mf", []byte("bad"))
	if response.Code != 400 {
		t.Fatal(response.Code)
	}
}
