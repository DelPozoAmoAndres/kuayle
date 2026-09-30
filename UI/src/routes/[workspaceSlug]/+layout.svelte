<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { authState } from '$lib/features/auth/auth.state.svelte';
	import { getWorkspace } from '$lib/api/workspaces';
	import { listProjects } from '$lib/api/projects';
	import { listLabels } from '$lib/api/labels';
	import { listMembers } from '$lib/api/members';
	import { listViews } from '$lib/api/views';
	import { listNotifications } from '$lib/api/notifications';
	import type { Workspace } from '$lib/types/workspace';
	import type { Project } from '$lib/types/project';
	import type { Label } from '$lib/types/label';
	import type { WorkspaceMember } from '$lib/types/workspace';
	import type { View } from '$lib/types/view';
	import type { IssuePriority } from '$lib/types/issue';
	import Sidebar from '$lib/components/layout/Sidebar.svelte';
	import CommandPalette from '$lib/components/layout/CommandPalette.svelte';
	import CreateIssueDialog from '$lib/features/issues/CreateIssueDialog.svelte';
	import ShortcutHelp from '$lib/components/shared/ShortcutHelp.svelte';
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { IsMobile } from '$lib/hooks/is-mobile.svelte';
	import { issuesState } from '$lib/features/issues/issues.state.svelte';
	import { showIssueCreatedToast } from '$lib/features/issues/issue-created-toast';
	import { preferencesState } from '$lib/features/preferences/preferences.state.svelte';
	import { statusesState } from '$lib/features/issues/statuses.state.svelte';
	import { sidebarState } from '$lib/features/layout/sidebar.state.svelte';
	import { createShortcutEngine, type ShortcutDef } from '$lib/utils/keyboard';
	import { Menu, Search, SquarePen } from 'lucide-svelte';
	import { appToast } from '$lib/features/toast/toast';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale, setLocale } from '$lib/paraglide/runtime.js';
	import TerminalDock from '$lib/features/dev-machines/TerminalDock.svelte';
	import { setTerminalDock } from '$lib/features/dev-machines/terminal-dock-context.svelte';

	let { children } = $props();
	let workspace = $state<Workspace | null>(null);
	let projects = $state<Project[]>([]);
	let labels = $state<Label[]>([]);
	let members = $state<WorkspaceMember[]>([]);
	let views = $state<View[]>([]);
	let unreadCount = $state(0);
	let showCommandPalette = $state(false);
	let showCreateIssue = $state(false);
	let showShortcutHelp = $state(false);
	let showMobileSidebar = $state(false);
	let authReady = $state(false);
	let workspaceLoadId = 0;
	const isMobile = new IsMobile();
	const terminalDock = setTerminalDock();
	const slug = $derived(page.params.workspaceSlug ?? '');
	const isSettings = $derived(page.url.pathname.includes('/settings'));

	$effect(() => {
		terminalDock.setWorkspace(slug);
	});

	async function loadWorkspaceData(workspaceSlug: string) {
		const loadId = ++workspaceLoadId;
		// Only a failure on the workspace itself (not authenticated / not a member)
		// should send the user back to login. A transient failure loading labels,
		// members, views... must never kick an invited member out of the app.
		const renderRequest = getWorkspace(workspaceSlug).then(
			(ws) => {
				if (loadId !== workspaceLoadId) return;
				workspace = ws;
			},
			(error) => {
				if (loadId === workspaceLoadId) goto('/login');
				throw error;
			}
		);
		const navigationRequest = Promise.all([
			listProjects(workspaceSlug),
			listLabels(workspaceSlug),
			listMembers(workspaceSlug),
			listViews(workspaceSlug),
			listNotifications()
		])
			.then(([p, l, m, v, notifRes]) => {
				if (loadId !== workspaceLoadId) return;
				projects = p;
				sidebarState.projects = p;
				labels = l;
				members = m;
				views = v;
				unreadCount = notifRes.unread_count;
			})
			.catch(() => {
				// Keep whatever navigation data we already had.
			});
		await Promise.all([renderRequest, navigationRequest]).catch(() => {});
		// Workspace statuses are shared across the whole app: load them once.
		void statusesState.load(workspaceSlug).catch(() => {});
	}

	async function reloadViews(workspaceSlug: string) {
		try {
			views = await listViews(workspaceSlug);
		} catch {
			// Keep the current navigation list if a background refresh fails.
		}
	}

	function handleAppRefresh(e: Event) {
		const detail = (e as CustomEvent<{ slug?: string; resources?: string[] }>).detail;
		if (detail?.slug && detail.slug !== slug) return;
		const resources = detail?.resources;
		if (!slug) return;
		if (!resources || resources.length === 0) {
			loadWorkspaceData(slug);
			if (issuesState.issues.length > 0) {
				issuesState.load(slug, issuesState.filters);
			}
			return;
		}
		if (resources.includes('issues') && issuesState.issues.length > 0) {
			issuesState.load(slug, issuesState.filters);
		}
		if (resources.includes('workspace')) {
			getWorkspace(slug).then((ws) => { workspace = ws; }).catch(() => {});
		}
		if (resources.includes('projects')) {
			listProjects(slug).then((p) => {
				projects = p;
				sidebarState.projects = p;
			}).catch(() => {});
		}
		if (resources.includes('labels')) {
			listLabels(slug).then((l) => { labels = l; }).catch(() => {});
		}
		if (resources.includes('members')) {
			listMembers(slug).then((m) => { members = m; }).catch(() => {});
		}
		if (resources.includes('views')) {
			reloadViews(slug);
		}
		if (resources.includes('notifications')) {
			listNotifications().then((r) => { unreadCount = r.unread_count; }).catch(() => {});
		}
	}

	onMount(async () => {
		await authState.init();
		if (!authState.authenticated) {
			goto('/login');
			return;
		}
		void preferencesState.syncRemote();
		authReady = true;
	});

	// Re-fetch all data when workspace slug changes (e.g. workspace switch)
	let loadedSlug = '';
	$effect(() => {
		if (authReady && slug && slug !== loadedSlug) {
			loadedSlug = slug;
			workspace = null;
			projects = [];
			labels = [];
			members = [];
			views = [];
			sidebarState.projects = [];
			void loadWorkspaceData(slug);
		}
	});

	// Full shortcut definitions
	const shortcutDefs = $derived<ShortcutDef[]>([
		// Navigation sequences (G + key)
		{ keys: ['g', 'i'], handler: () => goto(`/${slug}/inbox`), label: m['sidebar.go_inbox'](), category: m['sidebar.navigation']() },
		{ keys: ['g', 'm'], handler: () => goto(`/${slug}/my-issues`), label: m['sidebar.go_my_issues'](), category: m['sidebar.navigation']() },
		{ keys: ['g', 'a'], handler: () => goto(`/${slug}/insights`), label: m['sidebar.go_insights'](), category: m['sidebar.navigation']() },
		{ keys: ['g', 'p'], handler: () => goto(`/${slug}/projects`), label: m['sidebar.go_projects'](), category: m['sidebar.navigation']() },
		{ keys: ['g', 's'], handler: () => goto(`/${slug}/settings`), label: m['sidebar.go_settings'](), category: m['sidebar.navigation']() },
		// Actions
		{
			key: 'c',
			handler: () => {
				// Ensure workspace statuses are loaded
				void statusesState.load(slug);
				showCreateIssue = true;
			},
			label: m['sidebar.create_issue'](),
			category: m['sidebar.actions']()
		},
		{ key: 'k', meta: true, handler: () => (showCommandPalette = !showCommandPalette), label: m['sidebar.command_palette'](), category: m['sidebar.actions']() },
		{ key: '/', handler: () => (showCommandPalette = true), label: m['sidebar.search'](), category: m['sidebar.actions']() },
		{ key: '?', shift: true, handler: () => (showShortcutHelp = !showShortcutHelp), label: m['sidebar.keyboard_shortcuts'](), category: m['sidebar.help']() },
	]);

	const shortcutEngine = createShortcutEngine(shortcutDefs);

	onMount(() => {
		document.addEventListener('keydown', shortcutEngine.handler);
		window.addEventListener('app:refresh', handleAppRefresh);
		return () => {
			document.removeEventListener('keydown', shortcutEngine.handler);
			window.removeEventListener('app:refresh', handleAppRefresh);
		};
	});

	function openCreateIssue() {
		void statusesState.load(slug);
		showCreateIssue = true;
		showMobileSidebar = false;
	}

	function singleFilterValue(value?: string): string | undefined {
		if (!value) return undefined;
		const values = value.split(',').filter(Boolean);
		return values.length === 1 ? values[0] : undefined;
	}

	function getActiveIssueFilters(): Record<string, string> {
		const pathname = page.url.pathname;
		const projectPath = page.params.projectId ? `/${slug}/projects/${page.params.projectId}` : '';

		if (pathname === `/${slug}/my-issues`) return issuesState.filters;
		if (projectPath && pathname === projectPath) return issuesState.filters;
		return {};
	}

	function getCreateStatusId(): string | undefined {
		const value = singleFilterValue(getActiveIssueFilters().status);
		if (!value) return undefined;
		return statusesState.statusById.get(value)?.id ?? statusesState.statusOrder.find((status) => status.slug === value)?.id;
	}

	function getCreatePriority(): IssuePriority | undefined {
		const value = singleFilterValue(getActiveIssueFilters().priority);
		const priority = value === undefined ? NaN : Number(value);
		return [0, 1, 2, 3, 4].includes(priority) ? priority as IssuePriority : undefined;
	}

	function getCreateProjectId(): string | null | undefined {
		const routeProjectId = page.params.projectId;
		if (routeProjectId) return routeProjectId;
		const value = singleFilterValue(getActiveIssueFilters().project);
		if (value === 'none') return null;
		return value;
	}

	function getCreateAssigneeIds(): string[] | undefined {
		const value = singleFilterValue(getActiveIssueFilters().assignee);
		if (value === 'none') return [];
		return value ? [value] : undefined;
	}

	function getCreateLabelIds(): string[] | undefined {
		const value = singleFilterValue(getActiveIssueFilters().label);
		return value ? [value] : undefined;
	}

	// WebSocket connection — reconnects when slug changes
	let ws_conn: WebSocket | null = null;
	let wsSlug = '';
	let wsReconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let wsDestroyed = false;

	$effect(() => {
		if (slug && slug !== wsSlug) {
			wsSlug = slug;
			clearWebSocketReconnect();
			ws_conn?.close();
			connectWebSocket(slug);
		}
	});

	onDestroy(() => {
		wsDestroyed = true;
		clearWebSocketReconnect();
		ws_conn?.close();
		window.removeEventListener('ws:send', handleWSSend as EventListener);
	});

	// Allow child components to send WebSocket messages
	function handleWSSend(e: CustomEvent<any>) {
		if (ws_conn?.readyState === WebSocket.OPEN) {
			ws_conn.send(JSON.stringify(e.detail));
		}
	}
	window.addEventListener('ws:send', handleWSSend as EventListener);

	function connectWebSocket(workspaceSlug: string) {
		if (wsDestroyed || wsSlug !== workspaceSlug) return;
		clearWebSocketReconnect();
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const wsUrl = `${protocol}//${window.location.host}/api/workspaces/${workspaceSlug}/ws`;
		const socket = new WebSocket(wsUrl);
		ws_conn = socket;

		socket.onmessage = (event) => {
			if (socket !== ws_conn) return;
			try {
				const data = JSON.parse(event.data);
				handleWSMessage(data);
			} catch {
				// Ignore malformed messages
			}
		};

		socket.onopen = () => {
			if (socket !== ws_conn) return;
			window.dispatchEvent(new CustomEvent('ws:reconnected'));
		};

		socket.onerror = () => {
			socket.close();
		};

		socket.onclose = () => {
			if (socket !== ws_conn) return;
			// Only reconnect if still on the same workspace
			if (!wsDestroyed && wsSlug === workspaceSlug) {
				wsReconnectTimer = setTimeout(() => connectWebSocket(workspaceSlug), 3000);
			}
		};
	}

	function clearWebSocketReconnect() {
		if (wsReconnectTimer) {
			clearTimeout(wsReconnectTimer);
			wsReconnectTimer = null;
		}
	}

	function handleWSMessage(msg: { type: string; payload: any }) {
		switch (msg.type) {
			case 'issue.created':
			case 'issue.updated':
			case 'issue.triaged':
			case 'issues.bulk_updated':
			case 'issues.bulk_deleted': {
				if (slug && issuesState.issues.length > 0) {
					issuesState.load(slug, issuesState.filters, false);
				}
				window.dispatchEvent(new CustomEvent('ws:issue-updated', { detail: msg.payload }));
				break;
			}
			case 'issue.deleted': {
				if (slug && issuesState.issues.length > 0) {
					issuesState.load(slug, issuesState.filters, false);
				}
				window.dispatchEvent(new CustomEvent('ws:issue-deleted', { detail: msg.payload }));
				break;
			}
			case 'comment.created': {
				window.dispatchEvent(new CustomEvent('ws:comment-created', { detail: msg.payload }));
				break;
			}
			case 'comment.updated': {
				window.dispatchEvent(new CustomEvent('ws:comment-updated', { detail: msg.payload }));
				break;
			}
			case 'comment.deleted': {
				window.dispatchEvent(new CustomEvent('ws:comment-deleted', { detail: msg.payload }));
				break;
			}
			case 'view.created':
			case 'view.updated':
			case 'view.deleted': {
				window.dispatchEvent(new CustomEvent('app:refresh', { detail: { ...msg.payload, resources: ['views'] } }));
				break;
			}
			case 'app.refresh': {
				window.dispatchEvent(new CustomEvent('app:refresh', { detail: msg.payload ?? {} }));
				break;
			}
			case 'github:pr_updated':
			case 'github:branch_created':
			case 'github:commit_pushed': {
				window.dispatchEvent(new CustomEvent(`ws:${msg.type}`, { detail: msg.payload }));
				break;
			}
			case 'notification.created': {
				unreadCount++;
				window.dispatchEvent(new CustomEvent('ws:notification', { detail: msg.payload }));
				break;
			}
			case 'presence.join':
			case 'presence.leave':
			case 'presence.sync':
			case 'cursor.move':
			case 'focus.update':
			case 'focus.leave': {
				window.dispatchEvent(new CustomEvent(`ws:${msg.type}`, { detail: msg.payload }));
				break;
			}
		}
	}
</script>

{#if workspace}
	<div class="flex h-dvh bg-[var(--color-bg)]">
		{#if !isSettings}
			<div class="hidden md:contents">
				<Sidebar
					{workspace}
					{views}
					{projects}
					{unreadCount}
					{slug}
					oncreateissue={openCreateIssue}
					onsearch={() => (showCommandPalette = true)}
					onshortcutshelp={() => (showShortcutHelp = true)}
				/>
			</div>

			<Sheet.Root bind:open={showMobileSidebar}>
				<Sheet.Content side="left" class="w-[min(88vw,320px)] p-0 [&>button]:hidden" showCloseButton={false}>
					<Sheet.Header class="sr-only">
						<Sheet.Title>{m['sidebar.workspace_navigation']()}</Sheet.Title>
						<Sheet.Description>{m['sidebar.navigate_sections']()}</Sheet.Description>
					</Sheet.Header>
					<Sidebar
						{workspace}
						{views}
						{projects}
						{unreadCount}
						{slug}
						mobile
						oncreateissue={openCreateIssue}
						onsearch={() => { showCommandPalette = true; showMobileSidebar = false; }}
						onnavigate={() => (showMobileSidebar = false)}
						onshortcutshelp={() => { showMobileSidebar = false; showShortcutHelp = true; }}
					/>
				</Sheet.Content>
			</Sheet.Root>
		{/if}
		<main class="flex min-w-0 flex-1 flex-col overflow-hidden">
			{#if !isSettings}
				<div class="flex h-12 shrink-0 items-center justify-between border-b border-[var(--app-border)] bg-[var(--color-bg)] px-3 md:hidden">
					<div class="flex min-w-0 items-center gap-2">
						<Button variant="ghost" size="icon-lg" onclick={() => (showMobileSidebar = true)} aria-label={m['sidebar.open_navigation']()}>
							<Menu size={18} />
						</Button>
						<span class="truncate text-sm font-medium text-[var(--color-text-primary)]">{workspace.name}</span>
					</div>
					<div class="flex shrink-0 items-center gap-1">
						<Button variant="ghost" size="icon-lg" onclick={() => (showCommandPalette = true)} aria-label={m['sidebar.search']()}>
							<Search size={18} />
						</Button>
						<Button variant="ghost" size="icon-lg" onclick={openCreateIssue} aria-label={m['sidebar.create_issue']()}>
							<SquarePen size={18} />
						</Button>
					</div>
				</div>
			{/if}
			<div class="min-h-0 flex-1 overflow-auto">
				{@render children()}
			</div>
			<TerminalDock />
		</main>
	</div>

	{#if showCommandPalette}
		<CommandPalette {slug} onclose={() => (showCommandPalette = false)} oncreateissue={openCreateIssue} />
	{/if}

	<CreateIssueDialog
		bind:open={showCreateIssue}
		{slug}
		{projects}
		{labels}
		{members}
		defaultStatusId={getCreateStatusId()}
		defaultPriority={getCreatePriority()}
		defaultProjectId={getCreateProjectId()}
		defaultAssigneeIds={getCreateAssigneeIds()}
		defaultLabelIds={getCreateLabelIds()}
		onlabelcreated={(label) => (labels = [label, ...labels.filter((existing) => existing.id !== label.id)])}
		onsubmit={async (req) => {
			try {
				const created = await issuesState.create(slug, req);
				showIssueCreatedToast(slug, created);
			} catch (err: any) {
				appToast.apiError(err, m['sidebar.failed_create_issue']());
			}
		}}
	/>

	<ShortcutHelp
		bind:open={showShortcutHelp}
		shortcuts={shortcutDefs}
	/>
{:else}
	<div class="flex h-screen items-center justify-center">
	</div>
{/if}
