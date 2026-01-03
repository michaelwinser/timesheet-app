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
	}

	let { events, projects, date, onclassify, onskip, onhover }: Props = $props();

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
	const firstEventHour = $derived.by(() => {
		if (timedEvents.length === 0) return 8; // Default to 8 AM if no events
		let minHour = 24;
		for (const event of timedEvents) {
			const hour = new Date(event.start_time).getHours();
			if (hour < minHour) minHour = hour;
		}
		// Start 1 hour before first event, minimum 0
		return Math.max(0, minHour - 1);
	});

	// Scroll to first event when events change
	$effect(() => {
		if (scrollContainer && timedEvents.length > 0) {
			const scrollTop = firstEventHour * hourHeight;
			scrollContainer.scrollTop = scrollTop;
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
	function getStatusBackground(status: string): string {
		switch (status) {
			case 'classified':
				return 'bg-green-50';
			case 'skipped':
				return 'bg-gray-100';
			default:
				return 'bg-white';
		}
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
				{@const calendarColor = event.calendar_color || '#9CA3AF'}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full cursor-pointer hover:shadow-sm transition-shadow {getStatusBackground(event.classification_status)}"
					style="border-left: 3px solid {calendarColor};"
					onmouseenter={(e) => onhover?.(event, e.currentTarget as HTMLElement)}
					onmouseleave={() => onhover?.(null, null)}
				>
					<span class="truncate max-w-[120px]" title={event.title}>{event.title}</span>
					{#if event.classification_status === 'classified' && event.project}
						<span
							class="w-2.5 h-2.5 rounded-full flex-shrink-0"
							style="background-color: {event.project.color}"
							title={event.project.name}
						></span>
					{:else if event.classification_status === 'skipped'}
						<span class="w-2.5 h-2.5 rounded-full flex-shrink-0 border border-dashed border-gray-300 text-gray-400 flex items-center justify-center text-[5px]">✕</span>
					{:else}
						<span class="w-2.5 h-2.5 rounded-full flex-shrink-0 bg-amber-200 border border-amber-300"></span>
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
		<!-- Time labels column -->
		<div class="w-12 flex-shrink-0 text-right pr-2">
			{#each hours as hour}
				<div class="text-xs text-gray-400" style="height: {hourHeight}px">
					{hour === 0 ? '12 AM' : hour === 12 ? '12 PM' : hour > 12 ? `${hour - 12} PM` : `${hour} AM`}
				</div>
			{/each}
		</div>

		<!-- Events grid -->
		<div class="flex-1 relative border-l border-gray-200" style="height: {gridHeight}px">
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
			{@const needsReview = event.needs_review === true}
			{@const calendarColor = event.calendar_color || '#9CA3AF'}
			{@const usePopup = !!onhover}

			<div
				class="absolute rounded-md border overflow-hidden text-xs {getStatusBackground(event.classification_status)} hover:shadow-md transition-shadow cursor-pointer"
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
				<div class="p-1.5 h-full flex flex-col relative">
					{#if needsReview}
						<div
							class="absolute top-1 right-1 w-2 h-2 bg-yellow-400 rounded-full"
							title="Needs review - auto-classified with medium confidence"
						></div>
					{/if}

					<!-- Compact view: title with classification indicator -->
					<div class="flex items-start justify-between gap-1 min-w-0">
						<span class="font-medium text-gray-900 truncate flex-1">{event.title}</span>
						<!-- Classification indicator (always visible, compact) -->
						{#if event.classification_status === 'classified' && event.project}
							<span
								class="w-3 h-3 rounded-full flex-shrink-0 mt-0.5"
								style="background-color: {event.project.color}"
								title={event.project.name}
							></span>
						{:else if event.classification_status === 'skipped'}
							<span
								class="w-3 h-3 rounded-full flex-shrink-0 mt-0.5 border border-dashed border-gray-300 text-gray-400 flex items-center justify-center text-[6px]"
								title="Skipped"
							>✕</span>
						{:else}
							<span
								class="w-3 h-3 rounded-full flex-shrink-0 mt-0.5 bg-amber-200 border border-amber-300"
								title="Pending classification"
							></span>
						{/if}
					</div>

					<!-- Only show full classification UI if not using popup -->
					{#if !usePopup}
						{#if event.project}
							<div class="mt-1">
								<ProjectChip project={event.project} size="sm" />
							</div>
						{/if}

						<!-- Classification UI -->
						<div class="mt-auto pt-1">
							{#if isPending || reclassifyingId === event.id}
								<!-- Project color circles for quick classification -->
								<div class="flex flex-wrap gap-1 items-center">
									{#each activeProjects.slice(0, 5) as project}
										<button
											type="button"
											class="w-4 h-4 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow"
											style="background-color: {project.color}"
											title={project.name}
											onclick={() => { onclassify?.(event.id, project.id); reclassifyingId = null; }}
										></button>
									{/each}
									<button
										type="button"
										class="w-4 h-4 rounded-full border border-dashed border-gray-300 text-gray-400 hover:border-gray-500 hover:text-gray-600 flex items-center justify-center text-[8px]"
										title="Skip - did not attend"
										onclick={() => { onskip?.(event.id); reclassifyingId = null; }}
									>
										✕
									</button>
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
							{:else}
								<!-- Classified/skipped - click to reclassify -->
								{#if event.classification_status === 'classified' && event.project}
									<button
										type="button"
										class="w-4 h-4 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow"
										style="background-color: {event.project.color}"
										title="{event.project.name} - click to reclassify"
										onclick={() => reclassifyingId = event.id}
									></button>
								{:else}
									<button
										type="button"
										class="w-4 h-4 rounded-full border border-dashed border-gray-300 text-gray-400 hover:border-gray-500 hover:text-gray-600 flex items-center justify-center text-[8px] hover:ring-1 ring-gray-300 transition-shadow"
										title="Skipped - click to reclassify"
										onclick={() => reclassifyingId = event.id}
									>
										✕
									</button>
								{/if}
							{/if}
						</div>
					{/if}
				</div>
			</div>
		{/each}
		</div>
	</div>
</div>
