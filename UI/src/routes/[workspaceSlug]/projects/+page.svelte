<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { listProjects, createProject } from '$lib/api/projects';
	import type { Project, ProjectStatus } from '$lib/types/project';
	import EmptyState from '$lib/components/shared/EmptyState.svelte';
	import CreateProjectDialog from '$lib/features/projects/CreateProjectDialog.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { sidebarState } from '$lib/features/layout/sidebar.state.svelte';
	import { appToast } from '$lib/features/toast/toast';
	import { Plus } from 'lucide-svelte';
	import SidebarToggle from '$lib/components/layout/SidebarToggle.svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale, setLocale } from '$lib/paraglide/runtime.js';

	const slug = $derived(page.params.workspaceSlug ?? '');
	let projects = $state<Project[]>([]);
	let loading = $state(true);
	let showCreateProject = $state(false);

	function statusLabel(status: ProjectStatus): string {
		return m[`projects.status.${status}`]();
	}

	onMount(async () => {
		try {
			projects = await listProjects(slug);
		} finally {
			loading = false;
		}
	});

	async function handleCreate(data: { name: string; description?: string }) {
		try {
			const project = await createProject(slug, data);
			projects = [...projects, project];
			sidebarState.addProject(project);
			appToast.success(m['projects.toast.created']());
		} catch (err: any) {
			appToast.apiError(err, m['projects.toast.failed_create']());
		}
	}

	function progressPercentage(project: Project): number {
		if (!project.progress || project.progress.total === 0) return 0;
		return Math.round(((project.progress.completed + project.progress.cancelled) / project.progress.total) * 100);
	}

	function statusVariant(status: ProjectStatus): 'default' | 'secondary' | 'outline' | 'destructive' {
		switch (status) {
			case 'in_progress': return 'default';
			case 'completed': return 'secondary';
			case 'cancelled': return 'destructive';
			default: return 'outline';
		}
	}
</script>

<div class="h-full">
	<div
		class="flex h-[49px] items-center justify-between border-b border-[var(--app-border)] px-6"
	>
		<div class="flex items-center gap-2">
			<SidebarToggle />
			<h1 class="text-sm font-medium text-[var(--color-text-primary)]">{m['projects.title']()}</h1>
		</div>
		<button
			onclick={() => (showCreateProject = true)}
			class="rounded-md p-1 text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-hover)] hover:text-[var(--color-text-primary)]"
			title={m['projects.new_project']()}
		>
			<Plus size={16} />
		</button>
	</div>

	{#if !loading && projects.length === 0}
		<EmptyState
			title={m['projects.no_projects']()}
			description={m['projects.no_projects_desc']()}
			action={{ label: m['projects.new_project'](), onclick: () => (showCreateProject = true) }}
		/>
	{:else}
		<div class="divide-y divide-[var(--app-border)]">
			{#each projects as project}
				<a
					href="/{slug}/projects/{project.id}"
					class="flex items-center gap-4 px-6 py-3 hover:bg-[var(--color-bg-hover)]"
				>
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2">
							<span class="text-sm font-medium text-[var(--color-text-primary)]">{project.name}</span>
							<Badge variant={statusVariant(project.status)} class="text-[10px]">
								{statusLabel(project.status)}
							</Badge>
						</div>
						{#if project.description}
							<p class="mt-0.5 truncate text-xs text-[var(--color-text-tertiary)]">{project.description}</p>
						{/if}
					</div>
					{#if project.progress && project.progress.total > 0}
						<div class="flex items-center gap-2 shrink-0">
							<div class="relative h-1.5 w-24 overflow-hidden rounded-full bg-[var(--color-bg-tertiary)]">
								<div
									class="absolute left-0 top-0 h-full rounded-full bg-[var(--color-success)]"
									style="width: {project.progress.total > 0 ? (project.progress.completed / project.progress.total) * 100 : 0}%"
								></div>
							</div>
							<span class="text-xs tabular-nums text-[var(--color-text-tertiary)]">
								{progressPercentage(project)}%
							</span>
						</div>
					{/if}
				</a>
			{/each}
		</div>
	{/if}
</div>

<CreateProjectDialog
	bind:open={showCreateProject}
	onsubmit={handleCreate}
/>
