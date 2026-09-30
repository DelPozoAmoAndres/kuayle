<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale, setLocale } from '$lib/paraglide/runtime.js';

	let {
		open = $bindable(false),
		onsubmit
	}: {
		open: boolean;
		onsubmit: (data: { name: string; description?: string }) => void;
	} = $props();

	let name = $state('');
	let description = $state('');

	$effect(() => {
		if (open) {
			name = '';
			description = '';
		}
	});

	function handleSubmit(e: Event) {
		e.preventDefault();
		if (!name.trim()) return;
		onsubmit({
			name: name.trim(),
			description: description.trim() || undefined
		});
		open = false;
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		class="sm:max-w-[420px] border-[var(--app-border)] bg-[var(--color-bg-secondary)] p-0 overflow-hidden rounded-xl"
	>
		<form onsubmit={handleSubmit}>
			<div class="px-5 pt-5 pb-4 space-y-4">
				<div>
					<h2 class="text-base font-semibold text-[var(--color-text-primary)]">
						{m['projects.create.title']()}
					</h2>
					<p class="mt-0.5 text-xs text-[var(--color-text-tertiary)]">
						{m['projects.create.description']()}
					</p>
				</div>

				<div class="space-y-1.5">
					<Label class="text-xs text-[var(--color-text-secondary)]">{m['cycles.field.name']()}</Label>
					<Input
						bind:value={name}
						placeholder={m['projects.create.name_placeholder']()}
						required
						class="bg-[var(--color-bg)] border-[var(--app-border)] text-[var(--color-text-primary)]"
					/>
				</div>

				<div class="space-y-1.5">
					<Label class="text-xs text-[var(--color-text-secondary)]"
						>{m['cycles.field.description']()} <span class="text-[var(--color-text-tertiary)]">{m['cycles.field.optional']()}</span
						></Label
					>
					<Input
						bind:value={description}
						placeholder={m['projects.create.description_placeholder']()}
						class="bg-[var(--color-bg)] border-[var(--app-border)] text-[var(--color-text-primary)]"
					/>
				</div>
			</div>

			<div class="flex justify-end gap-2 border-t border-[var(--app-border)] px-5 py-3">
				<Button variant="outline" size="sm" type="button" onclick={() => (open = false)}
					>{m['common.cancel']()}</Button
				>
				<Button size="sm" type="submit" disabled={!name.trim()}>{m['projects.create.title']()}</Button>
			</div>
		</form>
	</Dialog.Content>
</Dialog.Root>
