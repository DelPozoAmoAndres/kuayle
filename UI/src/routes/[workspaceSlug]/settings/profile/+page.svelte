<script lang="ts">
	import { onMount } from 'svelte';
	import { appToast } from '$lib/features/toast/toast';
	import { Button } from '$lib/components/ui/button';
	import { getMe, updateProfile } from '$lib/api/auth';
	import { authState } from '$lib/features/auth/auth.state.svelte';
	import type { User } from '$lib/types/auth';
	import { Copy } from 'lucide-svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale, setLocale } from '$lib/paraglide/runtime.js';

	let user = $state<User | null>(null);
	let name = $state('');
	let displayName = $state('');
	let avatarUrl = $state('');
	let giteaLogin = $state('');
	let saving = $state(false);

	const GITEA_LOGIN_PATTERN = /^[A-Za-z0-9._-]{1,100}$/;
	let giteaLoginInvalid = $derived(giteaLogin.trim() !== '' && !GITEA_LOGIN_PATTERN.test(giteaLogin.trim()));

	onMount(async () => {
		user = authState.user ?? await getMe();
		name = user.name;
		displayName = user.display_name;
		avatarUrl = user.avatar_url ?? '';
		giteaLogin = user.gitea_login ?? '';
	});

	async function saveProfile() {
		if (!user || !name.trim()) {
			appToast.error(m['settings.profile.name_required']());
			return;
		}
		if (giteaLoginInvalid) return;
		saving = true;
		try {
			const updated = await updateProfile({
				name: name.trim(),
				display_name: displayName.trim(),
				avatar_url: avatarUrl.trim() === '' ? null : avatarUrl.trim(),
				gitea_login: giteaLogin.trim()
			});
			user = updated;
			authState.setUser(updated);
			name = updated.name;
			displayName = updated.display_name;
			avatarUrl = updated.avatar_url ?? '';
			giteaLogin = updated.gitea_login ?? '';
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
					<p class="text-sm font-medium text-[var(--color-text-primary)]">{m['settings.profile.gitea_account']()}</p>
					<p class="text-xs text-[var(--color-text-tertiary)]">{m['settings.profile.gitea_account_desc']()}</p>
				</div>
				<div class="flex w-[240px] flex-col items-end gap-1">
					<input
						type="text"
						bind:value={giteaLogin}
						placeholder="username"
						aria-invalid={giteaLoginInvalid}
						class="w-full rounded-md border {giteaLoginInvalid ? 'border-red-500/70' : 'border-[var(--app-border)]'} bg-[var(--color-bg)] px-3 py-1.5 text-sm text-[var(--color-text-primary)] outline-none focus:border-[var(--app-accent)]"
					/>
					{#if giteaLoginInvalid}
						<p class="text-right text-xs text-red-400">{m['settings.profile.gitea_account_invalid']()}</p>
					{/if}
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
			<Button onclick={saveProfile} disabled={saving || !name.trim() || giteaLoginInvalid}>{saving ? m['settings.profile.saving']() : m['settings.profile.save_profile']()}</Button>
		</div>
	{:else}
		<div class="mt-8 flex justify-center py-8">
			<div class="h-5 w-5 animate-spin rounded-full border-2 border-[var(--color-text-tertiary)] border-t-transparent"></div>
		</div>
	{/if}
</div>
