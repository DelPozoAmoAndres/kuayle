export interface ViewFilter {
	[key: string]: string | undefined;
	view_scope?: ViewScope;
	status?: string;
	status_type?: string;
	priority?: string;
	assignee?: string;
	creator?: string;
	project?: string;
	label?: string;
	sub_issues?: string;
	search?: string;
	due_before?: string;
	due_after?: string;
	group_by?: string;
	sort?: string;
	order?: string;
}

export type ViewScope = 'personal' | 'workspace';
export type ViewLayout = 'list' | 'board';

export interface View {
	id: string;
	workspace_id: string;
	creator_id: string;
	name: string;
	description: string | null;
	filters: ViewFilter;
	is_shared: boolean;
	created_at: string;
	updated_at: string;
}

export interface CreateViewRequest {
	name: string;
	description?: string;
	filters: ViewFilter;
	is_shared?: boolean;
}

export interface UpdateViewRequest {
	name?: string;
	description?: string;
	filters?: ViewFilter;
	is_shared?: boolean;
}

export function getViewScope(view: Pick<View, 'filters' | 'is_shared'>): ViewScope {
	const scope = view.filters?.view_scope;
	if (scope === 'personal' || scope === 'workspace') return scope;
	return view.is_shared ? 'workspace' : 'personal';
}

export function isPersonalView(view: Pick<View, 'filters' | 'is_shared'>): boolean {
	return getViewScope(view) === 'personal';
}

export function isWorkspaceView(view: Pick<View, 'filters' | 'is_shared'>): boolean {
	return getViewScope(view) === 'workspace';
}

export function viewMetadata(filters: ViewFilter): ViewFilter {
	const metadata: ViewFilter = {};
	if (filters.view_scope) metadata.view_scope = filters.view_scope;
	return metadata;
}

export function issueFilters(filters: ViewFilter): ViewFilter {
	const { view_scope, ...rest } = filters;
	return rest;
}
