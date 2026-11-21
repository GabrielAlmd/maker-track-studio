package budget

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestProposalPDFPrintsCustomerTermsAndPaginates(t *testing.T) {
	record := Proposal{
		ID: uuid.New(), Revision: 2, CustomerName: "Oficina de São João",
		IssuedAt: new(time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)), EstimatedCostCents: 7654321,
		Terms: ProposalTerms{
			Title: "Produção de suportes", ValidUntil: "2026-12-20", DueDate: "2026-12-16", LeadTimeDays: 5,
			DeliveryTerms: "Recolha presencial\n\nDepois das 15h", PaymentTerms: "Pagamento na entrega",
		},
		Lines: []ProposalLine{{
			ProposalLineInput:  ProposalLineInput{Description: "Suporte"},
			EstimatedCostCents: 7654321, Quantity: 2, UnitPriceCents: 1201,
			LineDiscountCents: 73, OrderDiscountCents: 33, NetCents: 2296,
		}},
		LineDiscountCents: 73, OrderDiscountCents: 33, NetCents: 2296,
	}
	document := proposalDocument(record)
	var text strings.Builder
	for _, paragraph := range proposalParagraphs(document, "pt-PT") {
		text.WriteString(paragraph.text + "\n")
	}
	for _, want := range []string{"2026-12-20", "2026-12-16", "Prazo em dias: 5", "Oficina de São João",
		"Preço unitário: EUR 12,01", "Desconto de linha: EUR 0,73", "Desconto da proposta: EUR 0,33",
		"Total da proposta: EUR 22,96", "Recolha presencial", "Pagamento na entrega"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("PDF content missing %q", want)
		}
	}
	for _, private := range []string{"7654321", "76543,21", "Margem", "Lucro", "Custo"} {
		if strings.Contains(text.String(), private) {
			t.Errorf("PDF content includes internal data %q", private)
		}
	}
	for range 25 {
		document.Lines = append(document.Lines, document.Lines[0])
	}
	pdf, err := renderProposalPDF(document, "pt-PT")
	if err != nil || bytes.Count(pdf, []byte("/Type /Page\n")) < 2 {
		t.Fatalf("expected a valid multi-page proposal, got %d bytes and %v", len(pdf), err)
	}
}
