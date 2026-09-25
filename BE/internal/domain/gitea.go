package domain

import (
	"time"

	"github.com/google/uuid"
)

type GiteaInstance struct {
	ID           uuid.UUID `json:"id" db:"id"`
	WorkspaceID  uuid.UUID `json:"workspace_id" db:"workspace_id"`
	InstanceURL  string    `json:"instance_url" db:"instance_url"`
	AccountLogin string    `json:"account_login" db:"account_login"`
	AccessToken  string    `json:"-" db:"access_token"`
	InstalledBy  uuid.UUID `json:"installed_by" db:"installed_by"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type GiteaRepoModel struct {
	ID            uuid.UUID `json:"id" db:"id"`
	InstanceID    uuid.UUID `json:"instance_id" db:"instance_id"`
	WorkspaceID   uuid.UUID `json:"workspace_id" db:"workspace_id"`
	GiteaRepoID   int64     `json:"gitea_repo_id" db:"gitea_repo_id"`
	FullName      string    `json:"full_name" db:"full_name"`
	DefaultBranch string    `json:"default_branch" db:"default_branch"`
	IsActive      bool      `json:"is_active" db:"is_active"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type GiteaPullRequest struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	WorkspaceID     uuid.UUID  `json:"workspace_id" db:"workspace_id"`
	IssueID         *uuid.UUID `json:"issue_id" db:"issue_id"`
	GiteaRepoID     uuid.UUID  `json:"gitea_repo_id" db:"gitea_repo_id"`
	GiteaPRID       int64      `json:"gitea_pr_id" db:"gitea_pr_id"`
	Number          int        `json:"number" db:"number"`
	Title           string     `json:"title" db:"title"`
	State           string     `json:"state" db:"state"`
	AuthorLogin     string     `json:"author_login" db:"author_login"`
	AuthorAvatarURL *string    `json:"author_avatar_url" db:"author_avatar_url"`
	HTMLURL         string     `json:"html_url" db:"html_url"`
	HeadBranch      *string    `json:"head_branch" db:"head_branch"`
	BaseBranch      *string    `json:"base_branch" db:"base_branch"`
	Additions       int        `json:"additions" db:"additions"`
	Deletions       int        `json:"deletions" db:"deletions"`
	MergedAt        *time.Time `json:"merged_at" db:"merged_at"`
	ClosedAt        *time.Time `json:"closed_at" db:"closed_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type GiteaBranch struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	WorkspaceID  uuid.UUID  `json:"workspace_id" db:"workspace_id"`
	IssueID      *uuid.UUID `json:"issue_id" db:"issue_id"`
	GiteaRepoID  uuid.UUID  `json:"gitea_repo_id" db:"gitea_repo_id"`
	Name         string     `json:"name" db:"name"`
	HTMLURL      *string    `json:"html_url" db:"html_url"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

type GiteaCommit struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	WorkspaceID     uuid.UUID  `json:"workspace_id" db:"workspace_id"`
	IssueID         *uuid.UUID `json:"issue_id" db:"issue_id"`
	GiteaRepoID     uuid.UUID  `json:"gitea_repo_id" db:"gitea_repo_id"`
	PRID            *uuid.UUID `json:"pr_id" db:"pr_id"`
	SHA             string     `json:"sha" db:"sha"`
	Message         string     `json:"message" db:"message"`
	AuthorLogin     *string    `json:"author_login" db:"author_login"`
	AuthorAvatarURL *string    `json:"author_avatar_url" db:"author_avatar_url"`
	HTMLURL         string     `json:"html_url" db:"html_url"`
	CommittedAt     time.Time  `json:"committed_at" db:"committed_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
}

type GiteaAutoTransition struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	WorkspaceID    uuid.UUID  `json:"workspace_id" db:"workspace_id"`
	Event          string     `json:"event" db:"event"`
	TargetStatus   string     `json:"target_status" db:"target_status"`
	TargetStatusID *uuid.UUID `json:"target_status_id" db:"target_status_id"`
	IsActive       bool       `json:"is_active" db:"is_active"`
}

type GiteaOAuthConfig struct {
	ID            uuid.UUID `json:"id" db:"id"`
	WorkspaceID   uuid.UUID `json:"workspace_id" db:"workspace_id"`
	InstanceURL   string    `json:"instance_url" db:"instance_url"`
	WebhookSecret string    `json:"-" db:"webhook_secret"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}
