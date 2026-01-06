<script lang="ts">
	import type { TimeEntry } from '$lib/api/types';
	import ProjectChip from './ProjectChip.svelte';

	interface Props {
		entry: TimeEntry;
		editable?: boolean;
		onupdate?: (data: { hours?: number; description?: string }) => void;
		ondelete?: () => void;
		onrefresh?: () => void;
	}

	let { entry, editable = true, onupdate, ondelete, onrefresh }: Props = $props();

	let editing = $state(false);
	let showDetails = $state(false);
	let editHours = $state(entry.hours);
	let editDescription = $state(entry.description || '');

	// Protection states
	const isInvoiced = $derived(!!entry.invoice_id);
	const isPinned = $derived(!!entry.is_pinned);
	const isLocked = $derived(!!entry.is_locked);
	const isStale = $derived(!!entry.is_stale);
	const isProtected = $derived(isInvoiced || isPinned || isLocked);

	function handleEdit() {
		if (!editable || isInvoiced) return;
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

	function formatMinutes(minutes: number): string {
		const hours = Math.floor(minutes / 60);
		const mins = minutes % 60;
		if (hours === 0) return `${mins}m`;
		if (mins === 0) return `${hours}h`;
		return `${hours}h ${mins}m`;
	}
</script>

<div
	class="bg-white dark:bg-gray-700/50 border border-gray-200 dark:border-gray-600 rounded-lg p-3 relative {editable && !isInvoiced ? 'hover:shadow-sm cursor-pointer' : ''} {isInvoiced ? 'opacity-75' : ''}"
>
	<!-- Invoiced corner ribbon -->
	{#if isInvoiced}
		<a
			href="/invoices/{entry.invoice_id}"
			class="absolute top-0 right-0 overflow-hidden w-16 h-16 group"
			title="View invoice"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="absolute transform rotate-45 bg-green-500 group-hover:bg-green-600 text-white text-[10px] font-bold py-0.5 right-[-35px] top-[10px] w-[100px] text-center shadow transition-colors">
				INVOICED
			</div>
		</a>
	{/if}

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
				{#if entry.title || entry.description}
					<span class="text-gray-500 dark:text-gray-400 text-sm truncate max-w-xs">
						{entry.title || entry.description}
					</span>
				{/if}
			</div>
			<div class="flex items-center gap-2">
				<!-- Protection indicators -->
				{#if isPinned}
					<span class="text-gray-400 dark:text-gray-500 {isStale ? 'text-orange-500' : ''}" title="Pinned - user edited">
						<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
							<path d="M10 2a1 1 0 011 1v1.323l3.954 1.582 1.599-.8a1 1 0 01.894 1.79l-1.233.617 1.738 5.42a1 1 0 01-.285 1.05A3.989 3.989 0 0115 15a3.989 3.989 0 01-2.667-1.018 1 1 0 01-.285-1.05l1.715-5.349L10 6.418l-3.763 1.165 1.715 5.349a1 1 0 01-.285 1.05A3.989 3.989 0 015 15a3.989 3.989 0 01-2.667-1.018 1 1 0 01-.285-1.05l1.738-5.42-1.233-.617a1 1 0 01.894-1.79l1.599.8L9 4.323V3a1 1 0 011-1z" />
						</svg>
					</span>
				{/if}
				{#if isLocked && !isInvoiced}
					<span class="text-gray-400 dark:text-gray-500 {isStale ? 'text-orange-500' : ''}" title="Locked">
						<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
							<path fill-rule="evenodd" d="M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z" clip-rule="evenodd" />
						</svg>
					</span>
				{/if}
				<!-- Reset to Computed button (for any edited/locked/stale entry) -->
				{#if (isPinned || isLocked || isStale) && !isInvoiced && entry.computed_hours}
					<button
						type="button"
						class="{isStale ? 'text-orange-500 hover:text-orange-600' : 'text-gray-400 hover:text-blue-600 dark:hover:text-blue-400'}"
						title="{isStale ? `Reset to computed (${entry.computed_hours}h)` : 'Reset to computed values'}"
						onclick={(e) => { e.stopPropagation(); onrefresh?.(); }}
					>
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
						</svg>
					</button>
				{/if}
				<!-- Delete button -->
				{#if editable && !isInvoiced}
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
				<!-- Details toggle -->
				{#if entry.calculation_details}
					<button
						type="button"
						class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
						title="Show calculation details"
						onclick={(e) => { e.stopPropagation(); showDetails = !showDetails; }}
					>
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
						</svg>
					</button>
				{/if}
			</div>
		</div>

		<!-- Calculation details panel -->
		{#if showDetails && entry.calculation_details}
			<div class="mt-3 pt-3 border-t border-gray-200 dark:border-gray-600 text-xs text-gray-500 dark:text-gray-400">
				<div class="font-medium mb-1">Calculation breakdown:</div>
				<ul class="space-y-1">
					{#each entry.calculation_details.events as evt}
						<li class="flex justify-between">
							<span class="truncate">{evt.title}</span>
							<span class="ml-2 whitespace-nowrap">
								{formatMinutes(evt.raw_minutes)}
								{#if evt.is_all_day}(all-day){/if}
							</span>
						</li>
					{/each}
				</ul>
				{#if entry.calculation_details.time_ranges.length > 1}
					<div class="mt-2">
						<span class="font-medium">Time ranges:</span>
						{#each entry.calculation_details.time_ranges as range}
							<span class="ml-1">{range.start}-{range.end}</span>
						{/each}
					</div>
				{/if}
				<div class="mt-2 flex justify-between">
					<span>Union: {formatMinutes(entry.calculation_details.union_minutes)}</span>
					<span>Rounding: {entry.calculation_details.rounding_applied}</span>
					<span class="font-medium">Final: {formatMinutes(entry.calculation_details.final_minutes)}</span>
				</div>
			</div>
		{/if}
	{/if}
</div>
