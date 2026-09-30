<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { appToast } from '$lib/features/toast/toast';
	import { Button } from '$lib/components/ui/button';
	import { getMe, updateProfile } from '$lib/api/auth';
	import { getGiteaUserToken, setGiteaUserToken } from '$lib/api/gitea';
	import { authState } from '$lib/features/auth/auth.state.svelte';
	import type { User } from '$lib/types/auth';
	import { Copy } from 'lucide-svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale, setLocale } from '$lib/paraglide/runtime.js';

	const slug = $derived(page.params.workspaceSlug ?? '');

	let user = $state<User | null>(null);
	let name = $state('');
	let displayName = $state('');
	let avatarUrl = $state('');
	// Write-only: the stored token is never read back from the API.
	let giteaToken = $state('');
	let hasGiteaToken = $state(false);
	let saving = $state(false);

	onMount(async () => {
		user = authState.user ?? await getMe();
		name = user.name;
		displayName = user.display_name;
		avatarUrl = user.avatar_url ?? '';
		hasGiteaToken = user.has_gitea_token ?? false;
		if (slug) {
			try {
				hasGiteaToken = (await getGiteaUserToken(slug)).has_gitea_token;
			} catch {
				// Non-fatal: fall back to the value from the profile payload.
			}
		}
	});

	async function saveProfile() {
		if (!user || !name.trim()) {
			appToast.error(m['settings.profile.name_required']());
			return;
		}
		saving = true;
		try {
			// The token is saved first: if Gitea rejects it we surface the error
			// and skip the rest so the user can correct it in one go.
			if (giteaToken.trim() !== '') {
				const state = await setGiteaUserToken(slug, giteaToken.trim());
				hasGiteaToken = state.has_gitea_token;
				giteaToken = '';
			}
			const updated = await updateProfile({
				name: name.trim(),
				display_name: displayName.trim(),
				avatar_url: avatarUrl.trim() === '' ? null : avatarUrl.trim()
			});
			user = updated;
			authState.setUser({ ...updated, has_gitea_token: hasGiteaToken });
			name = updated.name;
			displayName = updated.display_name;
			avatarUrl = updated.avatar_url ?? '';
			appToast.success(m['settings.profile.updated']());
		} catch (err: any) {
			appToast.apiError(err, m['settings.profile.failed_update']());
		} finally {
			saving = false;
		}
	}

	async function clearGiteaToken() {
		if (!slug) return;
		saving = true;
		try {
			const state = await setGiteaUserToken(slug, '');
			hasGiteaToken = state.has_gitea_token;
			giteaToken = '';
			appToast.success(m['settings.profile.updated']());
		} catch (err: any) {
			appToast.apiError(err, m['settings.profile.failed_update']());
		} finally {
			saving = false;
		}
	}

	async function copyUserId() {
		if (!user) return;
		try {
			await navigator.clipboard.writeText(user.id);
			appToast.success(m['settings.profile.id_copied']());
		} catch {
			appToast.error(m['settings.profile.failed_copy_id']());
		}
	}
</script>

<div class="mx-auto max-w-2xl px-8 py-10">
	<h1 class="text-2xl font-semibold text-[var(--color-text-primary)]">{m['settings.profile.title']()}</h1>
	<p class="mt-1 text-sm text-[var(--color-text-tertiary)]">{m['settings.profile.desc']()}</p>

	{#if user}
		<div class="mt-8 rounded-lg border border-[var(--app-border)] bg-[var(--color-bg-secondary)]">
			<div class="flex items-center justify-between gap-4 px-5 py-4">
				<div>
					<p class="text-sm font-medium text-[var(--color-text-primary)]">{m['settings.profile.email']()}</p>
					<p class="text-xs text-[var(--color-text-tertiary)]">{m['settings.profile.email_desc']()}</p>
				</div>
				<span class="truncate text-sm text-[var(--color-text-secondary)]">{user.email}</span>
			</div>

			<div class="border-t border-[var(--app-border)]"></div>

			<div class="flex items-center justify-between gap-4 px-5 py-4">
				<div>
					<p class="text-sm font-medium text-[var(--color-text-primary)]">{m['settings.profile.user_id']()}</p>
					<p class="text-xs text-[var(--color-text-tertiary)]">{m['settings.profile.user_id_desc']()}</p>
				</div>
				<div class="flex min-w-0 items-center gap-2">
					<span class="truncate font-mono text-xs text-[var(--color-text-secondary)]">{user.id}</span>
					<Button variant="outline" size="sm" onclick={copyUserId}>
						<Copy size={13} />
						{m['settings.profile.copy']()}
					</Button>
				</div>
			</div>

			<div class="border-t border-[var(--app-border)]"></div>

			<div class="flex items-center justify-between gap-4 px-5 py-4">
				<div>
					<p class="text-sm font-medium text-[var(--color-text-primary)]">{m['settings.profile.name']()}</p>
					<p class="text-xs text-[var(--color-text-tertiary)]">{m['settings.profile.name_desc']()}</p>
				</div>
				<input
					type="text"
					bind:value={name}
					class="w-[240px] rounded-md border border-[var(--app-border)] bg-[var(--color-bg)] px-3 py-1.5 text-sm text-[var(--color-text-primary)] outline-none focus:border-[var(--app-accent)]"
				/>
			</div>

			<div class="border-t border-[var(--app-border)]"></div>

			<div class="flex items-center justify-between gap-4 px-5 py-4">
				<div>
					<p class="text-sm font-medium text-[var(--color-text-primary)]">{m['settings.profile.display_name']()}</p>
					<p class="text-xs text-[var(--color-text-tertiary)]">{m['settings.profile.display_name_desc']()}</p>
				</div>
				<input
					type="text"
					bind:value={displayName}
					class="w-[240px] rounded-md border border-[var(--app-border)] bg-[var(--color-bg)] px-3 py-1.5 text-sm text-[var(--color-text-primary)] outline-none focus:border-[var(--app-accent)]"
				/>
			</div>

			<div class="border-t border-[var(--app-border)]"></div>

			<div class="flex items-center justify-between gap-4 px-5 py-4">
				<div>
					<p class="text-sm font-medium text-[var(--color-text-primary)]">{m['settings.profile.gitea_token']()}</p>
					<p class="text-xs text-[var(--color-text-tertiary)]">{m['settings.profile.gitea_token_desc']()}</p>
				</div>
				<div class="flex w-[240px] flex-col items-end gap-1">
					<input
						type="password"
						bind:value={giteaToken}
						placeholder={hasGiteaToken ? '••••••••••••' : 'token'}
						autocomplete="off"
						class="w-full rounded-md border border-[var(--app-border)] bg-[var(--color-bg)] px-3 py-1.5 text-sm text-[var(--color-text-primary)] outline-none focus:border-[var(--app-accent)]"
					/>
					<div class="flex w-full items-center justify-between gap-2">
						<span class="text-[11px] {hasGiteaToken ? 'text-[var(--color-success)]' : 'text-[var(--color-text-tertiary)]'}">
							{hasGiteaToken ? m['settings.profile.gitea_token_set']() : m['settings.profile.gitea_token_unset']()}
						</span>
						{#if hasGiteaToken}
							<button
								type="button"
								onclick={clearGiteaToken}
								disabled={saving}
								class="text-[11px] text-[var(--color-text-tertiary)] underline hover:text-[var(--color-error)] disabled:opacity-50"
							>
								{m['settings.profile.gitea_token_remove']()}
							</button>
						{/if}
					</div>
				</div>
			</div>

			<div class="border-t border-[var(--app-border)]"></div>

			<div class="flex items-center justify-between gap-4 px-5 py-4">
				<div>
					<p class="text-sm font-medium text-[var(--color-text-primary)]">{m['settings.profile.avatar_url']()}</p>
					<p class="text-xs text-[var(--color-text-tertiary)]">{m['settings.profile.avatar_url_desc']()}</p>
				</div>
				<input
					type="url"
					bind:value={avatarUrl}
					placeholder="https://"
					class="w-[240px] rounded-md border border-[var(--app-border)] bg-[var(--color-bg)] px-3 py-1.5 text-sm text-[var(--color-text-primary)] outline-none focus:border-[var(--app-accent)]"
				/>
			</div>
		</div>

		<div class="mt-4 flex justify-end">
			<Button onclick={saveProfile} disabled={saving || !name.trim()}>{saving ? m['settings.profile.saving']() : m['settings.profile.save_profile']()}</Button>
		</div>
	{:else}
		<div class="mt-8 flex justify-center py-8">
			<div class="h-5 w-5 animate-spin rounded-full border-2 border-[var(--color-text-tertiary)] border-t-transparent"></div>
		</div>
	{/if}
</div>
