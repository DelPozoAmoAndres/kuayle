package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kuayle/kuayle-backend/internal/domain"
)

type CommentRepository struct {
	db *sqlx.DB
}

func NewCommentRepository(db *sqlx.DB) *CommentRepository {
	return &CommentRepository{db: db}
}

func (r *CommentRepository) Create(ctx context.Context, comment *domain.Comment) error {
	query := `INSERT INTO comments (id, issue_id, user_id, body, parent_id, author_login, author_avatar_url, gitea_comment_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		comment.ID, comment.IssueID, comment.UserID, comment.Body, comment.ParentID,
		comment.AuthorLogin, comment.AuthorAvatarURL, comment.GiteaCommentID,
	).Scan(&comment.CreatedAt, &comment.UpdatedAt)
}

func (r *CommentRepository) ListByIssue(ctx context.Context, issueID uuid.UUID) ([]domain.Comment, error) {
	var comments []domain.Comment
	err := r.db.SelectContext(ctx, &comments, `SELECT * FROM comments WHERE issue_id = $1 AND parent_id IS NULL ORDER BY created_at ASC`, issueID)
	return comments, err
}

func (r *CommentRepository) ListReplies(ctx context.Context, parentID uuid.UUID) ([]domain.Comment, error) {
	var comments []domain.Comment
	err := r.db.SelectContext(ctx, &comments, `SELECT * FROM comments WHERE parent_id = $1 ORDER BY created_at ASC`, parentID)
	return comments, err
}

func (r *CommentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Comment, error) {
	var comment domain.Comment
	err := r.db.GetContext(ctx, &comment, `SELECT * FROM comments WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// GetByGiteaCommentID returns the comment linked to a Gitea comment, or nil
// when no comment matches that Gitea comment ID.
func (r *CommentRepository) GetByGiteaCommentID(ctx context.Context, giteaCommentID int64) (*domain.Comment, error) {
	var comment domain.Comment
	err := r.db.GetContext(ctx, &comment, `SELECT * FROM comments WHERE gitea_comment_id = $1`, giteaCommentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// Update replaces the body of an existing comment.
func (r *CommentRepository) Update(ctx context.Context, id uuid.UUID, body string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE comments SET body = $2, updated_at = NOW() WHERE id = $1`, id, body)
	return err
}

// Delete removes a comment after deleting its replies.
func (r *CommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM comments WHERE parent_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM comments WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetGiteaCommentID stores the Gitea comment ID used to edit/delete the
// comment later on.
func (r *CommentRepository) SetGiteaCommentID(ctx context.Context, id uuid.UUID, giteaCommentID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE comments SET gitea_comment_id = $2, updated_at = NOW() WHERE id = $1`, id, giteaCommentID)
	return err
}

func (r *CommentRepository) Resolve(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE comments SET resolved_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *CommentRepository) Reopen(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE comments SET resolved_at = NULL WHERE id = $1`, id)
	return err
}
