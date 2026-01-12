<script lang="ts">
	import { Modal, Button } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import type { Project, ClassificationExplanation } from '$lib/api/types';

	interface Props {
		open: boolean;
		eventId: string | null;
		projects: Project[];
		onclose?: () => void;
	}

	let { open = $bindable(false), eventId, projects, onclose }: Props = $props();

	let loading = $state(false);
	let error = $state<string | null>(null);
	let explanation = $state<ClassificationExplanation | null>(null);

	// Collapsible section states
	let showScores = $state(true);
	let showMatchedRules = $state(true);
	let showUnmatchedRules = $state(false);
	let showSkipRules = $state(true);

	// Build project lookup map
	const projectMap = $derived(new Map(projects.map((p) => [p.id, p])));

	// Fetch explanation when modal opens
	$effect(() => {
		if (open && eventId) {
			fetchExplanation(eventId);
		} else if (!open) {
			// Reset state when modal closes
			explanation = null;
			error = null;
			loading = false;
		}
	});

	async function fetchExplanation(id: string) {
		loading = true;
		error = null;
		try {
			explanation = await api.explainEventClassification(id);
		} catch (e) {
			console.error('Failed to fetch explanation:', e);
			error = e instanceof Error ? e.message : 'Failed to load classification explanation';
		} finally {
			loading = false;
		}
	}

	// Separate matched and unmatched rules
	const matchedRules = $derived(
		(explanation?.rule_evaluations ?? []).filter((r) => r.matched)
	);
	const unmatchedRules = $derived(
		(explanation?.rule_evaluations ?? []).filter((r) => !r.matched)
	);

	// Separate matched and unmatched skip rules
	const matchedSkipRules = $derived(
		(explanation?.skip_evaluations ?? []).filter((r) => r.matched)
	);
	const unmatchedSkipRules = $derived(
		(explanation?.skip_evaluations ?? []).filter((r) => !r.matched)
	);

	// Get winner project
	const winnerProject = $derived(
		explanation?.winner_project_id ? projectMap.get(explanation.winner_project_id) : null
	);

	// Sort target scores by total weight descending
	const sortedScores = $derived(
		[...(explanation?.target_scores ?? [])].sort((a, b) => b.total_weight - a.total_weight)
	);

	// Format percentage
	function formatPercent(value: number | undefined): string {
		if (value === undefined) return '-';
		return `${Math.round(value * 100)}%`;
	}

	// Format time range
	function formatTimeRange(start: string, end: string): string {
		const startDate = new Date(start);
		const endDate = new Date(end);
		const options: Intl.DateTimeFormatOptions = { hour: 'numeric', minute: '2-digit' };
		return `${startDate.toLocaleTimeString([], options)} - ${endDate.toLocaleTimeString([], options)}`;
	}

	// Format date
	function formatDate(dateStr: string): string {
		const date = new Date(dateStr);
		return date.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
	}

	function handleClose() {
		open = false;
		onclose?.();
	}
</script>

<Modal bind:open title="Classification Explanation" onclose={handleClose}>
	{#if loading}
		<div class="flex items-center justify-center gap-2 py-8 text-gray-500">
			<svg class="h-5 w-5 animate-spin" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"
				></circle>
				<path
					class="opacity-75"
					fill="currentColor"
					d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
				></path>
			</svg>
			<span>Analyzing classification...</span>
		</div>
	{:else if error}
		<div class="space-y-4">
			<div
				class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/30 dark:text-red-200"
			>
				{error}
			</div>
			<div class="flex justify-center">
				<Button variant="secondary" onclick={() => eventId && fetchExplanation(eventId)}>
					Retry
				</Button>
			</div>
		</div>
	{:else if explanation}
		<div class="space-y-4 max-h-[60vh] overflow-y-auto">
			<!-- Event Header -->
			<div class="rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-zinc-700 dark:bg-zinc-800">
				<h4 class="font-medium text-gray-900 dark:text-white line-clamp-2">
					{explanation.event.title}
				</h4>
				<p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
					{formatDate(explanation.event.start_time)} &middot;
					{formatTimeRange(explanation.event.start_time, explanation.event.end_time)}
				</p>
			</div>

			<!-- Outcome Summary -->
			<div
				class="rounded-lg px-4 py-3 {explanation.would_be_skipped
					? 'border border-amber-200 bg-amber-50 dark:border-amber-800 dark:bg-amber-900/30'
					: winnerProject
						? 'border border-green-200 bg-green-50 dark:border-green-800 dark:bg-green-900/30'
						: 'border border-gray-200 bg-gray-50 dark:border-zinc-700 dark:bg-zinc-800'}"
			>
				<p
					class="text-sm font-medium {explanation.would_be_skipped
						? 'text-amber-800 dark:text-amber-200'
						: winnerProject
							? 'text-green-800 dark:text-green-200'
							: 'text-gray-700 dark:text-gray-300'}"
				>
					{explanation.outcome}
				</p>
				{#if winnerProject && explanation.winner_confidence !== undefined}
					<div class="mt-2 flex items-center gap-2">
						<span
							class="h-3 w-3 rounded-full"
							style="background-color: {winnerProject.color}"
						></span>
						<span class="text-sm text-gray-700 dark:text-gray-300">{winnerProject.name}</span>
						<span class="text-xs text-gray-500 dark:text-gray-400">
							({formatPercent(explanation.winner_confidence)} confidence)
						</span>
					</div>
				{/if}
				{#if explanation.would_be_skipped && explanation.skip_confidence !== undefined}
					<p class="mt-1 text-xs text-amber-600 dark:text-amber-400">
						Skip confidence: {formatPercent(explanation.skip_confidence)}
					</p>
				{/if}
			</div>

			<!-- Skip Rules Section -->
			{#if (explanation.skip_evaluations ?? []).length > 0}
				<div class="space-y-2">
					<button
						type="button"
						class="flex w-full items-center justify-between text-left"
						onclick={() => (showSkipRules = !showSkipRules)}
					>
						<h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
							Skip Rules ({matchedSkipRules.length} matched, {unmatchedSkipRules.length} unmatched)
						</h4>
						<svg
							class="h-4 w-4 text-gray-500 transition-transform {showSkipRules ? 'rotate-180' : ''}"
							fill="none"
							viewBox="0 0 24 24"
							stroke="currentColor"
						>
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
						</svg>
					</button>
					{#if showSkipRules}
						<div class="space-y-1">
							{#each matchedSkipRules as rule}
								<div
									class="flex items-center gap-2 rounded border-l-2 border-amber-500 bg-amber-50 px-3 py-1.5 text-sm dark:bg-amber-900/20"
								>
									<svg class="h-4 w-4 flex-shrink-0 text-amber-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
									</svg>
									<code class="flex-1 truncate text-xs text-gray-700 dark:text-gray-300">{rule.query}</code>
									{#if rule.weight}
										<span class="text-xs text-gray-500">w:{rule.weight}</span>
									{/if}
								</div>
							{/each}
							{#each unmatchedSkipRules as rule}
								<div
									class="flex items-center gap-2 rounded border-l-2 border-gray-300 bg-gray-50 px-3 py-1.5 text-sm opacity-60 dark:border-zinc-600 dark:bg-zinc-800"
								>
									<svg class="h-4 w-4 flex-shrink-0 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
									</svg>
									<code class="flex-1 truncate text-xs text-gray-500 dark:text-gray-400">{rule.query}</code>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/if}

			<!-- Target Scores Table -->
			{#if sortedScores.length > 0}
				<div class="space-y-2">
					<button
						type="button"
						class="flex w-full items-center justify-between text-left"
						onclick={() => (showScores = !showScores)}
					>
						<h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
							Scores by Project
							{#if explanation.total_weight}
								<span class="font-normal text-gray-500">(total weight: {explanation.total_weight})</span>
							{/if}
						</h4>
						<svg
							class="h-4 w-4 text-gray-500 transition-transform {showScores ? 'rotate-180' : ''}"
							fill="none"
							viewBox="0 0 24 24"
							stroke="currentColor"
						>
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
						</svg>
					</button>
					{#if showScores}
						<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-zinc-700">
							<table class="w-full text-sm">
								<thead class="bg-gray-50 dark:bg-zinc-800">
									<tr>
										<th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-400">Project</th>
										<th class="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-400">Rules</th>
										<th class="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-400">Fingerprints</th>
										<th class="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-400">Total</th>
									</tr>
								</thead>
								<tbody class="divide-y divide-gray-200 dark:divide-zinc-700">
									{#each sortedScores as score}
										{@const project = projectMap.get(score.target_id)}
										<tr class="{score.is_winner ? 'bg-green-50 dark:bg-green-900/20' : ''}">
											<td class="px-3 py-2">
												<div class="flex items-center gap-2">
													{#if project}
														<span
															class="h-2.5 w-2.5 rounded-full"
															style="background-color: {project.color}"
														></span>
													{/if}
													<span class="text-gray-900 dark:text-white {score.is_winner ? 'font-medium' : ''}">
														{score.target_name ?? project?.name ?? 'Unknown'}
													</span>
													{#if score.is_winner}
														<span class="text-xs text-green-600 dark:text-green-400">Winner</span>
													{/if}
												</div>
											</td>
											<td class="px-3 py-2 text-right text-gray-600 dark:text-gray-400">
												{score.rule_weight ?? 0}
											</td>
											<td class="px-3 py-2 text-right text-gray-600 dark:text-gray-400">
												{score.fingerprint_weight ?? 0}
											</td>
											<td class="px-3 py-2 text-right font-medium text-gray-900 dark:text-white">
												{score.total_weight}
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/if}
				</div>
			{/if}

			<!-- Matched Rules Section -->
			{#if matchedRules.length > 0}
				<div class="space-y-2">
					<button
						type="button"
						class="flex w-full items-center justify-between text-left"
						onclick={() => (showMatchedRules = !showMatchedRules)}
					>
						<h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
							Matching Rules ({matchedRules.length})
						</h4>
						<svg
							class="h-4 w-4 text-gray-500 transition-transform {showMatchedRules ? 'rotate-180' : ''}"
							fill="none"
							viewBox="0 0 24 24"
							stroke="currentColor"
						>
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
						</svg>
					</button>
					{#if showMatchedRules}
						<div class="space-y-1">
							{#each matchedRules as rule}
								{@const project = rule.target_id ? projectMap.get(rule.target_id) : null}
								<div
									class="flex items-center gap-2 rounded border-l-2 border-green-500 bg-green-50 px-3 py-1.5 text-sm dark:bg-green-900/20"
								>
									<svg class="h-4 w-4 flex-shrink-0 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
									</svg>
									<code class="flex-1 truncate text-xs text-gray-700 dark:text-gray-300">{rule.query}</code>
									{#if project}
										<span
											class="h-2.5 w-2.5 rounded-full flex-shrink-0"
											style="background-color: {project.color}"
										></span>
									{/if}
									{#if rule.weight}
										<span class="text-xs text-gray-500">w:{rule.weight}</span>
									{/if}
									<span
										class="inline-flex h-4 w-4 items-center justify-center rounded bg-gray-200 text-[9px] font-medium dark:bg-zinc-600"
										title={rule.source === 'fingerprint' ? 'Fingerprint' : 'Rule'}
									>
										{rule.source === 'fingerprint' ? 'F' : 'R'}
									</span>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			{/if}

			<!-- Unmatched Rules Section -->
			{#if unmatchedRules.length > 0}
				<div class="space-y-2">
					<button
						type="button"
						class="flex w-full items-center justify-between text-left"
						onclick={() => (showUnmatchedRules = !showUnmatchedRules)}
					>
						<h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
							Non-Matching Rules ({unmatchedRules.length})
						</h4>
						<svg
							class="h-4 w-4 text-gray-500 transition-transform {showUnmatchedRules ? 'rotate-180' : ''}"
							fill="none"
							viewBox="0 0 24 24"
							stroke="currentColor"
						>
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
						</svg>
					</button>
					{#if showUnmatchedRules}
						<div class="space-y-1 max-h-48 overflow-y-auto">
							{#each unmatchedRules.slice(0, 20) as rule}
								{@const project = rule.target_id ? projectMap.get(rule.target_id) : null}
								<div
									class="flex items-center gap-2 rounded border-l-2 border-gray-300 bg-gray-50 px-3 py-1.5 text-sm opacity-60 dark:border-zinc-600 dark:bg-zinc-800"
								>
									<svg class="h-4 w-4 flex-shrink-0 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
									</svg>
									<code class="flex-1 truncate text-xs text-gray-500 dark:text-gray-400">{rule.query}</code>
									{#if project}
										<span
											class="h-2.5 w-2.5 rounded-full flex-shrink-0 opacity-50"
											style="background-color: {project.color}"
										></span>
									{/if}
								</div>
							{/each}
							{#if unmatchedRules.length > 20}
								<p class="px-3 py-1 text-xs text-gray-500 dark:text-gray-400">
									... and {unmatchedRules.length - 20} more
								</p>
							{/if}
						</div>
					{/if}
				</div>
			{/if}

			<!-- No rules message -->
			{#if matchedRules.length === 0 && unmatchedRules.length === 0 && (explanation.skip_evaluations ?? []).length === 0}
				<div class="rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-600 dark:border-zinc-700 dark:bg-zinc-800 dark:text-gray-400">
					No classification rules have been defined yet.
				</div>
			{/if}
		</div>
	{/if}

	{#snippet footer()}
		<Button variant="ghost" onclick={handleClose}>Close</Button>
	{/snippet}
</Modal>
