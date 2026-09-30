package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/kuayle/kuayle-backend/internal/domain"
)

var ErrDuplicateEmail = errors.New("email already exists")

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `INSERT INTO users (id, email, name, display_name, password_hash) VALUES ($1, $2, $3, $4, $5) RETURNING created_at, updated_at`
	err := r.db.QueryRowContext(ctx, query, user.ID, user.Email, user.Name, user.DisplayName, user.PasswordHash).Scan(&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateEmail
		}
		return err
	}
	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user, `SELECT * FROM users WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &user, err
}

func (r *UserRepository) ListByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.User, error) {
	users := make(map[uuid.UUID]domain.User, len(ids))
	if len(ids) == 0 {
		return users, nil
	}

	query, args, err := sqlx.In(`SELECT * FROM users WHERE id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	query = r.db.Rebind(query)

	var rows []domain.User
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	for _, user := range rows {
		users[user.ID] = user
	}
	return users, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user, `SELECT * FROM users WHERE email = $1`, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &user, err
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	query := `UPDATE users SET name = $1, display_name = $2, avatar_url = $3, gitea_login = $4, updated_at = NOW() WHERE id = $5 RETURNING updated_at`
	return r.db.QueryRowContext(ctx, query, user.Name, user.DisplayName, user.AvatarURL, user.GiteaLogin, user.ID).Scan(&user.UpdatedAt)
}

// GetWorkspaceMemberByGiteaLogin resolves a Gitea login to a workspace member.
// The login is not globally unique across Gitea instances, so it is always
// scoped to the membership of the given workspace. It returns (nil, nil) when
// no member of the workspace has that Gitea login linked.
func (r *UserRepository) GetWorkspaceMemberByGiteaLogin(ctx context.Context, workspaceID uuid.UUID, login string) (*domain.User, error) {
	var user domain.User
	query := `SELECT u.* FROM users u
		INNER JOIN workspace_members m ON m.user_id = u.id
		WHERE m.workspace_id = $1 AND u.gitea_login IS NOT NULL AND LOWER(u.gitea_login) = LOWER($2)
		LIMIT 1`
	err := r.db.GetContext(ctx, &user, query, workspaceID, login)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
