<script lang="ts">
	import type { CalendarEvent, Project } from '$lib/api/types';
	import {
		getClassificationStyles,
		getPrimaryTextClasses,
		getPrimaryTextStyle,
		getSecondaryTextClasses,
		getSecondaryTextStyle,
		getTertiaryTextClasses,
		getTertiaryTextStyle,
		formatConfidenceTitle
	} from '$lib/styles';

	interface Props {
		event: CalendarEvent;
		projects: Project[];
		onclassify?: (projectId: string) => void;
		onskip?: () => void;
		onunclassify?: () => void;
		highlightedTarget?: string | null;
	}

	let { event, projects, onclassify, onskip, onunclassify, highlightedTarget = null }: Props = $props();

	let showReclassify = $state(false);

	// Determine if this event should be dimmed
	function shouldDimEvent(): boolean {
		if (!highlightedTarget) return false;
		if (highlightedTarget === 'skipped') return !event.is_skipped;
		if (highlightedTarget === 'hidden') return !event.project?.is_hidden_by_default;
		if (highlightedTarget === 'archived') return !event.project?.is_archived;
		return event.project_id !== highlightedTarget;
	}
	const isDimmed = $derived(shouldDimEvent());
	const activeProjects = $derived(projects.filter((p) => !p.is_archived));

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
		return Math.round(((endDate.getTime() - startDate.getTime()) / (1000 * 60 * 60)) * 100) / 100;
	}

	// Format date
	function formatDate(dateStr: string): string {
		const date = new Date(dateStr);
		return date.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' });
	}

	// Derived state
	const timeRange = $derived(formatTimeRange(event.start_time, event.end_time));
	const duration = $derived(getDuration(event.start_time, event.end_time));
	const dateStr = $derived(formatDate(event.start_time));
	const isPending = $derived(event.classification_status === 'pending');
	const isClassified = $derived(event.classification_status === 'classified');
	const isSkipped = $derived(event.is_skipped === true);
	const needsReview = $derived(event.needs_review === true);
	const projectColor = $derived(isSkipped ? null : (event.project?.color ?? null));

	// Get computed styles from the style system
	const styles = $derived(
		getClassificationStyles({
			status: event.classification_status as 'pending' | 'classified' | 'skipped',
			needsReview,
			isSkipped,
			projectColor
		})
	);
</script>

<div class="p-3 transition-all hover:shadow-sm {styles.containerClasses} {isDimmed ? 'opacity-25' : ''}" style={styles.containerStyle}>
	<div class="flex flex-col gap-1">
		<!-- Top row: title and project dot -->
		<div class="flex items-start justify-between gap-2">
			<div class="min-w-0 flex-1">
				<h4
					class="truncate font-medium {getPrimaryTextClasses(styles, isSkipped)}"
					style={getPrimaryTextStyle(styles, isSkipped)}
				>
					{event.title}
				</h4>
				<div
					class="mt-0.5 flex items-center gap-3 text-sm {getSecondaryTextClasses(styles, isSkipped)}"
					style={getSecondaryTextStyle(styles, isSkipped)}
				>
					<span>{dateStr}</span>
					<span>{timeRange}</span>
					<span class="font-medium">{duration}h</span>
				</div>
				{#if event.attendees && event.attendees.length > 0}
					<div
						class="mt-1 truncate text-xs {getTertiaryTextClasses(styles, isSkipped)}"
						style={getTertiaryTextStyle(styles, isSkipped)}
					>
						{event.attendees.slice(0, 3).join(', ')}{event.attendees.length > 3
							? ` +${event.attendees.length - 3} more`
							: ''}
					</div>
				{/if}
			</div>
			{#if isClassified && !needsReview && event.project}
				<!-- Confirmed: small indicator dot (background already shows project color) -->
				<span
					class="h-3 w-3 flex-shrink-0 rounded-full border-2 {styles.textColors?.isDark
						? 'border-white/50'
						: 'border-black/20'}"
					style="background-color: {event.project.color}"
					title={formatConfidenceTitle(
						event.project.name,
						event.classification_confidence,
						event.classification_source
					)}
				></span>
			{/if}
		</div>

		<!-- Bottom row: project buttons (left) and skip button (right) -->
		{#if isPending || showReclassify}
			<div class="flex items-center justify-between pt-1">
				<div class="flex items-center gap-1.5">
					{#each activeProjects as project, i}
						{@const isBestGuess =
							event.suggested_project_id === project.id ||
							(!event.suggested_project_id && i === 0 && isPending)}
						<button
							type="button"
							class="h-5 w-5 rounded-full transition-all {isBestGuess
								? 'ring-2 ring-black/40 ring-offset-1 ring-offset-white dark:ring-white/60 dark:ring-offset-zinc-900'
								: 'hover:ring-2 hover:ring-gray-400 hover:ring-offset-1'}"
							style="background-color: {project.color}"
							title="{project.name}{isBestGuess ? ' (suggested)' : ''}"
							onclick={() => {
								onclassify?.(project.id);
								showReclassify = false;
							}}
						></button>
					{/each}
					{#if showReclassify}
						<button
							type="button"
							class="ml-2 text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
							onclick={() => (showReclassify = false)}
						>
							Cancel
						</button>
					{/if}
				</div>
				<button
					type="button"
					class="flex h-5 w-5 items-center justify-center rounded border border-dashed border-gray-400 text-xs text-gray-400 hover:border-gray-600 hover:text-gray-600 dark:border-gray-500 dark:hover:border-gray-300 dark:hover:text-gray-300"
					title="Skip - did not attend"
					onclick={() => {
						onskip?.();
						showReclassify = false;
					}}
				>
					✕
				</button>
			</div>
		{:else if isSkipped}
			<!-- Skip indicator in bottom right -->
			<div class="flex justify-end pt-1">
				<button
					type="button"
					class="flex h-5 w-5 items-center justify-center rounded border border-dashed border-gray-400 text-xs text-gray-400 hover:border-gray-600 hover:text-gray-600 dark:border-gray-500 dark:hover:border-gray-300 dark:hover:text-gray-300"
					title="Skipped - click to reclassify"
					onclick={() => (showReclassify = true)}
				>
					✕
				</button>
			</div>
		{:else if isClassified && event.project}
			<!-- Classified - click project dot to reclassify -->
			<div class="flex justify-end pt-1">
				<button
					type="button"
					class="h-5 w-5 rounded-full transition-shadow hover:ring-2 hover:ring-offset-1 {styles
						.textColors?.isDark
						? 'ring-offset-current'
						: ''} ring-gray-400"
					style="background-color: {event.project.color}"
					title="{event.project.name} - click to reclassify"
					onclick={() => (showReclassify = true)}
				></button>
			</div>
		{/if}
	</div>
</div>
