<script lang="ts">
	import { onMount } from 'svelte';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Modal } from '$lib/components/primitives';
	import RuleCard from '$lib/components/widgets/RuleCard.svelte';
	import { api } from '$lib/api/client';
	import type {
		ClassificationRule,
		Project,
		RuleCreate,
		RuleUpdate,
		RulePreviewResponse
	} from '$lib/api/types';

	// State
	let rules = $state<ClassificationRule[]>([]);
	let projects = $state<Project[]>([]);
	let loading = $state(true);
	let error = $state('');

	// Editor modal state
	let showEditor = $state(false);
	let editingRule = $state<ClassificationRule | null>(null);
	let editorQuery = $state('');
	let editorProjectId = $state<string | null>(null);
	let editorIsAttendance = $state(false);
	let editorIsPriority = $state(false);
	let editorError = $state('');
	let saving = $state(false);

	// Preview modal state
	let showPreview = $state(false);
	let previewLoading = $state(false);
	let previewResult = $state<RulePreviewResponse | null>(null);
	let previewQuery = $state('');
	let previewProjectId = $state<string | null>(null);

	// Delete confirmation
	let showDeleteConfirm = $state(false);
	let deletingRule = $state<ClassificationRule | null>(null);
	let deleting = $state(false);

	// Apply rules state
	let applying = $state(false);
	let applyResult = $state<{ classified: number; skipped: number } | null>(null);

	async function loadData() {
		loading = true;
		error = '';
		try {
			[rules, projects] = await Promise.all([api.listRules(true), api.listProjects()]);
		} catch (e) {
			console.error('Failed to load data:', e);
			error = e instanceof Error ? e.message : 'Failed to load rules';
		} finally {
			loading = false;
		}
	}

	function openNewRule() {
		editingRule = null;
		editorQuery = '';
		editorProjectId = null;
		editorIsAttendance = false;
		editorIsPriority = false;
		editorError = '';
		showEditor = true;
	}

	function openEditRule(rule: ClassificationRule) {
		editingRule = rule;
		editorQuery = rule.query;
		editorProjectId = rule.project_id || null;
		editorIsAttendance = rule.attended !== null && rule.attended !== undefined;
		editorIsPriority = rule.weight >= 2;
		editorError = '';
		showEditor = true;
	}

	async function handleSaveRule() {
		editorError = '';

		if (!editorQuery.trim()) {
			editorError = 'Query is required';
			return;
		}

		if (!editorIsAttendance && !editorProjectId) {
			editorError = 'Please select a project or choose "Did not attend"';
			return;
		}

		saving = true;

		try {
			if (editingRule) {
				// Update existing rule
				const update: RuleUpdate = {
					query: editorQuery,
					weight: editorIsPriority ? 2.0 : 1.0
				};

				if (editorIsAttendance) {
					update.project_id = null;
					update.attended = false; // Did not attend
				} else {
					update.project_id = editorProjectId;
					update.attended = null;
				}

				await api.updateRule(editingRule.id, update);
			} else {
				// Create new rule
				const create: RuleCreate = {
					query: editorQuery,
					weight: editorIsPriority ? 2.0 : 1.0
				};

				if (editorIsAttendance) {
					create.attended = false;
				} else {
					create.project_id = editorProjectId!;
				}

				await api.createRule(create);
			}

			showEditor = false;
			await loadData();
		} catch (e) {
			console.error('Failed to save rule:', e);
			editorError = e instanceof Error ? e.message : 'Failed to save rule';
		} finally {
			saving = false;
		}
	}

	async function handlePreviewBeforeSave() {
		editorError = '';

		if (!editorQuery.trim()) {
			editorError = 'Query is required';
			return;
		}

		previewQuery = editorQuery;
		previewProjectId = editorIsAttendance ? null : editorProjectId;
		showEditor = false;
		showPreview = true;
		previewLoading = true;

		try {
			previewResult = await api.previewRule({
				query: previewQuery,
				project_id: previewProjectId || undefined
			});
		} catch (e) {
			console.error('Failed to preview rule:', e);
			editorError = e instanceof Error ? e.message : 'Failed to preview rule';
			showPreview = false;
			showEditor = true;
		} finally {
			previewLoading = false;
		}
	}

	function backToEditor() {
		showPreview = false;
		showEditor = true;
	}

	async function saveFromPreview() {
		showPreview = false;
		showEditor = true;
		await handleSaveRule();
	}

	async function openPreviewForRule(rule: ClassificationRule) {
		previewQuery = rule.query;
		previewProjectId = rule.project_id || null;
		showPreview = true;
		previewLoading = true;

		try {
			previewResult = await api.previewRule({
				query: rule.query,
				project_id: rule.project_id || undefined
			});
		} catch (e) {
			console.error('Failed to preview rule:', e);
			showPreview = false;
		} finally {
			previewLoading = false;
		}
	}

	async function handleToggleRule(rule: ClassificationRule) {
		try {
			await api.updateRule(rule.id, { is_enabled: !rule.is_enabled });
			await loadData();
		} catch (e) {
			console.error('Failed to toggle rule:', e);
			error = e instanceof Error ? e.message : 'Failed to toggle rule';
		}
	}

	function confirmDeleteRule(rule: ClassificationRule) {
		deletingRule = rule;
		showDeleteConfirm = true;
	}

	async function handleDeleteRule() {
		if (!deletingRule) return;

		deleting = true;
		try {
			await api.deleteRule(deletingRule.id);
			showDeleteConfirm = false;
			deletingRule = null;
			await loadData();
		} catch (e) {
			console.error('Failed to delete rule:', e);
			error = e instanceof Error ? e.message : 'Failed to delete rule';
		} finally {
			deleting = false;
		}
	}

	async function handleApplyRules() {
		applying = true;
		applyResult = null;
		error = '';
		try {
			const result = await api.applyRules({});
			applyResult = {
				classified: result.classified.length,
				skipped: result.skipped
			};
			// Clear the result after 5 seconds
			setTimeout(() => {
				applyResult = null;
			}, 5000);
		} catch (e) {
			console.error('Failed to apply rules:', e);
			error = e instanceof Error ? e.message : 'Failed to apply rules';
		} finally {
			applying = false;
		}
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString([], {
			weekday: 'short',
			month: 'short',
			day: 'numeric'
		});
	}

	function formatTime(dateStr: string): string {
		return new Date(dateStr).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
	}

	onMount(() => {
		loadData();
	});
</script>

<svelte:head>
	<title>Rules - Timesheet</title>
</svelte:head>

<AppShell>
	<div class="max-w-3xl mx-auto">
		<div class="flex items-center justify-between mb-6">
			<h1 class="text-2xl font-bold text-gray-900">Rules</h1>
			<div class="flex items-center gap-3">
				{#if rules.length > 0}
					<Button variant="secondary" loading={applying} onclick={handleApplyRules}>
						Apply Rules
					</Button>
				{/if}
				<Button variant="primary" onclick={openNewRule}>+ New Rule</Button>
			</div>
		</div>

		{#if applyResult}
			<div class="mb-4 bg-green-50 border border-green-200 text-green-700 px-4 py-3 rounded text-sm">
				Applied rules: {applyResult.classified} events classified, {applyResult.skipped} skipped
			</div>
		{/if}

		{#if error}
			<div class="mb-4 bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded text-sm">
				{error}
			</div>
		{/if}

		{#if loading}
			<div class="flex items-center justify-center py-12">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		{:else if rules.length === 0}
			<!-- Empty state -->
			<div class="bg-white border rounded-lg p-8 text-center">
				<div class="text-gray-400 mb-4">
					<svg class="w-12 h-12 mx-auto" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M9 5H7a2 2 0 00-2 2v10a2 2 0 002 2h8a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
						/>
					</svg>
				</div>
				<h3 class="text-lg font-medium text-gray-900 mb-2">No classification rules yet</h3>
				<p class="text-gray-500 mb-6">
					Rules automatically classify events based on patterns like attendee domains, meeting
					titles, or time of day.
				</p>
				<Button variant="primary" onclick={openNewRule}>Create Your First Rule</Button>
			</div>
		{:else}
			<!-- Rules list -->
			<div class="space-y-3">
				{#each rules as rule (rule.id)}
					<RuleCard
						{rule}
						onedit={() => openEditRule(rule)}
						onpreview={() => openPreviewForRule(rule)}
						ontoggle={() => handleToggleRule(rule)}
						ondelete={() => confirmDeleteRule(rule)}
					/>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Rule Editor Modal -->
	<Modal bind:open={showEditor} title={editingRule ? 'Edit Rule' : 'New Rule'}>
		<div class="space-y-4">
			{#if editorError}
				<div class="bg-red-50 border border-red-200 text-red-700 px-3 py-2 rounded text-sm">
					{editorError}
				</div>
			{/if}

			<div>
				<label for="query" class="block text-sm font-medium text-gray-700 mb-1">Query</label>
				<input
					id="query"
					type="text"
					bind:value={editorQuery}
					placeholder="domain:acme.com title:sync"
					class="w-full px-3 py-2 border rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 font-mono text-sm"
				/>
				<p class="mt-1 text-xs text-gray-500">
					Examples: title:standup, domain:client.com, attendees:alice@, response:declined
				</p>
			</div>

			<div class="border-t pt-4">
				<div class="flex items-center gap-4 mb-3">
					<label class="flex items-center gap-2 cursor-pointer">
						<input
							type="radio"
							name="ruleType"
							checked={!editorIsAttendance}
							onchange={() => (editorIsAttendance = false)}
							class="h-4 w-4 text-primary-600 focus:ring-primary-500"
						/>
						<span class="text-sm text-gray-700">Assign to project</span>
					</label>
					<label class="flex items-center gap-2 cursor-pointer">
						<input
							type="radio"
							name="ruleType"
							checked={editorIsAttendance}
							onchange={() => (editorIsAttendance = true)}
							class="h-4 w-4 text-primary-600 focus:ring-primary-500"
						/>
						<span class="text-sm text-gray-700">Did not attend</span>
					</label>
				</div>

				{#if !editorIsAttendance}
					<div>
						<label for="project" class="block text-sm font-medium text-gray-700 mb-1"
							>Project</label
						>
						<select
							id="project"
							bind:value={editorProjectId}
							class="w-full px-3 py-2 border rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
						>
							<option value={null}>Select project...</option>
							{#each projects.filter((p) => !p.is_archived) as project}
								<option value={project.id}>
									{project.name}
								</option>
							{/each}
						</select>
					</div>
				{/if}
			</div>

			<div class="border-t pt-4">
				<label class="flex items-center gap-2 cursor-pointer">
					<input
						type="checkbox"
						bind:checked={editorIsPriority}
						class="h-4 w-4 rounded text-primary-600 focus:ring-primary-500"
					/>
					<span class="text-sm text-gray-700">Priority rule (counts twice in scoring)</span>
				</label>
			</div>
		</div>

		{#snippet footer()}
			<Button variant="secondary" onclick={() => (showEditor = false)}>Cancel</Button>
			<Button variant="primary" loading={saving} onclick={handlePreviewBeforeSave}>
				Preview & Save
			</Button>
		{/snippet}
	</Modal>

	<!-- Preview Modal -->
	<Modal bind:open={showPreview} title="Rule Preview">
		{#if previewLoading}
			<div class="flex items-center justify-center py-8">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		{:else if previewResult}
			<div class="space-y-4">
				<div class="bg-gray-50 rounded-lg p-3">
					<div class="text-sm text-gray-500 mb-1">Query</div>
					<code class="text-sm font-mono">{previewQuery}</code>
				</div>

				<!-- Stats -->
				<div class="grid grid-cols-3 gap-3 text-center">
					<div class="bg-blue-50 rounded-lg p-3">
						<div class="text-2xl font-bold text-blue-700">{previewResult.stats.total_matches}</div>
						<div class="text-xs text-blue-600">matches</div>
					</div>
					<div class="bg-green-50 rounded-lg p-3">
						<div class="text-2xl font-bold text-green-700">
							{previewResult.stats.already_correct}
						</div>
						<div class="text-xs text-green-600">already correct</div>
					</div>
					<div class="bg-yellow-50 rounded-lg p-3">
						<div class="text-2xl font-bold text-yellow-700">{previewResult.stats.would_change}</div>
						<div class="text-xs text-yellow-600">would change</div>
					</div>
				</div>

				{#if previewResult.stats.manual_conflicts > 0}
					<div class="bg-yellow-50 border border-yellow-200 rounded-lg p-3 text-sm text-yellow-800">
						<strong>{previewResult.stats.manual_conflicts}</strong> events have manual classifications
						that would conflict. Rules will NOT override manual classifications.
					</div>
				{/if}

				<!-- Matches (collapsed by default) -->
				{#if previewResult.matches.length > 0}
					<details>
						<summary class="cursor-pointer text-sm font-medium text-gray-700">
							Matching Events ({previewResult.matches.length})
						</summary>
						<div class="mt-2 space-y-1 max-h-48 overflow-y-auto">
							{#each previewResult.matches as match}
								<div class="text-sm py-1 px-2 bg-gray-50 rounded flex justify-between">
									<span class="truncate">{match.title}</span>
									<span class="text-gray-500 flex-shrink-0 ml-2"
										>{formatDate(match.start_time)}</span
									>
								</div>
							{/each}
						</div>
					</details>
				{/if}

				<!-- Conflicts -->
				{#if previewResult.conflicts.length > 0}
					<details open>
						<summary class="cursor-pointer text-sm font-medium text-yellow-700">
							Conflicts ({previewResult.conflicts.length})
						</summary>
						<div class="mt-2 space-y-1 max-h-32 overflow-y-auto">
							{#each previewResult.conflicts as conflict}
								<div
									class="text-sm py-2 px-2 bg-yellow-50 border border-yellow-100 rounded flex justify-between"
								>
									<span class="text-yellow-800">
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
			{#if editingRule === null && !previewLoading}
				<Button variant="secondary" onclick={backToEditor}>Back to Edit</Button>
				<Button variant="primary" onclick={saveFromPreview}>Save Rule</Button>
			{:else}
				<Button variant="secondary" onclick={() => (showPreview = false)}>Close</Button>
			{/if}
		{/snippet}
	</Modal>

	<!-- Delete Confirmation Modal -->
	<Modal bind:open={showDeleteConfirm} title="Delete Rule">
		<p class="text-gray-600">
			Are you sure you want to delete this rule? This action cannot be undone.
		</p>
		{#if deletingRule}
			<div class="mt-3 p-3 bg-gray-50 rounded">
				<code class="text-sm font-mono">{deletingRule.query}</code>
			</div>
		{/if}

		{#snippet footer()}
			<Button variant="secondary" onclick={() => (showDeleteConfirm = false)}>Cancel</Button>
			<Button variant="danger" loading={deleting} onclick={handleDeleteRule}>Delete</Button>
		{/snippet}
	</Modal>
</AppShell>
