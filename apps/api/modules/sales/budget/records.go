// Package budget implements Business Operations customers, estimates and proposals.
package budget

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const BackupSchema = 12

type Command struct {
	OperationID      uuid.UUID `json:"operation_id"`
	ExpectedRevision int64     `json:"expected_revision"`
}

type CustomerInput struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Address    string `json:"address"`
	IsArchived bool   `json:"is_archived"`
}

type Customer struct {
	ID            uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Revision      int64     `json:"revision" gorm:"primaryKey;autoIncrement:false"`
	CustomerInput `gorm:"embedded"`
	CreatedAt     time.Time `json:"created_at"`
}

func (Customer) TableName() string { return "business_customers" }

func (c *Customer) BeforeCreate(_ *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

type MaterialInput struct {
	FilamentID uuid.UUID `json:"filament_id"`
	WeightG    float64   `json:"weight_g"`
	WasteG     *float64  `json:"waste_g"`
}

type Assumptions struct {
	PrinterID        *uuid.UUID      `json:"printer_id"`
	PrintTimeMinutes int             `json:"print_time_minutes"`
	LaborHours       float32         `json:"labor_hours"`
	Materials        []MaterialInput `json:"materials"`
	PostProcessCents int64           `json:"post_process_cents"`
	OtherCents       int64           `json:"other_cents"`
}

type ItemInput struct {
	ID          uuid.UUID   `json:"id"`
	ModelID     *uuid.UUID  `json:"model_id"`
	Name        string      `json:"name"`
	Quantity    int64       `json:"quantity"`
	Assumptions Assumptions `json:"assumptions"`
}

type Estimate struct {
	MaterialCents    int64 `json:"material_cents"`
	EnergyCents      int64 `json:"energy_cents"`
	WearCents        int64 `json:"wear_cents"`
	LaborCents       int64 `json:"labor_cents"`
	PostProcessCents int64 `json:"post_process_cents"`
	OtherCents       int64 `json:"other_cents"`
	UnitCents        int64 `json:"unit_cents"`
	TotalCents       int64 `json:"total_cents"`
}

type MaterialSource struct {
	Name       string  `json:"name"`
	PricePerKg float64 `json:"price_eur_per_kg"`
}

type Sources struct {
	CalculatorVersion int                `json:"calculator_version"`
	PrinterName       string             `json:"printer_name"`
	Config            map[string]float64 `json:"config"`
	Materials         []MaterialSource   `json:"materials"`
	CapturedAt        time.Time          `json:"captured_at"`
}

type Item struct {
	ItemInput
	Estimate Estimate `json:"estimate"`
	Sources  Sources  `json:"sources"`
}

type Header struct {
	Title      string     `json:"title"`
	CustomerID *uuid.UUID `json:"customer_id" gorm:"type:uuid;index"`
	IsArchived bool       `json:"is_archived"`
}

type Record struct {
	ID                 uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Revision           int64     `json:"revision" gorm:"primaryKey;autoIncrement:false"`
	Header             `gorm:"embedded"`
	Items              []Item    `json:"items" gorm:"serializer:json;not null"`
	EstimatedCostCents int64     `json:"estimated_cost_cents"`
	CreatedAt          time.Time `json:"created_at"`
}

func (Record) TableName() string { return "business_budgets" }

func (r *Record) BeforeCreate(_ *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	for i := range r.Items {
		if r.Items[i].ID == uuid.Nil {
			r.Items[i].ID = uuid.New()
		}
	}
	return nil
}

type operation struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	Resource   string    `gorm:"not null"`
	Hash       string    `gorm:"not null"`
	ResourceID uuid.UUID `gorm:"type:uuid;not null"`
	Revision   int64     `gorm:"not null"`
}

func (operation) TableName() string { return "business_budget_operations" }

func budgetTables() []any { return []any{&Customer{}, &Record{}, &operation{}} }

func Tables() []any { return append(budgetTables(), &Proposal{}, &OrderRevision{}) }
