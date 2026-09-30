package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kuayle/kuayle-backend/internal/domain"
)

// StatusRepository manages workspace statuses. Rows live in the legacy
// `team_statuses` table; `team_id` is never written anymore.
type StatusRepository struct {
	db *sqlx.DB
}

func NewStatusRepository(db *sqlx.DB) *StatusRepository {
	return &StatusRepository{db: db}
}

func (r *StatusRepository) Create(ctx context.Context, status *domain.WorkspaceStatus) error {
	query := `INSERT INTO team_statuses (id, workspace_id, team_id, name, slug, category, color, position, is_default)
		VALUES ($1, $2, NULL, $3, $4, $5, $6, $7, $8) RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		status.ID, status.WorkspaceID, status.Name, status.Slug,
		status.Category, status.Color, status.Position, status.IsDefault,
	).Scan(&status.CreatedAt, &status.UpdatedAt)
}

func (r *StatusRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.WorkspaceStatus, error) {
	var status domain.WorkspaceStatus
	err := r.db.GetContext(ctx, &status, `SELECT * FROM team_statuses WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &status, err
}

func (r *StatusRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.WorkspaceStatus, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query, args, err := sqlx.In(`SELECT * FROM team_statuses WHERE id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	query = r.db.Rebind(query)
	var statuses []domain.WorkspaceStatus
	err = r.db.SelectContext(ctx, &statuses, query, args...)
	return statuses, err
}

func (r *StatusRepository) GetByWorkspaceAndSlug(ctx context.Context, workspaceID uuid.UUID, slug string) (*domain.WorkspaceStatus, error) {
	var status domain.WorkspaceStatus
	err := r.db.GetContext(ctx, &status,
		`SELECT * FROM team_statuses WHERE workspace_id = $1 AND slug = $2`, workspaceID, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &status, err
}

func (r *StatusRepository) ListByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]domain.WorkspaceStatus, error) {
	var statuses []domain.WorkspaceStatus
	err := r.db.SelectContext(ctx, &statuses,
		`SELECT * FROM team_statuses WHERE workspace_id = $1 ORDER BY position`, workspaceID)
	return statuses, err
}

func (r *StatusRepository) Update(ctx context.Context, status *domain.WorkspaceStatus) error {
	query := `UPDATE team_statuses SET name = $1, color = $2, position = $3, updated_at = NOW()
		WHERE id = $4 RETURNING updated_at`
	return r.db.QueryRowContext(ctx, query, status.Name, status.Color, status.Position, status.ID).Scan(&status.UpdatedAt)
}

func (r *StatusRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM team_statuses WHERE id = $1`, id)
	return err
}

func (r *StatusRepository) NextPosition(ctx context.Context, workspaceID uuid.UUID) (int, error) {
	var pos int
	err := r.db.GetContext(ctx, &pos,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM team_statuses WHERE workspace_id = $1`, workspaceID)
	return pos, err
}
