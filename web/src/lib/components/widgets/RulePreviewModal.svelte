<script lang="ts">
	import { Button, Modal } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import type { RulePreviewResponse } from '$lib/api/types';

	interface Props {
		open: boolean;
		query: string;
		projectId: string | null;
		onback?: () => void;
		onsave?: () => void;
	}

	let {
		open = $bindable(false),
		query,
		projectId,
		onback,
		onsave
	}: Props = $props();

	let loading = $state(false);
	let result = $state<RulePreviewResponse | null>(null);

	// Load preview when modal opens
	$effect(() => {
		if (open && query) {
			loadPreview();
		}
	});

	async function loadPreview() {
		loading = true;
		try {
			result = await api.previewRule({
				query,
				project_id: projectId || undefined
			});
		} catch (e) {
			console.error('Failed to preview rule:', e);
			open = false;
			onback?.();
		} finally {
			loading = false;
		}
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString([], {
			weekday: 'short',
			month: 'short',
			day: 'numeric'
		});
	}

	function handleBack() {
		open = false;
		onback?.();
	}

	function handleSave() {
		open = false;
		onsave?.();
	}
</script>

<Modal bind:open title="Rule Preview">
	{#if loading}
		<div class="flex items-center justify-center py-8">
			<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
		</div>
	{:else if result}
		<div class="space-y-4">
			<div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
				<div class="text-sm text-gray-500 dark:text-gray-400 mb-1">Query</div>
				<code class="text-sm font-mono text-gray-900 dark:text-white">{query}</code>
			</div>

			<!-- Stats -->
			<div class="grid grid-cols-3 gap-3 text-center">
				<div class="bg-blue-50 dark:bg-blue-900/30 rounded-lg p-3">
					<div class="text-2xl font-bold text-blue-700 dark:text-blue-300">{result.stats.total_matches}</div>
					<div class="text-xs text-blue-600 dark:text-blue-400">matches</div>
				</div>
				<div class="bg-green-50 dark:bg-green-900/30 rounded-lg p-3">
					<div class="text-2xl font-bold text-green-700 dark:text-green-300">
						{result.stats.already_correct}
					</div>
					<div class="text-xs text-green-600 dark:text-green-400">already correct</div>
				</div>
				<div class="bg-yellow-50 dark:bg-yellow-900/30 rounded-lg p-3">
					<div class="text-2xl font-bold text-yellow-700 dark:text-yellow-300">{result.stats.would_change}</div>
					<div class="text-xs text-yellow-600 dark:text-yellow-400">would change</div>
				</div>
			</div>

			{#if result.stats.manual_conflicts > 0}
				<div class="bg-yellow-50 dark:bg-yellow-900/30 border border-yellow-200 dark:border-yellow-800 rounded-lg p-3 text-sm text-yellow-800 dark:text-yellow-200">
					<strong>{result.stats.manual_conflicts}</strong> events have manual classifications
					that would conflict. Rules will NOT override manual classifications.
				</div>
			{/if}

			<!-- Matches (collapsed by default) -->
			{#if result.matches.length > 0}
				<details>
					<summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-300">
						Matching Events ({result.matches.length})
					</summary>
					<div class="mt-2 space-y-1 max-h-48 overflow-y-auto">
						{#each result.matches as match}
							<div class="text-sm py-1 px-2 bg-gray-50 dark:bg-gray-700/50 rounded flex justify-between">
								<span class="truncate text-gray-900 dark:text-white">{match.title}</span>
								<span class="text-gray-500 dark:text-gray-400 flex-shrink-0 ml-2">{formatDate(match.start_time)}</span>
							</div>
						{/each}
					</div>
				</details>
			{/if}

			<!-- Conflicts -->
			{#if result.conflicts.length > 0}
				<details open>
					<summary class="cursor-pointer text-sm font-medium text-yellow-700 dark:text-yellow-300">
						Conflicts ({result.conflicts.length})
					</summary>
					<div class="mt-2 space-y-1 max-h-32 overflow-y-auto">
						{#each result.conflicts as conflict}
							<div class="text-sm py-2 px-2 bg-yellow-50 dark:bg-yellow-900/30 border border-yellow-100 dark:border-yellow-800 rounded flex justify-between">
								<span class="text-yellow-800 dark:text-yellow-200">
									Currently: {conflict.current_source || 'unknown'}
								</span>
							</div>
						{/each}
					</div>
				</details>
			{/if}
		</div>
	{/if}

	{#snippet footer()}
		{#if !loading}
			<Button variant="secondary" onclick={handleBack}>Back to Edit</Button>
			<Button variant="primary" onclick={handleSave}>Save Rule</Button>
		{:else}
			<Button variant="secondary" onclick={() => (open = false)}>Close</Button>
		{/if}
	{/snippet}
</Modal>
