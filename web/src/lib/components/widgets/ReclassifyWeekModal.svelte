<script lang="ts">
	import { Modal, Button } from '$lib/components/primitives';
	import type { CalendarEvent, ClassifiedEvent, Project } from '$lib/api/types';

	interface Props {
		open: boolean;
		weekStart: Date;
		weekEnd: Date;
		events: CalendarEvent[];
		projects: Project[];
		loading?: boolean;
		previewResults?: ClassifiedEvent[] | null;
		onclose?: () => void;
		onpreview?: () => void;
		onconfirm?: () => void;
	}

	let {
		open = $bindable(false),
		weekStart,
		weekEnd,
		events,
		projects,
		loading = false,
		previewResults = null,
		onclose,
		onpreview,
		onconfirm
	}: Props = $props();

	// Compute counts by classification source
	const counts = $derived.by(() => {
		const pending = events.filter(e => e.classification_status === 'pending' && !e.is_skipped);
		const rule = events.filter(e => e.classification_source === 'rule' && !e.is_skipped);
		const fingerprint = events.filter(e => e.classification_source === 'fingerprint' && !e.is_skipped);
		const manual = events.filter(e => e.classification_source === 'manual' && !e.is_skipped);
		const skipped = events.filter(e => e.is_skipped);

		return {
			pending: pending.length,
			rule: rule.length,
			fingerprint: fingerprint.length,
			manual: manual.length,
			skipped: skipped.length,
			autoClassified: rule.length + fingerprint.length,
			total: events.length
		};
	});

	// Build a project lookup map
	const projectMap = $derived(
		new Map(projects.map(p => [p.id, p]))
	);

	// Parse preview results to show what would change
	const previewChanges = $derived.by(() => {
		if (!previewResults) return [];

		return previewResults
			.map(result => {
				const event = events.find(e => e.id === result.event_id);
				if (!event) return null;

				const newProject = projectMap.get(result.project_id);
				const oldProject = event.project_id ? projectMap.get(event.project_id) : null;

				// Only show if it's actually changing
				const isChange = event.classification_status === 'pending' ||
					event.project_id !== result.project_id;

				if (!isChange) return null;

				return {
					eventId: result.event_id,
					title: event.title,
					oldProject: oldProject?.name ?? 'Unclassified',
					oldColor: oldProject?.color ?? null,
					newProject: newProject?.name ?? 'Unknown',
					newColor: newProject?.color ?? '#888',
					confidence: result.confidence,
					needsReview: result.needs_review
				};
			})
			.filter((c): c is NonNullable<typeof c> => c !== null);
	});

	function formatDateRange(start: Date, end: Date): string {
		const startStr = start.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
		const endStr = end.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
		return `${startStr} - ${endStr}`;
	}
</script>

<Modal bind:open title="Reclassify Week of {formatDateRange(weekStart, weekEnd)}" {onclose}>
	<div class="space-y-4">
		<!-- Event counts by source -->
		<div class="space-y-2">
			<h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">Events in this week:</h4>
			<div class="grid grid-cols-2 gap-2 text-sm">
				<div class="flex items-center justify-between rounded bg-gray-50 px-3 py-2 dark:bg-zinc-800">
					<span class="text-gray-600 dark:text-gray-400">Pending</span>
					<span class="font-medium text-gray-900 dark:text-white">{counts.pending}</span>
				</div>
				<div class="flex items-center justify-between rounded bg-gray-50 px-3 py-2 dark:bg-zinc-800">
					<span class="flex items-center gap-1 text-gray-600 dark:text-gray-400">
						<span class="inline-flex h-4 w-4 items-center justify-center rounded bg-gray-200 text-[9px] font-medium dark:bg-zinc-600">R</span>
						Rule
					</span>
					<span class="font-medium text-gray-900 dark:text-white">{counts.rule}</span>
				</div>
				<div class="flex items-center justify-between rounded bg-gray-50 px-3 py-2 dark:bg-zinc-800">
					<span class="flex items-center gap-1 text-gray-600 dark:text-gray-400">
						<span class="inline-flex h-4 w-4 items-center justify-center rounded bg-gray-200 text-[9px] font-medium dark:bg-zinc-600">F</span>
						Fingerprint
					</span>
					<span class="font-medium text-gray-900 dark:text-white">{counts.fingerprint}</span>
				</div>
				<div class="flex items-center justify-between rounded bg-gray-50 px-3 py-2 dark:bg-zinc-800">
					<span class="flex items-center gap-1 text-gray-600 dark:text-gray-400">
						<span class="text-sm">&#x1F512;</span>
						Manual
					</span>
					<span class="font-medium text-gray-500 dark:text-gray-400">{counts.manual}</span>
				</div>
			</div>
		</div>

		<!-- Info about what will be reclassified -->
		<div class="rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 text-sm dark:border-blue-800 dark:bg-blue-900/30">
			<p class="text-blue-800 dark:text-blue-200">
				<strong>{counts.pending + counts.autoClassified}</strong> events will be evaluated
				({counts.pending} pending + {counts.autoClassified} auto-classified).
			</p>
			{#if counts.manual > 0}
				<p class="mt-1 text-blue-600 dark:text-blue-300">
					{counts.manual} manually classified events will be preserved.
				</p>
			{/if}
		</div>

		<!-- Preview button or results -->
		{#if !previewResults && !loading}
			<div class="flex justify-center">
				<Button variant="secondary" onclick={onpreview}>
					Preview Changes
				</Button>
			</div>
		{:else if loading}
			<div class="flex items-center justify-center gap-2 py-4 text-gray-500">
				<svg class="h-5 w-5 animate-spin" fill="none" viewBox="0 0 24 24">
					<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
					<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
				</svg>
				<span>Analyzing events...</span>
			</div>
		{:else if previewResults}
			<div class="space-y-2">
				<h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
					Preview: {previewChanges.length} event{previewChanges.length === 1 ? '' : 's'} will change
				</h4>

				{#if previewChanges.length === 0}
					<p class="text-sm text-gray-500 dark:text-gray-400">
						No events will be reclassified. All events are already classified optimally.
					</p>
				{:else}
					<div class="max-h-48 space-y-1 overflow-y-auto rounded border border-gray-200 p-2 dark:border-zinc-700">
						{#each previewChanges.slice(0, 10) as change}
							<div class="flex items-center gap-2 text-sm">
								<span class="max-w-[120px] truncate text-gray-700 dark:text-gray-300" title={change.title}>
									{change.title}
								</span>
								<span class="text-gray-400">:</span>
								<span class="flex items-center gap-1">
									{#if change.oldColor}
										<span class="h-2.5 w-2.5 rounded-full" style="background-color: {change.oldColor}"></span>
									{/if}
									<span class="text-gray-500 dark:text-gray-400">{change.oldProject}</span>
								</span>
								<span class="text-gray-400">&rarr;</span>
								<span class="flex items-center gap-1">
									<span class="h-2.5 w-2.5 rounded-full" style="background-color: {change.newColor}"></span>
									<span class="font-medium text-gray-900 dark:text-white">{change.newProject}</span>
								</span>
								{#if change.needsReview}
									<span class="text-xs text-amber-600" title="Needs review">?</span>
								{/if}
							</div>
						{/each}
						{#if previewChanges.length > 10}
							<p class="text-xs text-gray-500 dark:text-gray-400">
								... and {previewChanges.length - 10} more
							</p>
						{/if}
					</div>
				{/if}
			</div>
		{/if}
	</div>

	{#snippet footer()}
		<Button variant="ghost" onclick={onclose} disabled={loading}>
			Cancel
		</Button>
		<Button
			variant="primary"
			onclick={onconfirm}
			disabled={loading || (previewResults && previewChanges.length === 0)}
		>
			{#if loading}
				Reclassifying...
			{:else}
				Reclassify {counts.pending + counts.autoClassified} Events
			{/if}
		</Button>
	{/snippet}
</Modal>
