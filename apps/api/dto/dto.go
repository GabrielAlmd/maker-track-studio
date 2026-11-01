package dto

import (
	"time"

	"github.com/GabrielAlmd/maker-track-studio/apps/api/models"
	"github.com/google/uuid"
)

type FilamentPurchaseDTO struct {
	ID           uuid.UUID    `json:"id"`
	FilamentID   uuid.UUID    `json:"filament_id"`
	Price        float64      `json:"price"`
	Amount       float64      `json:"amount"`
	PurchaseDate time.Time    `json:"purchase_date"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	Filament     *FilamentDTO `json:"filament,omitempty"` // optional, for expanded view
}

type FilamentDTO struct {
	ID             uuid.UUID             `json:"id"`
	Brand          string                `json:"brand"`
	Type           string                `json:"type"`
	Color          string                `json:"color"`
	AvgPrice       float64               `json:"avg_price"`
	PersonalScore  int                   `json:"personal_score"`
	TimesUsed      int                   `json:"times_used"`
	TimesPurchased int                   `json:"times_purchased"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
	Purchases      []FilamentPurchaseDTO `json:"purchases,omitempty"`
	PrintJobs      []PrintJobFilamentDTO `json:"print_jobs,omitempty"`
}

type PrintJobFilamentDTO struct {
	ID         uuid.UUID            `json:"id"`
	PrintJobID uuid.UUID            `json:"print_job_id"`
	FilamentID uuid.UUID            `json:"filament_id"`
	WeightUsed float64              `json:"weight_used"`
	Filament   FilamentReferenceDTO `json:"filament"`
	PrintJob   *PrintJobDTO         `json:"print_job,omitempty"` // optional, for expanded view
}

// ModelReferenceDTO is the sole representation of a print job's model link.
// ID is only present when it can safely be used to navigate to an existing model.
type ModelReferenceDTO struct {
	State string     `json:"state"` // assigned, not_assigned, deleted
	ID    *uuid.UUID `json:"id,omitempty"`
	Name  string     `json:"name"`
}

// FilamentReferenceDTO describes the target of a persisted usage row.
type FilamentReferenceDTO struct {
	State string     `json:"state"` // available, unavailable
	ID    *uuid.UUID `json:"id,omitempty"`
	Name  string     `json:"name"`
}

type PrintJobCostStateDTO struct {
	State  string   `json:"state"` // calculated, not_calculated
	Reason string   `json:"reason,omitempty"`
	Total  *float64 `json:"total,omitempty"`
}

type PrintJobDTO struct {
	PrinterID        *uuid.UUID                  `json:"printer_id,omitempty"`
	PrinterCost      *models.PrinterCostSnapshot `json:"printer_cost,omitempty"`
	ID               uuid.UUID                   `json:"id"`
	ModelID          *uuid.UUID                  `json:"model_id,omitempty"`
	PrintDate        time.Time                   `json:"print_date"`
	UnitsPrinted     int                         `json:"units_printed"`
	LaborHours       float32                     `json:"labor_hours"`
	PrintTimeMinutes *int                        `json:"print_time_minutes,omitempty"`
	EstimatedCost    *float64                    `json:"estimated_cost,omitempty"`
	Model            ModelReferenceDTO           `json:"model"`
	Cost             PrintJobCostStateDTO        `json:"cost"`
	CreatedAt        time.Time                   `json:"created_at"`
	UpdatedAt        time.Time                   `json:"updated_at"`
	Filaments        []PrintJobFilamentDTO       `json:"filaments,omitempty"`
	// Model omitted to avoid circular reference
}

type ModelDTO struct {
	ID             uuid.UUID     `json:"id"`
	Name           string        `json:"name"`
	ImageURL       string        `json:"image_url"`
	MaterialWeight float64       `json:"material_weight"`
	PrintTime      int           `json:"print_time"`
	WasteMaterial  float64       `json:"waste_material"`
	IsMulticolor   bool          `json:"is_multicolor"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	PrintJobs      []PrintJobDTO `json:"print_jobs,omitempty"`
	PrintJobCount  int           `json:"print_job_count"`
}

type AppConfigDTO struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PrintJobCostDTO struct {
	PrintJobID      uuid.UUID `json:"print_job_id"`
	State           string    `json:"state"`
	Reason          string    `json:"reason,omitempty"`
	FilamentCost    *float64  `json:"filament_cost,omitempty"`
	ElectricityCost *float64  `json:"electricity_cost,omitempty"`
	WearCost        *float64  `json:"wear_cost,omitempty"`
	LaborCost       *float64  `json:"labor_cost,omitempty"`
	TotalCost       *float64  `json:"total_cost,omitempty"`
}

type PrintJobStatsDTO struct {
	TotalJobs            int     `json:"total_jobs"`
	CalculatedJobs       int     `json:"calculated_jobs"`
	TotalUnitsPrinted    int     `json:"total_units_printed"`
	TotalLaborHours      float64 `json:"total_labor_hours"`
	TotalFilamentWeightG float64 `json:"total_filament_weight_g"`
	TotalFilamentCost    float64 `json:"total_filament_cost"`
	TotalElectricityCost float64 `json:"total_electricity_cost"`
	TotalWearCost        float64 `json:"total_wear_cost"`
	TotalLaborCost       float64 `json:"total_labor_cost"`
	TotalCost            float64 `json:"total_cost"`
}

type DashboardRecentPrintJobDTO struct {
	ID            uuid.UUID            `json:"id"`
	PrinterID     *uuid.UUID           `json:"printer_id,omitempty"`
	PrinterName   string               `json:"printer_name"`
	ModelID       *uuid.UUID           `json:"model_id,omitempty"`
	ModelName     string               `json:"model_name"`
	PrintDate     time.Time            `json:"print_date"`
	UnitsPrinted  int                  `json:"units_printed"`
	LaborHours    float32              `json:"labor_hours"`
	EstimatedCost *float64             `json:"estimated_cost,omitempty"`
	Model         ModelReferenceDTO    `json:"model"`
	Cost          PrintJobCostStateDTO `json:"cost"`
}

type DashboardSummaryDTO struct {
	TotalModels     int                          `json:"total_models"`
	TotalFilaments  int                          `json:"total_filaments"`
	TotalPrintJobs  int                          `json:"total_print_jobs"`
	TotalPurchases  int                          `json:"total_purchases"`
	Expenses        DashboardExpensesDTO         `json:"expenses"`
	CostStats       PrintJobStatsDTO             `json:"cost_stats"`
	RecentPrintJobs []DashboardRecentPrintJobDTO `json:"recent_print_jobs"`
}

// DashboardExpensesDTO reports recorded acquisition spend. These values are
// deliberately separate from PrintJobStatsDTO, which contains calculated
// production costs and may include depreciation of the same equipment.
type DashboardExpensesDTO struct {
	PrinterCost    float64 `json:"printer_cost"`
	FilamentCost   float64 `json:"filament_cost"`
	SparePartsCost float64 `json:"spare_parts_cost"`
	AdditionalCost float64 `json:"additional_cost"`
	OtherCost      float64 `json:"other_cost"`
	TotalExpenses  float64 `json:"total_expenses"`
}

// Auth DTOs
type LoginRequest struct {
	UserIdentifier string `json:"user_identifier" binding:"required"`
	Password       string `json:"password" binding:"required"`
}

type LoginResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int64     `json:"expires_in"` // seconds
	TokenType    string    `json:"token_type"` // "Bearer"
	CreatedAt    time.Time `json:"created_at"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type RefreshTokenResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresIn   int64     `json:"expires_in"` // seconds
	TokenType   string    `json:"token_type"` // "Bearer"
	CreatedAt   time.Time `json:"created_at"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type LogoutResponse struct {
	Message string `json:"message"`
}

type TokenAuditDTO struct {
	ID           uuid.UUID  `json:"id"`
	TokenID      *uuid.UUID `json:"token_id,omitempty"`
	Action       string     `json:"action"`
	Status       string     `json:"status"`
	IPAddress    string     `json:"ip_address"`
	UserAgent    string     `json:"user_agent"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type AuthTokenDTO struct {
	ID             uuid.UUID  `json:"id"`
	UserIdentifier string     `json:"user_identifier"`
	TokenType      string     `json:"token_type"`
	ExpiresAt      time.Time  `json:"expires_at"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	RevokedReason  string     `json:"revoked_reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type RevokeTokenRequest struct {
	Reason string `json:"reason"`
}
