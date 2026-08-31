<script lang="ts">
	import { onMount } from 'svelte';
	import { Button, Modal } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import type {
		IngestionFilter,
		SuppressionEffect,
		SuppressedEvent
	} from '$lib/api/types';

	interface Props {
		error: string;
		successMessage: string;
	}

	let { error = $bindable(''), successMessage = $bindable('') }: Props = $props();

	let filters = $state<IngestionFilter[]>([]);
	let loading = $state(true);
	let saving = $state(false);

	// Editor state. Store the id and derive the filter, so the modal never holds
	// a stale copy when the list reloads.
	let showEditor = $state(false);
	let editingFilterId = $state<string | null>(null);
	const editingFilter = $derived(filters.find((f) => f.id === editingFilterId) ?? null);
	let editorName = $state('');
	let editorQuery = $state('');
	let editorError = $state('');

	// Delete confirmation
	let showDeleteConfirm = $state(false);
	let deletingFilterId = $state<string | null>(null);
	const deletingFilter = $derived(filters.find((f) => f.id === deletingFilterId) ?? null);

	// Hidden events
	let showHidden = $state(false);
	let hiddenTotal = $state(0);
	let hiddenEvents = $state<SuppressedEvent[]>([]);
	let hiddenLoading = $state(false);

	async function load() {
		loading = true;
		try {
			filters = await api.listIngestionFilters();
			await loadHiddenCount();
		} catch (e) {
			console.error('Failed to load ingestion filters:', e);
			error = e instanceof Error ? e.message : 'Failed to load filters';
		} finally {
			loading = false;
		}
	}

	async function loadHiddenCount() {
		try {
			const result = await api.listSuppressedEvents(50);
			hiddenTotal = result.total;
			hiddenEvents = result.events ?? [];
		} catch (e) {
			console.error('Failed to load hidden events:', e);
		}
	}

	function describeEffect(effect: SuppressionEffect): string {
		const parts: string[] = [];
		if (effect.now_hidden > 0) parts.push(`${effect.now_hidden} event(s) hidden`);
		if (effect.now_visible > 0) parts.push(`${effect.now_visible} event(s) restored`);
		if (parts.length === 0) return 'No events changed';
		return parts.join(', ');
	}

	function openNew() {
		editingFilterId = null;
		editorName = '';
		editorQuery = '';
		editorError = '';
		showEditor = true;
	}

	function openEdit(filter: IngestionFilter) {
		editingFilterId = filter.id;
		editorName = filter.name;
		editorQuery = filter.query;
		editorError = '';
		showEditor = true;
	}

	async function handleSave() {
		editorError = '';
		if (!editorName.trim()) {
			editorError = 'Name is required';
			return;
		}
		if (!editorQuery.trim()) {
			editorError = 'Query is required';
			return;
		}

		saving = true;
		try {
			const result = editingFilterId
				? await api.updateIngestionFilter(editingFilterId, {
						name: editorName,
						query: editorQuery
					})
				: await api.createIngestionFilter({ name: editorName, query: editorQuery });

			showEditor = false;
			await load();
			successMessage = `Filter saved. ${describeEffect(result.effect)}.`;
		} catch (e) {
			console.error('Failed to save filter:', e);
			editorError = e instanceof Error ? e.message : 'Failed to save filter';
		} finally {
			saving = false;
		}
	}

	async function handleToggle(filter: IngestionFilter) {
		try {
			const result = await api.updateIngestionFilter(filter.id, {
				is_enabled: !filter.is_enabled
			});
			await load();
			successMessage = `Filter ${result.filter.is_enabled ? 'enabled' : 'disabled'}. ${describeEffect(result.effect)}.`;
		} catch (e) {
			console.error('Failed to toggle filter:', e);
			error = e instanceof Error ? e.message : 'Failed to toggle filter';
		}
	}

	async function handleDelete() {
		if (!deletingFilterId) return;
		try {
			const effect = await api.deleteIngestionFilter(deletingFilterId);
			showDeleteConfirm = false;
			deletingFilterId = null;
			await load();
			successMessage = `Filter deleted. ${describeEffect(effect)}.`;
		} catch (e) {
			console.error('Failed to delete filter:', e);
			error = e instanceof Error ? e.message : 'Failed to delete filter';
		}
	}

	async function toggleHidden() {
		showHidden = !showHidden;
		if (showHidden) {
			hiddenLoading = true;
			await loadHiddenCount();
			hiddenLoading = false;
		}
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString([], {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	onMount(load);
</script>

<div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4 mb-6">
	<div class="flex items-start justify-between mb-1">
		<h2 class="text-lg font-medium text-gray-900 dark:text-white">Ingestion Filters</h2>
		<Button variant="primary" onclick={openNew}>New Filter</Button>
	</div>
	<p class="text-sm text-gray-500 dark:text-gray-400 mb-4">
		Hide events before they reach classification — calendar sync placeholders and similar
		noise. Matching events are stored but hidden everywhere, and contribute no time.
		Disabling a filter brings its events back.
	</p>

	{#if loading}
		<div class="py-6 text-center text-gray-500 dark:text-gray-400">Loading…</div>
	{:else if filters.length === 0}
		<div class="py-6 text-center text-gray-500 dark:text-gray-400 text-sm">
			No filters. Every event reaches classification.
		</div>
	{:else}
		<div class="space-y-2">
			{#each filters as filter (filter.id)}
				<div
					class="flex items-center gap-3 p-3 border border-gray-200 dark:border-gray-700 rounded-lg"
					class:opacity-50={!filter.is_enabled}
				>
					<div class="flex-1 min-w-0">
						<div class="font-medium text-gray-900 dark:text-white truncate">
							{filter.name}
							{#if !filter.is_enabled}
								<span class="ml-2 text-xs font-normal text-gray-500 dark:text-gray-400">disabled</span>
							{/if}
						</div>
						<code class="text-xs font-mono text-gray-600 dark:text-gray-400 break-all">
							{filter.query}
						</code>
					</div>
					<Button variant="secondary" onclick={() => handleToggle(filter)}>
						{filter.is_enabled ? 'Disable' : 'Enable'}
					</Button>
					<Button variant="secondary" onclick={() => openEdit(filter)}>Edit</Button>
					<Button
						variant="secondary"
						onclick={() => {
							deletingFilterId = filter.id;
							showDeleteConfirm = true;
						}}>Delete</Button
					>
				</div>
			{/each}
		</div>
	{/if}

	<div class="mt-4 pt-3 border-t border-gray-200 dark:border-gray-700">
		<button
			type="button"
			onclick={toggleHidden}
			class="text-sm text-primary-600 dark:text-primary-400 hover:underline"
		>
			{hiddenTotal}
			{hiddenTotal === 1 ? 'event is' : 'events are'} currently hidden
			{showHidden ? '(hide list)' : '(show them)'}
		</button>

		{#if showHidden}
			{#if hiddenLoading}
				<div class="mt-2 text-sm text-gray-500 dark:text-gray-400">Loading…</div>
			{:else if hiddenEvents.length === 0}
				<div class="mt-2 text-sm text-gray-500 dark:text-gray-400">Nothing is hidden.</div>
			{:else}
				<div class="mt-2 max-h-64 overflow-y-auto space-y-1">
					{#each hiddenEvents as event (event.id)}
						<div class="flex justify-between gap-3 text-sm py-1">
							<span class="text-gray-700 dark:text-gray-300 truncate">{event.title}</span>
							<span class="text-gray-500 dark:text-gray-400 whitespace-nowrap">
								{formatDate(event.start_time)}
							</span>
						</div>
					{/each}
					{#if hiddenTotal > hiddenEvents.length}
						<div class="text-xs text-gray-500 dark:text-gray-400 pt-1">
							+{hiddenTotal - hiddenEvents.length} more
						</div>
					{/if}
				</div>
			{/if}
		{/if}
	</div>
</div>

<Modal bind:open={showEditor} title={editingFilter ? 'Edit Filter' : 'New Filter'}>
	<div class="space-y-4">
		{#if editorError}
			<div
				class="bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 px-3 py-2 rounded text-sm"
			>
				{editorError}
			</div>
		{/if}

		<div
			class="bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-800 text-amber-800 dark:text-amber-300 px-3 py-2 rounded text-sm"
		>
			Matching events will be hidden from the calendar, excluded from classification, and
			contribute no time. Nothing is deleted — disabling this filter brings them back.
		</div>

		<div>
			<label for="filter-name" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
				Name
			</label>
			<input
				id="filter-name"
				type="text"
				bind:value={editorName}
				placeholder="Calendar sync placeholders"
				class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded-lg focus:ring-2 focus:ring-primary-500"
			/>
		</div>

		<div>
			<label for="filter-query" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
				Query
			</label>
			<input
				id="filter-query"
				type="text"
				bind:value={editorQuery}
				placeholder="property:calendarSyncMarker"
				class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded-lg focus:ring-2 focus:ring-primary-500 font-mono text-sm"
			/>
			<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
				Same syntax as event search. Matching on a custom property is more durable than
				matching a title, which the tool that wrote it can change at any time.
			</p>
		</div>
	</div>

	{#snippet footer()}
		<Button variant="secondary" onclick={() => (showEditor = false)}>Cancel</Button>
		<Button variant="primary" onclick={handleSave} disabled={saving}>
			{saving ? 'Saving…' : 'Save Filter'}
		</Button>
	{/snippet}
</Modal>

<Modal bind:open={showDeleteConfirm} title="Delete Filter">
	<p class="text-gray-600 dark:text-gray-300">
		Delete this filter? Events it was hiding will reappear, unless another filter matches them.
	</p>
	{#if deletingFilter}
		<div class="mt-3 p-3 bg-gray-50 dark:bg-gray-700/50 rounded">
			<div class="font-medium text-gray-900 dark:text-white">{deletingFilter.name}</div>
			<code class="text-sm font-mono text-gray-600 dark:text-gray-400">{deletingFilter.query}</code>
		</div>
	{/if}

	{#snippet footer()}
		<Button variant="secondary" onclick={() => (showDeleteConfirm = false)}>Cancel</Button>
		<Button variant="primary" onclick={handleDelete}>Delete</Button>
	{/snippet}
</Modal>
