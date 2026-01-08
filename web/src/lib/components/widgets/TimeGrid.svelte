<script lang="ts">
	import type { CalendarEvent, Project } from '$lib/api/types';
	import ProjectChip from './ProjectChip.svelte';
	import {
		getClassificationStyles,
		getPrimaryTextClasses,
		getPrimaryTextStyle,
		formatConfidenceTitle
	} from '$lib/styles';

	interface Props {
		events: CalendarEvent[];
		projects: Project[];
		date: Date;
		scrollTrigger?: number;
		onclassify?: (eventId: string, projectId: string) => void;
		onskip?: (eventId: string) => void;
		onunskip?: (eventId: string) => void;
		onhover?: (event: CalendarEvent | null, element: HTMLElement | null) => void;
		showTimeLegend?: boolean;
	}

	let {
		events,
		projects,
		date,
		scrollTrigger = 0,
		onclassify,
		onskip,
		onunskip,
		onhover,
		showTimeLegend = true
	}: Props = $props();

	const activeProjects = $derived(projects.filter((p) => !p.is_archived));

	// Track which events are showing reclassify UI (only used if no onhover)
	let reclassifyingId = $state<string | null>(null);

	// Scroll container reference
	let scrollContainer: HTMLDivElement;

	// Time grid configuration
	const startHour = 0; // Midnight
	const endHour = 24; // Midnight (full 24h)
	const hourHeight = 60; // pixels per hour
	const viewportHours = 15; // 15 hours visible in viewport (50% taller than original 10)

	// Calculate first event hour for auto-scroll (based on timed events only)
	function getFirstEventHour(): number {
		if (timedEvents.length === 0) return 8; // Default to 8 AM if no events
		let minHour = 24;
		for (const event of timedEvents) {
			const hour = new Date(event.start_time).getHours();
			if (hour < minHour) minHour = hour;
		}
		// Start 1 hour before first event, minimum 0
		return Math.max(0, minHour - 1);
	}

	// Scroll to first event
	function scrollToFirstEvent() {
		if (!scrollContainer) return;
		const firstHour = getFirstEventHour();
		const scrollTop = firstHour * hourHeight;
		scrollContainer.scrollTop = scrollTop;
	}

	// Scroll to first event only when scrollTrigger changes (not on event classification)
	$effect(() => {
		// Only track scrollTrigger - not events, to avoid scroll on classification
		const _trigger = scrollTrigger;

		if (scrollContainer) {
			// Use requestAnimationFrame to ensure DOM is ready
			requestAnimationFrame(() => scrollToFirstEvent());
		}
	});

	// Detect all-day events (>= 23 hours or spans midnight to midnight)
	function isAllDayEvent(event: CalendarEvent): boolean {
		const start = new Date(event.start_time);
		const end = new Date(event.end_time);
		const durationHours = (end.getTime() - start.getTime()) / (1000 * 60 * 60);

		// If >= 23 hours, treat as all-day
		if (durationHours >= 23) return true;

		// Check if it spans midnight to midnight (00:00 to 00:00 or 23:59)
		if (start.getHours() === 0 && start.getMinutes() === 0) {
			if (
				(end.getHours() === 0 && end.getMinutes() === 0) ||
				(end.getHours() === 23 && end.getMinutes() >= 59)
			) {
				return true;
			}
		}

		return false;
	}

	// Separate all-day and timed events
	const allDayEvents = $derived(events.filter((e) => isAllDayEvent(e)));
	const timedEvents = $derived(events.filter((e) => !isAllDayEvent(e)));

	// Calculate position and height for an event
	function getEventStyle(event: CalendarEvent): {
		top: string;
		height: string;
		left: string;
		width: string;
	} {
		const start = new Date(event.start_time);
		const end = new Date(event.end_time);

		const startMinutes = start.getHours() * 60 + start.getMinutes();
		const endMinutes = end.getHours() * 60 + end.getMinutes();

		const gridStartMinutes = startHour * 60;
		const gridEndMinutes = endHour * 60;

		// Clamp to grid bounds
		const clampedStart = Math.max(startMinutes, gridStartMinutes);
		const clampedEnd = Math.min(endMinutes, gridEndMinutes);

		const top = ((clampedStart - gridStartMinutes) / 60) * hourHeight;
		const height = Math.max(((clampedEnd - clampedStart) / 60) * hourHeight, 20); // Min height 20px

		// For now, full width - overlapping logic would go here
		return {
			top: `${top}px`,
			height: `${height}px`,
			left: '0',
			width: '100%'
		};
	}

	// Calculate overlapping events and assign columns
	function getEventsWithColumns(
		events: CalendarEvent[]
	): Array<{ event: CalendarEvent; column: number; totalColumns: number }> {
		if (events.length === 0) return [];

		// Sort by start time
		const sorted = [...events].sort(
			(a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime()
		);

		const result: Array<{
			event: CalendarEvent;
			column: number;
			totalColumns: number;
			endTime: number;
		}> = [];
		const columns: number[] = []; // Track end times for each column

		for (const event of sorted) {
			const startTime = new Date(event.start_time).getTime();
			const endTime = new Date(event.end_time).getTime();

			// Find first available column
			let column = 0;
			while (column < columns.length && columns[column] > startTime) {
				column++;
			}

			// Update or add column end time
			columns[column] = endTime;

			result.push({ event, column, totalColumns: 1, endTime });
		}

		// Calculate total columns for overlapping groups
		for (let i = 0; i < result.length; i++) {
			const current = result[i];
			const currentStart = new Date(current.event.start_time).getTime();
			const currentEnd = current.endTime;

			// Find all overlapping events
			let maxColumn = current.column;
			for (let j = 0; j < result.length; j++) {
				const other = result[j];
				const otherStart = new Date(other.event.start_time).getTime();
				const otherEnd = other.endTime;

				// Check if they overlap
				if (currentStart < otherEnd && currentEnd > otherStart) {
					maxColumn = Math.max(maxColumn, other.column);
				}
			}

			current.totalColumns = maxColumn + 1;
		}

		return result;
	}

	function getEventPositionStyle(
		column: number,
		totalColumns: number
	): { left: string; width: string } {
		const width = 100 / totalColumns;
		const left = column * width;
		return {
			left: `${left}%`,
			width: `${width}%`
		};
	}

	// Helper to get styles for an event
	function getEventStyles(event: CalendarEvent) {
		const isSkipped = event.is_skipped === true;
		const needsReview = event.needs_review === true;
		const projectColor = isSkipped ? null : (event.project?.color ?? null);

		return getClassificationStyles({
			status: event.classification_status as 'pending' | 'classified' | 'skipped',
			needsReview,
			isSkipped,
			projectColor
		});
	}

	const hours = $derived(Array.from({ length: endHour - startHour }, (_, i) => startHour + i));
	const gridHeight = $derived((endHour - startHour) * hourHeight);
	const eventsWithColumns = $derived(getEventsWithColumns(timedEvents));
</script>

<!-- All-day events section -->
{#if allDayEvents.length > 0}
	<div class="mb-2 border-b border-gray-200 pb-2 dark:border-gray-700">
		<div class="flex flex-wrap items-center gap-1">
			<span class="w-12 flex-shrink-0 pr-2 text-right text-xs text-gray-400 dark:text-gray-500"
				>All day</span
			>
			{#each allDayEvents as event (event.id)}
				{@const isPending = event.classification_status === 'pending'}
				{@const isClassified = event.classification_status === 'classified'}
				{@const isSkipped = event.is_skipped === true}
				{@const needsReview = event.needs_review === true}
				{@const styles = getEventStyles(event)}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="inline-flex cursor-pointer items-center gap-1 rounded-full px-2 py-0.5 text-xs transition-shadow hover:shadow-sm {styles.containerClasses}"
					style={styles.containerStyle}
					onmouseenter={(e) => onhover?.(event, e.currentTarget as HTMLElement)}
					onmouseleave={() => onhover?.(null, null)}
				>
					<span
						class="max-w-[120px] truncate {getPrimaryTextClasses(styles, isSkipped)}"
						style={getPrimaryTextStyle(styles, isSkipped)}
						title={event.title}>{event.title}</span
					>
					{#if isClassified && !needsReview && event.project}
						<span
							class="h-2.5 w-2.5 flex-shrink-0 rounded-full {styles.textColors?.isDark
								? 'border border-white/50'
								: ''}"
							style="background-color: {event.project.color}"
							title={formatConfidenceTitle(
								event.project.name,
								event.classification_confidence,
								event.classification_source
							)}
						></span>
					{:else if isSkipped}
						<span
							class="flex h-2.5 w-2.5 items-center justify-center rounded border border-dashed border-gray-400 text-[5px] text-gray-400 dark:border-gray-500"
							>✕</span
						>
					{:else if isPending}
						<!-- Quick classify buttons for pending all-day events with best-guess highlight -->
						<div class="ml-1 flex items-center gap-0.5">
							{#each activeProjects.slice(0, 3) as project, i}
								{@const isBestGuess =
									event.suggested_project_id === project.id ||
									(!event.suggested_project_id && i === 0)}
								<button
									type="button"
									class="h-2.5 w-2.5 rounded-full transition-shadow {isBestGuess
										? 'ring-1 ring-black/40 ring-offset-1 ring-offset-white dark:ring-white/60 dark:ring-offset-zinc-900'
										: 'ring-gray-400 hover:ring-1 hover:ring-offset-1'}"
									style="background-color: {project.color}"
									title="{project.name}{isBestGuess ? ' (suggested)' : ''}"
									onclick={(e) => {
										e.stopPropagation();
										onclassify?.(event.id, project.id);
									}}
								></button>
							{/each}
						</div>
						<button
							type="button"
							class="ml-1 flex h-2.5 w-2.5 items-center justify-center rounded border border-dashed border-gray-400 text-[5px] text-gray-400 hover:border-gray-600 dark:border-gray-500 dark:hover:border-gray-300"
							title="Did not attend"
							onclick={(e) => {
								e.stopPropagation();
								onskip?.(event.id);
							}}
						>
							✕
						</button>
					{/if}
				</div>
			{/each}
		</div>
	</div>
{/if}

<!-- Scrollable container with 10h viewport -->
<div
	class="overflow-x-hidden overflow-y-auto"
	style="height: {viewportHours * hourHeight}px"
	bind:this={scrollContainer}
>
	<div class="flex">
		<!-- Time labels column (optional) -->
		{#if showTimeLegend}
			<div class="w-12 flex-shrink-0 pr-2 text-right">
				{#each hours as hour}
					<div class="text-xs text-gray-400 dark:text-gray-500" style="height: {hourHeight}px">
						{hour === 0
							? '12 AM'
							: hour === 12
								? '12 PM'
								: hour > 12
									? `${hour - 12} PM`
									: `${hour} AM`}
					</div>
				{/each}
			</div>
		{/if}

		<!-- Events grid -->
		<div
			class="relative flex-1 {showTimeLegend ? 'border-l border-gray-200 dark:border-gray-700' : ''}"
			style="height: {gridHeight}px"
		>
			<!-- Hour lines -->
			{#each hours as hour, i}
				<div
					class="absolute w-full border-t border-gray-100 dark:border-gray-800"
					style="top: {i * hourHeight}px"
				></div>
			{/each}

			<!-- Events -->
			{#each eventsWithColumns as { event, column, totalColumns } (event.id)}
				{@const style = getEventStyle(event)}
				{@const posStyle = getEventPositionStyle(column, totalColumns)}
				{@const isPending = event.classification_status === 'pending'}
				{@const isClassified = event.classification_status === 'classified'}
				{@const isSkipped = event.is_skipped === true}
				{@const needsReview = event.needs_review === true}
				{@const styles = getEventStyles(event)}
				{@const usePopup = !!onhover}
				{@const eventHeight = parseFloat(style.height)}

				<div
					class="absolute cursor-pointer overflow-hidden rounded-md text-xs transition-shadow hover:shadow-md {styles.containerClasses}"
					style="
						top: {style.top};
						height: calc({style.height} - 1px);
						left: {posStyle.left};
						width: calc({posStyle.width} - 4px);
						margin-left: 2px;
						{styles.containerStyle}
					"
					onmouseenter={(e) => onhover?.(event, e.currentTarget as HTMLElement)}
					onmouseleave={() => onhover?.(null, null)}
				>
					<div class="flex h-full flex-col p-1.5">
						<!-- Title row with project dot for confirmed -->
						<div class="flex min-w-0 items-start justify-between gap-1">
							<span
								class="flex-1 truncate font-medium {getPrimaryTextClasses(styles, isSkipped)}"
								style={getPrimaryTextStyle(styles, isSkipped)}>{event.title}</span
							>
							{#if isClassified && !needsReview && event.project}
								<span
									class="mt-0.5 h-3 w-3 flex-shrink-0 rounded-full {styles.textColors?.isDark
										? 'border border-white/50'
										: ''}"
									style="background-color: {event.project.color}"
									title={formatConfidenceTitle(
										event.project.name,
										event.classification_confidence,
										event.classification_source
									)}
								></span>
							{/if}
						</div>

						<!-- Bottom row: project buttons (left) and skip button (right) for pending -->
						{#if usePopup}
							<!-- Using popup mode - show quick actions inline if tall enough -->
							{#if isPending && eventHeight >= 40}
								<div class="mt-auto flex items-center justify-between pt-1">
									<div class="flex items-center gap-0.5">
										{#each activeProjects.slice(0, 4) as project, i}
											{@const isBestGuess =
												event.suggested_project_id === project.id ||
												(!event.suggested_project_id && i === 0)}
											<button
												type="button"
												class="h-3.5 w-3.5 rounded-full transition-all {isBestGuess
													? 'ring-2 ring-black/40 ring-offset-1 ring-offset-white dark:ring-white/60 dark:ring-offset-zinc-900'
													: 'ring-gray-400 hover:ring-2 hover:ring-offset-1'}"
												style="background-color: {project.color}"
												title="{project.name}{isBestGuess ? ' (suggested)' : ''}"
												onclick={(e) => {
													e.stopPropagation();
													onclassify?.(event.id, project.id);
												}}
											></button>
										{/each}
									</div>
									<button
										type="button"
										class="flex h-3.5 w-3.5 items-center justify-center rounded border border-dashed border-gray-400 text-[7px] text-gray-400 hover:border-gray-600 hover:text-gray-600 dark:border-gray-500 dark:hover:border-gray-300 dark:hover:text-gray-300"
										title="Did not attend"
										onclick={(e) => {
											e.stopPropagation();
											onskip?.(event.id);
										}}
									>
										✕
									</button>
								</div>
							{:else if isSkipped}
								<!-- Skip indicator in bottom right - clickable to unskip -->
								<div class="mt-auto flex justify-end">
									<button
										type="button"
										class="flex h-3.5 w-3.5 items-center justify-center rounded border border-dashed border-gray-400 text-[7px] text-gray-400 hover:border-gray-600 hover:text-gray-600 dark:border-gray-500 dark:hover:border-gray-300 dark:hover:text-gray-300"
										title="Click to mark as attended"
										onclick={(e) => {
											e.stopPropagation();
											onunskip?.(event.id);
										}}
									>✕</button>
								</div>
							{/if}
						{:else}
							<!-- Not using popup - show full classification UI -->
							{#if event.project}
								<div class="mt-1">
									<ProjectChip project={event.project} size="sm" />
								</div>
							{/if}

							<!-- Classification UI -->
							<div class="mt-auto pt-1">
								{#if isPending || reclassifyingId === event.id}
									<!-- Project color circles for quick classification with best-guess highlight -->
									<div class="flex items-center justify-between">
										<div class="flex flex-wrap items-center gap-1">
											{#each activeProjects.slice(0, 4) as project, i}
												{@const isBestGuess =
													event.suggested_project_id === project.id ||
													(!event.suggested_project_id && i === 0 && isPending)}
												<button
													type="button"
													class="h-3.5 w-3.5 rounded-full transition-all {isBestGuess
														? 'ring-2 ring-black/40 ring-offset-1 ring-offset-white dark:ring-white/60 dark:ring-offset-zinc-900'
														: 'ring-gray-400 hover:ring-2 hover:ring-offset-1'}"
													style="background-color: {project.color}"
													title="{project.name}{isBestGuess ? ' (suggested)' : ''}"
													onclick={() => {
														onclassify?.(event.id, project.id);
														reclassifyingId = null;
													}}
												></button>
											{/each}
											{#if reclassifyingId === event.id}
												<button
													type="button"
													class="text-[10px] text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
													onclick={() => (reclassifyingId = null)}
												>
													Cancel
												</button>
											{/if}
										</div>
										<button
											type="button"
											class="flex h-3.5 w-3.5 items-center justify-center rounded border border-dashed border-gray-400 text-[7px] text-gray-400 hover:border-gray-600 hover:text-gray-600 dark:border-gray-500 dark:hover:border-gray-300 dark:hover:text-gray-300"
											title="Did not attend"
											onclick={() => {
												onskip?.(event.id);
												reclassifyingId = null;
											}}
										>
											✕
										</button>
									</div>
								{:else if isSkipped}
									<!-- Skip indicator in bottom right - clickable to unskip -->
									<div class="flex justify-end">
										<button
											type="button"
											class="flex h-3.5 w-3.5 items-center justify-center rounded border border-dashed border-gray-400 text-[7px] text-gray-400 hover:border-gray-600 hover:text-gray-600 dark:border-gray-500 dark:hover:border-gray-300 dark:hover:text-gray-300"
											title="Click to mark as attended"
											onclick={() => {
												onunskip?.(event.id);
												reclassifyingId = null;
											}}
										>✕</button>
									</div>
								{:else if isClassified && event.project}
									<!-- Classified - click to reclassify -->
									<button
										type="button"
										class="h-3.5 w-3.5 rounded-full ring-gray-400 transition-shadow hover:ring-2 hover:ring-offset-1 {styles
											.textColors?.isDark
											? 'ring-offset-current'
											: ''}"
										style="background-color: {event.project.color}"
										title="{event.project.name} - click to reclassify"
										onclick={() => (reclassifyingId = event.id)}
									></button>
								{/if}
							</div>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	</div>
</div>
