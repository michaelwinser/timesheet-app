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
		onnavigateprevious: () => void;
		onnavigatenext: () => void;
		ongototoday: () => void;
		onscopechange: (mode: ScopeMode) => void;
		ondisplaychange: (mode: DisplayMode) => void;
	}

	let {
		currentDate,
		scopeMode,
		displayMode,
		weekStart,
		weekdaysEnd,
		weekEnd,
		weekendEventCount = 0,
		onnavigateprevious,
		onnavigatenext,
		ongototoday,
		onscopechange,
		ondisplaychange
	}: Props = $props();

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
	</div>

	<!-- Right: View toggle -->
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
