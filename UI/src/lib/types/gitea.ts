export interface GiteaInstance {
	id: string;
	instance_url: string;
	account_login: string;
	created_at: string;
}

export interface GiteaRepo {
	id: string;
	gitea_repo_id: number;
	full_name: string;
	default_branch: string;
	is_active: boolean;
}

export interface GiteaAvailableRepo {
	gitea_repo_id: number;
	full_name: string;
	default_branch: string;
	private: boolean;
	linked: boolean;
}

export interface GiteaStatus {
	connected: boolean;
	instance?: GiteaInstance;
	repos: GiteaRepo[];
	synced_issues_count?: number;
	sync_errors?: string[];
}

export interface GiteaPullRequest {
	id: string;
	number: number;
	title: string;
	state: 'open' | 'closed' | 'merged';
	author_login: string;
	author_avatar_url: string;
	html_url: string;
	head_branch: string;
	base_branch: string;
	additions: number;
	deletions: number;
	repo_full_name: string;
	merged_at: string | null;
	created_at: string;
	updated_at: string;
}

export interface GiteaBranch {
	id: string;
	name: string;
	html_url: string;
	repo_full_name: string;
}

export interface GiteaCommit {
	id: string;
	sha: string;
	short_sha: string;
	message: string;
	author_login: string;
	author_avatar_url: string;
	html_url: string;
	repo_full_name: string;
	committed_at: string;
}

export interface GiteaIssueActivity {
	pull_requests: GiteaPullRequest[];
	branches: GiteaBranch[];
	commits: GiteaCommit[];
}

export interface GiteaAutoTransition {
	event: string;
	target_status: string;
	target_status_id: string | null;
	is_active: boolean;
}
