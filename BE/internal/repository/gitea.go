package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kuayle/kuayle-backend/internal/domain"
)

type GiteaRepository struct {
	db *sqlx.DB
}

func NewGiteaRepository(db *sqlx.DB) *GiteaRepository {
	return &GiteaRepository{db: db}
}

// --- Instances ---

func (r *GiteaRepository) CreateInstance(ctx context.Context, inst *domain.GiteaInstance) error {
	query := `INSERT INTO gitea_instances (id, workspace_id, instance_url, account_login, access_token, installed_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		inst.ID, inst.WorkspaceID, inst.InstanceURL, inst.AccountLogin, inst.AccessToken, inst.InstalledBy,
	).Scan(&inst.CreatedAt, &inst.UpdatedAt)
}

func (r *GiteaRepository) GetInstanceByWorkspace(ctx context.Context, workspaceID uuid.UUID) (*domain.GiteaInstance, error) {
	var inst domain.GiteaInstance
	err := r.db.GetContext(ctx, &inst, `SELECT * FROM gitea_instances WHERE workspace_id = $1`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &inst, err
}

func (r *GiteaRepository) UpdateInstanceToken(ctx context.Context, id uuid.UUID, accessToken string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE gitea_instances SET access_token = $1, updated_at = NOW() WHERE id = $2`, accessToken, id)
	return err
}

func (r *GiteaRepository) DeleteInstance(ctx context.Context, workspaceID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM gitea_instances WHERE workspace_id = $1`, workspaceID)
	return err
}

// --- Repos ---

func (r *GiteaRepository) CreateRepo(ctx context.Context, repo *domain.GiteaRepoModel) error {
	query := `INSERT INTO gitea_repos (id, instance_id, workspace_id, gitea_repo_id, full_name, default_branch, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (workspace_id, gitea_repo_id) DO UPDATE SET
			instance_id=EXCLUDED.instance_id,full_name=EXCLUDED.full_name,
			default_branch=EXCLUDED.default_branch,is_active=EXCLUDED.is_active
		RETURNING created_at`
	return r.db.QueryRowContext(ctx, query,
		repo.ID, repo.InstanceID, repo.WorkspaceID, repo.GiteaRepoID, repo.FullName, repo.DefaultBranch, repo.IsActive,
	).Scan(&repo.CreatedAt)
}

func (r *GiteaRepository) ListReposByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]domain.GiteaRepoModel, error) {
	var repos []domain.GiteaRepoModel
	err := r.db.SelectContext(ctx, &repos, `SELECT * FROM gitea_repos WHERE workspace_id = $1 AND is_active = true ORDER BY full_name`, workspaceID)
	return repos, err
}

func (r *GiteaRepository) GetRepoByGiteaID(ctx context.Context, workspaceID uuid.UUID, giteaRepoID int64) (*domain.GiteaRepoModel, error) {
	var repo domain.GiteaRepoModel
	err := r.db.GetContext(ctx, &repo, `SELECT * FROM gitea_repos WHERE workspace_id = $1 AND gitea_repo_id = $2`, workspaceID, giteaRepoID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &repo, err
}

func (r *GiteaRepository) DeleteRepo(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM gitea_repos WHERE id = $1`, id)
	return err
}

// --- Pull Requests ---

func (r *GiteaRepository) UpsertPullRequest(ctx context.Context, pr *domain.GiteaPullRequest) error {
	query := `INSERT INTO gitea_pull_requests (id, workspace_id, issue_id, gitea_repo_id, gitea_pr_id, number, title, state, author_login, author_avatar_url, html_url, head_branch, base_branch, additions, deletions, merged_at, closed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (gitea_repo_id, gitea_pr_id) DO UPDATE SET
			issue_id = COALESCE(EXCLUDED.issue_id, gitea_pull_requests.issue_id),
			title = EXCLUDED.title, state = EXCLUDED.state, additions = EXCLUDED.additions, deletions = EXCLUDED.deletions,
			merged_at = EXCLUDED.merged_at, closed_at = EXCLUDED.closed_at, updated_at = NOW()
		RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		pr.ID, pr.WorkspaceID, pr.IssueID, pr.GiteaRepoID, pr.GiteaPRID, pr.Number,
		pr.Title, pr.State, pr.AuthorLogin, pr.AuthorAvatarURL, pr.HTMLURL,
		pr.HeadBranch, pr.BaseBranch, pr.Additions, pr.Deletions, pr.MergedAt, pr.ClosedAt,
	).Scan(&pr.CreatedAt, &pr.UpdatedAt)
}

func (r *GiteaRepository) ListPRsByIssue(ctx context.Context, issueID uuid.UUID) ([]domain.GiteaPullRequest, error) {
	var prs []domain.GiteaPullRequest
	err := r.db.SelectContext(ctx, &prs, `SELECT * FROM gitea_pull_requests WHERE issue_id = $1 ORDER BY created_at DESC`, issueID)
	return prs, err
}

// --- Branches ---

func (r *GiteaRepository) UpsertBranch(ctx context.Context, b *domain.GiteaBranch) error {
	query := `INSERT INTO gitea_branches (id, workspace_id, issue_id, gitea_repo_id, name, html_url)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (gitea_repo_id, name) DO UPDATE SET
			issue_id = COALESCE(EXCLUDED.issue_id, gitea_branches.issue_id)
		RETURNING created_at`
	return r.db.QueryRowContext(ctx, query,
		b.ID, b.WorkspaceID, b.IssueID, b.GiteaRepoID, b.Name, b.HTMLURL,
	).Scan(&b.CreatedAt)
}

func (r *GiteaRepository) ListBranchesByIssue(ctx context.Context, issueID uuid.UUID) ([]domain.GiteaBranch, error) {
	var branches []domain.GiteaBranch
	err := r.db.SelectContext(ctx, &branches, `SELECT * FROM gitea_branches WHERE issue_id = $1 ORDER BY created_at DESC`, issueID)
	return branches, err
}

// --- Commits ---

func (r *GiteaRepository) UpsertCommit(ctx context.Context, c *domain.GiteaCommit) error {
	query := `INSERT INTO gitea_commits (id, workspace_id, issue_id, gitea_repo_id, pr_id, sha, message, author_login, author_avatar_url, html_url, committed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (gitea_repo_id, sha) DO UPDATE SET
			issue_id = COALESCE(EXCLUDED.issue_id, gitea_commits.issue_id),
			pr_id = COALESCE(EXCLUDED.pr_id, gitea_commits.pr_id)
		RETURNING created_at`
	return r.db.QueryRowContext(ctx, query,
		c.ID, c.WorkspaceID, c.IssueID, c.GiteaRepoID, c.PRID, c.SHA,
		c.Message, c.AuthorLogin, c.AuthorAvatarURL, c.HTMLURL, c.CommittedAt,
	).Scan(&c.CreatedAt)
}

func (r *GiteaRepository) ListCommitsByIssue(ctx context.Context, issueID uuid.UUID) ([]domain.GiteaCommit, error) {
	var commits []domain.GiteaCommit
	err := r.db.SelectContext(ctx, &commits, `SELECT * FROM gitea_commits WHERE issue_id = $1 ORDER BY committed_at DESC`, issueID)
	return commits, err
}

// --- Auto Transitions ---

func (r *GiteaRepository) UpsertAutoTransition(ctx context.Context, t *domain.GiteaAutoTransition) error {
	query := `INSERT INTO gitea_auto_transitions (id, workspace_id, event, target_status, target_status_id, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (workspace_id, event) DO UPDATE SET
			target_status = EXCLUDED.target_status, target_status_id = EXCLUDED.target_status_id, is_active = EXCLUDED.is_active`
	_, err := r.db.ExecContext(ctx, query, t.ID, t.WorkspaceID, t.Event, t.TargetStatus, t.TargetStatusID, t.IsActive)
	return err
}

func (r *GiteaRepository) ListAutoTransitions(ctx context.Context, workspaceID uuid.UUID) ([]domain.GiteaAutoTransition, error) {
	var transitions []domain.GiteaAutoTransition
	err := r.db.SelectContext(ctx, &transitions, `SELECT * FROM gitea_auto_transitions WHERE workspace_id = $1`, workspaceID)
	return transitions, err
}

func (r *GiteaRepository) GetAutoTransitionByEvent(ctx context.Context, workspaceID uuid.UUID, event string) (*domain.GiteaAutoTransition, error) {
	var t domain.GiteaAutoTransition
	err := r.db.GetContext(ctx, &t, `SELECT * FROM gitea_auto_transitions WHERE workspace_id = $1 AND event = $2 AND is_active = true`, workspaceID, event)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}

// --- Queries joining repos for display ---

type GiteaPRWithRepo struct {
	domain.GiteaPullRequest
	RepoFullName string `db:"repo_full_name"`
}

type GiteaBranchWithRepo struct {
	domain.GiteaBranch
	RepoFullName string `db:"repo_full_name"`
}

type GiteaCommitWithRepo struct {
	domain.GiteaCommit
	RepoFullName string `db:"repo_full_name"`
}

func (r *GiteaRepository) ListPRsWithRepoByIssue(ctx context.Context, issueID uuid.UUID) ([]GiteaPRWithRepo, error) {
	var prs []GiteaPRWithRepo
	query := `SELECT p.*, r.full_name AS repo_full_name FROM gitea_pull_requests p JOIN gitea_repos r ON r.id = p.gitea_repo_id WHERE p.issue_id = $1 ORDER BY p.created_at DESC`
	err := r.db.SelectContext(ctx, &prs, query, issueID)
	return prs, err
}

func (r *GiteaRepository) ListBranchesWithRepoByIssue(ctx context.Context, issueID uuid.UUID) ([]GiteaBranchWithRepo, error) {
	var branches []GiteaBranchWithRepo
	query := `SELECT b.*, r.full_name AS repo_full_name FROM gitea_branches b JOIN gitea_repos r ON r.id = b.gitea_repo_id WHERE b.issue_id = $1 ORDER BY b.created_at DESC`
	err := r.db.SelectContext(ctx, &branches, query, issueID)
	return branches, err
}

func (r *GiteaRepository) ListCommitsWithRepoByIssue(ctx context.Context, issueID uuid.UUID) ([]GiteaCommitWithRepo, error) {
	var commits []GiteaCommitWithRepo
	query := `SELECT c.*, r.full_name AS repo_full_name FROM gitea_commits c JOIN gitea_repos r ON r.id = c.gitea_repo_id WHERE c.issue_id = $1 ORDER BY c.committed_at DESC LIMIT 50`
	err := r.db.SelectContext(ctx, &commits, query, issueID)
	return commits, err
}

// --- OAuth Config (stores webhook secret) ---

func (r *GiteaRepository) CreateOAuthConfig(ctx context.Context, cfg *domain.GiteaOAuthConfig) error {
	query := `INSERT INTO gitea_oauth_configs (id, workspace_id, instance_url, webhook_secret)
		VALUES ($1, $2, $3, $4) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query,
		cfg.ID, cfg.WorkspaceID, cfg.InstanceURL, cfg.WebhookSecret,
	).Scan(&cfg.CreatedAt)
}

func (r *GiteaRepository) GetOAuthConfigByWorkspace(ctx context.Context, workspaceID uuid.UUID) (*domain.GiteaOAuthConfig, error) {
	var cfg domain.GiteaOAuthConfig
	err := r.db.GetContext(ctx, &cfg, `SELECT * FROM gitea_oauth_configs WHERE workspace_id = $1`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &cfg, err
}

func (r *GiteaRepository) DeleteOAuthConfig(ctx context.Context, workspaceID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM gitea_oauth_configs WHERE workspace_id = $1`, workspaceID)
	return err
}

// --- Webhook helper queries ---

func (r *GiteaRepository) ListInstances(ctx context.Context) ([]domain.GiteaInstance, error) {
	var instances []domain.GiteaInstance
	err := r.db.SelectContext(ctx, &instances, `SELECT * FROM gitea_instances`)
	return instances, err
}

func (r *GiteaRepository) GetOAuthConfigByInstanceURL(ctx context.Context, instanceURL string) (*domain.GiteaOAuthConfig, error) {
	var cfg domain.GiteaOAuthConfig
	err := r.db.GetContext(ctx, &cfg, `SELECT * FROM gitea_oauth_configs WHERE instance_url = $1`, instanceURL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &cfg, err
}
