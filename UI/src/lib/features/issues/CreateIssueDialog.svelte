<script lang="ts">
	import { untrack } from 'svelte';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import * as Popover from '$lib/components/ui/popover';
	import { Switch } from '$lib/components/ui/switch';
	import type { Project } from '$lib/types/project';
	import type { Label } from '$lib/types/label';
	import type { WorkspaceMember } from '$lib/types/workspace';
	import type { Issue, IssueStatus, IssuePriority } from '$lib/types/issue';
	import { getPriorityLabel } from '$lib/types/issue';
	import type { IssueTemplate } from '$lib/types/issue';
	import { statusesState } from './statuses.state.svelte';
	import { getIssueCreateDefaults } from './create-defaults';
	import type { StatusCategory } from '$lib/types/status';
	import RichEditor from '$lib/components/shared/RichEditor.svelte';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import IssueStatusIcon from './IssueStatusIcon.svelte';
	import IssuePriorityIcon from './IssuePriorityIcon.svelte';
	import { issuesState } from './issues.state.svelte';
	import DatePickerPopover from '$lib/components/shared/DatePickerPopover.svelte';
	import { StatusSelector, PrioritySelector, AssigneeSelector, LabelSelector, ProjectSelector } from './selectors';
	import { listTemplates } from '$lib/api/issue-templates';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale, setLocale } from '$lib/paraglide/runtime.js';
	import {
		User,
		Tag,
		FolderKanban,
		FileText,
		AlertTriangle
	} from 'lucide-svelte';

	let {
		open = $bindable(false),
		slug = '',
		projects = [],
		labels = [],
		members = [],
		defaultStatus,
		defaultStatusId,
		defaultPriority,
		defaultAssigneeId,
		defaultProjectId,
		defaultAssigneeIds,
		defaultLabelIds,
		defaultDueDate,
		defaultTitle,
		parentIssue = null,
		onlabelcreated,
		onbulkcreate,
		onsubmit
	}: {
		open: boolean;
		slug?: string;
		projects?: Project[];
		labels?: Label[];
		members?: WorkspaceMember[];
		defaultStatus?: IssueStatus;
		defaultStatusId?: string;
		defaultPriority?: IssuePriority;
		defaultAssigneeId?: string;
		defaultProjectId?: string | null;
		defaultAssigneeIds?: string[];
		defaultLabelIds?: string[];
		defaultDueDate?: string | null;
		defaultTitle?: string;
		parentIssue?: Issue | null;
		onlabelcreated?: (label: Label) => void;
		onbulkcreate?: (titles: string[]) => void;
		onsubmit: (req: {
			title: string;
			description?: string;
			status?: IssueStatus;
			status_id?: string;
			priority: IssuePriority;
			project_id: string;
			assignee_id?: string;
			assignee_ids?: string[];
			label_ids?: string[];
			parent_id?: string;
			due_date?: string;
		}) => void;
	} = $props();

	let title = $state('');
	let description = $state('');
	let statusId = $state<string>('');
	let priority = $state<IssuePriority>(0);
	let projectId = $state<string | null>(null);
	let assigneeIds = $state<string[]>([]);
	let labelIds = $state<string[]>([]);
	let dueDate = $state<string | null>(null);
	let createMore = $state(false);

	let templates = $state<IssueTemplate[]>([]);
	let selectedTemplate = $state<IssueTemplate | null>(null);
	let templateOpen = $state(false);
	let descriptionVersion = $state(0);

	let statusOpen = $state(false);
	let priorityOpen = $state(false);
	let projectOpen = $state(false);
	let assigneeOpen = $state(false);
	let labelsOpen = $state(false);

	function validProject(id: string | null | undefined): string | null {
		if (!id) return null;
		return projects.some((p) => p.id === id) ? id : null;
	}

	function validMemberIds(ids: string[] | undefined): string[] {
		if (!ids) return [];
		const memberIds = new Set(members.map((m) => m.user_id));
		return ids.filter((id) => memberIds.has(id));
	}

	function validLabelIds(ids: string[] | undefined): string[] {
		if (!ids) return [];
		const availableLabelIds = new Set(labels.map((l) => l.id));
		return ids.filter((id) => availableLabelIds.has(id));
	}

	function applyDefaultStatus(preferredStatusId?: string) {
		const available = statusesState.statusesForProject(projectId);
		statusId =
			available.find((s) => s.id === preferredStatusId)?.id ??
			available.find((s) => s.category === 'backlog' && s.is_default)?.id ??
			available[0]?.id ??
			'';
	}

	function loadStatuses(preferredStatusId?: string) {
		if (!slug) {
			applyDefaultStatus(preferredStatusId);
			return;
		}

		statusesState.load(slug).then(() => {
			if (open) {
				applyDefaultStatus(preferredStatusId);
			}
		});
	}

	function resetForm() {
		const savedDefaults = getIssueCreateDefaults(slug);
		title = defaultTitle ?? '';
		description = '';
		descriptionVersion++;
		selectedTemplate = null;
		priority = defaultPriority ?? savedDefaults.priority ?? 0;
		// Project is mandatory: fall back to the first project of the workspace.
		projectId =
			(defaultProjectId !== undefined ? validProject(defaultProjectId) : validProject(savedDefaults.projectId)) ??
			projects[0]?.id ??
			null;
		statusId = '';
		loadStatuses(defaultStatusId ?? savedDefaults.statusId);
		assigneeIds = defaultAssigneeIds ? validMemberIds(defaultAssigneeIds) : (defaultAssigneeId ? validMemberIds([defaultAssigneeId]) : validMemberIds(savedDefaults.assigneeIds));
		labelIds = defaultLabelIds ? validLabelIds(defaultLabelIds) : validLabelIds(savedDefaults.labelIds);
		dueDate = defaultDueDate !== undefined ? defaultDueDate : savedDefaults.dueDate ?? null;
		if (slug) listTemplates(slug).then(t => templates = t).catch(() => {});
	}

	$effect(() => {
		if (open) {
			untrack(() => resetForm());
		}
	});

	let selectedProject = $derived(projects.find((p) => p.id === projectId));
	let selectedAssignees = $derived(members.filter((m) => assigneeIds.includes(m.user_id)));
	let selectedLabels = $derived(labels.filter((l) => labelIds.includes(l.id)));
	let hasProject = $derived(projects.length > 0 && !!projectId);

	const priorityValues: IssuePriority[] = [0, 1, 2, 3, 4];

	const selectedStatus = $derived(statusesState.statusById.get(statusId));

	function handleSubmit() {
		if (!title.trim() || !projectId) return;
		onsubmit({
			title: title.trim(),
			description: description.trim() || undefined,
			status_id: statusId || undefined,
			priority,
			project_id: projectId,
			assignee_ids: assigneeIds.length > 0 ? assigneeIds : undefined,
			label_ids: labelIds.length > 0 ? labelIds : undefined,
			parent_id: parentIssue?.id,
			due_date: dueDate || undefined
		});
		if (createMore) {
			title = selectedTemplate?.title || '';
			description = selectedTemplate?.description ?? '';
			descriptionVersion++;
		} else {
			open = false;
		}
	}

	function titleFromListLine(line: string): string {
		return line
			.replace(/^\s*(?:[-*+]\s+\[[ xX]?\]\s+|\[[ xX]?\]\s+|[-*+]\s+|\d+[.)]\s+)/, '')
			.trim();
	}

	function handleTitlePaste(e: ClipboardEvent) {
		if (!parentIssue || !onbulkcreate) return;
		const text = e.clipboardData?.getData('text') ?? '';
		const titles = text.split(/\r?\n/).map(titleFromListLine).filter(Boolean);
		if (titles.length < 2) return;
		e.preventDefault();
		onbulkcreate(titles);
		if (!createMore) open = false;
	}

	const STATUS_TO_CATEGORY: Record<string, StatusCategory> = {
		backlog: 'backlog',
		todo: 'unstarted',
		in_progress: 'started',
		in_review: 'started',
		done: 'completed',
		cancelled: 'cancelled',
	};

	function applyTemplate(tmpl: IssueTemplate) {
		selectedTemplate = tmpl;
		title = tmpl.title || '';
		description = tmpl.description ?? '';
		descriptionVersion = Date.now();
		priority = tmpl.priority ?? 0;
		labelIds = Array.isArray(tmpl.label_ids) ? tmpl.label_ids : [];
		if (tmpl.assignee_id) assigneeIds = [tmpl.assignee_id];
		if (tmpl.status) {
			const category = STATUS_TO_CATEGORY[tmpl.status];
			if (category) {
				const defaultStatus = statusesState.defaultForCategory(category);
				if (defaultStatus) statusId = defaultStatus.id;
			}
		}
		templateOpen = false;
	}

	function clearTemplateSelection() {
		selectedTemplate = null;
		templateOpen = false;
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
			handleSubmit();
		}
	}

	function toggleLabel(id: string) {
		if (labelIds.includes(id)) {
			labelIds = labelIds.filter((l) => l !== id);
		} else {
			labelIds = [...labelIds, id];
		}
	}

	function handleProjectChange(id: string | null) {
		projectId = id;
		applyDefaultStatus(statusId);
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		class="sm:max-w-[640px] gap-0 overflow-hidden rounded-xl border-[var(--app-border)] bg-[var(--color-bg-secondary)] p-0 max-sm:w-screen max-sm:h-dvh max-sm:max-w-none max-sm:rounded-none max-sm:top-0 max-sm:left-0 max-sm:translate-x-0 max-sm:flex max-sm:flex-col"
		onOpenAutoFocus={(e) => {
			e.preventDefault();
			const input = document.getElementById('create-issue-title');
			input?.focus();
		}}
	>
		<!-- Top bar: Template -->
		<div class="flex items-center gap-1.5 px-3 pr-10 py-2 max-sm:shrink-0">
			{#if templates.length > 0}
				<Popover.Root bind:open={templateOpen}>
					<Popover.Trigger>
						<button tabindex="-1" class="flex max-w-52 items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-hover)]">
							<FileText size={12} class="shrink-0 text-[var(--color-text-tertiary)]" />
							<span class="truncate">{selectedTemplate?.title || m['sharedComponents.create_issue.template']()}</span>
						</button>
					</Popover.Trigger>
					<Popover.Content class="w-56 p-1" align="start">
						<button
							onclick={clearTemplateSelection}
							class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm text-[var(--color-text-primary)] hover:bg-[var(--color-bg-hover)]"
						>
							<FileText size={14} class="shrink-0 text-[var(--color-text-tertiary)]" />
							<span class="truncate">{m['sharedComponents.create_issue.no_template']()}</span>
						</button>
						{#each templates as tmpl (tmpl.id)}
							<button
								onclick={() => applyTemplate(tmpl)}
								class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm text-[var(--color-text-primary)] hover:bg-[var(--color-bg-hover)]"
							>
								<FileText size={14} class="shrink-0 text-[var(--color-text-tertiary)]" />
								<span class="truncate">{tmpl.title || m['sharedComponents.create_issue.untitled_template']()}</span>
							</button>
						{/each}
					</Popover.Content>
				</Popover.Root>
			{:else}
				<span class="text-xs font-medium text-[var(--color-text-secondary)]">
					{parentIssue ? m['sharedComponents.create_issue.new_sub_issue']({ identifier: parentIssue.identifier }) : m['sharedComponents.create_issue.new_issue']()}
				</span>
			{/if}
		</div>

		<!-- Title + Description -->
		<!-- svelte-ignore a11y_autofocus -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="px-4 py-3 max-sm:flex max-sm:flex-col max-sm:flex-1 max-sm:min-h-0 max-sm:overflow-y-auto" onkeydown={handleKeydown}>
			<input
				id="create-issue-title"
				type="text"
				bind:value={title}
				onpaste={handleTitlePaste}
				placeholder={m['sharedComponents.create_issue.issue_title_placeholder']()}
				class="w-full bg-transparent text-lg font-semibold text-[var(--color-text-primary)] outline-none placeholder:text-[var(--color-text-tertiary)] max-sm:shrink-0"
			/>
			<div class="mt-4 max-h-[calc(60vh-120px)] overflow-y-auto max-sm:flex-1 max-sm:[max-height:none] max-sm:overflow-y-auto">
				{#key descriptionVersion}
				<RichEditor
					content={description}
					workspaceSlug={slug}
					{members}
					issues={issuesState.issues}
					placeholder={m['sharedComponents.create_issue.description_placeholder']()}
					bubbleMenu={true}
					borderless={true}
					minHeight="120px"
					onupdate={(html) => (description = html)}
				/>
				{/key}
			</div>
		</div>

		<!-- Property pills -->
		<div class="flex flex-wrap items-center gap-1.5 px-4 py-2.5 max-sm:shrink-0">
			<!-- Status -->
			<StatusSelector
				bind:open={statusOpen}
				statuses={statusesState.statusesForProject(projectId)}
				value={statusId}
				onchange={(id) => { statusId = id; }}
				width="w-44"
			>
				{#snippet trigger()}
					<button class="flex items-center gap-1.5 rounded-full border border-[var(--app-border)] px-2.5 py-1 max-sm:px-3 max-sm:py-1.5 text-xs text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-hover)]">
						<IssueStatusIcon category={selectedStatus?.category} color={selectedStatus?.color} size={12} />
						{selectedStatus?.name ?? m['sharedComponents.create_issue.status']()}
					</button>
				{/snippet}
			</StatusSelector>

			<!-- Priority -->
			<PrioritySelector
				bind:open={priorityOpen}
				value={priority}
				onchange={(p) => { priority = p; }}
			>
				{#snippet trigger()}
					<button class="flex items-center gap-1.5 rounded-full border border-[var(--app-border)] px-2.5 py-1 max-sm:px-3 max-sm:py-1.5 text-xs text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-hover)]">
						<IssuePriorityIcon {priority} size={12} />
						{getPriorityLabel(priority)}
					</button>
				{/snippet}
			</PrioritySelector>

			<!-- Project (mandatory) -->
			<ProjectSelector
				bind:open={projectOpen}
				{projects}
				value={projectId}
				onchange={handleProjectChange}
				showNone={false}
			>
				{#snippet trigger()}
					<button class="flex items-center gap-1.5 rounded-full border {projectId ? 'border-[var(--app-border)]' : 'border-[var(--color-error)]'} px-2.5 py-1 max-sm:px-3 max-sm:py-1.5 text-xs {selectedProject ? 'text-[var(--color-text-secondary)]' : 'text-[var(--color-text-tertiary)]'} hover:bg-[var(--color-bg-hover)]">
						<FolderKanban size={12} />
						{selectedProject?.name ?? m['sharedComponents.create_issue.project']()}
					</button>
				{/snippet}
			</ProjectSelector>

			<!-- Assignees -->
			<AssigneeSelector
				bind:open={assigneeOpen}
				{members}
				value={assigneeIds}
				onchange={(userId) => {
					if (assigneeIds.includes(userId)) {
						assigneeIds = assigneeIds.filter(id => id !== userId);
					} else {
						assigneeIds = [...assigneeIds, userId];
					}
				}}
			>
				{#snippet trigger()}
					<button class="flex items-center gap-1.5 rounded-full border border-[var(--app-border)] px-2.5 py-1 max-sm:px-3 max-sm:py-1.5 text-xs {selectedAssignees.length > 0 ? 'text-[var(--color-text-secondary)]' : 'text-[var(--color-text-tertiary)]'} hover:bg-[var(--color-bg-hover)]">
						<User size={12} />
						{#if selectedAssignees.length === 0}
							{m['sharedComponents.create_issue.assignee']()}
						{:else if selectedAssignees.length === 1}
							{selectedAssignees[0].name || selectedAssignees[0].email}
						{:else}
							{m['sharedComponents.create_issue.assignees_count']({ count: selectedAssignees.length })}
						{/if}
					</button>
				{/snippet}
			</AssigneeSelector>

			<!-- Labels -->
			<LabelSelector
				bind:open={labelsOpen}
				{labels}
				value={labelIds}
				onchange={(labelId) => toggleLabel(labelId)}
				oncreated={(label) => onlabelcreated?.(label)}
				{slug}
			>
				{#snippet trigger()}
					<button class="flex items-center gap-1.5 rounded-full border border-[var(--app-border)] px-2.5 py-1 max-sm:px-3 max-sm:py-1.5 text-xs {selectedLabels.length > 0 ? 'text-[var(--color-text-secondary)]' : 'text-[var(--color-text-tertiary)]'} hover:bg-[var(--color-bg-hover)]">
						<Tag size={12} />
						{#if selectedLabels.length === 0}
							{m['sharedComponents.create_issue.labels']()}
						{:else if selectedLabels.length === 1}
							{selectedLabels[0].name}
						{:else}
							{m['sharedComponents.create_issue.labels_count']({ count: selectedLabels.length })}
						{/if}
					</button>
				{/snippet}
			</LabelSelector>

			<!-- Due Date -->
			<DatePickerPopover
				value={dueDate}
				onchange={(d) => (dueDate = d)}
				placeholder={m['sharedComponents.create_issue.due_date_placeholder']()}
				dueDateMode
			/>
		</div>

		<!-- No projects warning -->
		{#if projects.length === 0}
			<div class="mx-4 mb-2 flex items-start gap-2 rounded-md border border-[var(--app-border)] bg-[var(--color-bg-tertiary)] px-3 py-2 text-xs text-[var(--color-text-secondary)]">
				<AlertTriangle size={14} class="mt-0.5 shrink-0 text-[var(--color-text-tertiary)]" />
				<span>{m['issue.create_project_first']()}</span>
			</div>
		{/if}

		<!-- Footer -->
		<div class="flex items-center justify-end gap-3 px-4 py-2.5 max-sm:sticky max-sm:bottom-0 max-sm:shrink-0 max-sm:flex-col max-sm:items-stretch max-sm:border-t max-sm:border-[var(--app-border)] max-sm:bg-[var(--color-bg-secondary)] max-sm:pb-[calc(1rem+env(safe-area-inset-bottom,0px))]">
			<label class="flex items-center gap-2 text-xs text-[var(--color-text-tertiary)]">
				<Switch bind:checked={createMore} size="sm" />
				{m['sharedComponents.create_issue.create_more']()}
			</label>
			<Button
				class="max-sm:w-full"
				size="sm"
				disabled={!title.trim() || !hasProject}
				onclick={handleSubmit}
			>
				{m['sharedComponents.create_issue.create_issue']()}
			</Button>
		</div>
	</Dialog.Content>
</Dialog.Root>
