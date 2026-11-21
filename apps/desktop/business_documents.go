package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules/sales/storage"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) ExportProposalPDF(proposal string, revision int64, language string) (string, error) {
	id, err := uuid.Parse(proposal)
	if err != nil || id == uuid.Nil || revision < 1 || revision > 1<<53-1 || (language != "en" && language != "pt-PT") {
		return "", errors.New("Invalid proposal PDF options")
	}
	query := url.Values{"revision": {strconv.FormatInt(revision, 10)}, "language": {language}}
	return saveDesktopDownload(a.ctx, runtime.SaveFileDialog, &http.Client{Timeout: time.Minute}, runtime.SaveDialogOptions{
		Title: "Save proposal PDF", DefaultFilename: fmt.Sprintf("proposal-%s-r%d.pdf", id, revision),
		Filters: []runtime.FileFilter{{DisplayName: "PDF (*.pdf)", Pattern: "*.pdf"}},
	}, "proposal PDF", "/v2/business/proposals/"+id.String()+"/document/pdf", query.Encode(), "application/pdf")
}

func (a *App) ExportOrderAttachment(order, attachment, name string) (string, error) {
	orderID, orderErr := uuid.Parse(order)
	attachmentID, attachmentErr := uuid.Parse(attachment)
	if orderErr != nil || attachmentErr != nil || orderID == uuid.Nil || attachmentID == uuid.Nil || !storage.ValidAttachmentName(name) {
		return "", errors.New("Invalid order attachment options")
	}
	return saveDesktopDownload(a.ctx, runtime.SaveFileDialog, &http.Client{Timeout: time.Minute}, runtime.SaveDialogOptions{
		Title: "Save order attachment", DefaultFilename: name,
	}, "order attachment", "/v2/business/orders/"+orderID.String()+"/attachments/"+attachmentID.String(), "", "application/octet-stream")
}
