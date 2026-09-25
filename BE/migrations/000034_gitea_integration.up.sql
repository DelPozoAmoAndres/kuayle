-- Gitea instances (one per workspace)
CREATE TABLE gitea_instances (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL UNIQUE REFERENCES workspaces(id) ON DELETE CASCADE,
    instance_url        VARCHAR(1024) NOT NULL,
    account_login       VARCHAR(255) NOT NULL,
    access_token        TEXT NOT NULL,
    installed_by        UUID NOT NULL REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Linked repositories
CREATE TABLE gitea_repos (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id         UUID NOT NULL REFERENCES gitea_instances(id) ON DELETE CASCADE,
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    gitea_repo_id       BIGINT NOT NULL,
    full_name           VARCHAR(512) NOT NULL,
    default_branch      VARCHAR(255) NOT NULL DEFAULT 'main',
    is_active           BOOLEAN NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, gitea_repo_id)
);

CREATE INDEX idx_gitea_repos_workspace ON gitea_repos(workspace_id);

-- Pull requests linked to issues
CREATE TABLE gitea_pull_requests (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    issue_id            UUID REFERENCES issues(id) ON DELETE SET NULL,
    gitea_repo_id       UUID NOT NULL REFERENCES gitea_repos(id) ON DELETE CASCADE,
    gitea_pr_id         BIGINT NOT NULL,
    number              INT NOT NULL,
    title               VARCHAR(1024) NOT NULL,
    state               VARCHAR(20) NOT NULL CHECK (state IN ('open','closed','merged','draft')),
    author_login        VARCHAR(255) NOT NULL,
    author_avatar_url   TEXT,
    html_url            TEXT NOT NULL,
    head_branch         VARCHAR(512),
    base_branch         VARCHAR(512),
    additions           INT NOT NULL DEFAULT 0,
    deletions           INT NOT NULL DEFAULT 0,
    merged_at           TIMESTAMPTZ,
    closed_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(gitea_repo_id, gitea_pr_id)
);

CREATE INDEX idx_gitea_prs_issue ON gitea_pull_requests(issue_id);
CREATE INDEX idx_gitea_prs_workspace ON gitea_pull_requests(workspace_id);

-- Branches linked to issues
CREATE TABLE gitea_branches (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    issue_id            UUID REFERENCES issues(id) ON DELETE SET NULL,
    gitea_repo_id       UUID NOT NULL REFERENCES gitea_repos(id) ON DELETE CASCADE,
    name                VARCHAR(512) NOT NULL,
    html_url            TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(gitea_repo_id, name)
);

CREATE INDEX idx_gitea_branches_issue ON gitea_branches(issue_id);

-- Commits linked to issues
CREATE TABLE gitea_commits (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    issue_id            UUID REFERENCES issues(id) ON DELETE SET NULL,
    gitea_repo_id       UUID NOT NULL REFERENCES gitea_repos(id) ON DELETE CASCADE,
    pr_id               UUID REFERENCES gitea_pull_requests(id) ON DELETE SET NULL,
    sha                 VARCHAR(40) NOT NULL,
    message             TEXT NOT NULL,
    author_login        VARCHAR(255),
    author_avatar_url   TEXT,
    html_url            TEXT NOT NULL,
    committed_at        TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(gitea_repo_id, sha)
);

CREATE INDEX idx_gitea_commits_issue ON gitea_commits(issue_id);
CREATE INDEX idx_gitea_commits_pr ON gitea_commits(pr_id);

-- Auto-transition rules per workspace
CREATE TABLE gitea_auto_transitions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    event               VARCHAR(50) NOT NULL,
    target_status       VARCHAR(50) NOT NULL,
    target_status_id    UUID REFERENCES team_statuses(id) ON DELETE SET NULL,
    is_active           BOOLEAN NOT NULL DEFAULT true,
    UNIQUE(workspace_id, event)
);

-- OAuth config (stores webhook secret per workspace)
CREATE TABLE gitea_oauth_configs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL UNIQUE REFERENCES workspaces(id) ON DELETE CASCADE,
    instance_url        VARCHAR(1024) NOT NULL,
    webhook_secret      TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
