import { api } from './client';

export type IssuesGroupByPreference = 'status' | 'priority' | 'assignee' | 'project' | 'none';

export interface PreferencesData {
	font_size: string;
	pointer_cursors: boolean;
	theme_mode: string;
	light_theme: string;
	dark_theme: string;
	workflow_sort_mode: string;
	workflow_sort_order: string[];
	recent_due_dates: string[];
	issues_group_by: IssuesGroupByPreference;
}

export function getPreferences(): Promise<PreferencesData> {
	return api.get<PreferencesData>('/api/preferences');
}

export function updatePreferences(data: Partial<PreferencesData>): Promise<PreferencesData> {
	return api.patch<PreferencesData>('/api/preferences', data);
}
