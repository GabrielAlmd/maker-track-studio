package budget

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/internal/pdfassets"
	"github.com/gin-gonic/gin"
	"github.com/signintech/gopdf"
)

const maxProposalPDFRequests = 2

var proposalPDFSlots = make(chan struct{}, maxProposalPDFRequests)

func downloadProposalPDF(c *gin.Context, document ProposalDocument) {
	language := c.DefaultQuery("language", "en")
	if language != "en" && language != "pt-PT" {
		reply(c, 0, nil, invalid("language", "Choose en or pt-PT"))
		return
	}
	select {
	case proposalPDFSlots <- struct{}{}:
		defer func() { <-proposalPDFSlots }()
	default:
		reply(c, 0, nil, &problem{status: http.StatusServiceUnavailable, Code: "pdf_busy", Message: "Try exporting again shortly"})
		return
	}
	body, err := renderProposalPDF(document, language)
	if err != nil {
		reply(c, 0, nil, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=proposal-%s-r%d.pdf", document.ID, document.Revision))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "application/pdf", body)
}

type proposalPDFLabels struct {
	title, revision, issued, expires, due, lead, delivery, payment,
	quantity, unit, lineDiscount, orderDiscount, subtotal, allLineDiscounts, total string
}

func proposalLabels(language string) proposalPDFLabels {
	if language == "pt-PT" {
		return proposalPDFLabels{"Proposta", "Revisão", "Emissão", "Válida até", "Entrega até", "Prazo em dias", "Condições de entrega", "Condições de pagamento",
			"Quantidade", "Preço unitário", "Desconto de linha", "Desconto da proposta", "Preço líquido da linha", "Total de descontos de linha", "Total da proposta"}
	}
	return proposalPDFLabels{"Proposal", "Revision", "Issued", "Valid until", "Delivery due", "Lead time in days", "Delivery terms", "Payment terms",
		"Quantity", "Unit price", "Line discount", "Proposal discount", "Net line price", "Total line discounts", "Proposal total"}
}

func proposalMoney(currency string, cents int64, language string) string {
	separator := "."
	if language == "pt-PT" {
		separator = ","
	}
	return fmt.Sprintf("%s %d%s%02d", currency, cents/100, separator, cents%100)
}

type proposalParagraph struct {
	text string
	size int
}

func proposalParagraphs(document ProposalDocument, language string) []proposalParagraph {
	labels := proposalLabels(language)
	money := func(cents int64) string { return proposalMoney(document.Currency, cents, language) }
	paragraphs := []proposalParagraph{
		{document.Terms.Title, 22},
		{document.CustomerName + "\n" + document.CustomerAddress, 12},
		{fmt.Sprintf("%s %d  |  %s %s  |  %s %s", labels.revision, document.Revision, labels.issued, document.IssuedAt.UTC().Format(time.DateOnly), labels.expires, document.Terms.ValidUntil), 10},
	}
	if document.Terms.DueDate != "" {
		paragraphs = append(paragraphs, proposalParagraph{labels.due + ": " + document.Terms.DueDate, 10})
	}
	if document.Terms.LeadTimeDays > 0 {
		paragraphs = append(paragraphs, proposalParagraph{fmt.Sprintf("%s: %d", labels.lead, document.Terms.LeadTimeDays), 10})
	}
	for i, line := range document.Lines {
		paragraphs = append(paragraphs,
			proposalParagraph{fmt.Sprintf("%d. %s", i+1, line.Description), 12},
			proposalParagraph{fmt.Sprintf("%s: %d  |  %s: %s", labels.quantity, line.Quantity, labels.unit, money(line.UnitPriceCents)), 10},
			proposalParagraph{fmt.Sprintf("%s: %s  |  %s: %s  |  %s: %s", labels.lineDiscount, money(line.LineDiscountCents), labels.orderDiscount, money(line.OrderDiscountCents), labels.subtotal, money(line.NetCents)), 10},
		)
	}
	paragraphs = append(paragraphs,
		proposalParagraph{labels.allLineDiscounts + ": " + money(document.LineDiscountCents), 10},
		proposalParagraph{labels.orderDiscount + ": " + money(document.OrderDiscountCents), 10},
		proposalParagraph{labels.total + ": " + money(document.NetCents), 15},
	)
	for _, term := range []struct{ label, text string }{
		{labels.delivery, document.Terms.DeliveryTerms}, {labels.payment, document.Terms.PaymentTerms},
	} {
		if term.text != "" {
			paragraphs = append(paragraphs, proposalParagraph{term.label, 12}, proposalParagraph{term.text, 10})
		}
	}
	return paragraphs
}

func renderProposalPDF(document ProposalDocument, language string) ([]byte, error) {
	var pdf gopdf.GoPdf
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.SetInfo(gopdf.PdfInfo{Title: proposalLabels(language).title, Creator: "Maker Track Studio"})
	if err := pdf.AddTTFFontData("Inter", pdfassets.Inter()); err != nil {
		return nil, err
	}

	var y float64
	page := 0
	newPage := func() error {
		page++
		pdf.AddPage()
		pdf.SetFillColor(15, 27, 50)
		pdf.RectFromUpperLeftWithStyle(0, 0, 595, 70, "F")
		if err := pdf.SetFont("Inter", "", 15); err != nil {
			return err
		}
		pdf.SetXY(42, 25)
		pdf.SetTextColor(255, 255, 255)
		if err := pdf.Cell(&gopdf.Rect{W: 510, H: 20}, proposalLabels(language).title+"  /  Maker Track Studio"); err != nil {
			return err
		}
		pdf.SetTextColor(29, 43, 65)
		pdf.SetXY(42, 802)
		y = 93
		return pdf.Cell(&gopdf.Rect{W: 510, H: 18}, fmt.Sprintf("%s  |  %d", document.ID, page))
	}
	if err := newPage(); err != nil {
		return nil, err
	}
	for _, paragraph := range proposalParagraphs(document, language) {
		height := float64(paragraph.size + 5)
		for _, part := range strings.Split(strings.ReplaceAll(paragraph.text, "\r\n", "\n"), "\n") {
			part = strings.TrimSpace(strings.ReplaceAll(part, "\t", "    "))
			if part == "" {
				continue
			}
			if err := pdf.SetFont("Inter", "", paragraph.size); err != nil {
				return nil, err
			}
			lines, err := pdf.SplitTextWithWordWrap(part, 510)
			if err != nil {
				return nil, err
			}
			for _, line := range lines {
				if y+height > 785 {
					if err := newPage(); err != nil {
						return nil, err
					}
					if err := pdf.SetFont("Inter", "", paragraph.size); err != nil {
						return nil, err
					}
				}
				pdf.SetXY(42, y)
				if err := pdf.Cell(&gopdf.Rect{W: 510, H: height}, line); err != nil {
					return nil, err
				}
				y += height
			}
		}
		y += 5
	}
	var output bytes.Buffer
	err := pdf.Write(&output)
	return output.Bytes(), err
}
