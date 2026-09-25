package dto

import "time"

// --- Requests ---

type GiteaConnectRequest struct {
	InstanceURL   string `json:"instance_url" validate:"required"`
	AccessToken   string `json:"access_token" validate:"required"`
	WebhookSecret string `json:"webhook_secret"`
}

type LinkGiteaReposRequest struct {
	GiteaRepoIDs []int64 `json:"gitea_repo_ids" validate:"required,min=1"`
}

// --- Responses ---

type GiteaStatusResponse struct {
	Connected bool                       `json:"connected"`
	Instance  *GiteaInstanceResponse     `json:"instance,omitempty"`
	Repos     []GiteaRepoResponse        `json:"repos"`
}

type GiteaInstanceResponse struct {
	ID           string `json:"id"`
	InstanceURL  string `json:"instance_url"`
	AccountLogin string `json:"account_login"`
	CreatedAt    string `json:"created_at"`
}

type GiteaRepoResponse struct {
	ID            string `json:"id"`
	GiteaRepoID   int64  `json:"gitea_repo_id"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	IsActive      bool   `json:"is_active"`
}

type GiteaAvailableRepoResponse struct {
	GiteaRepoID   int64  `json:"gitea_repo_id"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Linked        bool   `json:"linked"`
}

type GiteaPullRequestResponse struct {
	ID              string     `json:"id"`
	Number          int        `json:"number"`
	Title           string     `json:"title"`
	State           string     `json:"state"`
	AuthorLogin     string     `json:"author_login"`
	AuthorAvatarURL string     `json:"author_avatar_url"`
	HTMLURL         string     `json:"html_url"`
	HeadBranch      string     `json:"head_branch"`
	BaseBranch      string     `json:"base_branch"`
	Additions       int        `json:"additions"`
	Deletions       int        `json:"deletions"`
	RepoFullName    string     `json:"repo_full_name"`
	MergedAt        *time.Time `json:"merged_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type GiteaBranchResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	HTMLURL      string `json:"html_url"`
	RepoFullName string `json:"repo_full_name"`
}

type GiteaCommitResponse struct {
	ID              string    `json:"id"`
	SHA             string    `json:"sha"`
	ShortSHA        string    `json:"short_sha"`
	Message         string    `json:"message"`
	AuthorLogin     string    `json:"author_login"`
	AuthorAvatarURL string    `json:"author_avatar_url"`
	HTMLURL         string    `json:"html_url"`
	RepoFullName    string    `json:"repo_full_name"`
	CommittedAt     time.Time `json:"committed_at"`
}

type GiteaIssueActivityResponse struct {
	PullRequests []GiteaPullRequestResponse `json:"pull_requests"`
	Branches     []GiteaBranchResponse      `json:"branches"`
	Commits      []GiteaCommitResponse      `json:"commits"`
}

type GiteaAutoTransitionResponse struct {
	Event          string  `json:"event"`
	TargetStatus   string  `json:"target_status"`
	TargetStatusID *string `json:"target_status_id"`
	IsActive       bool    `json:"is_active"`
}
