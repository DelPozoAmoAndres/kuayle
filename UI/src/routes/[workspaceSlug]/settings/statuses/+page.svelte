<script lang="ts">
	import { flip } from 'svelte/animate';
	import { page } from '$app/state';
	import type { WorkspaceStatus, StatusCategory } from '$lib/types/status';
	import { CATEGORY_ORDER, getCategoryLabel } from '$lib/types/status';
	import type { Project } from '$lib/types/project';
	import { listStatuses, createStatus, updateStatus, deleteStatus } from '$lib/api/statuses';
	import { listProjects } from '$lib/api/projects';
	import { statusesState } from '$lib/features/issues/statuses.state.svelte';
	import IssueStatusIcon from '$lib/features/issues/IssueStatusIcon.svelte';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Popover from '$lib/components/ui/popover';
	import { appToast } from '$lib/features/toast/toast';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale, setLocale } from '$lib/paraglide/runtime.js';
	import { Plus, Trash2, Pencil, X, Check, GripVertical, FolderKanban, Globe } from 'lucide-svelte';

	const slug = $derived(page.params.workspaceSlug ?? '');

	let statuses = $state<WorkspaceStatus[]>([]);
	let projects = $state<Project[]>([]);
	let loading = $state(true);

	let addingCategory = $state<StatusCategory | null>(null);
	let addName = $state('');
	let addColor = $state('');

	let editingId = $state<string | null>(null);
	let editName = $state('');
	let editColor = $state('');

	let visibilityOpenId = $state<string | null>(null);

	let dragStatusId = $state<string | null>(null);
	let dragOverStatusId = $state<string | null>(null);
	let dragCategory = $state<StatusCategory | null>(null);
	let dragOverCategory = $state<StatusCategory | null>(null);
	let dropIndicator = $state<'above' | 'below'>('below');

	$effect(() => {
		const s = slug;
		if (!s) return;
		loading = true;
		editingId = null;
		addingCategory = null;
		Promise.all([listStatuses(s), listProjects(s)])
			.then(([st, pr]) => {
				statuses = st;
				projects = pr;
				loading = false;
			})
			.catch(() => {
				loading = false;
			});
	});

	async function loadStatuses() {
		statuses = await listStatuses(slug);
		void statusesState.reload(slug);
	}

	async function handleAdd() {
		if (!addName.trim() || !addingCategory) return;
		try {
			await createStatus(slug, {
				name: addName.trim(),
				category: addingCategory,
				color: addColor || undefined
			});
			addName = '';
			addColor = '';
			addingCategory = null;
			await loadStatuses();
			appToast.success(m['settings.statuses.created']());
		} catch (err: any) {
			appToast.apiError(err, m['settings.statuses.failed_create']());
		}
	}

	function startEdit(status: WorkspaceStatus) {
		editingId = status.id;
		editName = status.name;
		editColor = status.color ?? '';
	}

	async function saveEdit() {
		if (!editingId || !editName.trim()) return;
		try {
			await updateStatus(slug, editingId, {
				name: editName.trim(),
				color: editColor || undefined
			});
			editingId = null;
			await loadStatuses();
			appToast.success(m['settings.statuses.updated']());
		} catch (err: any) {
			appToast.apiError(err, m['settings.statuses.failed_update']());
		}
	}

	async function handleDelete(statusId: string) {
		try {
			await deleteStatus(slug, statusId);
			await loadStatuses();
			appToast.success(m['settings.statuses.deleted']());
		} catch (err: any) {
			appToast.apiError(err, m['settings.statuses.failed_delete']());
		}
	}

	function statusesByCategory(cat: StatusCategory): WorkspaceStatus[] {
		return statuses.filter((s) => s.category === cat).sort((a, b) => a.position - b.position);
	}

	function startAdd(cat: StatusCategory) {
		addingCategory = cat;
		addName = '';
		addColor = '';
	}

	// ── Project visibility (project_ids) ──
	function visibleProjectIds(status: WorkspaceStatus): Set<string> {
		const explicit = status.project_ids;
		if (!explicit || explicit.length === 0) {
			return new Set(projects.map((p) => p.id));
		}
		return new Set(explicit);
	}

	function visibilityLabel(status: WorkspaceStatus): string {
		const explicit = status.project_ids;
		if (!explicit || explicit.length === 0 || explicit.length === projects.length) {
			return m['settings.statuses.visible_all_projects']();
		}
		return m['settings.statuses.visible_projects_count']({ count: explicit.length });
	}

	async function toggleProjectVisibility(status: WorkspaceStatus, projectId: string) {
		const current = visibleProjectIds(status);
		if (current.has(projectId)) {
			current.delete(projectId);
		} else {
			current.add(projectId);
		}
		// Every project selected (or none selected) means "visible everywhere".
		const next =
			current.size === 0 || current.size === projects.length
				? []
				: projects.map((p) => p.id).filter((id) => current.has(id));
		await persistProjectIds(status, next);
	}

	async function showAllProjects(status: WorkspaceStatus) {
		await persistProjectIds(status, []);
	}

	async function persistProjectIds(status: WorkspaceStatus, projectIds: string[]) {
		try {
			await updateStatus(slug, status.id, { project_ids: projectIds });
			await loadStatuses();
		} catch (err: any) {
			appToast.apiError(err, m['settings.statuses.failed_update']());
		}
	}

	// ── Drag & drop reordering inside a category ──
	function handleDragStart(e: DragEvent, status: WorkspaceStatus) {
		dragStatusId = status.id;
		dragCategory = status.category;
		if (e.dataTransfer) {
			e.dataTransfer.effectAllowed = 'move';
			e.dataTransfer.setData('text/plain', status.id);
		}
	}

	function handleDragOver(e: DragEvent, status: WorkspaceStatus) {
		if (!dragStatusId) return;
		dragOverCategory = status.category;

		if (dragCategory !== status.category) {
			dragOverStatusId = null;
			return;
		}

		e.preventDefault();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
		dragOverStatusId = status.id;
		const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
		const midY = rect.top + rect.height / 2;
		dropIndicator = e.clientY < midY ? 'above' : 'below';
	}

	function handleDragLeave() {
		dragOverStatusId = null;
	}

	function handleDragEnd() {
		dragStatusId = null;
		dragOverStatusId = null;
		dragCategory = null;
		dragOverCategory = null;
		dropIndicator = 'below';
	}

	function handleSectionDragOver(e: DragEvent, cat: StatusCategory) {
		if (!dragStatusId) return;
		dragOverCategory = cat;
	}

	function handleSectionDragLeave(e: DragEvent) {
		const related = e.relatedTarget as HTMLElement | null;
		if (related && (e.currentTarget as HTMLElement).contains(related)) return;
		dragOverCategory = null;
		dragOverStatusId = null;
	}

	async function handleDrop(e: DragEvent, targetStatus: WorkspaceStatus) {
		e.preventDefault();
		if (!dragStatusId || dragStatusId === targetStatus.id) {
			handleDragEnd();
			return;
		}

		const cat = targetStatus.category;
		const catStatuses = statusesByCategory(cat);
		const dragIdx = catStatuses.findIndex((s) => s.id === dragStatusId);
		const targetIdx = catStatuses.findIndex((s) => s.id === targetStatus.id);

		if (dragIdx === -1 || targetIdx === -1) {
			handleDragEnd();
			return;
		}

		const reordered = [...catStatuses];
		const [moved] = reordered.splice(dragIdx, 1);
		const newTargetIdx = reordered.findIndex((s) => s.id === targetStatus.id);
		const insertIdx = dropIndicator === 'below' ? newTargetIdx + 1 : newTargetIdx;
		reordered.splice(insertIdx, 0, moved);

		const otherStatuses = statuses.filter((s) => s.category !== cat);
		const updatedCat = reordered.map((s, i) => ({ ...s, position: i }));
		statuses = [...otherStatuses, ...updatedCat];

		handleDragEnd();

		try {
			await Promise.all(updatedCat.map((s, i) => updateStatus(slug, s.id, { position: i })));
			void statusesState.reload(slug);
		} catch {
			appToast.error(m['settings.statuses.failed_reorder']());
			await loadStatuses();
		}
	}

	const PRESET_COLORS = ['#ef4444', '#f97316', '#eab308', '#22c55e', '#06b6d4', '#3b82f6', '#8b5cf6', '#ec4899'];
</script>

<div class="mx-auto max-w-2xl px-8 py-10">
	<h1 class="text-2xl font-semibold text-[var(--color-text-primary)]">{m['settings.statuses.title']()}</h1>
	<p class="mt-2 text-sm text-[var(--color-text-tertiary)]">
		{m['settings.statuses.desc']()}
	</p>

	<div class="mt-8">
		{#if loading}
			<div class="flex justify-center py-8">
				<div
					class="h-5 w-5 animate-spin rounded-full border-2 border-[var(--color-text-tertiary)] border-t-transparent"
				></div>
			</div>
		{:else}
			<div class="rounded-lg border border-[var(--app-border)] bg-[var(--color-bg-secondary)] overflow-hidden">
				{#each CATEGORY_ORDER as cat, catIdx}
					{@const catStatuses = statusesByCategory(cat)}

					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div
						class="transition-colors {catIdx > 0 ? 'border-t' : ''} {dragStatusId && dragOverCategory === cat
							? 'border-[var(--app-accent)]'
							: 'border-[var(--app-border)]'}"
						ondragover={(e) => handleSectionDragOver(e, cat)}
						ondragleave={(e) => handleSectionDragLeave(e)}
					>
						<div class="flex items-center justify-between px-5 py-2.5">
							<span class="text-xs font-medium text-[var(--color-text-tertiary)]">{getCategoryLabel(cat)}</span>
							<button
								onclick={() => startAdd(cat)}
								class="rounded p-0.5 text-[var(--color-text-tertiary)] hover:text-[var(--color-text-primary)] hover:bg-[var(--color-bg-hover)] transition-colors"
								title={m['settings.statuses.add_status_to_aria']({ name: getCategoryLabel(cat) })}
							>
								<Plus size={14} />
							</button>
						</div>

						{#each catStatuses as status (status.id)}
							<!-- svelte-ignore a11y_no_static_element_interactions -->
							<div
								class="relative flex items-center gap-1 px-1 py-0 border-t border-[var(--app-border)] transition-colors
								{dragStatusId === status.id ? 'opacity-40' : 'hover:bg-[var(--color-bg-hover)]/50'}"
								draggable={editingId !== status.id}
								ondragstart={(e) => handleDragStart(e, status)}
								ondragover={(e) => handleDragOver(e, status)}
								ondragleave={handleDragLeave}
								ondragend={handleDragEnd}
								ondrop={(e) => handleDrop(e, status)}
							>
								{#if dragOverStatusId === status.id && dragStatusId !== status.id}
									<div
										class="absolute {dropIndicator === 'above'
											? '-top-px'
											: '-bottom-px'} left-3 right-3 h-0.5 bg-[var(--app-accent)] z-10 rounded-full"
									></div>
								{/if}
								<span
									class="shrink-0 cursor-grab text-[var(--color-text-tertiary)] opacity-0 hover:opacity-100 transition-opacity group-hover:opacity-50 {dragStatusId
										? 'opacity-50'
										: ''}"
									style="cursor: grab;"
								>
									<GripVertical size={14} />
								</span>

								<div class="flex flex-1 items-center gap-3 px-2 py-2.5 group">
									<IssueStatusIcon category={status.category} color={status.color} size={18} />

									{#if editingId === status.id}
										<div class="flex flex-1 items-center gap-2">
											<input
												type="text"
												bind:value={editName}
												onkeydown={(e) => {
													if (e.key === 'Enter') saveEdit();
													if (e.key === 'Escape') editingId = null;
												}}
												class="flex-1 rounded border border-[var(--app-border)] bg-[var(--color-bg)] px-2 py-0.5 text-sm text-[var(--color-text-primary)] outline-none focus:border-[var(--app-accent)]"
											/>
											<div class="flex items-center gap-0.5">
												{#each PRESET_COLORS as c}
													<button
														onclick={() => (editColor = c)}
														class="h-3.5 w-3.5 rounded-full {editColor === c
															? 'ring-2 ring-[var(--app-accent)] ring-offset-1 ring-offset-[var(--color-bg)]'
															: ''}"
														style="background-color: {c}"
														aria-label={m['settings.statuses.select_color_aria']({ color: c })}
													></button>
												{/each}
											</div>
											<button
												onclick={saveEdit}
												class="rounded p-0.5 text-[var(--color-success)] hover:bg-[var(--color-bg-tertiary)]"
											>
												<Check size={14} />
											</button>
											<button
												onclick={() => (editingId = null)}
												class="rounded p-0.5 text-[var(--color-text-tertiary)] hover:bg-[var(--color-bg-tertiary)]"
											>
												<X size={14} />
											</button>
										</div>
									{:else}
										<div class="flex-1 min-w-0">
											<div class="flex items-center gap-2">
												<span class="text-sm font-medium text-[var(--color-text-primary)]">{status.name}</span>
												{#if status.is_default}
													<span class="text-[10px] text-[var(--color-text-tertiary)]">· {m['settings.statuses.default_badge']()}</span>
												{/if}
											</div>
											<button
												type="button"
												onclick={() => (visibilityOpenId = visibilityOpenId === status.id ? null : status.id)}
												class="mt-0.5 flex items-center gap-1 text-[11px] text-[var(--color-text-tertiary)] hover:text-[var(--color-text-secondary)] transition-colors"
											>
												<Globe size={11} />
												{visibilityLabel(status)}
											</button>
										</div>
										<div class="flex items-center gap-1">
											<Popover.Root open={visibilityOpenId === status.id} onOpenChange={(open) => (visibilityOpenId = open ? status.id : null)}>
												<Popover.Trigger>
													<button
														class="hidden rounded p-1 text-[var(--color-text-tertiary)] hover:text-[var(--color-text-primary)] hover:bg-[var(--color-bg-tertiary)] group-hover:block transition-colors"
														title={m['settings.statuses.visibility']()}
													>
														<FolderKanban size={12} />
													</button>
												</Popover.Trigger>
												<Popover.Content class="w-56 p-2" align="end">
													<p class="px-1 pb-1.5 text-[11px] font-medium uppercase tracking-wide text-[var(--color-text-tertiary)]">
														{m['settings.statuses.visibility']}
													</p>
													<button
														onclick={() => showAllProjects(status)}
														class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-[var(--color-text-primary)] hover:bg-[var(--color-bg-hover)]"
													>
														<Globe size={13} class="text-[var(--color-text-tertiary)]" />
														{m['settings.statuses.visible_all_projects']()}
													</button>
													<div class="my-1 h-px bg-[var(--app-border)]"></div>
													{#if projects.length === 0}
														<p class="px-2 py-1.5 text-xs text-[var(--color-text-tertiary)]">
															{m['settings.statuses.no_projects']()}
														</p>
													{:else}
														<div class="max-h-56 space-y-px overflow-y-auto">
															{#each projects as project (project.id)}
																<button
																	onclick={() => toggleProjectVisibility(status, project.id)}
																	class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-[var(--color-text-primary)] hover:bg-[var(--color-bg-hover)]"
																>
																	<Checkbox checked={visibleProjectIds(status).has(project.id)} />
																	<span class="truncate">{project.name}</span>
																</button>
															{/each}
														</div>
													{/if}
												</Popover.Content>
											</Popover.Root>
											<button
												onclick={() => startEdit(status)}
												class="hidden rounded p-1 text-[var(--color-text-tertiary)] hover:text-[var(--color-text-primary)] hover:bg-[var(--color-bg-tertiary)] group-hover:block transition-colors"
											>
												<Pencil size={12} />
											</button>
											{#if !status.is_default}
												<button
													onclick={() => handleDelete(status.id)}
													class="hidden rounded p-1 text-[var(--color-text-tertiary)] hover:text-[var(--color-error)] hover:bg-[var(--color-bg-tertiary)] group-hover:block transition-colors"
												>
													<Trash2 size={12} />
												</button>
											{/if}
										</div>
									{/if}
								</div>
							</div>
						{/each}

						{#if addingCategory === cat}
							<div
								class="flex items-center gap-3 px-5 py-2.5 border-t border-[var(--app-border)] bg-[var(--color-bg-hover)]/30"
							>
								<IssueStatusIcon category={cat} size={18} />
								<input
									type="text"
									bind:value={addName}
									placeholder={m['settings.statuses.name_placeholder']()}
									onkeydown={(e) => {
										if (e.key === 'Enter') handleAdd();
										if (e.key === 'Escape') addingCategory = null;
									}}
									class="flex-1 rounded border border-[var(--app-border)] bg-[var(--color-bg)] px-2 py-1 text-sm text-[var(--color-text-primary)] outline-none focus:border-[var(--app-accent)]"
								/>
								<div class="flex items-center gap-0.5">
									{#each PRESET_COLORS as c}
										<button
											onclick={() => (addColor = c)}
											class="h-3.5 w-3.5 rounded-full {addColor === c
												? 'ring-2 ring-[var(--app-accent)] ring-offset-1 ring-offset-[var(--color-bg)]'
												: ''}"
											style="background-color: {c}"
											aria-label={m['settings.statuses.select_color_only_aria']()}
										></button>
									{/each}
								</div>
								<button
									onclick={handleAdd}
									disabled={!addName.trim()}
									class="rounded-md bg-[var(--app-accent)] px-2.5 py-1 text-xs text-[var(--app-accent-foreground)] hover:bg-[var(--app-accent-hover)] disabled:opacity-50"
								>
									{m['settings.statuses.add']()}
								</button>
								<button
									onclick={() => (addingCategory = null)}
									class="rounded p-0.5 text-[var(--color-text-tertiary)] hover:bg-[var(--color-bg-tertiary)]"
								>
									<X size={14} />
								</button>
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
