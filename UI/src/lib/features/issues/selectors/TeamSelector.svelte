<script lang="ts">
	import ComboboxPopover from '$lib/components/shared/ComboboxPopover.svelte';
	import * as Command from '$lib/components/ui/command/index.js';
	import type { Team } from '$lib/types/team';
	import type { Snippet } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale, setLocale } from '$lib/paraglide/runtime.js';

	let {
		open = $bindable(false),
		teams,
		value,
		onchange,
		trigger,
		width = 'w-48',
		align = 'start' as 'start' | 'center' | 'end',
		shortcutKey,
		showNone = false,
	}: {
		open?: boolean;
		teams: Team[];
		value: string | undefined;
		onchange: (teamId: string) => void;
		trigger: Snippet;
		width?: string;
		align?: 'start' | 'center' | 'end';
		shortcutKey?: string;
		showNone?: boolean;
	} = $props();
</script>

<ComboboxPopover bind:open placeholder={m['sharedComponents.selectors.search_teams']()} {width} {align} {shortcutKey} {trigger}>
	{#if showNone}
		<Command.Item
			value="Sin equipo"
			keywords={['no team', 'none', 'sin equipo']}
			onSelect={() => { onchange(''); open = false; }}
			data-checked={!value || value === ''}
			class="flex items-center gap-2"
		>
			<span class="flex h-5 w-5 items-center justify-center rounded bg-[var(--color-bg-tertiary)] text-[10px] font-medium shrink-0">
				–
			</span>
			<span class="text-[var(--color-text-tertiary)]">Sin equipo</span>
		</Command.Item>
		<Command.Separator />
	{/if}
	{#each teams as team (team.id)}
		<Command.Item
			value={team.name}
			keywords={[team.key]}
			onSelect={() => { onchange(team.id); open = false; }}
			data-checked={value === team.id}
			class="flex items-center gap-2"
		>
			<span class="flex h-5 w-5 items-center justify-center rounded bg-[var(--color-bg-tertiary)] text-[10px] font-medium shrink-0">
				{team.key.charAt(0)}
			</span>
			{team.name}
		</Command.Item>
	{/each}
</ComboboxPopover>
