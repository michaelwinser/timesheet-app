<script lang="ts">
	import type { CalendarEvent, Project } from '$lib/api/types';
	import ProjectChip from './ProjectChip.svelte';

	interface Props {
		events: CalendarEvent[];
		projects: Project[];
		date: Date;
		onclassify?: (eventId: string, projectId: string) => void;
		onskip?: (eventId: string) => void;
	}

	let { events, projects, date, onclassify, onskip }: Props = $props();

	const activeProjects = $derived(projects.filter(p => !p.is_archived));

	// Track which events are showing reclassify UI
	let reclassifyingId = $state<string | null>(null);

	// Time grid configuration
	const startHour = 7; // 7 AM
	const endHour = 20; // 8 PM
	const hourHeight = 60; // pixels per hour

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
	const eventsWithColumns = $derived(getEventsWithColumns(events));
</script>

<div class="flex">
	<!-- Time labels column -->
	<div class="w-16 flex-shrink-0 text-right pr-2">
		{#each hours as hour}
			<div class="text-xs text-gray-400" style="height: {hourHeight}px">
				{hour === 12 ? '12 PM' : hour > 12 ? `${hour - 12} PM` : `${hour} AM`}
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
			{@const calendarColor = event.calendar_color || '#9CA3AF'}

			<div
				class="absolute rounded-md border overflow-hidden text-xs {getStatusBackground(event.classification_status)} hover:shadow-md transition-shadow"
				style="
					top: {style.top};
					height: {style.height};
					left: {posStyle.left};
					width: calc({posStyle.width} - 4px);
					margin-left: 2px;
					border-left: 3px solid {calendarColor};
				"
			>
				<div class="p-1.5 h-full flex flex-col">
					<div class="font-medium text-gray-900 line-clamp-3">{event.title}</div>

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
				</div>
			</div>
		{/each}
	</div>
</div>
