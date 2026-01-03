<script lang="ts">
	import type { CalendarEvent, Project } from '$lib/api/types';
	import ProjectChip from './ProjectChip.svelte';

	interface Props {
		event: CalendarEvent;
		projects: Project[];
		onclassify?: (projectId: string) => void;
		onskip?: () => void;
		onunclassify?: () => void;
	}

	let { event, projects, onclassify, onskip, onunclassify }: Props = $props();

	let showReclassify = $state(false);
	const activeProjects = $derived(projects.filter(p => !p.is_archived));

	// Format time range
	function formatTimeRange(start: string, end: string): string {
		const startDate = new Date(start);
		const endDate = new Date(end);
		const options: Intl.DateTimeFormatOptions = { hour: 'numeric', minute: '2-digit' };
		return `${startDate.toLocaleTimeString([], options)} - ${endDate.toLocaleTimeString([], options)}`;
	}

	// Calculate duration in hours
	function getDuration(start: string, end: string): number {
		const startDate = new Date(start);
		const endDate = new Date(end);
		return Math.round((endDate.getTime() - startDate.getTime()) / (1000 * 60 * 60) * 100) / 100;
	}

	// Format date
	function formatDate(dateStr: string): string {
		const date = new Date(dateStr);
		return date.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' });
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

	const timeRange = $derived(formatTimeRange(event.start_time, event.end_time));
	const duration = $derived(getDuration(event.start_time, event.end_time));
	const dateStr = $derived(formatDate(event.start_time));
	const isPending = $derived(event.classification_status === 'pending');
	const isClassified = $derived(event.classification_status === 'classified');
	const isSkipped = $derived(event.classification_status === 'skipped');
	const needsReview = $derived(event.needs_review === true);
	const calendarColor = $derived(isSkipped ? '#9CA3AF' : (event.calendar_color || '#9CA3AF'));
</script>

<div
	class="border rounded-lg p-3 hover:shadow-sm {getStatusBackground(event.classification_status, needsReview)}"
	style="border-left: 3px solid {calendarColor};"
>
	<div class="flex flex-col gap-1">
		<!-- Top row: title and project dot -->
		<div class="flex items-start justify-between gap-2">
			<div class="flex-1 min-w-0">
				<h4 class="font-medium truncate {isSkipped ? 'line-through text-gray-400' : 'text-gray-900'}">{event.title}</h4>
				<div class="flex items-center gap-3 text-sm {isSkipped ? 'text-gray-400' : 'text-gray-500'} mt-0.5">
					<span>{dateStr}</span>
					<span>{timeRange}</span>
					<span class="font-medium">{duration}h</span>
				</div>
				{#if event.attendees && event.attendees.length > 0}
					<div class="text-xs text-gray-400 mt-1 truncate">
						{event.attendees.slice(0, 3).join(', ')}{event.attendees.length > 3 ? ` +${event.attendees.length - 3} more` : ''}
					</div>
				{/if}
			</div>
			{#if isClassified && event.project}
				<span
					class="w-4 h-4 rounded-full flex-shrink-0"
					style="background-color: {event.project.color}"
					title={formatConfidenceTitle(event.project.name, event.classification_confidence, event.classification_source)}
				></span>
			{/if}
		</div>

		<!-- Bottom row: project buttons (left) and skip button (right) -->
		{#if isPending || showReclassify}
			<div class="flex items-center justify-between pt-1">
				<div class="flex items-center gap-1">
					{#each activeProjects as project}
						<button
							type="button"
							class="w-5 h-5 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow"
							style="background-color: {project.color}"
							title={project.name}
							onclick={() => { onclassify?.(project.id); showReclassify = false; }}
						></button>
					{/each}
					{#if showReclassify}
						<button
							type="button"
							class="ml-2 text-xs text-gray-400 hover:text-gray-600"
							onclick={() => showReclassify = false}
						>
							Cancel
						</button>
					{/if}
				</div>
				<button
					type="button"
					class="w-5 h-5 rounded border border-dashed border-gray-400 text-gray-400 hover:border-gray-600 hover:text-gray-600 flex items-center justify-center text-xs"
					title="Skip - did not attend"
					onclick={() => { onskip?.(); showReclassify = false; }}
				>
					✕
				</button>
			</div>
		{:else if isSkipped}
			<!-- Skip indicator in bottom right -->
			<div class="flex justify-end pt-1">
				<button
					type="button"
					class="w-5 h-5 rounded border border-dashed border-gray-400 text-gray-400 hover:border-gray-600 hover:text-gray-600 flex items-center justify-center text-xs"
					title="Skipped - click to reclassify"
					onclick={() => showReclassify = true}
				>
					✕
				</button>
			</div>
		{:else if isClassified && event.project}
			<!-- Classified - click to reclassify (hidden, use project dot) -->
			<div class="flex justify-end pt-1">
				<button
					type="button"
					class="w-5 h-5 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow"
					style="background-color: {event.project.color}"
					title="{event.project.name} - click to reclassify"
					onclick={() => showReclassify = true}
				></button>
			</div>
		{/if}
	</div>
</div>
