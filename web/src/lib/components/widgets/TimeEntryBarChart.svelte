<script lang="ts">
	import type { TimeEntry } from '$lib/api/types';
	import { getContrastColor, getVerificationTextColor } from '$lib/utils/colors';

	interface Props {
		entriesByDate: Record<string, TimeEntry[]>;
		date: Date;
		maxHours?: number;
		containerHeight?: number;
		onentryclick: (entryId: string, event: MouseEvent) => void;
		onaddclick: () => void;
		highlightedTarget?: string | null;
	}

	let {
		entriesByDate,
		date,
		maxHours = 8,
		containerHeight = 300,
		onentryclick,
		onaddclick,
		highlightedTarget = null
	}: Props = $props();

	// Determine if an entry should be dimmed based on highlight target
	function shouldDimEntry(entry: TimeEntry): boolean {
		if (!highlightedTarget) return false;

		if (highlightedTarget === 'hidden') {
			return !entry.project?.is_hidden_by_default;
		}
		if (highlightedTarget === 'archived') {
			return !entry.project?.is_archived;
		}
		// 'skipped' doesn't apply to entries, so dim all
		if (highlightedTarget === 'skipped') {
			return true;
		}
		// Regular project ID
		return entry.project_id !== highlightedTarget;
	}

	const MIN_BAR_HEIGHT = 32;

	// Format date to YYYY-MM-DD string for lookup
	function formatDate(d: Date): string {
		const year = d.getFullYear();
		const month = String(d.getMonth() + 1).padStart(2, '0');
		const day = String(d.getDate()).padStart(2, '0');
		return `${year}-${month}-${day}`;
	}

	// REACTIVE: These derivations track changes to entriesByDate and date
	const dateStr = $derived(formatDate(date));
	const dayEntries = $derived(entriesByDate[dateStr] || []);
	const sortedEntries = $derived(
		[...dayEntries].sort((a, b) => b.hours - a.hours)
	);
	const totalHours = $derived(dayEntries.reduce((sum, e) => sum + e.hours, 0));


	function getBarHeight(hours: number): number {
		const proportionalHeight = (hours / maxHours) * containerHeight;
		return Math.max(MIN_BAR_HEIGHT, proportionalHeight);
	}

	function getDisplayCode(entry: TimeEntry): string {
		if (!entry.project) return '?';
		return entry.project.short_code || entry.project.name.substring(0, 3).toUpperCase();
	}

	function formatHours(hours: number): string {
		if (hours === Math.floor(hours)) {
			return `${hours}h`;
		}
		return `${hours}h`;
	}
</script>

<div class="flex flex-col gap-1" style="min-height: {containerHeight}px;">
	<!-- Add button at top -->
	<button
		type="button"
		class="flex items-center justify-center gap-1 rounded border-2 border-dashed border-gray-300 dark:border-zinc-600 text-gray-400 dark:text-zinc-500 hover:border-primary-500 hover:text-primary-500 dark:hover:border-primary-400 dark:hover:text-primary-400 transition-colors py-1.5 text-sm"
		onclick={onaddclick}
	>
		<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
		</svg>
		Add
	</button>

	<!-- Stacked bars -->
	{#each sortedEntries as entry}
		{@const projectColor = entry.project?.color || '#6B7280'}
		{@const isZeroHours = entry.hours === 0}
		{@const bgColor = isZeroHours ? 'transparent' : projectColor}
		{@const textColor = isZeroHours ? getVerificationTextColor(projectColor) : getContrastColor(projectColor)}
		{@const borderStyle = isZeroHours ? `border: 2px solid ${projectColor};` : ''}
		{@const isInvoiced = !!entry.invoice_id}
		{@const isDimmed = shouldDimEntry(entry)}
		<button
			type="button"
			class="relative flex items-center justify-center rounded-lg font-semibold text-lg transition-all hover:ring-2 hover:ring-offset-2 hover:ring-black/30 dark:hover:ring-white/50 dark:ring-offset-zinc-900 cursor-pointer {isDimmed ? 'opacity-25' : ''} {isZeroHours ? 'bg-white dark:bg-zinc-900' : ''}"
			class:opacity-75={isInvoiced && !isDimmed}
			style="background-color: {bgColor}; color: {textColor}; height: {getBarHeight(entry.hours)}px; {borderStyle}"
			onclick={(e) => onentryclick(entry.id, e)}
		>
			<!-- Status indicators in top-right corner -->
			{#if isInvoiced}
				<div class="absolute top-1 right-1 flex gap-0.5">
					<span class="text-xs opacity-80" title="Invoiced">
						<svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20">
							<path fill-rule="evenodd" d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4zm2 6a1 1 0 011-1h6a1 1 0 110 2H7a1 1 0 01-1-1zm1 3a1 1 0 100 2h6a1 1 0 100-2H7z" clip-rule="evenodd" />
						</svg>
					</span>
				</div>
			{/if}

			<!-- Main content: project code + hours -->
			<span>{getDisplayCode(entry)} {formatHours(entry.hours)}</span>
		</button>
	{/each}

	<!-- Empty state -->
	{#if dayEntries.length === 0}
		<div class="flex-1 flex items-center justify-center text-gray-400 dark:text-zinc-500 text-sm">
			No entries
		</div>
	{/if}
</div>
