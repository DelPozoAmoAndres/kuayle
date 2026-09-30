import type { WorkspaceStatus, StatusCategory } from '$lib/types/status';
import { CATEGORY_ORDER } from '$lib/types/status';
import { listStatuses } from '$lib/api/statuses';

const CATEGORY_ICONS: Record<StatusCategory, string> = {
	backlog: 'CircleDashed',
	unstarted: 'Circle',
	started: 'Loader',
	completed: 'CheckCircle2',
	cancelled: 'XCircle',
};

const CATEGORY_COLORS: Record<StatusCategory, string> = {
	backlog: 'text-[var(--color-text-tertiary)]',
	unstarted: 'text-[var(--color-text-secondary)]',
	started: 'text-yellow-500',
	completed: 'text-[var(--color-success)]',
	cancelled: 'text-[var(--color-text-tertiary)]',
};

class StatusesState {
	statuses = $state<WorkspaceStatus[]>([]);
	loading = $state(false);
	private loadedSlug = '';

	/** Statuses sorted by position. */
	statusOrder = $derived(
		[...this.statuses].sort((a, b) => a.position - b.position)
	);

	/** Lookup by status ID. */
	statusById = $derived(
		new Map(this.statuses.map((s) => [s.id, s]))
	);

	/** Labels map: id → name. */
	statusLabels = $derived(
		Object.fromEntries(this.statuses.map((s) => [s.id, s.name])) as Record<string, string>
	);

	/** Statuses grouped by category, each group sorted by position. */
	statusesByCategory = $derived.by(() => {
		const groups: Record<string, WorkspaceStatus[]> = {};
		for (const cat of CATEGORY_ORDER) {
			groups[cat] = [];
		}
		for (const s of this.statusOrder) {
			if (groups[s.category]) {
				groups[s.category].push(s);
			}
		}
		return groups;
	});

	/** Load the workspace statuses (a single request per workspace). */
	async load(workspaceSlug: string) {
		if (!workspaceSlug) return;
		if (this.loadedSlug === workspaceSlug && this.statuses.length > 0) return;
		this.loading = true;
		try {
			this.statuses = await listStatuses(workspaceSlug);
			this.loadedSlug = workspaceSlug;
		} catch {
			this.statuses = [];
		} finally {
			this.loading = false;
		}
	}

	/** Force reload (e.g. after creating/deleting a status). */
	async reload(workspaceSlug: string) {
		this.loadedSlug = '';
		await this.load(workspaceSlug);
	}

	/** Get statuses visible to a specific project. If no project, returns all. */
	statusesForProject(projectId?: string | null): WorkspaceStatus[] {
		if (!projectId) return this.statusOrder;
		return this.statusOrder.filter((s) => {
			// Default statuses are always visible
			if (s.is_default) return true;
			// Custom statuses: visible if no project restriction OR project is in the list
			if (!s.project_ids || s.project_ids.length === 0) return true;
			return s.project_ids.includes(projectId);
		});
	}

	/** Get CSS color class for a status (uses custom color or falls back to category default). */
	getColorClass(status: WorkspaceStatus): string {
		if (status.color) return '';
		return CATEGORY_COLORS[status.category] ?? CATEGORY_COLORS.backlog;
	}

	/** Get inline color style if custom color is set. */
	getColorStyle(status: WorkspaceStatus): string {
		if (status.color) return `color: ${status.color}`;
		return '';
	}

	/** Get the category icon name for a status. */
	getCategoryIcon(category: StatusCategory): string {
		return CATEGORY_ICONS[category] ?? CATEGORY_ICONS.backlog;
	}

	/** Find the default status for a category. */
	defaultForCategory(category: StatusCategory): WorkspaceStatus | undefined {
		return this.statuses.find((s) => s.category === category && s.is_default);
	}

	clear() {
		this.statuses = [];
		this.loadedSlug = '';
	}
}

export const statusesState = new StatusesState();
