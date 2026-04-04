package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"refina-transaction/internal/types/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrBudgetNotFound      = errors.New("budget not found")
	ErrBudgetAlreadyExists = errors.New("budget already exists for this scope and period")
)

type BudgetsRepository interface {
	GetBudgetsByUserIDAndPeriod(ctx context.Context, tx Transaction, userID, period string) ([]model.BudgetWithSpent, error)
	GetBudgetByID(ctx context.Context, tx Transaction, id string) (model.UserBudgets, error)
	CreateBudget(ctx context.Context, tx Transaction, budget model.UserBudgets) (model.UserBudgets, error)
	UpdateBudget(ctx context.Context, tx Transaction, budget model.UserBudgets) (model.UserBudgets, error)
	DeleteBudget(ctx context.Context, tx Transaction, id string) (model.UserBudgets, error)
	GetBudgetStreak(ctx context.Context, tx Transaction, budgetID string) (model.BudgetStreaks, error)
	CreateBudgetStreak(ctx context.Context, tx Transaction, streak model.BudgetStreaks) (model.BudgetStreaks, error)
	UpdateBudgetStreak(ctx context.Context, tx Transaction, streak model.BudgetStreaks) (model.BudgetStreaks, error)
	ResetBudgetStreak(ctx context.Context, tx Transaction, budgetID string) error
	CheckDuplicateBudget(ctx context.Context, tx Transaction, userID, scope string, categoryID, walletID *uuid.UUID, period string) (bool, error)
	CalculateCurrentSpent(ctx context.Context, tx Transaction, userID, period string, categoryID, walletID *uuid.UUID) (float64, error)
}

type budgetsRepository struct {
	db *gorm.DB
}

func NewBudgetsRepository(db *gorm.DB) BudgetsRepository {
	return &budgetsRepository{db}
}

func (r *budgetsRepository) getDB(ctx context.Context, tx Transaction) (*gorm.DB, error) {
	if tx != nil {
		gormTx, ok := tx.(*GormTx)
		if !ok {
			return nil, errors.New("invalid transaction type")
		}
		return gormTx.db.WithContext(ctx), nil
	}
	return r.db.WithContext(ctx), nil
}

// GetBudgetsByUserIDAndPeriod returns all budgets for a user in a specific period with calculated current_spent
func (r *budgetsRepository) GetBudgetsByUserIDAndPeriod(ctx context.Context, tx Transaction, userID, period string) ([]model.BudgetWithSpent, error) {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return nil, err
	}

	var budgets []model.UserBudgets
	if err := db.
		Preload("Category").
		Preload("BudgetStreak").
		Where("user_id = ? AND period = ?", userID, period).
		Find(&budgets).Error; err != nil {
		return nil, fmt.Errorf("get budgets by user and period: %w", err)
	}

	// Build result with calculated current_spent
	result := make([]model.BudgetWithSpent, 0, len(budgets))
	for _, b := range budgets {
		spent, err := r.CalculateCurrentSpent(ctx, tx, userID, period, b.CategoryID, b.WalletID)
		if err != nil {
			return nil, fmt.Errorf("calculate current spent: %w", err)
		}

		bws := model.BudgetWithSpent{
			ID:           b.ID,
			UserID:       b.UserID,
			Scope:        b.Scope,
			CategoryID:   b.CategoryID,
			WalletID:     b.WalletID,
			MonthlyLimit: b.MonthlyLimit,
			Period:       b.Period,
			CurrentSpent: spent,
			CreatedAt:    b.CreatedAt,
			UpdatedAt:    b.UpdatedAt,
		}

		// Populate category name if available
		if b.Category != nil {
			name := b.Category.Name
			bws.CategoryName = &name
		}

		// Populate streak info if available
		if b.BudgetStreak != nil {
			bws.StreakCount = b.BudgetStreak.StreakCount
			bws.StreakActive = b.BudgetStreak.StreakActive
			bws.LastEvaluatedPeriod = b.BudgetStreak.LastEvaluatedPeriod
		}

		result = append(result, bws)
	}

	return result, nil
}

// GetBudgetByID returns a budget by its ID
func (r *budgetsRepository) GetBudgetByID(ctx context.Context, tx Transaction, id string) (model.UserBudgets, error) {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return model.UserBudgets{}, err
	}

	var budget model.UserBudgets
	if err := db.
		Preload("Category").
		Preload("BudgetStreak").
		Where("id = ?", id).
		First(&budget).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.UserBudgets{}, ErrBudgetNotFound
		}
		return model.UserBudgets{}, fmt.Errorf("get budget by id: %w", err)
	}

	return budget, nil
}

// CreateBudget creates a new budget and its associated streak record
func (r *budgetsRepository) CreateBudget(ctx context.Context, tx Transaction, budget model.UserBudgets) (model.UserBudgets, error) {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return model.UserBudgets{}, err
	}

	if err := db.Create(&budget).Error; err != nil {
		return model.UserBudgets{}, fmt.Errorf("create budget: %w", err)
	}

	// Create associated streak record
	streak := model.BudgetStreaks{
		BudgetID:     budget.ID,
		StreakCount:  0,
		StreakActive: false,
	}
	if err := db.Create(&streak).Error; err != nil {
		return model.UserBudgets{}, fmt.Errorf("create budget streak: %w", err)
	}

	// Reload with associations
	return r.GetBudgetByID(ctx, tx, budget.ID.String())
}

// UpdateBudget updates a budget (limit change resets streak)
func (r *budgetsRepository) UpdateBudget(ctx context.Context, tx Transaction, budget model.UserBudgets) (model.UserBudgets, error) {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return model.UserBudgets{}, err
	}

	if err := db.Model(&budget).Updates(map[string]any{
		"monthly_limit": budget.MonthlyLimit,
		"updated_at":    time.Now(),
	}).Error; err != nil {
		return model.UserBudgets{}, fmt.Errorf("update budget: %w", err)
	}

	// Reset streak when limit changes
	if err := r.ResetBudgetStreak(ctx, tx, budget.ID.String()); err != nil {
		return model.UserBudgets{}, fmt.Errorf("reset streak after update: %w", err)
	}

	return r.GetBudgetByID(ctx, tx, budget.ID.String())
}

// DeleteBudget soft-deletes a budget
func (r *budgetsRepository) DeleteBudget(ctx context.Context, tx Transaction, id string) (model.UserBudgets, error) {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return model.UserBudgets{}, err
	}

	budget, err := r.GetBudgetByID(ctx, tx, id)
	if err != nil {
		return model.UserBudgets{}, err
	}

	// Reset streak before deletion
	if err := r.ResetBudgetStreak(ctx, tx, id); err != nil {
		return model.UserBudgets{}, fmt.Errorf("reset streak before delete: %w", err)
	}

	if err := db.Delete(&budget).Error; err != nil {
		return model.UserBudgets{}, fmt.Errorf("delete budget: %w", err)
	}

	return budget, nil
}

// GetBudgetStreak returns the streak record for a budget
func (r *budgetsRepository) GetBudgetStreak(ctx context.Context, tx Transaction, budgetID string) (model.BudgetStreaks, error) {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return model.BudgetStreaks{}, err
	}

	var streak model.BudgetStreaks
	if err := db.Where("budget_id = ?", budgetID).First(&streak).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.BudgetStreaks{}, fmt.Errorf("streak not found for budget %s", budgetID)
		}
		return model.BudgetStreaks{}, fmt.Errorf("get budget streak: %w", err)
	}

	return streak, nil
}

// CreateBudgetStreak creates a new streak record
func (r *budgetsRepository) CreateBudgetStreak(ctx context.Context, tx Transaction, streak model.BudgetStreaks) (model.BudgetStreaks, error) {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return model.BudgetStreaks{}, err
	}

	if err := db.Create(&streak).Error; err != nil {
		return model.BudgetStreaks{}, fmt.Errorf("create budget streak: %w", err)
	}

	return streak, nil
}

// UpdateBudgetStreak updates a streak record
func (r *budgetsRepository) UpdateBudgetStreak(ctx context.Context, tx Transaction, streak model.BudgetStreaks) (model.BudgetStreaks, error) {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return model.BudgetStreaks{}, err
	}

	if err := db.Save(&streak).Error; err != nil {
		return model.BudgetStreaks{}, fmt.Errorf("update budget streak: %w", err)
	}

	return streak, nil
}

// ResetBudgetStreak resets the streak to 0 and sets active to false
func (r *budgetsRepository) ResetBudgetStreak(ctx context.Context, tx Transaction, budgetID string) error {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return err
	}

	if err := db.Model(&model.BudgetStreaks{}).
		Where("budget_id = ?", budgetID).
		Updates(map[string]any{
			"streak_count":  0,
			"streak_active": false,
			"updated_at":    time.Now(),
		}).Error; err != nil {
		return fmt.Errorf("reset budget streak: %w", err)
	}

	return nil
}

// CheckDuplicateBudget checks if a budget already exists for the given scope and period
func (r *budgetsRepository) CheckDuplicateBudget(ctx context.Context, tx Transaction, userID, scope string, categoryID, walletID *uuid.UUID, period string) (bool, error) {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return false, err
	}

	query := db.Model(&model.UserBudgets{}).
		Where("user_id = ? AND scope = ? AND period = ?", userID, scope, period)

	if categoryID != nil {
		query = query.Where("category_id = ?", categoryID)
	} else {
		query = query.Where("category_id IS NULL")
	}

	if walletID != nil {
		query = query.Where("wallet_id = ?", walletID)
	} else {
		query = query.Where("wallet_id IS NULL")
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check duplicate budget: %w", err)
	}

	return count > 0, nil
}

// CalculateCurrentSpent calculates the total spent for a budget in the given period
func (r *budgetsRepository) CalculateCurrentSpent(ctx context.Context, tx Transaction, userID, period string, categoryID, walletID *uuid.UUID) (float64, error) {
	db, err := r.getDB(ctx, tx)
	if err != nil {
		return 0, err
	}

	// Parse period to get date range
	startDate, err := time.Parse("2006-01", period)
	if err != nil {
		return 0, fmt.Errorf("invalid period format: %w", err)
	}
	endDate := startDate.AddDate(0, 1, 0)

	// Build query for expense transactions
	query := db.Model(&model.Transactions{}).
		Select("COALESCE(SUM(amount), 0)").
		Joins("JOIN categories ON transactions.category_id = categories.id").
		Where("categories.type = ?", "expense").
		Where("transactions.transaction_date >= ? AND transactions.transaction_date < ?", startDate, endDate).
		Where("transactions.deleted_at IS NULL")

	// Filter by wallet if specified
	if walletID != nil {
		query = query.Where("transactions.wallet_id = ?", walletID)
	}

	// Filter by category if specified (for category-scoped budgets)
	if categoryID != nil {
		query = query.Where("transactions.category_id = ?", categoryID)
	}

	var total float64
	if err := query.Scan(&total).Error; err != nil {
		return 0, fmt.Errorf("calculate current spent: %w", err)
	}

	return total, nil
}
