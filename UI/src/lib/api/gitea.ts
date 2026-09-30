import { api } from './client';
import type { GiteaStatus, GiteaAvailableRepo, GiteaIssueActivity, GiteaAutoTransition } from '$lib/types/gitea';

export function getGiteaStatus(slug: string): Promise<GiteaStatus> {
	return api.get<GiteaStatus>(`/api/workspaces/${slug}/gitea/status`);
}

export function connectGitea(slug: string, data: { instance_url: string; access_token: string; webhook_secret?: string }): Promise<{ id: string; instance_url: string; account_login: string }> {
	return api.post(`/api/workspaces/${slug}/gitea/connect`, data);
}

export function disconnectGitea(slug: string): Promise<void> {
	return api.delete<void>(`/api/workspaces/${slug}/gitea/disconnect`);
}

export function listGiteaRepos(slug: string): Promise<GiteaAvailableRepo[]> {
	return api.get<GiteaAvailableRepo[]>(`/api/workspaces/${slug}/gitea/repos`);
}

export function linkGiteaRepos(slug: string, giteaRepoIds: number[]): Promise<void> {
	return api.post<void>(`/api/workspaces/${slug}/gitea/repos`, { gitea_repo_ids: giteaRepoIds });
}

export function unlinkGiteaRepo(slug: string, id: string): Promise<void> {
	return api.delete<void>(`/api/workspaces/${slug}/gitea/repos/${id}`);
}

export function getIssueGiteaActivity(slug: string, identifier: string): Promise<GiteaIssueActivity> {
	return api.get<GiteaIssueActivity>(`/api/workspaces/${slug}/issues/${identifier}/gitea`);
}

export function listGiteaAutoTransitions(slug: string): Promise<GiteaAutoTransition[]> {
	return api.get<GiteaAutoTransition[]>(`/api/workspaces/${slug}/gitea/auto-transitions`);
}

export function updateGiteaAutoTransitions(slug: string, transitions: GiteaAutoTransition[]): Promise<void> {
	return api.patch<void>(`/api/workspaces/${slug}/gitea/auto-transitions`, { transitions });
}

export interface GiteaUserTokenState {
	has_gitea_token: boolean;
	gitea_login?: string | null;
}

export function getGiteaUserToken(slug: string): Promise<GiteaUserTokenState> {
	return api.get<GiteaUserTokenState>(`/api/workspaces/${slug}/gitea/user-token`);
}

/** Stores the user's own Gitea token; an empty token clears it. */
export function setGiteaUserToken(slug: string, token: string): Promise<GiteaUserTokenState> {
	return api.put<GiteaUserTokenState>(`/api/workspaces/${slug}/gitea/user-token`, { token });
}
