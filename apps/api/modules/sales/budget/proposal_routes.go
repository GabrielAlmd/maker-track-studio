package budget

import (
	"net/http"
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/modules"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProposalView struct {
	Proposal
	ProfitCents   int64    `json:"profit_cents"`
	MarginPercent *float64 `json:"margin_percent"`
	MarkupPercent *float64 `json:"markup_percent"`
}

type proposalSummary struct {
	ID                 uuid.UUID     `json:"id"`
	Revision           int64         `json:"revision"`
	CustomerName       string        `json:"customer_name"`
	Terms              ProposalTerms `json:"terms" gorm:"serializer:json"`
	EstimatedCostCents int64         `json:"estimated_cost_cents"`
	NetCents           int64         `json:"net_cents"`
	CreatedAt          time.Time     `json:"created_at"`
	IssuedAt           *time.Time    `json:"issued_at"`
	SupersedesRevision int64         `json:"supersedes_revision,omitempty"`
}

// ProposalDocument is an allowlisted contract for customer-facing output.
type ProposalDocument struct {
	ID                 uuid.UUID              `json:"id"`
	Revision           int64                  `json:"revision"`
	Currency           string                 `json:"currency"`
	CustomerName       string                 `json:"customer_name"`
	CustomerAddress    string                 `json:"customer_address"`
	Terms              ProposalTerms          `json:"terms"`
	Lines              []ProposalDocumentLine `json:"lines"`
	LineDiscountCents  int64                  `json:"line_discount_cents"`
	OrderDiscountCents int64                  `json:"order_discount_cents"`
	NetCents           int64                  `json:"net_cents"`
	IssuedAt           time.Time              `json:"issued_at"`
}

type ProposalDocumentLine struct {
	UnitPriceCents     int64  `json:"unit_price_cents"`
	Description        string `json:"description"`
	Quantity           int64  `json:"quantity"`
	LineDiscountCents  int64  `json:"line_discount_cents"`
	OrderDiscountCents int64  `json:"order_discount_cents"`
	NetCents           int64  `json:"net_cents"`
}

func proposalView(record Proposal) ProposalView {
	profit := record.NetCents - record.EstimatedCostCents
	return ProposalView{
		Proposal: record, ProfitCents: profit,
		MarginPercent: percentOf(profit, record.NetCents),
		MarkupPercent: percentOf(profit, record.EstimatedCostCents),
	}
}

func proposalDocument(record Proposal) ProposalDocument {
	document := ProposalDocument{
		ID: record.ID, Revision: record.Revision, Currency: "EUR", CustomerName: record.CustomerName,
		CustomerAddress: record.CustomerAddress, Terms: record.Terms, IssuedAt: *record.IssuedAt,
		LineDiscountCents: record.LineDiscountCents, OrderDiscountCents: record.OrderDiscountCents, NetCents: record.NetCents,
		Lines: make([]ProposalDocumentLine, 0, len(record.Lines)),
	}
	for _, line := range record.Lines {
		document.Lines = append(document.Lines, ProposalDocumentLine{
			Description: line.Description, Quantity: line.Quantity, UnitPriceCents: line.UnitPriceCents, LineDiscountCents: line.LineDiscountCents,
			OrderDiscountCents: line.OrderDiscountCents, NetCents: line.NetCents,
		})
	}
	return document
}

func registerProposalRoutes(internal *gin.RouterGroup, db *gorm.DB) {
	bind := func(factory func(*gorm.DB) gin.HandlerFunc) gin.HandlerFunc {
		return modules.WithRequestDB(db, factory)
	}
	internal.GET("/proposals", bind(list[Proposal, proposalSummary]))
	internal.GET("/proposals/:id", bind(proposalDetail))
	internal.GET("/proposals/:id/document", bind(issuedProposal(false)))
	internal.GET("/proposals/:id/document/pdf", bind(issuedProposal(true)))
	internal.GET("/proposals/:id/revisions", bind(proposalHistory))
	internal.POST("/proposals", bind(command(false, proposalCommand(writeProposal))))
	internal.PUT("/proposals/:id", bind(command(true, proposalCommand(writeProposal))))
	internal.POST("/proposals/:id/issue", bind(command(true, proposalCommand(issueProposal))))
}

func proposalCommand[Input any](write func(*gorm.DB, uuid.UUID, Input) (Proposal, error)) func(*gorm.DB, uuid.UUID, Input) (ProposalView, error) {
	return func(db *gorm.DB, id uuid.UUID, input Input) (ProposalView, error) {
		result, err := write(db, id, input)
		return proposalView(result), err
	}
}

func proposalDetail(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			reply(c, 0, nil, err)
			return
		}
		revision, err := positiveQuery(c, "revision", 0, maxSafeInteger)
		if err != nil {
			reply(c, 0, nil, err)
			return
		}
		record, err := readRevision[Proposal](db, id, revision)
		if err != nil {
			reply(c, 0, nil, err)
			return
		}
		reply(c, http.StatusOK, proposalView(record), nil)
	}
}

func issuedProposal(pdf bool) func(*gorm.DB) gin.HandlerFunc {
	return func(db *gorm.DB) gin.HandlerFunc {
		return func(c *gin.Context) {
			id, err := pathID(c)
			if err != nil {
				reply(c, 0, nil, err)
				return
			}
			revision, err := positiveQuery(c, "revision", 0, maxSafeInteger)
			if err == nil && revision == 0 {
				err = invalid("revision", "Select an issued revision to export")
			}
			if err != nil {
				reply(c, 0, nil, err)
				return
			}
			record, err := readRevision[Proposal](db, id, revision)
			if err == nil && record.IssuedAt == nil {
				err = missing(gorm.ErrRecordNotFound, "revision", "Issued proposal revision was not found")
			}
			if err != nil {
				reply(c, 0, nil, err)
				return
			}
			document := proposalDocument(record)
			if pdf {
				downloadProposalPDF(c, document)
				return
			}
			reply(c, http.StatusOK, document, nil)
		}
	}
}

func proposalHistory(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			reply(c, 0, nil, err)
			return
		}
		if err := db.Select("id").Take(&Proposal{}, "id = ?", id).Error; err != nil {
			reply(c, 0, nil, missing(err, "id", "Proposal was not found"))
			return
		}
		paginate[proposalSummary](c, db.Model(&Proposal{}).Where("id = ?", id).Order("revision DESC"))
	}
}
