<script lang="ts">
	import { onMount } from 'svelte';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Modal } from '$lib/components/primitives';
	import {
		RuleCard,
		EventSearchSection,
		RuleEditorModal,
		RulePreviewModal
	} from '$lib/components/widgets';
	import { api } from '$lib/api/client';
	import type { ClassificationRule, Project } from '$lib/api/types';

	// State
	let rules = $state<ClassificationRule[]>([]);
	let projects = $state<Project[]>([]);
	let loading = $state(true);
	let error = $state('');
	let successMessage = $state('');

	// Editor modal state
	let showEditor = $state(false);
	let editingRuleId = $state<string | null>(null);
	const editingRule = $derived(rules.find((r) => r.id === editingRuleId) ?? null);
	let editorInitialQuery = $state('');
	let editorInitialProjectId = $state<string | null>(null);

	// Preview modal state
	let showPreview = $state(false);
	let previewQuery = $state('');
	let previewProjectId = $state<string | null>(null);
	let pendingIsAttendance = $state(false);
	let pendingIsPriority = $state(false);

	// Delete confirmation
	let showDeleteConfirm = $state(false);
	let deletingRuleId = $state<string | null>(null);
	const deletingRule = $derived(rules.find((r) => r.id === deletingRuleId) ?? null);
	let deleting = $state(false);

	// Apply rules state
	let applying = $state(false);

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

	function showSuccess(message: string) {
		successMessage = message;
		setTimeout(() => {
			successMessage = '';
		}, 5000);
	}

	function openNewRule() {
		editingRuleId = null;
		editorInitialQuery = '';
		editorInitialProjectId = null;
		showEditor = true;
	}

	function handleSaveAsRule(query: string, projectId: string | null) {
		editingRuleId = null;
		editorInitialQuery = query;
		editorInitialProjectId = projectId;
		showEditor = true;
	}

	function openEditRule(rule: ClassificationRule) {
		editingRuleId = rule.id;
		showEditor = true;
	}

	async function handleEditorSave() {
		await loadData();
		showSuccess(editingRule ? 'Rule updated' : 'Rule created');
	}

	function handleEditorPreview(query: string, projectId: string | null, isAttendance: boolean, isPriority: boolean) {
		previewQuery = query;
		previewProjectId = projectId;
		pendingIsAttendance = isAttendance;
		pendingIsPriority = isPriority;
		// Carry the pending values back into the editor so "Back to Edit" restores them
		editorInitialQuery = query;
		editorInitialProjectId = projectId;
		showPreview = true;
	}

	function handlePreviewBack() {
		showEditor = true;
	}

	// Saves the rule the user previewed. The editor modal is closed at this point,
	// so the page owns the save using the pending values captured in handleEditorPreview.
	async function handlePreviewSave() {
		error = '';
		const weight = pendingIsPriority ? 2.0 : 1.0;

		try {
			if (editingRuleId) {
				await api.updateRule(editingRuleId, {
					query: previewQuery,
					weight,
					project_id: pendingIsAttendance ? null : previewProjectId,
					attended: pendingIsAttendance ? false : null
				});
			} else {
				await api.createRule({
					query: previewQuery,
					weight,
					...(pendingIsAttendance ? { attended: false } : { project_id: previewProjectId! })
				});
			}

			const wasEditing = editingRuleId !== null;
			await loadData();
			showSuccess(wasEditing ? 'Rule updated' : 'Rule created');
		} catch (e) {
			console.error('Failed to save rule:', e);
			error = e instanceof Error ? e.message : 'Failed to save rule';
			// Reopen the editor so the user does not lose what they typed
			showEditor = true;
		}
	}

	async function openPreviewForRule(rule: ClassificationRule) {
		previewQuery = rule.query;
		previewProjectId = rule.project_id || null;
		showPreview = true;
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
		deletingRuleId = rule.id;
		showDeleteConfirm = true;
	}

	async function handleDeleteRule() {
		if (!deletingRule) return;

		deleting = true;
		try {
			await api.deleteRule(deletingRule.id);
			showDeleteConfirm = false;
			deletingRuleId = null;
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
		error = '';
		try {
			const result = await api.applyRules({});
			showSuccess(`Applied rules: ${result.classified.length} classified, ${result.skipped} skipped`);
		} catch (e) {
			console.error('Failed to apply rules:', e);
			error = e instanceof Error ? e.message : 'Failed to apply rules';
		} finally {
			applying = false;
		}
	}

	onMount(() => {
		loadData();
	});
</script>

<svelte:head>
	<title>Classification Hub - Timesheet</title>
</svelte:head>

<AppShell>
	<div class="max-w-3xl mx-auto">
		<div class="flex items-center justify-between mb-6">
			<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Classification Hub</h1>
			<div class="flex items-center gap-3">
				{#if rules.length > 0}
					<Button variant="secondary" loading={applying} onclick={handleApplyRules}>
						Apply Rules
					</Button>
				{/if}
				<Button variant="primary" onclick={openNewRule}>+ New Rule</Button>
			</div>
		</div>

		{#if successMessage}
			<div class="mb-4 bg-green-50 dark:bg-green-900/30 border border-green-200 dark:border-green-800 text-green-700 dark:text-green-300 px-4 py-3 rounded text-sm">
				{successMessage}
			</div>
		{/if}

		{#if error}
			<div class="mb-4 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 px-4 py-3 rounded text-sm">
				{error}
			</div>
		{/if}

		<!-- Search Section -->
		<EventSearchSection
			{projects}
			onsaveasrule={handleSaveAsRule}
		/>

		<!-- Existing Rules Section -->
		{#if loading}
			<div class="flex items-center justify-center py-12">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		{:else if rules.length === 0}
			<!-- Empty state -->
			<div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-8 text-center">
				<div class="text-gray-400 dark:text-gray-500 mb-4">
					<svg class="w-12 h-12 mx-auto" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M9 5H7a2 2 0 00-2 2v10a2 2 0 002 2h8a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
						/>
					</svg>
				</div>
				<h3 class="text-lg font-medium text-gray-900 dark:text-white mb-2">No classification rules yet</h3>
				<p class="text-gray-500 dark:text-gray-400 mb-6">
					Use the search bar above to find events and classify them, or create rules to automatically classify future events.
				</p>
				<Button variant="primary" onclick={openNewRule}>Create Your First Rule</Button>
			</div>
		{:else}
			<div class="mb-3 flex items-center justify-between">
				<h2 class="text-lg font-medium text-gray-900 dark:text-white">Saved Rules</h2>
			</div>
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
	<RuleEditorModal
		bind:open={showEditor}
		rule={editingRule}
		{projects}
		initialQuery={editorInitialQuery}
		initialProjectId={editorInitialProjectId}
		onsave={handleEditorSave}
		onpreview={handleEditorPreview}
	/>

	<!-- Preview Modal -->
	<RulePreviewModal
		bind:open={showPreview}
		query={previewQuery}
		projectId={previewProjectId}
		onback={handlePreviewBack}
		onsave={handlePreviewSave}
	/>

	<!-- Delete Confirmation Modal -->
	<Modal bind:open={showDeleteConfirm} title="Delete Rule">
		<p class="text-gray-600 dark:text-gray-300">
			Are you sure you want to delete this rule? This action cannot be undone.
		</p>
		{#if deletingRule}
			<div class="mt-3 p-3 bg-gray-50 dark:bg-gray-700/50 rounded">
				<code class="text-sm font-mono text-gray-900 dark:text-white">{deletingRule.query}</code>
			</div>
		{/if}

		{#snippet footer()}
			<Button variant="secondary" onclick={() => (showDeleteConfirm = false)}>Cancel</Button>
			<Button variant="danger" loading={deleting} onclick={handleDeleteRule}>Delete</Button>
		{/snippet}
	</Modal>
</AppShell>
