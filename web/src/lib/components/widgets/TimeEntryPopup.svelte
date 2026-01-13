<script lang="ts">
	import type { TimeEntry } from '$lib/api/types';
	import ProjectChip from './ProjectChip.svelte';

	interface Props {
		entry: TimeEntry;
		anchor: { x: number; y: number };
		onupdate?: (data: { hours?: number; description?: string; project_id?: string; date?: string }) => void;
		ondelete?: () => void;
		onrefresh?: () => void;
		onclose: () => void;
	}

	let { entry, anchor, onupdate, ondelete, onrefresh, onclose }: Props = $props();

	// Edit state - reset when entry changes
	let editHours = $state(0);
	let editDescription = $state('');

	// Auto-save state
	let saveStatus = $state<'idle' | 'saving' | 'saved'>('idle');
	let saveTimeout: ReturnType<typeof setTimeout>;
	let savedTimeout: ReturnType<typeof setTimeout>;

	// Reset edit state when entry changes
	$effect(() => {
		// Track entry.id to reset when switching entries
		const _id = entry.id;
		editHours = entry.hours;
		editDescription = entry.description || '';
		saveStatus = 'idle';
	});

	// Auto-save with debounce
	function triggerAutoSave() {
		if (!canEdit) return;

		// Clear any pending save
		clearTimeout(saveTimeout);
		clearTimeout(savedTimeout);

		// Debounce the save
		saveTimeout = setTimeout(() => {
			// Only save if values actually changed
			if (editHours !== entry.hours || editDescription !== (entry.description || '')) {
				saveStatus = 'saving';
				onupdate?.({
					hours: editHours,
					description: editDescription,
					project_id: entry.project_id,
					date: entry.date
				});
				// Show "Saved" briefly after save
				savedTimeout = setTimeout(() => {
					saveStatus = 'saved';
					setTimeout(() => {
						saveStatus = 'idle';
					}, 1500);
				}, 300);
			}
		}, 500);
	}

	// Protection states - invoice_id is the sole locking mechanism
	const isInvoiced = $derived(!!entry.invoice_id);
	const isStale = $derived(!!entry.is_stale);
	const canEdit = $derived(!isInvoiced);

	// Calculate popup position
	const popupPosition = $derived.by(() => {
		const popupWidth = 360;
		const popupHeight = 400;
		const gap = 12;
		const viewportWidth = typeof window !== 'undefined' ? window.innerWidth : 1200;
		const viewportHeight = typeof window !== 'undefined' ? window.innerHeight : 800;

		let left: number;
		let top: number;

		// Horizontal: prefer right of anchor, fallback to left
		if (anchor.x + gap + popupWidth <= viewportWidth) {
			left = anchor.x + gap;
		} else if (anchor.x - gap - popupWidth >= 0) {
			left = anchor.x - gap - popupWidth;
		} else {
			left = Math.max(8, (viewportWidth - popupWidth) / 2);
		}

		// Vertical: center on anchor, clamp to viewport
		top = anchor.y - popupHeight / 2;
		if (top < 8) top = 8;
		if (top + popupHeight > viewportHeight - 8) {
			top = viewportHeight - popupHeight - 8;
		}

		return { top, left };
	});

	function handleDelete() {
		if (confirm('Delete this time entry?')) {
			ondelete?.();
		}
	}

	function formatDate(dateStr: string): string {
		const date = new Date(dateStr + 'T00:00:00');
		return date.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
	}

	function formatMinutes(minutes: number): string {
		const hours = Math.floor(minutes / 60);
		const mins = minutes % 60;
		if (hours === 0) return `${mins}m`;
		if (mins === 0) return `${hours}h`;
		return `${hours}h ${mins}m`;
	}

	// Close on escape key
	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			onclose();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- Backdrop -->
<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="fixed inset-0 z-40" onclick={onclose}></div>

<!-- Popup -->
<div
	class="fixed z-50 bg-white dark:bg-zinc-800 rounded-lg shadow-2xl border border-gray-200 dark:border-white/15 w-[360px] max-h-[400px] overflow-hidden"
	style="top: {popupPosition.top}px; left: {popupPosition.left}px;"
>
	<!-- Header -->
	<div class="px-4 py-3 border-b border-gray-200 dark:border-zinc-700">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2">
				{#if entry.project}
					<ProjectChip project={entry.project} size="md" />
				{/if}
			</div>
			<button
				type="button"
				class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
				onclick={onclose}
			>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
				</svg>
			</button>
		</div>
		<div class="mt-1 text-sm text-gray-600 dark:text-gray-300">
			{formatDate(entry.date)}
		</div>

		<!-- Status badges -->
		{#if isInvoiced}
			<div class="flex gap-2 mt-2">
				<a
					href="/invoices/{entry.invoice_id}"
					class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400 hover:bg-green-200 dark:hover:bg-green-900/50"
				>
					<svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
						<path fill-rule="evenodd" d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4z" clip-rule="evenodd" />
					</svg>
					Invoiced
				</a>
			</div>
		{/if}
	</div>

	<!-- Content -->
	<div class="px-4 py-3 space-y-4 max-h-[260px] overflow-y-auto">
		<!-- Hours field - editable when not invoiced -->
		<div>
			<label class="block text-xs text-gray-500 dark:text-zinc-400 uppercase tracking-wide mb-1">
				Hours
			</label>
			{#if canEdit}
				<input
					type="number"
					step="0.25"
					min="0"
					bind:value={editHours}
					oninput={triggerAutoSave}
					class="w-full rounded border px-3 py-2 text-sm border-gray-300 dark:border-zinc-600 bg-white dark:bg-zinc-700 text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
				/>
			{:else}
				<span class="text-2xl font-semibold text-gray-900 dark:text-white">{entry.hours}h</span>
			{/if}
		</div>

		<!-- Description field - editable when not invoiced -->
		<div>
			<label class="block text-xs text-gray-500 dark:text-zinc-400 uppercase tracking-wide mb-1">
				{entry.title ? 'Title' : 'Description'}
			</label>
			{#if canEdit}
				<textarea
					bind:value={editDescription}
					oninput={triggerAutoSave}
					rows="2"
					placeholder="Optional description..."
					class="w-full rounded border px-3 py-2 text-sm border-gray-300 dark:border-zinc-600 bg-white dark:bg-zinc-700 text-gray-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 resize-none"
				></textarea>
			{:else if entry.title || entry.description}
				<p class="text-sm text-gray-700 dark:text-zinc-300">
					{entry.title || entry.description}
				</p>
			{:else}
				<p class="text-sm text-gray-400 dark:text-zinc-500 italic">No description</p>
			{/if}
		</div>

		<!-- Staleness warning -->
		{#if isStale && entry.computed_hours !== undefined && entry.snapshot_computed_hours !== undefined}
			<div class="rounded bg-orange-100 dark:bg-orange-900/30 px-3 py-2 text-sm">
				<div class="flex items-center gap-2 text-orange-700 dark:text-orange-400">
					<svg class="h-4 w-4 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
						<path fill-rule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clip-rule="evenodd" />
					</svg>
					<span class="font-medium">Events have changed</span>
				</div>
				<p class="mt-1 text-xs text-orange-600 dark:text-orange-300">
					You set {entry.hours}h when computed was {entry.snapshot_computed_hours}h.
					Now computed is {entry.computed_hours}h.
				</p>
				<div class="mt-2 flex gap-2">
					<button
						type="button"
						class="text-xs px-2 py-1 rounded bg-orange-200 dark:bg-orange-800 text-orange-700 dark:text-orange-200 hover:bg-orange-300 dark:hover:bg-orange-700"
						onclick={() => onupdate?.({ hours: entry.computed_hours, project_id: entry.project_id, date: entry.date })}
					>
						Accept {entry.computed_hours}h
					</button>
					<button
						type="button"
						class="text-xs px-2 py-1 rounded text-orange-600 dark:text-orange-300 hover:bg-orange-200 dark:hover:bg-orange-800"
						onclick={() => onupdate?.({ hours: entry.hours, project_id: entry.project_id, date: entry.date })}
					>
						Keep {entry.hours}h
					</button>
				</div>
			</div>
		{/if}

		<!-- Calculation details -->
		{#if entry.calculation_details && entry.calculation_details.events.length > 0}
			<div>
				<span class="block text-xs text-gray-500 dark:text-zinc-400 uppercase tracking-wide mb-1">
					Source Events ({entry.calculation_details.events.length})
				</span>
				<ul class="space-y-1 text-sm text-gray-700 dark:text-zinc-300">
					{#each entry.calculation_details.events.slice(0, 5) as evt}
						<li class="flex justify-between">
							<span class="truncate mr-2">{evt.title}</span>
							<span class="text-gray-500 dark:text-zinc-400 whitespace-nowrap">
								{formatMinutes(evt.raw_minutes)}
							</span>
						</li>
					{/each}
					{#if entry.calculation_details.events.length > 5}
						<li class="text-xs text-gray-500 dark:text-zinc-400">
							+{entry.calculation_details.events.length - 5} more
						</li>
					{/if}
				</ul>
			</div>
		{/if}
	</div>

	<!-- Footer actions -->
	<div class="px-4 py-3 border-t border-gray-200 dark:border-zinc-700 bg-gray-50 dark:bg-zinc-800/50">
		<div class="flex justify-between items-center">
			<div class="flex items-center gap-2">
				{#if isStale && !isInvoiced && entry.computed_hours}
					<button
						type="button"
						class="px-3 py-1.5 text-sm text-gray-600 dark:text-zinc-400 hover:text-primary-600"
						title="Reset to computed values"
						onclick={() => onrefresh?.()}
					>
						Reset
					</button>
				{/if}
				<!-- Auto-save status indicator -->
				{#if saveStatus === 'saving'}
					<span class="text-xs text-gray-400 dark:text-zinc-500">Saving...</span>
				{:else if saveStatus === 'saved'}
					<span class="text-xs text-green-600 dark:text-green-400">Saved</span>
				{/if}
			</div>
			{#if canEdit}
				<button
					type="button"
					class="px-3 py-1.5 text-sm text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300"
					onclick={handleDelete}
				>
					Delete
				</button>
			{/if}
		</div>
	</div>
</div>
