<script lang="ts">
	import type { CalendarEvent, Project } from '$lib/api/types';
	import {
		getClassificationStyles,
		getPrimaryTextClasses,
		getPrimaryTextStyle,
		getSecondaryTextClasses,
		getSecondaryTextStyle,
		formatConfidenceTitle
	} from '$lib/styles';

	type Variant = 'chip' | 'card' | 'compact';

	interface Props {
		event: CalendarEvent;
		projects: Project[];
		variant?: Variant;
		showTime?: boolean;
		maxProjectButtons?: number;
		onclassify?: (projectId: string) => void;
		onskip?: () => void;
		onhover?: (element: HTMLElement | null) => void;
	}

	let {
		event,
		projects,
		variant = 'card',
		showTime = false,
		maxProjectButtons = 4,
		onclassify,
		onskip,
		onhover
	}: Props = $props();

	const activeProjects = $derived(projects.filter((p) => !p.is_archived));

	// Classification state
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

	// Format time range
	function formatTimeRange(): string {
		const start = new Date(event.start_time);
		const end = new Date(event.end_time);
		const opts: Intl.DateTimeFormatOptions = { hour: 'numeric', minute: '2-digit' };
		return `${start.toLocaleTimeString([], opts)} - ${end.toLocaleTimeString([], opts)}`;
	}

	// Variant-specific classes
	const variantClasses = {
		chip: 'inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full',
		card: 'p-1.5 text-xs rounded',
		compact: 'p-1.5 text-xs rounded'
	};

	const projectButtonSize = {
		chip: 'w-2.5 h-2.5',
		card: 'w-3 h-3',
		compact: 'w-3 h-3'
	};

	const skipButtonSize = {
		chip: 'w-2.5 h-2.5 text-[5px]',
		card: 'w-3 h-3 text-[6px]',
		compact: 'w-3.5 h-3.5 text-[7px]'
	};
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="cursor-pointer transition-shadow hover:shadow-sm {variantClasses[variant]} {styles.containerClasses}"
	style={styles.containerStyle}
	onmouseenter={(e) => onhover?.(e.currentTarget as HTMLElement)}
	onmouseleave={() => onhover?.(null)}
>
	{#if variant === 'chip'}
		<!-- Chip variant: single line with inline actions -->
		<span
			class="max-w-[80px] truncate {getPrimaryTextClasses(styles, isSkipped)}"
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
				class="flex h-2.5 w-2.5 items-center justify-center rounded border border-dashed border-gray-400 text-[5px] text-gray-400"
				>✕</span
			>
		{:else if isPending}
			<div class="ml-1 flex items-center gap-0.5">
				{#each activeProjects.slice(0, 3) as project, i}
					{@const isBestGuess =
						event.suggested_project_id === project.id || (!event.suggested_project_id && i === 0)}
					<button
						type="button"
						class="h-2.5 w-2.5 rounded-full transition-shadow {isBestGuess
							? 'ring-1 ring-black/40 ring-offset-1 ring-offset-white dark:ring-white/60 dark:ring-offset-zinc-900'
							: 'ring-gray-400 hover:ring-1 hover:ring-offset-1'}"
						style="background-color: {project.color}"
						title="{project.name}{isBestGuess ? ' (suggested)' : ''}"
						onclick={(e) => {
							e.stopPropagation();
							onclassify?.(project.id);
						}}
					></button>
				{/each}
			</div>
			<button
				type="button"
				class="ml-1 flex h-2.5 w-2.5 items-center justify-center rounded border border-dashed border-gray-400 text-[5px] text-gray-400 hover:border-gray-600"
				title="Skip - did not attend"
				onclick={(e) => {
					e.stopPropagation();
					onskip?.();
				}}
			>
				✕
			</button>
		{/if}
	{:else}
		<!-- Card/Compact variant: multi-line layout -->
		<div class="flex flex-col gap-1">
			<!-- Top row: title and project dot -->
			<div class="flex items-start justify-between gap-1">
				<div class="min-w-0 flex-1">
					<div
						class="truncate font-medium {getPrimaryTextClasses(styles, isSkipped)}"
						style={getPrimaryTextStyle(styles, isSkipped)}
					>
						{event.title}
					</div>
					{#if showTime}
						<div
							class="mt-0.5 {getSecondaryTextClasses(styles, isSkipped)}"
							style={getSecondaryTextStyle(styles, isSkipped)}
						>
							{formatTimeRange()}
						</div>
					{/if}
				</div>
				{#if isClassified && !needsReview && event.project}
					<span
						class="{projectButtonSize[variant]} mt-0.5 flex-shrink-0 rounded-full {styles.textColors
							?.isDark
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

			<!-- Bottom row: classification actions -->
			{#if isPending}
				<div class="flex items-center justify-between pt-0.5">
					<div class="flex items-center gap-0.5">
						{#each activeProjects.slice(0, maxProjectButtons) as project, i}
							{@const isBestGuess =
								event.suggested_project_id === project.id ||
								(!event.suggested_project_id && i === 0)}
							<button
								type="button"
								class="{projectButtonSize[variant]} rounded-full transition-shadow {isBestGuess
									? 'ring-1 ring-black/40 ring-offset-1 ring-offset-white dark:ring-white/60 dark:ring-offset-zinc-900'
									: 'ring-gray-400 hover:ring-1 hover:ring-offset-1'}"
								style="background-color: {project.color}"
								title="{project.name}{isBestGuess ? ' (suggested)' : ''}"
								onclick={(e) => {
									e.stopPropagation();
									onclassify?.(project.id);
								}}
							></button>
						{/each}
					</div>
					<button
						type="button"
						class="{skipButtonSize[variant]} flex items-center justify-center rounded border border-dashed border-gray-400 text-gray-400 hover:border-gray-600"
						title="Skip - did not attend"
						onclick={(e) => {
							e.stopPropagation();
							onskip?.();
						}}
					>
						✕
					</button>
				</div>
			{:else if isSkipped}
				<div class="flex justify-end">
					<span
						class="{skipButtonSize[variant]} flex items-center justify-center rounded border border-dashed border-gray-400 text-gray-400"
						>✕</span
					>
				</div>
			{/if}
		</div>
	{/if}
</div>
