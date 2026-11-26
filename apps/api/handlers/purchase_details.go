package handlers

import (
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/dto"
	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
)

// Pointer inputs distinguish legacy callers that omit fields from explicit clears.
type purchaseDetailsInput struct {
	Supplier    *string `json:"supplier"`
	SupplierRef *string `json:"supplier_reference"`
	PurchaseURL *string `json:"purchase_url"`
}

func optionalText(value *string, limit int) bool {
	if value == nil {
		return true
	}
	*value = strings.TrimSpace(*value)
	return utf8.RuneCountInString(*value) <= limit && strings.IndexFunc(*value, unicode.IsControl) < 0
}

func optionalPurchaseURL(value *string) bool {
	if !optionalText(value, 2048) {
		return false
	}
	if value == nil || *value == "" {
		return true
	}
	if strings.Contains(*value, "\\") || strings.IndexFunc(*value, unicode.IsSpace) >= 0 {
		return false
	}
	parsed, err := url.Parse(*value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Hostname() == "" || parsed.User != nil || strings.Contains(parsed.Host, "%") {
		return false
	}
	if strings.HasPrefix(parsed.Host, "[") {
		address, err := netip.ParseAddr(parsed.Hostname())
		if err != nil || !address.Is6() {
			return false
		}
	} else if strings.Contains(parsed.Hostname(), ":") {
		return false
	}
	if port := parsed.Port(); port != "" {
		_, err = strconv.ParseUint(port, 10, 16)
	}
	return err == nil
}

func (input purchaseDetailsInput) valid() bool {
	return optionalText(input.Supplier, 200) && optionalText(input.SupplierRef, 200) && optionalPurchaseURL(input.PurchaseURL)
}

func (input purchaseDetailsInput) apply(p *models.FilamentPurchase) {
	if input.Supplier != nil {
		p.Supplier = *input.Supplier
	}
	if input.SupplierRef != nil {
		p.SupplierReference = *input.SupplierRef
	}
	if input.PurchaseURL != nil {
		p.PurchaseURL = *input.PurchaseURL
	}
}

func purchaseDTO(p models.FilamentPurchase) dto.FilamentPurchaseDTO {
	return dto.FilamentPurchaseDTO{
		ID: p.ID, FilamentID: p.FilamentID, Price: p.Price, Amount: p.Amount,
		PurchaseDate: p.PurchaseDate, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		Supplier: p.Supplier, SupplierReference: p.SupplierReference, PurchaseURL: p.PurchaseURL,
	}
}
