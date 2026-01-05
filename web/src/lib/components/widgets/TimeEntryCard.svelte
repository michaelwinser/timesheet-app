<script lang="ts">
	import type { TimeEntry } from '$lib/api/types';
	import ProjectChip from './ProjectChip.svelte';

	interface Props {
		entry: TimeEntry;
		editable?: boolean;
		onupdate?: (data: { hours?: number; description?: string }) => void;
		ondelete?: () => void;
	}

	let { entry, editable = true, onupdate, ondelete }: Props = $props();

	let editing = $state(false);
	let editHours = $state(entry.hours);
	let editDescription = $state(entry.description || '');

	const isLocked = $derived(!!entry.invoice_id);

	function handleEdit() {
		if (!editable || isLocked) return;
		editing = true;
		editHours = entry.hours;
		editDescription = entry.description || '';
	}

	function handleSave() {
		onupdate?.({ hours: editHours, description: editDescription });
		editing = false;
	}

	function handleCancel() {
		editing = false;
	}
</script>

<div
	class="bg-white dark:bg-gray-700/50 border border-gray-200 dark:border-gray-600 rounded-lg p-3 {editable && !isLocked ? 'hover:shadow-sm cursor-pointer' : ''} {isLocked ? 'opacity-75' : ''}"
>
	{#if editing}
		<div class="space-y-2">
			<div class="flex items-center gap-2">
				{#if entry.project}
					<ProjectChip project={entry.project} />
				{/if}
				<input
					type="number"
					step="0.25"
					min="0"
					bind:value={editHours}
					class="w-20 px-2 py-1 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded text-sm"
				/>
				<span class="text-sm text-gray-500 dark:text-gray-400">hours</span>
			</div>
			<input
				type="text"
				bind:value={editDescription}
				placeholder="Description (optional)"
				class="w-full px-2 py-1 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded text-sm"
			/>
			<div class="flex justify-end gap-2">
				<button
					type="button"
					class="px-2 py-1 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white"
					onclick={handleCancel}
				>
					Cancel
				</button>
				<button
					type="button"
					class="px-2 py-1 text-sm bg-primary-600 text-white rounded hover:bg-primary-700"
					onclick={handleSave}
				>
					Save
				</button>
			</div>
		</div>
	{:else}
		<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
		<div class="flex items-center justify-between" onclick={handleEdit}>
			<div class="flex items-center gap-2">
				{#if entry.project}
					<ProjectChip project={entry.project} />
				{/if}
				<span class="font-medium text-gray-900 dark:text-white">{entry.hours}h</span>
				{#if entry.description}
					<span class="text-gray-500 dark:text-gray-400 text-sm truncate max-w-xs">{entry.description}</span>
				{/if}
			</div>
			<div class="flex items-center gap-2">
				{#if isLocked}
					<span class="text-xs text-gray-400 dark:text-gray-500">Invoiced</span>
				{:else if editable}
					<button
						type="button"
						class="text-gray-400 hover:text-red-600 dark:hover:text-red-400"
						onclick={(e) => { e.stopPropagation(); ondelete?.(); }}
					>
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
						</svg>
					</button>
				{/if}
			</div>
		</div>
	{/if}
</div>
