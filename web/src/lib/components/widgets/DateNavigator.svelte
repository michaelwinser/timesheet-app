<script lang="ts">
	import { Button } from '$lib/components/primitives';

	type ScopeMode = 'day' | 'week' | 'full-week';
	type DisplayMode = 'calendar' | 'list';

	interface Props {
		currentDate: Date;
		scopeMode: ScopeMode;
		displayMode: DisplayMode;
		weekStart: Date;
		weekdaysEnd: Date;
		weekEnd: Date;
		weekendEventCount?: number;
		lastSyncedAt?: Date | null;
		syncing?: boolean;
		hasCalendarConnections?: boolean;
		reclassifying?: boolean;
		onnavigateprevious: () => void;
		onnavigatenext: () => void;
		ongototoday: () => void;
		onscopechange: (mode: ScopeMode) => void;
		ondisplaychange: (mode: DisplayMode) => void;
		onsync?: () => void;
		onreclassify?: () => void;
	}

	let {
		currentDate,
		scopeMode,
		displayMode,
		weekStart,
		weekdaysEnd,
		weekEnd,
		weekendEventCount = 0,
		lastSyncedAt = null,
		syncing = false,
		hasCalendarConnections = false,
		reclassifying = false,
		onnavigateprevious,
		onnavigatenext,
		ongototoday,
		onscopechange,
		ondisplaychange,
		onsync,
		onreclassify
	}: Props = $props();

	// Format relative time (e.g., "2 hours ago", "just now")
	function formatRelativeTime(date: Date): string {
		const now = new Date();
		const diffMs = now.getTime() - date.getTime();
		const diffMins = Math.floor(diffMs / (1000 * 60));
		const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
		const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

		if (diffMins < 1) return 'just now';
		if (diffMins < 60) return `${diffMins}m ago`;
		if (diffHours < 24) return `${diffHours}h ago`;
		if (diffDays === 1) return 'yesterday';
		return `${diffDays}d ago`;
	}

	// Determine sync status text and style
	const syncStatus = $derived.by(() => {
		if (!hasCalendarConnections) {
			return { text: 'No calendars', style: 'text-gray-400 dark:text-gray-500', isStale: false };
		}
		if (!lastSyncedAt) {
			return { text: 'Never synced', style: 'text-amber-600 dark:text-amber-400', isStale: true };
		}
		const hoursSinceSync = (Date.now() - lastSyncedAt.getTime()) / (1000 * 60 * 60);
		const isStale = hoursSinceSync > 24;
		return {
			text: formatRelativeTime(lastSyncedAt),
			style: isStale
				? 'text-amber-600 dark:text-amber-400'
				: 'text-gray-500 dark:text-gray-400',
			isStale
		};
	});

	function formatFullDayLabel(date: Date): string {
		return date.toLocaleDateString('en-US', {
			weekday: 'long',
			month: 'long',
			day: 'numeric',
			year: 'numeric'
		});
	}

	function formatDateRange(start: Date, end: Date): string {
		const startStr = start.toLocaleDateString('en-US', { month: 'long', day: 'numeric' });
		const endStr = end.toLocaleDateString('en-US', {
			month: 'long',
			day: 'numeric',
			year: 'numeric'
		});
		return `${startStr} - ${endStr}`;
	}

	const dateLabel = $derived.by(() => {
		if (scopeMode === 'day') {
			return formatFullDayLabel(currentDate);
		} else if (scopeMode === 'week') {
			return formatDateRange(weekStart, weekdaysEnd);
		} else {
			return formatDateRange(weekStart, weekEnd);
		}
	});
</script>

<div class="date-navigator">
	<!-- Left: Scope mode toggle -->
	<div class="scope-toggle">
		<button
			type="button"
			class="scope-button"
			class:scope-button--active={scopeMode === 'day'}
			onclick={() => onscopechange('day')}
			title="Day (D)"
		>
			Day
		</button>
		<button
			type="button"
			class="scope-button"
			class:scope-button--active={scopeMode === 'week'}
			onclick={() => onscopechange('week')}
			title="Week Mon-Fri (W)"
		>
			Week
		</button>
		<button
			type="button"
			class="scope-button"
			class:scope-button--active={scopeMode === 'full-week'}
			onclick={() => onscopechange('full-week')}
			title="Full Week Mon-Sun (F)"
		>
			Full
		</button>
	</div>

	<!-- Center: Date navigation -->
	<div class="flex items-center gap-2">
		<Button variant="ghost" size="sm" onclick={onnavigateprevious} title="Previous (K)">
			<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
			</svg>
		</Button>
		<Button variant="secondary" size="sm" onclick={ongototoday} title="Today (T)">Today</Button>
		<Button variant="ghost" size="sm" onclick={onnavigatenext} title="Next (J)">
			<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
			</svg>
		</Button>
		<h1 class="ml-2 text-base font-semibold text-text-primary">
			{dateLabel}
		</h1>
		<!-- Weekend warning -->
		{#if weekendEventCount > 0}
			<button
				type="button"
				class="ml-2 flex items-center gap-1 rounded-md bg-amber-100 px-2 py-1 text-xs text-amber-700 transition-colors hover:bg-amber-200"
				onclick={() => onscopechange('full-week')}
				title="Click to show full week"
			>
				<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
					/>
				</svg>
				{weekendEventCount} weekend
			</button>
		{/if}

		<!-- Reclassify Week button (only in week modes) -->
		{#if scopeMode !== 'day' && onreclassify}
			<button
				type="button"
				class="ml-2 flex items-center gap-1 rounded-md border border-gray-300 bg-white px-2 py-1 text-xs text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:border-zinc-600 dark:bg-zinc-800 dark:text-gray-400 dark:hover:bg-zinc-700 dark:hover:text-white"
				onclick={onreclassify}
				disabled={reclassifying}
				title="Re-run classification rules on this week"
			>
				{#if reclassifying}
					<svg class="h-3.5 w-3.5 animate-spin" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
					</svg>
				{:else}
					<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
					</svg>
				{/if}
				Reclassify Week
			</button>
		{/if}
	</div>

	<!-- Right: Sync status + View toggle -->
	<div class="flex items-center gap-3">
		<!-- Sync status -->
		{#if hasCalendarConnections}
			<div class="flex items-center gap-1.5">
				{#if syncing}
					<svg class="h-4 w-4 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
						<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
						<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
					</svg>
					<span class="text-xs text-primary-600 dark:text-primary-400">Syncing...</span>
				{:else}
					<button
						type="button"
						class="flex items-center gap-1.5 rounded px-1.5 py-0.5 text-xs transition-colors hover:bg-gray-100 dark:hover:bg-zinc-700 {syncStatus.style}"
						onclick={onsync}
						title="Sync calendars (R)"
					>
						<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
						</svg>
						<span>{syncStatus.text}</span>
						{#if syncStatus.isStale}
							<svg class="h-3 w-3 text-amber-500" fill="currentColor" viewBox="0 0 20 20">
								<path fill-rule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clip-rule="evenodd" />
							</svg>
						{/if}
					</button>
				{/if}
			</div>
		{/if}

		<!-- View toggle -->
		<div class="display-toggle">
			<button
				type="button"
				class="display-button"
				class:display-button--active={displayMode === 'calendar'}
				onclick={() => ondisplaychange('calendar')}
				title="Calendar view (C)"
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
					/>
				</svg>
			</button>
			<button
				type="button"
				class="display-button"
				class:display-button--active={displayMode === 'list'}
				onclick={() => ondisplaychange('list')}
				title="List view (L or A)"
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M4 6h16M4 10h16M4 14h16M4 18h16"
					/>
				</svg>
			</button>
		</div>
	</div>
</div>

<style>
	.date-navigator {
		@apply mb-4 flex items-center justify-between;
	}

	.scope-toggle {
		@apply flex rounded-lg bg-gray-100 p-0.5 dark:bg-zinc-700;
	}

	.scope-button {
		@apply rounded-md px-3 py-1 text-sm text-gray-600 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-white;
	}

	.scope-button--active {
		@apply bg-white text-gray-900 shadow-sm dark:bg-zinc-600 dark:text-white;
	}

	.display-toggle {
		@apply flex rounded-lg bg-gray-100 p-0.5 dark:bg-zinc-700;
	}

	.display-button {
		@apply rounded-md p-1.5 text-gray-600 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-white;
	}

	.display-button--active {
		@apply bg-white text-gray-900 shadow-sm dark:bg-zinc-600 dark:text-white;
	}
</style>
