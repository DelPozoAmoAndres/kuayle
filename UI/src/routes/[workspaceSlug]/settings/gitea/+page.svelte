<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import {
		getGiteaStatus,
		connectGitea,
		disconnectGitea,
		listGiteaRepos,
		linkGiteaRepos,
		unlinkGiteaRepo,
		listGiteaAutoTransitions,
		updateGiteaAutoTransitions
	} from '$lib/api/gitea';
	import type { GiteaStatus, GiteaAvailableRepo, GiteaAutoTransition } from '$lib/types/gitea';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Switch } from '$lib/components/ui/switch';
	import { Input } from '$lib/components/ui/input';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { appToast } from '$lib/features/toast/toast';
	import { ExternalLink, Plus, Loader2, Search, GitBranch, Trash2 } from 'lucide-svelte';

	const slug = $derived(page.params.workspaceSlug ?? '');
	let status = $state<GiteaStatus | null>(null);
	let loading = $state(true);
	let availableRepos = $state<GiteaAvailableRepo[]>([]);
	let loadingRepos = $state(false);
	let showRepoSelector = $state(false);
	let selectedRepoIds = $state<Set<number>>(new Set());
	let transitions = $state<GiteaAutoTransition[]>([]);
	let repoSearch = $state('');
	let savingRepos = $state(false);

	// Connect form state
	let instanceUrl = $state('https://gitea.com');
	let accessToken = $state('');
	let webhookSecret = $state('');
	let connecting = $state(false);
	let showToken = $state(false);

	const filteredRepos = $derived(
		repoSearch
			? availableRepos.filter((r) =>
					r.full_name.toLowerCase().includes(repoSearch.toLowerCase())
				)
			: availableRepos
	);

	const allFilteredSelected = $derived(
		filteredRepos.length > 0 && filteredRepos.every((r) => selectedRepoIds.has(r.gitea_repo_id))
	);

	const someFilteredSelected = $derived(
		!allFilteredSelected && filteredRepos.some((r) => selectedRepoIds.has(r.gitea_repo_id))
	);

	function toggleSelectAll() {
		const next = new Set(selectedRepoIds);
		if (allFilteredSelected) {
			for (const r of filteredRepos) next.delete(r.gitea_repo_id);
		} else {
			for (const r of filteredRepos) next.add(r.gitea_repo_id);
		}
		selectedRepoIds = next;
	}

	onMount(async () => {
		try {
			status = await getGiteaStatus(slug);
			if (status.connected) {
				transitions = await listGiteaAutoTransitions(slug);
			}
		} catch (err: any) {
			console.error('Gitea status error:', err);
		} finally {
			loading = false;
		}
	});

	async function handleConnect() {
		if (!instanceUrl.trim() || !accessToken.trim()) {
			appToast.error('Instance URL and Access Token are required.');
			return;
		}
		connecting = true;
		try {
			await connectGitea(slug, {
				instance_url: instanceUrl.trim(),
				access_token: accessToken.trim(),
				webhook_secret: webhookSecret.trim() || undefined
			});
			appToast.success('Connected to Gitea successfully.');
			accessToken = '';
			webhookSecret = '';
			status = await getGiteaStatus(slug);
			if (status.connected) {
				transitions = await listGiteaAutoTransitions(slug);
			}
		} catch (err: any) {
			console.error('Gitea connect error:', err);
			appToast.apiError(err, 'Failed to connect to Gitea.');
		} finally {
			connecting = false;
		}
	}

	async function handleDisconnect() {
		try {
			await disconnectGitea(slug);
			status = await getGiteaStatus(slug);
			transitions = [];
			appToast.success('Disconnected from Gitea.');
		} catch {
			appToast.error('Failed to disconnect from Gitea.');
		}
	}

	async function loadAvailableRepos() {
		loadingRepos = true;
		showRepoSelector = true;
		repoSearch = '';
		try {
			availableRepos = await listGiteaRepos(slug);
			selectedRepoIds = new Set(availableRepos.filter(r => r.linked).map(r => r.gitea_repo_id));
		} catch {
			appToast.error('Failed to load Gitea repositories.');
		} finally {
			loadingRepos = false;
		}
	}

	function toggleRepo(repoId: number) {
		const next = new Set(selectedRepoIds);
		if (next.has(repoId)) {
			next.delete(repoId);
		} else {
			next.add(repoId);
		}
		selectedRepoIds = next;
	}

	async function saveRepoSelection() {
		savingRepos = true;
		try {
			const newIds = [...selectedRepoIds].filter(id => !availableRepos.find(r => r.gitea_repo_id === id && r.linked));
			if (newIds.length > 0) {
				try {
					await linkGiteaRepos(slug, newIds);
					appToast.success('Repositories linked.');
				} catch {
					appToast.error('Failed to link repositories.');
				}
			}
			for (const repo of availableRepos.filter(r => r.linked)) {
				if (!selectedRepoIds.has(repo.gitea_repo_id)) {
					const linked = status?.repos.find(r => r.gitea_repo_id === repo.gitea_repo_id);
					if (linked) {
						try { await unlinkGiteaRepo(slug, linked.id); } catch { /* ignore */ }
					}
				}
			}
			status = await getGiteaStatus(slug);
			showRepoSelector = false;
			repoSearch = '';
		} finally {
			savingRepos = false;
		}
	}

	async function handleTransitionToggle(event: string, active: boolean) {
		const updated = transitions.map(t => t.event === event ? { ...t, is_active: active } : t);
		transitions = updated;
		try {
			await updateGiteaAutoTransitions(slug, updated);
		} catch {
			appToast.error('Failed to update auto-transition.');
		}
	}

	const TRANSITION_LABELS: Record<string, { label: string; description: string }> = {
		branch_created: { label: 'Branch created', description: 'Move issue to target status when a branch is created.' },
		pr_opened: { label: 'PR opened', description: 'Move issue to target status when a pull request is opened.' },
		pr_merged: { label: 'PR merged', description: 'Move issue to target status when a pull request is merged.' },
	};
</script>

<div class="mx-auto max-w-2xl space-y-8 p-6">
	<div>
		<h2 class="text-lg font-semibold text-[var(--color-text-primary)]">Gitea Integration</h2>
		<p class="mt-1 text-sm text-[var(--color-text-tertiary)]">
			Connect your Gitea instance to link repositories and track pull requests.
		</p>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Loader2 size={20} class="animate-spin text-[var(--color-text-tertiary)]" />
		</div>
	{:else if !status?.connected}
		<!-- State 1: Not connected — show connect form -->
		<div class="rounded-lg border border-[var(--app-border)] p-6">
			<div class="flex items-center gap-3 mb-4">
				<div class="flex h-10 w-10 items-center justify-center rounded-lg bg-[var(--color-bg-tertiary)]">
					<GitBranch size={20} class="text-[var(--color-text-secondary)]" />
				</div>
				<div>
					<h3 class="text-sm font-medium text-[var(--color-text-primary)]">Connect Gitea</h3>
					<p class="text-xs text-[var(--color-text-tertiary)]">Enter your Gitea instance details to get started.</p>
				</div>
			</div>

			<div class="space-y-4">
				<div>
					<label for="gitea-instance-url" class="mb-1 block text-xs font-medium text-[var(--color-text-secondary)]">Instance URL</label>
					<Input
						id="gitea-instance-url"
						type="url"
						placeholder="https://gitea.com"
						bind:value={instanceUrl}
					/>
				</div>

				<div>
					<label for="gitea-access-token" class="mb-1 block text-xs font-medium text-[var(--color-text-secondary)]">Access Token</label>
					<div class="relative">
						<Input
							id="gitea-access-token"
							type={showToken ? 'text' : 'password'}
							placeholder="Enter your personal access token"
							bind:value={accessToken}
							class="pr-16"
						/>
						<button
							type="button"
							onclick={() => (showToken = !showToken)}
							class="absolute right-2 top-1/2 -translate-y-1/2 text-xs text-[var(--color-text-tertiary)] hover:text-[var(--color-text-secondary)]"
						>
							{showToken ? 'Hide' : 'Show'}
						</button>
					</div>
				</div>

				<div>
					<label for="gitea-webhook-secret" class="mb-1 block text-xs font-medium text-[var(--color-text-secondary)]">Webhook Secret <span class="text-[var(--color-text-tertiary)]">(optional)</span></label>
					<Input
						id="gitea-webhook-secret"
						type="password"
						placeholder="Optional webhook secret"
						bind:value={webhookSecret}
					/>
				</div>

				<Button onclick={handleConnect} disabled={connecting || !instanceUrl.trim() || !accessToken.trim()}>
					{#if connecting}
						<Loader2 size={14} class="animate-spin" />
					{:else}
						Connect
					{/if}
				</Button>
			</div>
		</div>

		<div class="rounded-md bg-[var(--color-bg-secondary)] px-4 py-3">
			<p class="text-xs text-[var(--color-text-tertiary)]">
				Create a Personal Access Token in <strong>Gitea → Settings → Applications → Generate Token</strong> with the <code>repo</code> scope.
				Webhook URL: <code>/api/gitea/webhook</code>
				<br />
				Webhook events to enable: <code>issues</code>, <code>issue_comment</code> (comments sync), <code>pull_request</code>, <code>push</code>.
			</p>
		</div>
	{:else}
		<!-- State 2: Connected -->
		<div class="space-y-6">
			<!-- Connection info -->
			<div class="rounded-lg border border-[var(--app-border)] p-4">
				<div class="flex items-center justify-between">
					<div class="flex items-center gap-3">
						<div class="flex h-8 w-8 items-center justify-center rounded-lg bg-[var(--color-bg-tertiary)]">
							<GitBranch size={16} class="text-[var(--color-text-secondary)]" />
						</div>
						<div>
							<div class="flex items-center gap-2">
								<span class="text-sm font-medium text-[var(--color-text-primary)]">{status.instance?.account_login}</span>
							</div>
							<p class="text-xs text-[var(--color-text-tertiary)]">{status.instance?.instance_url}</p>
						</div>
					</div>
					<Button variant="destructive" size="sm" onclick={handleDisconnect}>
						Disconnect
					</Button>
				</div>
			</div>

			<!-- Linked repos -->
			<div>
				<div class="flex items-center justify-between">
					<h3 class="text-sm font-medium text-[var(--color-text-primary)]">Linked Repositories</h3>
					{#if !showRepoSelector}
						<button
							onclick={loadAvailableRepos}
							class="flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-hover)] hover:text-[var(--color-text-primary)] transition-colors"
						>
							<Plus size={13} />
							Manage
						</button>
					{/if}
				</div>

				{#if showRepoSelector}
					<div class="mt-3 rounded-lg border border-[var(--app-border)] p-3">
						{#if loadingRepos}
							<div class="flex items-center justify-center py-8">
								<Loader2 size={18} class="animate-spin text-[var(--color-text-tertiary)]" />
							</div>
						{:else}
							<!-- Search -->
							<div class="relative">
								<Search size={14} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-text-tertiary)]" />
								<Input
									type="text"
									placeholder="Search repositories..."
									bind:value={repoSearch}
									class="pl-8 h-8 text-sm"
								/>
							</div>

							<!-- Select all -->
							<button
								onclick={toggleSelectAll}
								class="mt-2 flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-sm text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-hover)]"
							>
								<Checkbox
									checked={allFilteredSelected}
									indeterminate={someFilteredSelected}
									class="pointer-events-none"
								/>
								<span class="text-xs">{repoSearch ? `Select all filtered` : 'Select all'} ({filteredRepos.length})</span>
							</button>

							<!-- Repo list -->
							<div class="mt-1 max-h-64 space-y-0.5 overflow-y-auto">
								{#each filteredRepos as repo}
									<button
										onclick={() => toggleRepo(repo.gitea_repo_id)}
										class="flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-sm hover:bg-[var(--color-bg-hover)] {selectedRepoIds.has(repo.gitea_repo_id) ? 'bg-[var(--color-bg-hover)]/50' : ''}"
									>
										<Checkbox
											checked={selectedRepoIds.has(repo.gitea_repo_id)}
											class="pointer-events-none"
										/>
										<GitBranch size={14} class="shrink-0 text-[var(--color-text-tertiary)]" />
										<span class="truncate text-[var(--color-text-primary)]">{repo.full_name}</span>
										{#if repo.private}
											<Badge variant="outline" class="ml-auto shrink-0 text-[9px]">Private</Badge>
										{/if}
									</button>
								{:else}
									<p class="py-4 text-center text-xs text-[var(--color-text-tertiary)]">
										{repoSearch ? 'No repositories match your search.' : 'No repositories available.'}
									</p>
								{/each}
							</div>

							<!-- Actions -->
							<div class="mt-3 flex items-center justify-between border-t border-[var(--app-border)] pt-3">
								<span class="text-xs text-[var(--color-text-tertiary)]">
									{selectedRepoIds.size} selected
								</span>
								<div class="flex gap-2">
									<Button variant="outline" size="sm" onclick={() => { showRepoSelector = false; repoSearch = ''; }}>Cancel</Button>
									<Button size="sm" onclick={saveRepoSelection} disabled={savingRepos}>
										{#if savingRepos}
											<Loader2 size={14} class="animate-spin" />
										{:else}
											Save
										{/if}
									</Button>
								</div>
							</div>
						{/if}
					</div>
				{:else if status.repos.length === 0}
					<p class="mt-3 text-sm text-[var(--color-text-tertiary)]">No repositories linked yet. Click Manage to link repositories.</p>
				{:else}
					<div class="mt-3 space-y-1">
						{#each status.repos as repo}
							<div class="flex items-center justify-between rounded-md border border-[var(--app-border)] px-3 py-2">
								<div class="flex items-center gap-2">
									<GitBranch size={14} class="text-[var(--color-text-tertiary)]" />
									<span class="text-sm text-[var(--color-text-primary)]">{repo.full_name}</span>
									<span class="text-xs text-[var(--color-text-tertiary)]">{repo.default_branch}</span>
								</div>
								<div class="flex items-center gap-2">
									<a
										href="{status.instance?.instance_url}/{repo.full_name}"
										target="_blank"
										rel="noopener noreferrer"
										class="text-[var(--color-text-tertiary)] hover:text-[var(--color-text-secondary)]"
									>
										<ExternalLink size={14} />
									</a>
									<button
										onclick={async () => {
											try {
												await unlinkGiteaRepo(slug, repo.id);
												status = await getGiteaStatus(slug);
												appToast.success('Repository unlinked');
											} catch {
												appToast.error('Failed to unlink repository');
											}
										}}
										class="text-[var(--color-text-tertiary)] hover:text-[var(--color-error)]"
										title="Unlink repository"
									>
										<Trash2 size={14} />
									</button>
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Issue sync status -->
			<div class="rounded-lg border border-[var(--app-border)] p-4">
				<div class="flex items-center gap-3">
					<div class="flex h-8 w-8 items-center justify-center rounded-lg bg-[var(--color-bg-tertiary)]">
						<GitBranch size={16} class="text-[var(--color-text-secondary)]" />
					</div>
					<div class="flex-1">
						<h3 class="text-sm font-medium text-[var(--color-text-primary)]">Issue Sync</h3>
						<p class="text-xs text-[var(--color-text-tertiary)]">Two-way synchronization between Kuayle and Gitea issues.</p>
					</div>
					{#if status.synced_issues_count !== undefined}
						<Badge variant="outline" class="text-xs">{status.synced_issues_count} synced</Badge>
					{/if}
				</div>
				{#if status.sync_errors && status.sync_errors.length > 0}
					<div class="mt-3 rounded-md bg-red-500/10 px-3 py-2">
						<p class="text-xs font-medium text-red-400">Sync errors:</p>
						<ul class="mt-1 space-y-0.5">
							{#each status.sync_errors as err}
								<li class="text-xs text-red-400/80">{err}</li>
							{/each}
						</ul>
					</div>
				{/if}
			</div>

			<!-- Auto-transitions -->
			{#if transitions.length > 0}
				<div>
					<h3 class="text-sm font-medium text-[var(--color-text-primary)]">Automations</h3>
					<p class="mt-1 text-xs text-[var(--color-text-tertiary)]">Automatically transition issues when Gitea events occur.</p>
					<div class="mt-3 space-y-2">
						{#each transitions as t}
							{@const info = TRANSITION_LABELS[t.event]}
							{#if info}
								<div class="flex items-center justify-between rounded-md border border-[var(--app-border)] px-3 py-2.5">
									<div>
										<span class="text-sm text-[var(--color-text-primary)]">{info.label}</span>
										<p class="text-xs text-[var(--color-text-tertiary)]">{info.description}</p>
									</div>
									<Switch checked={t.is_active} onCheckedChange={(v) => handleTransitionToggle(t.event, v)} />
								</div>
							{/if}
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{/if}
</div>
