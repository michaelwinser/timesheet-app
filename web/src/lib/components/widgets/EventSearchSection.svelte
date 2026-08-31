<script lang="ts">
	import { Button } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import type { Project, RulePreviewResponse } from '$lib/api/types';

	interface Props {
		projects: Project[];
		onsaveasrule?: (query: string, projectId: string | null) => void;
		onclassified?: () => void;
	}

	let { projects, onsaveasrule, onclassified }: Props = $props();

	// Search state
	let searchQuery = $state('');
	let searchTimeout: ReturnType<typeof setTimeout>;
	// Incremented on every new/cancelled search so late responses can be discarded
	let searchSeq = 0;
	let searchLoading = $state(false);
	let searchResults = $state<RulePreviewResponse | null>(null);
	let searchError = $state('');
	let selectedProjectId = $state<string | null>(null);
	let bulkClassifying = $state(false);

	// A query that is still being typed (open quote, dangling property, unclosed
	// group) parses as a syntax error. Searching it round-trips a guaranteed 400
	// and replaces the results with an error the user is already in the middle of
	// fixing - so wait for the next keystroke instead.
	function isIncompleteQuery(query: string): boolean {
		// Unclosed quote
		if ((query.split('"').length - 1) % 2 === 1) return true;

		// Quoted values are literal, not syntax - blank them before checking the rest
		const unquoted = query.replace(/"[^"]*"/g, '""').trimEnd();

		// Property with no value yet, e.g. title:
		if (unquoted.endsWith(':')) return true;

		// Dangling negation or OR with nothing after it yet
		if (/(^|\s)(-|OR)$/i.test(unquoted)) return true;

		// Unclosed group, e.g. (title:sync
		const opened = (unquoted.match(/\(/g) ?? []).length;
		const closed = (unquoted.match(/\)/g) ?? []).length;
		return opened > closed;
	}

	function handleSearchInput(e: Event) {
		const value = (e.target as HTMLInputElement).value;
		searchQuery = value;
		searchError = '';

		clearTimeout(searchTimeout);

		if (!value.trim()) {
			searchSeq++; // discard any response still in flight
			searchResults = null;
			searchLoading = false;
			return;
		}

		if (isIncompleteQuery(value)) {
			searchLoading = false;
			return;
		}

		searchLoading = true;
		searchTimeout = setTimeout(() => executeSearch(value), 300);
	}

	async function executeSearch(query: string) {
		const seq = ++searchSeq;

		try {
			const result = await api.previewRule({ query });
			if (seq !== searchSeq) return; // superseded by a newer search
			searchResults = result;
			searchError = '';
		} catch (e) {
			if (seq !== searchSeq) return;
			console.error('Search failed:', e);
			searchError = e instanceof Error ? e.message : 'Invalid query syntax';
			searchResults = null;
		} finally {
			if (seq === searchSeq) {
				searchLoading = false;
			}
		}
	}

	function clearSearch() {
		clearTimeout(searchTimeout);
		searchSeq++; // discard any response still in flight
		searchQuery = '';
		searchResults = null;
		searchError = '';
		selectedProjectId = null;
		searchLoading = false;
	}

	async function handleBulkClassify() {
		if (!searchQuery.trim() || !selectedProjectId) return;

		bulkClassifying = true;
		try {
			await api.bulkClassifyEvents({
				query: searchQuery,
				project_id: selectedProjectId
			});
			// Refresh search results
			await executeSearch(searchQuery);
			onclassified?.();
		} catch (e) {
			console.error('Bulk classify failed:', e);
			searchError = e instanceof Error ? e.message : 'Failed to classify events';
		} finally {
			bulkClassifying = false;
		}
	}

	async function handleBulkSkip() {
		if (!searchQuery.trim()) return;

		bulkClassifying = true;
		try {
			await api.bulkClassifyEvents({
				query: searchQuery,
				skip: true
			});
			// Refresh search results
			await executeSearch(searchQuery);
			onclassified?.();
		} catch (e) {
			console.error('Bulk skip failed:', e);
			searchError = e instanceof Error ? e.message : 'Failed to skip events';
		} finally {
			bulkClassifying = false;
		}
	}

	function handleSaveAsRule() {
		onsaveasrule?.(searchQuery, selectedProjectId);
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString([], {
			weekday: 'short',
			month: 'short',
			day: 'numeric'
		});
	}
</script>

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
				<div class="font-medium text-gray-700 dark:text-gray-300 mb-1">Text Search</div>
				<div class="font-mono text-gray-600 dark:text-gray-400 space-y-0.5">
					<div><span class="text-primary-600 dark:text-primary-400">standup</span> — search title, description, attendees</div>
					<div><span class="text-primary-600 dark:text-primary-400">text:meeting</span> — explicit text search</div>
				</div>
			</div>
			<div>
				<div class="font-medium text-gray-700 dark:text-gray-300 mb-1">Event Properties</div>
				<div class="font-mono text-gray-600 dark:text-gray-400 space-y-0.5">
					<div><span class="text-primary-600 dark:text-primary-400">title:standup</span> — event title contains "standup"</div>
					<div><span class="text-primary-600 dark:text-primary-400">description:agenda</span> — description contains "agenda"</div>
					<div><span class="text-primary-600 dark:text-primary-400">calendar:work</span> — from calendar named "work"</div>
					<div><span class="text-primary-600 dark:text-primary-400">attendees:alice</span> — attendee email contains "alice"</div>
					<div><span class="text-primary-600 dark:text-primary-400">domain:acme.com</span> — attendee from domain</div>
					<div><span class="text-primary-600 dark:text-primary-400">email:bob@acme.com</span> — exact attendee email</div>
				</div>
			</div>
			<div>
				<div class="font-medium text-gray-700 dark:text-gray-300 mb-1">Event Status</div>
				<div class="font-mono text-gray-600 dark:text-gray-400 space-y-0.5">
					<div><span class="text-primary-600 dark:text-primary-400">response:accepted</span> — accepted, declined, needsAction, tentative</div>
					<div><span class="text-primary-600 dark:text-primary-400">recurring:yes</span> — recurring events only</div>
					<div><span class="text-primary-600 dark:text-primary-400">transparency:transparent</span> — "free" events</div>
					<div><span class="text-primary-600 dark:text-primary-400">has-attendees:no</span> — events without attendees</div>
					<div><span class="text-primary-600 dark:text-primary-400">is-all-day:yes</span> — all-day events only</div>
				</div>
			</div>
			<div>
				<div class="font-medium text-gray-700 dark:text-gray-300 mb-1">Time Filters</div>
				<div class="font-mono text-gray-600 dark:text-gray-400 space-y-0.5">
					<div><span class="text-primary-600 dark:text-primary-400">day-of-week:mon</span> — mon, tue, wed, thu, fri, sat, sun</div>
					<div><span class="text-primary-600 dark:text-primary-400">time-of-day:&gt;17:00</span> — events starting after 5pm</div>
					<div><span class="text-primary-600 dark:text-primary-400">time-of-day:&lt;09:00</span> — events starting before 9am</div>
				</div>
			</div>
			<div>
				<div class="font-medium text-gray-700 dark:text-gray-300 mb-1">Classification Status</div>
				<div class="font-mono text-gray-600 dark:text-gray-400 space-y-0.5">
					<div><span class="text-primary-600 dark:text-primary-400">project:unclassified</span> — not yet classified</div>
					<div><span class="text-primary-600 dark:text-primary-400">project:acme</span> — assigned to project containing "acme"</div>
					<div><span class="text-primary-600 dark:text-primary-400">client:corp</span> — project client contains "corp"</div>
					<div><span class="text-primary-600 dark:text-primary-400">confidence:low</span> — low, medium, or high confidence</div>
				</div>
			</div>
			<div>
				<div class="font-medium text-gray-700 dark:text-gray-300 mb-1">Custom Properties</div>
				<div class="font-mono text-gray-600 dark:text-gray-400 space-y-0.5">
					<div><span class="text-primary-600 dark:text-primary-400">property:calendarSyncMarker</span> — has this property, any value</div>
					<div><span class="text-primary-600 dark:text-primary-400">property:reclaim.event.priority=P3</span> — has it with this value</div>
					<div><span class="text-primary-600 dark:text-primary-400">-property:calendarSyncMarker</span> — does not have it</div>
				</div>
				<div class="mt-1 text-gray-500 dark:text-gray-400">
					Properties written by other tools. Case-sensitive, unlike everything else.
					Prefer matching the key alone — values are often versioned.
				</div>
			</div>
			<div>
				<div class="font-medium text-gray-700 dark:text-gray-300 mb-1">Combining Conditions</div>
				<div class="font-mono text-gray-600 dark:text-gray-400 space-y-0.5">
					<div><span class="text-primary-600 dark:text-primary-400">standup domain:acme.com</span> — AND (both must match)</div>
					<div><span class="text-primary-600 dark:text-primary-400">standup OR sync</span> — OR (either matches)</div>
					<div><span class="text-primary-600 dark:text-primary-400">-response:declined</span> — NOT (exclude declined)</div>
					<div><span class="text-primary-600 dark:text-primary-400">"out of office"</span> — quoted multi-word phrase</div>
				</div>
			</div>
			<div>
				<div class="font-medium text-gray-700 dark:text-gray-300 mb-1">Complex Example</div>
				<div class="font-mono text-gray-600 dark:text-gray-400">
					<span class="text-primary-600 dark:text-primary-400">(standup OR sync) domain:acme.com -response:declined</span>
				</div>
				<div class="text-gray-500 dark:text-gray-400 mt-0.5">Events with "standup" or "sync" from acme.com, excluding declined</div>
			</div>
		</div>
	</details>

	{#if searchError}
		<div class="mt-3 text-sm text-red-600 dark:text-red-400">
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
					<div class="mt-3 text-xs text-yellow-700 dark:text-yellow-300 bg-yellow-50 dark:bg-yellow-900/30 rounded px-3 py-2">
						{searchResults.stats.manual_conflicts} events have manual classifications and will not be changed.
					</div>
				{/if}
			{:else}
				<div class="text-sm text-gray-500 dark:text-gray-400 text-center py-4">
					No events match this query
				</div>
			{/if}
		</div>
	{/if}
</div>
