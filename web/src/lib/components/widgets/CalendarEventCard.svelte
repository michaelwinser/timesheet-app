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
	function getStatusBackground(status: string): string {
		switch (status) {
			case 'classified':
				return 'bg-green-50';
			case 'skipped':
				return 'bg-gray-50';
			default:
				return 'bg-white';
		}
	}

	const timeRange = $derived(formatTimeRange(event.start_time, event.end_time));
	const duration = $derived(getDuration(event.start_time, event.end_time));
	const dateStr = $derived(formatDate(event.start_time));
	const isPending = $derived(event.classification_status === 'pending');
	const needsReview = $derived(event.needs_review === true);
	const calendarColor = $derived(event.calendar_color || '#9CA3AF');
</script>

<div
	class="border rounded-lg p-3 hover:shadow-sm {getStatusBackground(event.classification_status)}"
	style="border-left: 3px solid {calendarColor};"
>
	<div class="flex items-start justify-between gap-2">
		<div class="flex-1 min-w-0">
			<div class="flex items-center gap-2 mb-1">
				<h4 class="font-medium text-gray-900 truncate">{event.title}</h4>
				{#if needsReview}
					<span
						class="w-2 h-2 bg-yellow-400 rounded-full shrink-0"
						title="Needs review - auto-classified with medium confidence"
					></span>
				{/if}
				{#if event.project}
					<ProjectChip project={event.project} />
				{/if}
			</div>
			<div class="flex items-center gap-3 text-sm text-gray-500">
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

		<div class="flex items-center gap-1.5 shrink-0">
			{#if isPending || showReclassify}
				<!-- Project color circles for quick classification -->
				{#each activeProjects as project}
					<button
						type="button"
						class="w-6 h-6 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow"
						style="background-color: {project.color}"
						title={project.name}
						onclick={() => { onclassify?.(project.id); showReclassify = false; }}
					></button>
				{/each}
				<button
					type="button"
					class="w-6 h-6 rounded-full border-2 border-dashed border-gray-300 text-gray-400 hover:border-gray-400 hover:text-gray-500 flex items-center justify-center text-xs"
					title="Skip - did not attend"
					onclick={() => { onskip?.(); showReclassify = false; }}
				>
					✕
				</button>
				{#if showReclassify}
					<button
						type="button"
						class="ml-1 text-xs text-gray-400 hover:text-gray-600"
						onclick={() => showReclassify = false}
					>
						Cancel
					</button>
				{/if}
			{:else}
				<!-- Classified/skipped - click to reclassify -->
				{#if event.classification_status === 'classified' && event.project}
					<button
						type="button"
						class="w-6 h-6 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow"
						style="background-color: {event.project.color}"
						title="{event.project.name} - click to reclassify"
						onclick={() => showReclassify = true}
					></button>
				{:else}
					<button
						type="button"
						class="w-6 h-6 rounded-full border-2 border-dashed border-gray-300 text-gray-400 hover:border-gray-400 hover:text-gray-500 flex items-center justify-center text-xs hover:ring-2 hover:ring-offset-1 ring-gray-300 transition-shadow"
						title="Skipped - click to reclassify"
						onclick={() => showReclassify = true}
					>
						✕
					</button>
				{/if}
			{/if}
		</div>
	</div>
</div>
