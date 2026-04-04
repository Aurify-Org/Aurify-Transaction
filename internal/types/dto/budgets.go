package dto

import "time"

// BudgetRequest represents the request to create or update a budget
type BudgetRequest struct {
	Scope        string  `json:"scope"`       // "overall" or "category"
	CategoryID   *string `json:"category_id"` // optional
	WalletID     *string `json:"wallet_id"`   // optional, null = all wallets
	MonthlyLimit float64 `json:"monthly_limit"`
	Period       string  `json:"period"` // format: "YYYY-MM"
}

// BudgetUpdateRequest represents the request to update a budget
type BudgetUpdateRequest struct {
	MonthlyLimit float64 `json:"monthly_limit"`
}

// BudgetResponse represents the response for a budget
type BudgetResponse struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Scope        string    `json:"scope"`         // "overall" or "category"
	CategoryID   *string   `json:"category_id"`   // nullable
	CategoryName *string   `json:"category_name"` // nullable, populated from join
	WalletScope  string    `json:"wallet_scope"`  // "all" or wallet_id
	WalletID     *string   `json:"wallet_id"`     // nullable
	WalletName   *string   `json:"wallet_name"`   // nullable, populated externally
	MonthlyLimit float64   `json:"monthly_limit"`
	CurrentSpent float64   `json:"current_spent"` // calculated from transactions
	Period       string    `json:"period"`        // format: "YYYY-MM"
	StreakCount  int       `json:"streak_count"`
	StreakActive bool      `json:"streak_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GetBudgetsQuery represents query parameters for getting budgets
type GetBudgetsQuery struct {
	UserID string `json:"user_id"`
	Period string `json:"period"` // format: "YYYY-MM"
}
