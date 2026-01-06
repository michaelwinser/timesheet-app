<script lang="ts">
	import type { CalendarEvent, Project } from '$lib/api/types';
	import ProjectChip from './ProjectChip.svelte';
	import { getProjectTextColors, getVerificationTextColor } from '$lib/utils/colors';

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

	// Get styling based on classification status (Google Calendar inspired)
	function getStatusClasses(status: string, needsReview: boolean = false): string {
		if (status === 'classified' && needsReview) {
			// Needs verification: outlined style (border/text colored by project, handled via inline style)
			return 'bg-white dark:bg-zinc-900 border-2 border-solid';
		}
		switch (status) {
			case 'classified':
				// Confirmed: solid project color background (handled via inline style)
				return 'border border-solid';
			case 'skipped':
				return 'bg-gray-100 dark:bg-zinc-800 border border-gray-200 dark:border-zinc-700';
			default:
				// Pending: white/black with border
				return 'bg-white dark:bg-zinc-900 border-2 border-solid border-black/30 dark:border-white/50';
		}
	}

	// Get inline style for status-dependent coloring
	function getStatusStyle(status: string, needsReview: boolean, projectColor: string | null): string {
		if (status === 'classified' && !needsReview && projectColor) {
			// Confirmed: solid project color background
			return `background-color: ${projectColor}; border-color: ${projectColor};`;
		}
		if (status === 'classified' && needsReview && projectColor) {
			// Needs verification: outlined style with project color border
			return `border: 2px solid ${projectColor};`;
		}
		return '';
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
	const projectColor = $derived(event.project?.color || null);
	const statusClasses = $derived(getStatusClasses(event.classification_status, needsReview));
	const statusStyle = $derived(getStatusStyle(event.classification_status, needsReview, projectColor));
	const textColors = $derived(projectColor ? getProjectTextColors(projectColor) : null);
	// For needs verification, use project color for text
	const needsVerifyTextColor = $derived(isClassified && needsReview && projectColor ? getVerificationTextColor(projectColor) : null);
</script>

<div
	class="rounded-lg p-3 hover:shadow-sm transition-shadow {statusClasses}"
	style="{statusStyle}"
>
	<div class="flex flex-col gap-1">
		<!-- Top row: title and project dot -->
		<div class="flex items-start justify-between gap-2">
			<div class="flex-1 min-w-0">
				<h4
					class="font-medium truncate {isSkipped ? 'line-through text-gray-400' : !isClassified || needsReview ? 'text-gray-900 dark:text-gray-100' : ''}"
					style={needsVerifyTextColor ? `color: ${needsVerifyTextColor}` : isClassified && !needsReview && textColors ? `color: ${textColors.text}` : ''}
				>{event.title}</h4>
				<div
					class="flex items-center gap-3 text-sm mt-0.5 {isSkipped ? 'text-gray-400' : !isClassified || needsReview ? 'text-gray-500 dark:text-gray-400' : ''}"
					style={needsVerifyTextColor ? `color: ${needsVerifyTextColor}; opacity: 0.8` : isClassified && !needsReview && textColors ? `color: ${textColors.textMuted}` : ''}
				>
					<span>{dateStr}</span>
					<span>{timeRange}</span>
					<span class="font-medium">{duration}h</span>
				</div>
				{#if event.attendees && event.attendees.length > 0}
					<div
						class="text-xs mt-1 truncate {!isClassified || needsReview ? 'text-gray-400' : ''}"
						style={needsVerifyTextColor ? `color: ${needsVerifyTextColor}; opacity: 0.6` : isClassified && !needsReview && textColors ? `color: ${textColors.textSubtle}` : ''}
					>
						{event.attendees.slice(0, 3).join(', ')}{event.attendees.length > 3 ? ` +${event.attendees.length - 3} more` : ''}
					</div>
				{/if}
			</div>
			{#if isClassified && !needsReview && event.project}
				<!-- Confirmed: small indicator dot (background already shows project color) -->
				<span
					class="w-3 h-3 rounded-full flex-shrink-0 border-2 {textColors?.isDark ? 'border-white/50' : 'border-black/20'}"
					style="background-color: {event.project.color}"
					title={formatConfidenceTitle(event.project.name, event.classification_confidence, event.classification_source)}
				></span>
			{/if}
		</div>

		<!-- Bottom row: project buttons (left) and skip button (right) -->
		{#if isPending || showReclassify}
			<div class="flex items-center justify-between pt-1">
				<div class="flex items-center gap-1.5">
					{#each activeProjects as project, i}
						{@const isBestGuess = event.suggested_project_id === project.id || (!event.suggested_project_id && i === 0 && isPending)}
						<button
							type="button"
							class="w-5 h-5 rounded-full transition-all {isBestGuess ? 'ring-2 ring-offset-1 ring-offset-white dark:ring-offset-zinc-900 ring-black/40 dark:ring-white/60' : 'hover:ring-2 hover:ring-offset-1 hover:ring-gray-400'}"
							style="background-color: {project.color}"
							title="{project.name}{isBestGuess ? ' (suggested)' : ''}"
							onclick={() => { onclassify?.(project.id); showReclassify = false; }}
						></button>
					{/each}
					{#if showReclassify}
						<button
							type="button"
							class="ml-2 text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
							onclick={() => showReclassify = false}
						>
							Cancel
						</button>
					{/if}
				</div>
				<button
					type="button"
					class="w-5 h-5 rounded border border-dashed border-gray-400 dark:border-gray-500 text-gray-400 hover:border-gray-600 hover:text-gray-600 dark:hover:border-gray-300 dark:hover:text-gray-300 flex items-center justify-center text-xs"
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
					class="w-5 h-5 rounded border border-dashed border-gray-400 dark:border-gray-500 text-gray-400 hover:border-gray-600 hover:text-gray-600 dark:hover:border-gray-300 dark:hover:text-gray-300 flex items-center justify-center text-xs"
					title="Skipped - click to reclassify"
					onclick={() => showReclassify = true}
				>
					✕
				</button>
			</div>
		{:else if isClassified && event.project}
			<!-- Classified - click project dot to reclassify -->
			<div class="flex justify-end pt-1">
				<button
					type="button"
					class="w-5 h-5 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow {textColors?.isDark ? 'ring-offset-current' : ''}"
					style="background-color: {event.project.color}"
					title="{event.project.name} - click to reclassify"
					onclick={() => showReclassify = true}
				></button>
			</div>
		{/if}
	</div>
</div>
