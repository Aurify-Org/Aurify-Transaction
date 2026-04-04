package model

import (
	"time"

	"github.com/google/uuid"
)

// UserBudgets represents a user's budget configuration for a specific period
type UserBudgets struct {
	Base
	UserID       uuid.UUID  `gorm:"type:uuid;not null;index"`
	Scope        string     `gorm:"type:varchar(20);not null"` // "overall" or "category"
	CategoryID   *uuid.UUID `gorm:"type:uuid;index"`           // nullable, only for category scope
	WalletID     *uuid.UUID `gorm:"type:uuid"`                 // nullable, null = all wallets
	MonthlyLimit float64    `gorm:"type:decimal(18,2);not null"`
	Period       string     `gorm:"type:varchar(7);not null"` // format: "YYYY-MM"

	// Relations
	Category     *Categories    `gorm:"foreignKey:CategoryID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	BudgetStreak *BudgetStreaks `gorm:"foreignKey:BudgetID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

// TableName overrides the table name
func (UserBudgets) TableName() string {
	return "user_budgets"
}

// BudgetStreaks tracks the consecutive months a user stayed under budget
type BudgetStreaks struct {
	Base
	UserID              uuid.UUID `gorm:"type:uuid;not null;index"`
	BudgetID            uuid.UUID `gorm:"type:uuid;not null;index"`
	StreakCount         int       `gorm:"default:0"`
	StreakActive        bool      `gorm:"default:false"`
	LastEvaluatedPeriod *string   `gorm:"type:varchar(7)"` // format: "YYYY-MM"
}

// TableName overrides the table name
func (BudgetStreaks) TableName() string {
	return "budget_streaks"
}

// BudgetWithSpent is a view model combining budget config with calculated spending
type BudgetWithSpent struct {
	ID                  uuid.UUID
	UserID              uuid.UUID
	Scope               string
	CategoryID          *uuid.UUID
	CategoryName        *string
	WalletID            *uuid.UUID
	MonthlyLimit        float64
	Period              string
	CurrentSpent        float64 // calculated from transactions
	StreakCount         int
	StreakActive        bool
	LastEvaluatedPeriod *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
