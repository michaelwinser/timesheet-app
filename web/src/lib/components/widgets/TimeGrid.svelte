<script lang="ts">
	import type { CalendarEvent, Project } from '$lib/api/types';
	import ProjectChip from './ProjectChip.svelte';

	interface Props {
		events: CalendarEvent[];
		projects: Project[];
		date: Date;
		onclassify?: (eventId: string, projectId: string) => void;
		onskip?: (eventId: string) => void;
		onhover?: (event: CalendarEvent | null, element: HTMLElement | null) => void;
		showTimeLegend?: boolean;
	}

	let { events, projects, date, onclassify, onskip, onhover, showTimeLegend = true }: Props = $props();

	const activeProjects = $derived(projects.filter(p => !p.is_archived));

	// Track which events are showing reclassify UI (only used if no onhover)
	let reclassifyingId = $state<string | null>(null);

	// Scroll container reference
	let scrollContainer: HTMLDivElement;

	// Time grid configuration
	const startHour = 0; // Midnight
	const endHour = 24; // Midnight (full 24h)
	const hourHeight = 60; // pixels per hour
	const viewportHours = 10; // 10 hours visible in viewport

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

	// Scroll to first event when events or date change
	$effect(() => {
		// Track dependencies that should trigger a scroll
		const _events = timedEvents;
		const _date = date;

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
			if ((end.getHours() === 0 && end.getMinutes() === 0) ||
				(end.getHours() === 23 && end.getMinutes() >= 59)) {
				return true;
			}
		}

		return false;
	}

	// Separate all-day and timed events
	const allDayEvents = $derived(events.filter(e => isAllDayEvent(e)));
	const timedEvents = $derived(events.filter(e => !isAllDayEvent(e)));

	// Calculate position and height for an event
	function getEventStyle(event: CalendarEvent): { top: string; height: string; left: string; width: string } {
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
	function getEventsWithColumns(events: CalendarEvent[]): Array<{ event: CalendarEvent; column: number; totalColumns: number }> {
		if (events.length === 0) return [];

		// Sort by start time
		const sorted = [...events].sort((a, b) =>
			new Date(a.start_time).getTime() - new Date(b.start_time).getTime()
		);

		const result: Array<{ event: CalendarEvent; column: number; totalColumns: number; endTime: number }> = [];
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

	function getEventPositionStyle(column: number, totalColumns: number): { left: string; width: string } {
		const width = 100 / totalColumns;
		const left = column * width;
		return {
			left: `${left}%`,
			width: `${width}%`
		};
	}

	// Get background color based on classification status
	function getStatusBackground(status: string, needsReview: boolean = false): string {
		if (status === 'classified' && needsReview) return 'bg-amber-50';
		switch (status) {
			case 'classified':
				return 'bg-green-50';
			case 'skipped':
				return 'bg-gray-100';
			default:
				return 'bg-white';
		}
	}

	// Format tooltip with confidence score
	function formatConfidenceTitle(projectName: string, confidence: number | null | undefined, source: string | null | undefined): string {
		if (source === 'manual') return projectName;
		if (confidence != null) {
			return `${projectName} (confidence: ${Math.round(confidence * 100)}%)`;
		}
		return projectName;
	}

	const hours = $derived(Array.from({ length: endHour - startHour }, (_, i) => startHour + i));
	const gridHeight = $derived((endHour - startHour) * hourHeight);
	const eventsWithColumns = $derived(getEventsWithColumns(timedEvents));
</script>

<!-- All-day events section -->
{#if allDayEvents.length > 0}
	<div class="mb-2 border-b border-gray-200 pb-2">
		<div class="flex items-center gap-1 flex-wrap">
			<span class="text-xs text-gray-400 w-12 text-right pr-2 flex-shrink-0">All day</span>
			{#each allDayEvents as event (event.id)}
				{@const calendarColor = event.classification_status === 'skipped' ? '#9CA3AF' : (event.calendar_color || '#9CA3AF')}
				{@const isPending = event.classification_status === 'pending'}
				{@const isClassified = event.classification_status === 'classified'}
				{@const isSkipped = event.classification_status === 'skipped'}
				{@const needsReview = event.needs_review === true}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full cursor-pointer hover:shadow-sm transition-shadow {getStatusBackground(event.classification_status, needsReview)}"
					style="border-left: 3px solid {calendarColor};"
					onmouseenter={(e) => onhover?.(event, e.currentTarget as HTMLElement)}
					onmouseleave={() => onhover?.(null, null)}
				>
					<span class="truncate max-w-[120px] {isSkipped ? 'line-through text-gray-400' : ''}" title={event.title}>{event.title}</span>
					{#if isClassified && event.project}
						<span
							class="w-2.5 h-2.5 rounded-full flex-shrink-0"
							style="background-color: {event.project.color}"
							title={formatConfidenceTitle(event.project.name, event.classification_confidence, event.classification_source)}
						></span>
					{:else if isSkipped}
						<span class="w-2.5 h-2.5 rounded border border-dashed border-gray-400 text-gray-400 flex items-center justify-center text-[5px]">✕</span>
					{:else if isPending}
						<!-- Quick classify buttons for pending all-day events -->
						<div class="flex items-center gap-0.5 ml-1">
							{#each activeProjects.slice(0, 3) as project}
								<button
									type="button"
									class="w-2.5 h-2.5 rounded-full hover:ring-1 hover:ring-offset-1 ring-gray-400 transition-shadow"
									style="background-color: {project.color}"
									title={project.name}
									onclick={(e) => { e.stopPropagation(); onclassify?.(event.id, project.id); }}
								></button>
							{/each}
						</div>
						<button
							type="button"
							class="w-2.5 h-2.5 rounded border border-dashed border-gray-400 text-gray-400 hover:border-gray-600 flex items-center justify-center text-[5px] ml-1"
							title="Skip - did not attend"
							onclick={(e) => { e.stopPropagation(); onskip?.(event.id); }}
						>✕</button>
					{/if}
				</div>
			{/each}
		</div>
	</div>
{/if}

<!-- Scrollable container with 10h viewport -->
<div
	class="overflow-y-auto overflow-x-hidden"
	style="height: {viewportHours * hourHeight}px"
	bind:this={scrollContainer}
>
	<div class="flex">
		<!-- Time labels column (optional) -->
		{#if showTimeLegend}
			<div class="w-12 flex-shrink-0 text-right pr-2">
				{#each hours as hour}
					<div class="text-xs text-gray-400" style="height: {hourHeight}px">
						{hour === 0 ? '12 AM' : hour === 12 ? '12 PM' : hour > 12 ? `${hour - 12} PM` : `${hour} AM`}
					</div>
				{/each}
			</div>
		{/if}

		<!-- Events grid -->
		<div class="flex-1 relative {showTimeLegend ? 'border-l border-gray-200' : ''}" style="height: {gridHeight}px">
			<!-- Hour lines -->
			{#each hours as hour, i}
				<div
					class="absolute w-full border-t border-gray-100"
					style="top: {i * hourHeight}px"
				></div>
			{/each}

		<!-- Events -->
		{#each eventsWithColumns as { event, column, totalColumns } (event.id)}
			{@const style = getEventStyle(event)}
			{@const posStyle = getEventPositionStyle(column, totalColumns)}
			{@const isPending = event.classification_status === 'pending'}
			{@const isClassified = event.classification_status === 'classified'}
			{@const isSkipped = event.classification_status === 'skipped'}
			{@const needsReview = event.needs_review === true}
			{@const calendarColor = isSkipped ? '#9CA3AF' : (event.calendar_color || '#9CA3AF')}
			{@const usePopup = !!onhover}
			{@const eventHeight = parseFloat(style.height)}

			<div
				class="absolute rounded-md border overflow-hidden text-xs {getStatusBackground(event.classification_status, needsReview)} hover:shadow-md transition-shadow cursor-pointer"
				style="
					top: {style.top};
					height: {style.height};
					left: {posStyle.left};
					width: calc({posStyle.width} - 4px);
					margin-left: 2px;
					border-left: 3px solid {calendarColor};
				"
				onmouseenter={(e) => onhover?.(event, e.currentTarget as HTMLElement)}
				onmouseleave={() => onhover?.(null, null)}
			>
				<div class="p-1.5 h-full flex flex-col">
					<!-- Title row with project dot for classified -->
					<div class="flex items-start justify-between gap-1 min-w-0">
						<span class="font-medium truncate flex-1 {isSkipped ? 'line-through text-gray-400' : 'text-gray-900'}">{event.title}</span>
						{#if isClassified && event.project}
							<span
								class="w-3 h-3 rounded-full flex-shrink-0 mt-0.5"
								style="background-color: {event.project.color}"
								title={formatConfidenceTitle(event.project.name, event.classification_confidence, event.classification_source)}
							></span>
						{/if}
					</div>

					<!-- Bottom row: project buttons (left) and skip button (right) for pending -->
					{#if usePopup}
						<!-- Using popup mode - show quick actions inline if tall enough -->
						{#if isPending && eventHeight >= 40}
							<div class="mt-auto pt-1 flex items-center justify-between">
								<div class="flex items-center gap-0.5">
									{#each activeProjects.slice(0, 4) as project}
										<button
											type="button"
											class="w-3.5 h-3.5 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow"
											style="background-color: {project.color}"
											title={project.name}
											onclick={(e) => { e.stopPropagation(); onclassify?.(event.id, project.id); }}
										></button>
									{/each}
								</div>
								<button
									type="button"
									class="w-3.5 h-3.5 rounded border border-dashed border-gray-400 text-gray-400 hover:border-gray-600 hover:text-gray-600 flex items-center justify-center text-[7px]"
									title="Skip - did not attend"
									onclick={(e) => { e.stopPropagation(); onskip?.(event.id); }}
								>✕</button>
							</div>
						{:else if isSkipped}
							<!-- Skip indicator in bottom right -->
							<div class="mt-auto flex justify-end">
								<span class="w-3.5 h-3.5 rounded border border-dashed border-gray-400 text-gray-400 flex items-center justify-center text-[7px]">✕</span>
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
								<!-- Project color circles for quick classification -->
								<div class="flex items-center justify-between">
									<div class="flex flex-wrap gap-1 items-center">
										{#each activeProjects.slice(0, 4) as project}
											<button
												type="button"
												class="w-3.5 h-3.5 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow"
												style="background-color: {project.color}"
												title={project.name}
												onclick={() => { onclassify?.(event.id, project.id); reclassifyingId = null; }}
											></button>
										{/each}
										{#if reclassifyingId === event.id}
											<button
												type="button"
												class="text-[10px] text-gray-400 hover:text-gray-600"
												onclick={() => reclassifyingId = null}
											>
												Cancel
											</button>
										{/if}
									</div>
									<button
										type="button"
										class="w-3.5 h-3.5 rounded border border-dashed border-gray-400 text-gray-400 hover:border-gray-600 hover:text-gray-600 flex items-center justify-center text-[7px]"
										title="Skip - did not attend"
										onclick={() => { onskip?.(event.id); reclassifyingId = null; }}
									>✕</button>
								</div>
							{:else if isSkipped}
								<!-- Skip indicator in bottom right -->
								<div class="flex justify-end">
									<span class="w-3.5 h-3.5 rounded border border-dashed border-gray-400 text-gray-400 flex items-center justify-center text-[7px]">✕</span>
								</div>
							{:else if isClassified && event.project}
								<!-- Classified - click to reclassify -->
								<button
									type="button"
									class="w-3.5 h-3.5 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow"
									style="background-color: {event.project.color}"
									title="{event.project.name} - click to reclassify"
									onclick={() => reclassifyingId = event.id}
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
