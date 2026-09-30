import { api } from './client';
import type { WorkspaceStatus } from '$lib/types/status';

export function listStatuses(slug: string): Promise<WorkspaceStatus[]> {
	return api.get<WorkspaceStatus[]>(`/api/workspaces/${slug}/statuses`);
}

export function createStatus(
	slug: string,
	req: { name: string; category: string; color?: string; project_ids?: string[] }
): Promise<WorkspaceStatus> {
	return api.post<WorkspaceStatus>(`/api/workspaces/${slug}/statuses`, req);
}

export function updateStatus(
	slug: string,
	statusId: string,
	req: { name?: string; color?: string; position?: number; project_ids?: string[] }
): Promise<WorkspaceStatus> {
	return api.patch<WorkspaceStatus>(`/api/workspaces/${slug}/statuses/${statusId}`, req);
}

export function deleteStatus(slug: string, statusId: string): Promise<void> {
	return api.delete<void>(`/api/workspaces/${slug}/statuses/${statusId}`);
}
