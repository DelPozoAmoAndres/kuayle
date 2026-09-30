export interface User {
	id: string;
	email: string;
	name: string;
	display_name: string;
	avatar_url: string | null;
	is_sysadmin: boolean;
	gitea_login?: string | null;
	has_gitea_token?: boolean;
}

export interface LoginRequest {
	email: string;
	password: string;
}

export interface RegisterRequest {
	email: string;
	password: string;
	name: string;
}

export interface UpdateProfileRequest {
	name?: string;
	display_name?: string;
	avatar_url?: string | null;
}
