<script lang="ts">
	import type { TimeEntry } from '$lib/api/types';
	import { getContrastColor } from '$lib/utils/colors';

	interface Props {
		entriesByDate: Record<string, TimeEntry[]>;
		date: Date;
		maxHours?: number;
		containerHeight?: number;
		onentryclick: (entryId: string, event: MouseEvent) => void;
		onaddclick: () => void;
	}

	let {
		entriesByDate,
		date,
		maxHours = 8,
		containerHeight = 300,
		onentryclick,
		onaddclick
	}: Props = $props();

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
		{@const bgColor = entry.project?.color || '#6B7280'}
		{@const textColor = getContrastColor(bgColor)}
		{@const isInvoiced = !!entry.invoice_id}
		{@const isPinned = !!entry.is_pinned}
		{@const isLocked = !!entry.is_locked}
		<button
			type="button"
			class="relative flex items-center justify-center rounded-lg font-semibold text-lg transition-all hover:ring-2 hover:ring-offset-2 hover:ring-black/30 dark:hover:ring-white/50 dark:ring-offset-zinc-900 cursor-pointer"
			class:opacity-75={isInvoiced}
			style="background-color: {bgColor}; color: {textColor}; height: {getBarHeight(entry.hours)}px;"
			onclick={(e) => onentryclick(entry.id, e)}
		>
			<!-- Status indicators in top-right corner -->
			{#if isInvoiced || isPinned || isLocked}
				<div class="absolute top-1 right-1 flex gap-0.5">
					{#if isInvoiced}
						<span class="text-xs opacity-80" title="Invoiced">
							<svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20">
								<path fill-rule="evenodd" d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4zm2 6a1 1 0 011-1h6a1 1 0 110 2H7a1 1 0 01-1-1zm1 3a1 1 0 100 2h6a1 1 0 100-2H7z" clip-rule="evenodd" />
							</svg>
						</span>
					{/if}
					{#if isLocked && !isInvoiced}
						<span class="text-xs opacity-80" title="Locked">
							<svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20">
								<path fill-rule="evenodd" d="M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z" clip-rule="evenodd" />
							</svg>
						</span>
					{/if}
					{#if isPinned && !isInvoiced}
						<span class="text-xs opacity-80" title="Edited">
							<svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20">
								<path d="M10 2a1 1 0 011 1v1.323l3.954 1.582 1.599-.8a1 1 0 01.894 1.79l-1.233.617 1.738 5.42a1 1 0 01-.285 1.05A3.989 3.989 0 0115 15a3.989 3.989 0 01-2.667-1.018 1 1 0 01-.285-1.05l1.715-5.349L10 6.418l-3.763 1.165 1.715 5.349a1 1 0 01-.285 1.05A3.989 3.989 0 015 15a3.989 3.989 0 01-2.667-1.018 1 1 0 01-.285-1.05l1.738-5.42-1.233-.617a1 1 0 01.894-1.79l1.599.8L9 4.323V3a1 1 0 011-1z" />
							</svg>
						</span>
					{/if}
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
