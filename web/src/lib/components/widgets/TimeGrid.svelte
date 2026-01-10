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
		highlightedTarget?: string | null;
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
		showTimeLegend = true,
		highlightedTarget = null
	}: Props = $props();

	// Determine if an event should be dimmed based on highlight target
	function shouldDimEvent(event: CalendarEvent): boolean {
		if (!highlightedTarget) return false;

		if (highlightedTarget === 'skipped') {
			return !event.is_skipped;
		}
		if (highlightedTarget === 'hidden') {
			return !event.project?.is_hidden_by_default;
		}
		if (highlightedTarget === 'archived') {
			return !event.project?.is_archived;
		}
		// Regular project ID
		return event.project_id !== highlightedTarget;
	}

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

	// Detect all-day events using the is_all_day flag from the API
	function isAllDayEvent(event: CalendarEvent): boolean {
		return event.is_all_day === true;
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

	// Calculate overlapping events and assign columns using Greedy Expansion algorithm
	// This mimics Google Calendar's behavior where events expand to fill available space
	function getEventsWithColumns(
		events: CalendarEvent[]
	): Array<{ event: CalendarEvent; column: number; totalColumns: number; span: number }> {
		if (events.length === 0) return [];

		// Helper: check if two events collide (overlap in time)
		function collide(aStart: number, aEnd: number, bStart: number, bEnd: number): boolean {
			return aStart < bEnd && aEnd > bStart;
		}

		// Sort by start time, then by duration (longest first for better packing)
		const sorted = [...events].sort((a, b) => {
			const aStart = new Date(a.start_time).getTime();
			const bStart = new Date(b.start_time).getTime();
			if (aStart === bStart) {
				const aDuration = new Date(a.end_time).getTime() - aStart;
				const bDuration = new Date(b.end_time).getTime() - bStart;
				return bDuration - aDuration; // Longest first
			}
			return aStart - bStart;
		});

		// Step 1: Group events into overlapping clusters
		const clusters: CalendarEvent[][] = [];
		let currentCluster: CalendarEvent[] = [];
		let clusterEnd = -1;

		for (const event of sorted) {
			const startTime = new Date(event.start_time).getTime();
			const endTime = new Date(event.end_time).getTime();

			if (currentCluster.length === 0) {
				currentCluster.push(event);
				clusterEnd = endTime;
			} else if (startTime < clusterEnd) {
				// Overlap detected: add to current cluster
				currentCluster.push(event);
				clusterEnd = Math.max(clusterEnd, endTime);
			} else {
				// No overlap: seal the current cluster and start a new one
				clusters.push(currentCluster);
				currentCluster = [event];
				clusterEnd = endTime;
			}
		}
		if (currentCluster.length > 0) clusters.push(currentCluster);

		// Step 2: Process each cluster to assign columns and calculate expansion
		const result: Array<{
			event: CalendarEvent;
			column: number;
			totalColumns: number;
			span: number;
		}> = [];

		for (const cluster of clusters) {
			// Columns is an array of events in each column
			const columns: CalendarEvent[][] = [];
			const eventColIndex = new Map<string, number>();

			// Pack events into columns
			for (const event of cluster) {
				const startTime = new Date(event.start_time).getTime();
				let placed = false;

				// Try to fit in existing columns
				for (let i = 0; i < columns.length; i++) {
					const colEvents = columns[i];
					const lastEvent = colEvents[colEvents.length - 1];
					const lastEnd = new Date(lastEvent.end_time).getTime();

					// If the event starts after the last event in this column ends
					if (startTime >= lastEnd) {
						colEvents.push(event);
						eventColIndex.set(event.id, i);
						placed = true;
						break;
					}
				}

				// If it didn't fit, start a new column
				if (!placed) {
					columns.push([event]);
					eventColIndex.set(event.id, columns.length - 1);
				}
			}

			// Calculate dimensions with "Greedy Expansion"
			const totalCols = columns.length;

			for (const event of cluster) {
				const colIndex = eventColIndex.get(event.id)!;
				const eventStart = new Date(event.start_time).getTime();
				const eventEnd = new Date(event.end_time).getTime();

				// Default span is 1
				let span = 1;

				// Check columns to the right. If they are empty during this event's time, expand.
				for (let i = colIndex + 1; i < totalCols; i++) {
					// Check if any event in column `i` overlaps with our current event
					const hasCollision = columns[i].some((otherEvt) => {
						const otherStart = new Date(otherEvt.start_time).getTime();
						const otherEnd = new Date(otherEvt.end_time).getTime();
						return collide(eventStart, eventEnd, otherStart, otherEnd);
					});

					if (hasCollision) break;
					span++;
				}

				result.push({ event, column: colIndex, totalColumns: totalCols, span });
			}
		}

		return result;
	}

	function getEventPositionStyle(
		column: number,
		totalColumns: number,
		span: number = 1
	): { left: string; width: string } {
		const colWidth = 100 / totalColumns;
		const left = column * colWidth;
		const width = span * colWidth;
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
				{@const isDimmed = shouldDimEvent(event)}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="inline-flex cursor-pointer items-center gap-1 rounded-full px-2 py-0.5 text-xs transition-all hover:shadow-sm {styles.containerClasses} {isDimmed ? 'opacity-25' : ''}"
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
			{#each eventsWithColumns as { event, column, totalColumns, span } (event.id)}
				{@const style = getEventStyle(event)}
				{@const posStyle = getEventPositionStyle(column, totalColumns, span)}
				{@const isPending = event.classification_status === 'pending'}
				{@const isClassified = event.classification_status === 'classified'}
				{@const isSkipped = event.is_skipped === true}
				{@const needsReview = event.needs_review === true}
				{@const styles = getEventStyles(event)}
				{@const usePopup = !!onhover}
				{@const eventHeight = parseFloat(style.height)}
				{@const isDimmed = shouldDimEvent(event)}

				<div
					class="absolute cursor-pointer overflow-hidden rounded-md text-xs transition-all hover:shadow-md {styles.containerClasses} {isDimmed ? 'opacity-25' : ''}"
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
