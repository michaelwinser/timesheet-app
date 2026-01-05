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
	let successMessage = $state('');

	// Search state
	let searchQuery = $state('');
	let searchTimeout: ReturnType<typeof setTimeout>;
	let searchLoading = $state(false);
	let searchResults = $state<RulePreviewResponse | null>(null);
	let searchError = $state('');
	let selectedProjectId = $state<string | null>(null);
	let bulkClassifying = $state(false);

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

	// Search functionality
	function handleSearchInput(e: Event) {
		const value = (e.target as HTMLInputElement).value;
		searchQuery = value;
		searchError = '';

		clearTimeout(searchTimeout);
		if (value.trim()) {
			searchLoading = true;
			searchTimeout = setTimeout(() => executeSearch(value), 300);
		} else {
			searchResults = null;
			searchLoading = false;
		}
	}

	async function executeSearch(query: string) {
		try {
			searchResults = await api.previewRule({ query });
			searchError = '';
		} catch (e) {
			console.error('Search failed:', e);
			searchError = e instanceof Error ? e.message : 'Invalid query syntax';
			searchResults = null;
		} finally {
			searchLoading = false;
		}
	}

	function clearSearch() {
		searchQuery = '';
		searchResults = null;
		searchError = '';
		selectedProjectId = null;
	}

	async function handleBulkClassify() {
		if (!searchQuery.trim() || !selectedProjectId) return;

		bulkClassifying = true;
		try {
			const result = await api.bulkClassifyEvents({
				query: searchQuery,
				project_id: selectedProjectId
			});
			showSuccess(`Classified ${result.classified_count} events`);
			// Refresh search results
			await executeSearch(searchQuery);
		} catch (e) {
			console.error('Bulk classify failed:', e);
			error = e instanceof Error ? e.message : 'Failed to classify events';
		} finally {
			bulkClassifying = false;
		}
	}

	async function handleBulkSkip() {
		if (!searchQuery.trim()) return;

		bulkClassifying = true;
		try {
			const result = await api.bulkClassifyEvents({
				query: searchQuery,
				skip: true
			});
			showSuccess(`Skipped ${result.skipped_count} events`);
			// Refresh search results
			await executeSearch(searchQuery);
		} catch (e) {
			console.error('Bulk skip failed:', e);
			error = e instanceof Error ? e.message : 'Failed to skip events';
		} finally {
			bulkClassifying = false;
		}
	}

	function handleSaveAsRule() {
		// Pre-populate the editor with the current search query
		editingRule = null;
		editorQuery = searchQuery;
		editorProjectId = selectedProjectId;
		editorIsAttendance = false;
		editorIsPriority = false;
		editorError = '';
		showEditor = true;
	}

	function showSuccess(message: string) {
		successMessage = message;
		setTimeout(() => {
			successMessage = '';
		}, 5000);
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
			showSuccess(editingRule ? 'Rule updated' : 'Rule created');
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

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString([], {
			weekday: 'short',
			month: 'short',
			day: 'numeric'
		});
	}

	function formatTime(dateStr: string): string {
		const date = new Date(dateStr);
		return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
	}

	function formatDuration(startStr: string, endStr: string): string {
		const start = new Date(startStr);
		const end = new Date(endStr);
		const hours = (end.getTime() - start.getTime()) / (1000 * 60 * 60);
		return `${hours.toFixed(1)}h`;
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
		<div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4 mb-6">
			<div class="relative">
				<div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
					{#if searchLoading}
						<div class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary-600"></div>
					{:else}
						<svg class="h-5 w-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
						</svg>
					{/if}
				</div>
				<input
					type="text"
					value={searchQuery}
					oninput={handleSearchInput}
					placeholder="Search events: standup, title:sync, calendar:work..."
					class="w-full pl-10 pr-10 py-2 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 font-mono text-sm"
				/>
				{#if searchQuery}
					<button
						type="button"
						onclick={clearSearch}
						class="absolute inset-y-0 right-0 pr-3 flex items-center text-gray-400 hover:text-gray-600"
					>
						<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
						</svg>
					</button>
				{/if}
			</div>
			<details class="mt-2 text-xs text-gray-500 dark:text-gray-400">
				<summary class="cursor-pointer hover:text-gray-700 dark:hover:text-gray-300">Search syntax help</summary>
				<div class="mt-2 p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg space-y-3">
					<div>
						<div class="font-medium text-gray-700 mb-1">Text Search</div>
						<div class="font-mono text-gray-600 space-y-0.5">
							<div><span class="text-primary-600">standup</span> — search title, description, attendees</div>
							<div><span class="text-primary-600">text:meeting</span> — explicit text search</div>
						</div>
					</div>
					<div>
						<div class="font-medium text-gray-700 mb-1">Event Properties</div>
						<div class="font-mono text-gray-600 space-y-0.5">
							<div><span class="text-primary-600">title:standup</span> — event title contains "standup"</div>
							<div><span class="text-primary-600">description:agenda</span> — description contains "agenda"</div>
							<div><span class="text-primary-600">calendar:work</span> — from calendar named "work"</div>
							<div><span class="text-primary-600">attendees:alice</span> — attendee email contains "alice"</div>
							<div><span class="text-primary-600">domain:acme.com</span> — attendee from domain</div>
							<div><span class="text-primary-600">email:bob@acme.com</span> — exact attendee email</div>
						</div>
					</div>
					<div>
						<div class="font-medium text-gray-700 mb-1">Event Status</div>
						<div class="font-mono text-gray-600 space-y-0.5">
							<div><span class="text-primary-600">response:accepted</span> — accepted, declined, needsAction, tentative</div>
							<div><span class="text-primary-600">recurring:yes</span> — recurring events only</div>
							<div><span class="text-primary-600">transparency:transparent</span> — "free" events</div>
							<div><span class="text-primary-600">has-attendees:no</span> — events without attendees</div>
							<div><span class="text-primary-600">is-all-day:yes</span> — all-day events only</div>
						</div>
					</div>
					<div>
						<div class="font-medium text-gray-700 mb-1">Time Filters</div>
						<div class="font-mono text-gray-600 space-y-0.5">
							<div><span class="text-primary-600">day-of-week:mon</span> — mon, tue, wed, thu, fri, sat, sun</div>
							<div><span class="text-primary-600">time-of-day:&gt;17:00</span> — events starting after 5pm</div>
							<div><span class="text-primary-600">time-of-day:&lt;09:00</span> — events starting before 9am</div>
						</div>
					</div>
					<div>
						<div class="font-medium text-gray-700 mb-1">Classification Status</div>
						<div class="font-mono text-gray-600 space-y-0.5">
							<div><span class="text-primary-600">project:unclassified</span> — not yet classified</div>
							<div><span class="text-primary-600">project:acme</span> — assigned to project containing "acme"</div>
							<div><span class="text-primary-600">client:corp</span> — project client contains "corp"</div>
							<div><span class="text-primary-600">confidence:low</span> — low, medium, or high confidence</div>
						</div>
					</div>
					<div>
						<div class="font-medium text-gray-700 mb-1">Combining Conditions</div>
						<div class="font-mono text-gray-600 space-y-0.5">
							<div><span class="text-primary-600">standup domain:acme.com</span> — AND (both must match)</div>
							<div><span class="text-primary-600">standup OR sync</span> — OR (either matches)</div>
							<div><span class="text-primary-600">-response:declined</span> — NOT (exclude declined)</div>
							<div><span class="text-primary-600">"out of office"</span> — quoted multi-word phrase</div>
						</div>
					</div>
					<div>
						<div class="font-medium text-gray-700 mb-1">Complex Example</div>
						<div class="font-mono text-gray-600">
							<span class="text-primary-600">(standup OR sync) domain:acme.com -response:declined</span>
						</div>
						<div class="text-gray-500 mt-0.5">Events with "standup" or "sync" from acme.com, excluding declined</div>
					</div>
				</div>
			</details>

			{#if searchError}
				<div class="mt-3 text-sm text-red-600">
					{searchError}
				</div>
			{/if}

			<!-- Search Results -->
			{#if searchResults}
				<div class="mt-4 border-t border-gray-200 dark:border-gray-700 pt-4">
					<div class="flex items-center justify-between mb-3">
						<span class="text-sm font-medium text-gray-700 dark:text-gray-300">
							{searchResults.stats.total_matches} events match
						</span>
						<button
							type="button"
							onclick={handleSaveAsRule}
							class="text-sm text-primary-600 dark:text-primary-400 hover:text-primary-700 dark:hover:text-primary-300 font-medium"
						>
							Save as Rule
						</button>
					</div>

					{#if searchResults.matches.length > 0}
						<!-- Event list -->
						<div class="max-h-64 overflow-y-auto space-y-1 mb-4">
							{#each searchResults.matches.slice(0, 10) as match}
								<div class="text-sm py-2 px-3 bg-gray-50 dark:bg-gray-700/50 rounded flex items-center justify-between">
									<span class="truncate flex-1 text-gray-900 dark:text-white">{match.title}</span>
									<span class="text-gray-500 dark:text-gray-400 text-xs flex-shrink-0 ml-2">
										{formatDate(match.start_time)}
									</span>
								</div>
							{/each}
							{#if searchResults.matches.length > 10}
								<div class="text-sm text-gray-500 dark:text-gray-400 text-center py-2">
									+{searchResults.matches.length - 10} more events
								</div>
							{/if}
						</div>

						<!-- Bulk actions -->
						<div class="flex items-center gap-3 pt-3 border-t border-gray-200 dark:border-gray-700">
							<span class="text-sm text-gray-600 dark:text-gray-400">Classify all as:</span>
							<select
								bind:value={selectedProjectId}
								class="flex-1 px-3 py-1.5 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded text-sm focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
							>
								<option value={null}>Select project...</option>
								{#each projects.filter((p) => !p.is_archived) as project}
									<option value={project.id}>
										{project.name}
									</option>
								{/each}
							</select>
							<Button
								variant="primary"
								size="sm"
								loading={bulkClassifying}
								disabled={!selectedProjectId}
								onclick={handleBulkClassify}
							>
								Classify {searchResults.stats.total_matches}
							</Button>
							<Button
								variant="secondary"
								size="sm"
								loading={bulkClassifying}
								onclick={handleBulkSkip}
							>
								Skip All
							</Button>
						</div>

						{#if searchResults.stats.manual_conflicts > 0}
							<div class="mt-3 text-xs text-yellow-700 bg-yellow-50 rounded px-3 py-2">
								{searchResults.stats.manual_conflicts} events have manual classifications and will not be changed.
							</div>
						{/if}
					{:else}
						<div class="text-sm text-gray-500 text-center py-4">
							No events match this query
						</div>
					{/if}
				</div>
			{/if}
		</div>

		<!-- Existing Rules Section -->
		{#if loading}
			<div class="flex items-center justify-center py-12">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		{:else if rules.length === 0 && !searchQuery}
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
		{:else if rules.length > 0}
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
	<Modal bind:open={showEditor} title={editingRule ? 'Edit Rule' : 'New Rule'}>
		<div class="space-y-4">
			{#if editorError}
				<div class="bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 px-3 py-2 rounded text-sm">
					{editorError}
				</div>
			{/if}

			<div>
				<label for="query" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Query</label>
				<input
					id="query"
					type="text"
					bind:value={editorQuery}
					placeholder="domain:acme.com title:sync"
					class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 font-mono text-sm"
				/>
				<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
					e.g. <code class="bg-gray-100 dark:bg-gray-700 px-1 rounded">standup domain:acme.com -response:declined</code>
				</p>
			</div>

			<div class="border-t border-gray-200 dark:border-gray-700 pt-4">
				<div class="flex items-center gap-4 mb-3">
					<label class="flex items-center gap-2 cursor-pointer">
						<input
							type="radio"
							name="ruleType"
							checked={!editorIsAttendance}
							onchange={() => (editorIsAttendance = false)}
							class="h-4 w-4 text-primary-600 focus:ring-primary-500"
						/>
						<span class="text-sm text-gray-700 dark:text-gray-300">Assign to project</span>
					</label>
					<label class="flex items-center gap-2 cursor-pointer">
						<input
							type="radio"
							name="ruleType"
							checked={editorIsAttendance}
							onchange={() => (editorIsAttendance = true)}
							class="h-4 w-4 text-primary-600 focus:ring-primary-500"
						/>
						<span class="text-sm text-gray-700 dark:text-gray-300">Did not attend</span>
					</label>
				</div>

				{#if !editorIsAttendance}
					<div>
						<label for="project" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
							>Project</label
						>
						<select
							id="project"
							bind:value={editorProjectId}
							class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
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

			<div class="border-t border-gray-200 dark:border-gray-700 pt-4">
				<label class="flex items-center gap-2 cursor-pointer">
					<input
						type="checkbox"
						bind:checked={editorIsPriority}
						class="h-4 w-4 rounded text-primary-600 focus:ring-primary-500 dark:bg-gray-700 border-gray-300 dark:border-gray-600"
					/>
					<span class="text-sm text-gray-700 dark:text-gray-300">Priority rule (counts twice in scoring)</span>
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
				<div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-3">
					<div class="text-sm text-gray-500 dark:text-gray-400 mb-1">Query</div>
					<code class="text-sm font-mono text-gray-900 dark:text-white">{previewQuery}</code>
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
