<script lang="ts">
	import type { CreateIssueRequest, IssuePriority } from '$lib/types/issue';
	import type { Project } from '$lib/types/project';
	import { getPriorityLabels } from '$lib/types/issue';
	import { statusesState } from './statuses.state.svelte';
	import { m } from '$lib/paraglide/messages.js';

	let {
		projects = [],
		onsubmit,
		oncancel
	}: {
		projects?: Project[];
		onsubmit: (req: CreateIssueRequest) => void;
		oncancel: () => void;
	} = $props();

	let title = $state('');
	let description = $state('');
	let statusId = $state(statusesState.defaultForCategory('backlog')?.id ?? '');
	let priority = $state<IssuePriority>(0);
	// svelte-ignore state_referenced_locally
	let projectId = $state<string | null>(projects[0]?.id ?? null);

	function handleSubmit(e: Event) {
		e.preventDefault();
		if (!projectId) return;
		onsubmit({
			title,
			description: description || undefined,
			status_id: statusId || undefined,
			priority,
			project_id: projectId
		});
	}
</script>

<form onsubmit={handleSubmit} class="space-y-4 p-4">
	<div>
		<input
			type="text"
			bind:value={title}
			placeholder="Issue title"
			required
			class="w-full bg-transparent text-lg font-medium text-[var(--color-text-primary)] outline-none placeholder:text-[var(--color-text-tertiary)]"
		/>
	</div>

	<div>
		<textarea
			bind:value={description}
			placeholder="Add description..."
			rows={3}
			class="w-full rounded border border-[var(--app-border)] bg-[var(--color-bg-secondary)] px-3 py-2 text-sm text-[var(--color-text-primary)] outline-none placeholder:text-[var(--color-text-tertiary)] focus:border-[var(--app-accent)]"
		></textarea>
	</div>

	<div class="flex flex-wrap gap-3">
		{#if projects.length === 0}
			<p class="text-xs text-[var(--color-text-tertiary)]">{m['issue.create_project_first']()}</p>
		{:else}
			<select
				bind:value={projectId}
				required
				class="rounded border border-[var(--app-border)] bg-[var(--color-bg-secondary)] px-2 py-1.5 text-sm text-[var(--color-text-secondary)]"
			>
				{#each projects as project}
					<option value={project.id}>{project.name}</option>
				{/each}
			</select>
		{/if}

		<select
			bind:value={statusId}
			class="rounded border border-[var(--app-border)] bg-[var(--color-bg-secondary)] px-2 py-1.5 text-sm text-[var(--color-text-secondary)]"
		>
			{#each statusesState.statusesForProject(projectId) as ts}
				<option value={ts.id}>{ts.name}</option>
			{/each}
		</select>

		<select
			bind:value={priority}
			class="rounded border border-[var(--app-border)] bg-[var(--color-bg-secondary)] px-2 py-1.5 text-sm text-[var(--color-text-secondary)]"
		>
			{#each Object.entries(getPriorityLabels()) as [value, label]}
				<option value={Number(value)}>{label}</option>
			{/each}
		</select>
	</div>

	<div class="flex justify-end gap-2">
		<button
			type="button"
			onclick={oncancel}
			class="rounded-md border border-[var(--app-border)] px-3 py-1.5 text-sm text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-hover)]"
		>
			Cancel
		</button>
		<button
			type="submit"
			disabled={!title.trim() || !projectId}
			class="rounded-md bg-[var(--app-accent)] px-3 py-1.5 text-sm text-[var(--app-accent-foreground)] hover:bg-[var(--app-accent-hover)] disabled:opacity-50"
		>
			Create issue
		</button>
	</div>
</form>
