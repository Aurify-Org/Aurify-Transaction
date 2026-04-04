-- +goose Up
-- +goose StatementBegin

-- user_budgets: Store budget configuration per user
CREATE TABLE user_budgets (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    created_at timestamptz DEFAULT now(),
    updated_at timestamptz DEFAULT now(),
    deleted_at timestamptz,
    user_id uuid NOT NULL,
    scope VARCHAR(20) NOT NULL CHECK (scope IN ('overall', 'category')),
    category_id uuid REFERENCES categories(id) ON DELETE SET NULL ON UPDATE CASCADE,
    wallet_id uuid,  -- NULL = all wallets
    monthly_limit numeric(18,2) NOT NULL CHECK (monthly_limit > 0),
    period VARCHAR(7) NOT NULL  -- format: "YYYY-MM"
);

-- Prevent duplicate budgets for the same scope/category/wallet/period combination
CREATE UNIQUE INDEX idx_user_budgets_unique_overall ON user_budgets(user_id, scope, wallet_id, period) 
    WHERE scope = 'overall' AND deleted_at IS NULL;
CREATE UNIQUE INDEX idx_user_budgets_unique_category ON user_budgets(user_id, scope, category_id, wallet_id, period) 
    WHERE scope = 'category' AND deleted_at IS NULL;

-- Query indexes
CREATE INDEX idx_user_budgets_user_period ON user_budgets(user_id, period) WHERE deleted_at IS NULL;
CREATE INDEX idx_user_budgets_category ON user_budgets(category_id) WHERE category_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_user_budgets_wallet ON user_budgets(wallet_id) WHERE wallet_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_user_budgets_deleted_at ON user_budgets(deleted_at);

-- budget_streaks: Track consecutive months under budget
CREATE TABLE budget_streaks (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    created_at timestamptz DEFAULT now(),
    updated_at timestamptz DEFAULT now(),
    deleted_at timestamptz,
    budget_id uuid NOT NULL REFERENCES user_budgets(id) ON DELETE CASCADE ON UPDATE CASCADE,
    streak_count integer NOT NULL DEFAULT 0 CHECK (streak_count >= 0),
    streak_active boolean NOT NULL DEFAULT true,
    last_evaluated_period VARCHAR(7)  -- format: "YYYY-MM"
);

CREATE UNIQUE INDEX idx_budget_streaks_budget ON budget_streaks(budget_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_budget_streaks_deleted_at ON budget_streaks(deleted_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS budget_streaks CASCADE;
DROP TABLE IF EXISTS user_budgets CASCADE;
-- +goose StatementEnd
