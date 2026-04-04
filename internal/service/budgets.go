package service

import (
	"context"
	"errors"
	"fmt"

	"refina-transaction/internal/repository"
	"refina-transaction/internal/types/dto"
	"refina-transaction/internal/types/model"

	"github.com/google/uuid"
)

// Budget service error constants
const (
	errBudgetNotFound     = "budget not found [id=%s]: %w"
	errInvalidBudgetData  = "invalid budget data: %s"
	errCalculateSpentFail = "calculate spent: %w"
)

var (
	ErrBudgetScopeMissing  = errors.New("budget scope is required")
	ErrBudgetPeriodMissing = errors.New("budget period is required")
	ErrBudgetLimitInvalid  = errors.New("budget monthly limit must be greater than 0")
	ErrCategoryIDRequired  = errors.New("category_id is required for category-scoped budgets")
	ErrBudgetAlreadyExists = errors.New("budget already exists for this scope and period")
	ErrBudgetNotOwned      = errors.New("budget does not belong to user")
)

type BudgetsService interface {
	GetBudgets(ctx context.Context, userID, period string) ([]dto.BudgetResponse, error)
	CreateBudget(ctx context.Context, userID string, req dto.BudgetRequest) (dto.BudgetResponse, error)
	UpdateBudget(ctx context.Context, userID, budgetID string, req dto.BudgetUpdateRequest) (dto.BudgetResponse, error)
	DeleteBudget(ctx context.Context, userID, budgetID string) (dto.BudgetResponse, error)
	ResetBudget(ctx context.Context, userID, budgetID string) (dto.BudgetResponse, error)
}

type budgetsService struct {
	txManager  repository.TxManager
	budgetRepo repository.BudgetsRepository
}

func NewBudgetsService(txManager repository.TxManager, budgetRepo repository.BudgetsRepository) BudgetsService {
	return &budgetsService{
		txManager:  txManager,
		budgetRepo: budgetRepo,
	}
}

// GetBudgets retrieves all budgets for a user in a specific period
func (s *budgetsService) GetBudgets(ctx context.Context, userID, period string) ([]dto.BudgetResponse, error) {
	if period == "" {
		return nil, ErrBudgetPeriodMissing
	}

	budgets, err := s.budgetRepo.GetBudgetsByUserIDAndPeriod(ctx, nil, userID, period)
	if err != nil {
		return nil, fmt.Errorf("get budgets: %w", err)
	}

	responses := make([]dto.BudgetResponse, 0, len(budgets))
	for _, b := range budgets {
		responses = append(responses, toBudgetResponse(b))
	}

	return responses, nil
}

// CreateBudget creates a new budget for a user
func (s *budgetsService) CreateBudget(ctx context.Context, userID string, req dto.BudgetRequest) (dto.BudgetResponse, error) {
	// Validate request
	if err := validateBudgetRequest(req); err != nil {
		return dto.BudgetResponse{}, err
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf("invalid user id: %w", err)
	}

	var categoryID *uuid.UUID
	if req.CategoryID != nil && *req.CategoryID != "" {
		id, err := uuid.Parse(*req.CategoryID)
		if err != nil {
			return dto.BudgetResponse{}, fmt.Errorf("invalid category id: %w", err)
		}
		categoryID = &id
	}

	var walletID *uuid.UUID
	if req.WalletID != nil && *req.WalletID != "" {
		id, err := uuid.Parse(*req.WalletID)
		if err != nil {
			return dto.BudgetResponse{}, fmt.Errorf("invalid wallet id: %w", err)
		}
		walletID = &id
	}

	// Check for duplicate
	exists, err := s.budgetRepo.CheckDuplicateBudget(ctx, nil, userID, req.Scope, categoryID, walletID, req.Period)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf("check duplicate: %w", err)
	}
	if exists {
		return dto.BudgetResponse{}, ErrBudgetAlreadyExists
	}

	// Create budget model
	budget := model.UserBudgets{
		UserID:       userUUID,
		Scope:        req.Scope,
		CategoryID:   categoryID,
		WalletID:     walletID,
		MonthlyLimit: req.MonthlyLimit,
		Period:       req.Period,
	}

	created, err := s.budgetRepo.CreateBudget(ctx, nil, budget)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf("create budget: %w", err)
	}

	// Build response with current spent
	spent, err := s.budgetRepo.CalculateCurrentSpent(ctx, nil, userID, req.Period, categoryID, walletID)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf(errCalculateSpentFail, err)
	}

	return buildBudgetResponse(created, spent), nil
}

// UpdateBudget updates a budget's monthly limit (resets streak)
func (s *budgetsService) UpdateBudget(ctx context.Context, userID, budgetID string, req dto.BudgetUpdateRequest) (dto.BudgetResponse, error) {
	if req.MonthlyLimit <= 0 {
		return dto.BudgetResponse{}, ErrBudgetLimitInvalid
	}

	budget, err := s.budgetRepo.GetBudgetByID(ctx, nil, budgetID)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf(errBudgetNotFound, budgetID, err)
	}

	// Verify ownership
	if budget.UserID.String() != userID {
		return dto.BudgetResponse{}, ErrBudgetNotOwned
	}

	budget.MonthlyLimit = req.MonthlyLimit

	updated, err := s.budgetRepo.UpdateBudget(ctx, nil, budget)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf("update budget: %w", err)
	}

	// Calculate current spent
	spent, err := s.budgetRepo.CalculateCurrentSpent(ctx, nil, userID, updated.Period, updated.CategoryID, updated.WalletID)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf(errCalculateSpentFail, err)
	}

	return buildBudgetResponse(updated, spent), nil
}

// DeleteBudget soft-deletes a budget
func (s *budgetsService) DeleteBudget(ctx context.Context, userID, budgetID string) (dto.BudgetResponse, error) {
	budget, err := s.budgetRepo.GetBudgetByID(ctx, nil, budgetID)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf(errBudgetNotFound, budgetID, err)
	}

	// Verify ownership
	if budget.UserID.String() != userID {
		return dto.BudgetResponse{}, ErrBudgetNotOwned
	}

	deleted, err := s.budgetRepo.DeleteBudget(ctx, nil, budgetID)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf("delete budget: %w", err)
	}

	return buildBudgetResponse(deleted, 0), nil
}

// ResetBudget refreshes the current_spent calculation (no data change)
func (s *budgetsService) ResetBudget(ctx context.Context, userID, budgetID string) (dto.BudgetResponse, error) {
	budget, err := s.budgetRepo.GetBudgetByID(ctx, nil, budgetID)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf(errBudgetNotFound, budgetID, err)
	}

	// Verify ownership
	if budget.UserID.String() != userID {
		return dto.BudgetResponse{}, ErrBudgetNotOwned
	}

	// Calculate fresh current spent
	spent, err := s.budgetRepo.CalculateCurrentSpent(ctx, nil, userID, budget.Period, budget.CategoryID, budget.WalletID)
	if err != nil {
		return dto.BudgetResponse{}, fmt.Errorf(errCalculateSpentFail, err)
	}

	return buildBudgetResponse(budget, spent), nil
}

// validateBudgetRequest validates the budget request data
func validateBudgetRequest(req dto.BudgetRequest) error {
	if req.Scope == "" {
		return ErrBudgetScopeMissing
	}
	if req.Scope != "overall" && req.Scope != "category" {
		return fmt.Errorf(errInvalidBudgetData, "scope must be 'overall' or 'category'")
	}
	if req.Scope == "category" && (req.CategoryID == nil || *req.CategoryID == "") {
		return ErrCategoryIDRequired
	}
	if req.Period == "" {
		return ErrBudgetPeriodMissing
	}
	if req.MonthlyLimit <= 0 {
		return ErrBudgetLimitInvalid
	}
	return nil
}

// toBudgetResponse converts BudgetWithSpent to BudgetResponse
func toBudgetResponse(b model.BudgetWithSpent) dto.BudgetResponse {
	resp := dto.BudgetResponse{
		ID:           b.ID.String(),
		UserID:       b.UserID.String(),
		Scope:        b.Scope,
		MonthlyLimit: b.MonthlyLimit,
		CurrentSpent: b.CurrentSpent,
		Period:       b.Period,
		StreakCount:  b.StreakCount,
		StreakActive: b.StreakActive,
		CreatedAt:    b.CreatedAt,
		UpdatedAt:    b.UpdatedAt,
	}

	if b.CategoryID != nil {
		catID := b.CategoryID.String()
		resp.CategoryID = &catID
	}
	if b.CategoryName != nil {
		resp.CategoryName = b.CategoryName
	}

	if b.WalletID != nil {
		walletID := b.WalletID.String()
		resp.WalletID = &walletID
		resp.WalletScope = walletID
	} else {
		resp.WalletScope = "all"
	}

	return resp
}

// buildBudgetResponse builds a response from UserBudgets model
func buildBudgetResponse(b model.UserBudgets, currentSpent float64) dto.BudgetResponse {
	resp := dto.BudgetResponse{
		ID:           b.ID.String(),
		UserID:       b.UserID.String(),
		Scope:        b.Scope,
		MonthlyLimit: b.MonthlyLimit,
		CurrentSpent: currentSpent,
		Period:       b.Period,
		CreatedAt:    b.CreatedAt,
		UpdatedAt:    b.UpdatedAt,
	}

	if b.CategoryID != nil {
		catID := b.CategoryID.String()
		resp.CategoryID = &catID
	}
	if b.Category != nil {
		name := b.Category.Name
		resp.CategoryName = &name
	}

	if b.WalletID != nil {
		walletID := b.WalletID.String()
		resp.WalletID = &walletID
		resp.WalletScope = walletID
	} else {
		resp.WalletScope = "all"
	}

	if b.BudgetStreak != nil {
		resp.StreakCount = b.BudgetStreak.StreakCount
		resp.StreakActive = b.BudgetStreak.StreakActive
	}

	return resp
}
