<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button } from '$lib/components/primitives';
	import { api, type SyncStatusResponse, type CalendarSyncStatus } from '$lib/api/client';

	let status = $state<SyncStatusResponse | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let lastRefresh = $state<Date | null>(null);
	let autoRefresh = $state(true);
	let refreshInterval: ReturnType<typeof setInterval> | null = null;

	async function loadStatus() {
		loading = true;
		error = null;
		try {
			status = await api.getSyncStatus();
			lastRefresh = new Date();
		} catch (e) {
			console.error('Failed to load sync status:', e);
			error = e instanceof Error ? e.message : 'Failed to load sync status';
		} finally {
			loading = false;
		}
	}

	function startAutoRefresh() {
		if (refreshInterval) return;
		refreshInterval = setInterval(() => {
			if (!loading) {
				loadStatus();
			}
		}, 5000);
	}

	function stopAutoRefresh() {
		if (refreshInterval) {
			clearInterval(refreshInterval);
			refreshInterval = null;
		}
	}

	function toggleAutoRefresh() {
		autoRefresh = !autoRefresh;
		if (autoRefresh) {
			startAutoRefresh();
		} else {
			stopAutoRefresh();
		}
	}

	function formatDate(dateStr: string | null): string {
		if (!dateStr) return '-';
		const date = new Date(dateStr);
		return date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
	}

	function formatRelative(dateStr: string | null): string {
		if (!dateStr) return 'never';
		const date = new Date(dateStr);
		const now = new Date();
		const diffMs = now.getTime() - date.getTime();
		const diffMins = Math.floor(diffMs / 60000);
		const diffHours = Math.floor(diffMs / 3600000);
		const diffDays = Math.floor(diffMs / 86400000);

		if (diffMins < 1) return 'just now';
		if (diffMins < 60) return `${diffMins}m ago`;
		if (diffHours < 24) return `${diffHours}h ago`;
		return `${diffDays}d ago`;
	}

	function getStatusColor(cal: CalendarSyncStatus): string {
		if (cal.needs_reauth) return 'bg-red-100 dark:bg-red-900/30';
		if (cal.sync_failure_count > 0) return 'bg-orange-100 dark:bg-orange-900/30';
		if (cal.is_stale) return 'bg-yellow-100 dark:bg-yellow-900/30';
		return 'bg-green-100 dark:bg-green-900/30';
	}

	function getStatusLabel(cal: CalendarSyncStatus): string {
		if (cal.needs_reauth) return 'Needs Reauth';
		if (cal.sync_failure_count > 0) return `${cal.sync_failure_count} Failures`;
		if (cal.is_stale) return 'Stale';
		return 'Fresh';
	}

	onMount(() => {
		loadStatus();
	});
</script>

<svelte:head>
	<title>Sync Debug - Timesheet</title>
</svelte:head>

<AppShell>
	<div class="max-w-6xl mx-auto">
		<div class="flex items-center justify-between mb-6">
			<div>
				<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Sync Status Debug</h1>
				<p class="text-sm text-gray-500 dark:text-gray-400">
					Diagnostic information for calendar synchronization
				</p>
			</div>
			<div class="flex items-center gap-3">
				{#if lastRefresh}
					<span class="text-sm text-gray-500 dark:text-gray-400">
						Last refresh: {formatRelative(lastRefresh.toISOString())}
					</span>
				{/if}
				<Button onclick={loadStatus} loading={loading}>
					Refresh
				</Button>
			</div>
		</div>

		{#if error}
			<div class="mb-6 p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
				<p class="text-red-700 dark:text-red-300">{error}</p>
			</div>
		{/if}

		{#if loading && !status}
			<div class="flex items-center justify-center py-12">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		{:else if status}
			<!-- Configuration Info -->
			<div class="mb-6 bg-white dark:bg-zinc-800 border border-gray-200 dark:border-zinc-700 rounded-lg p-4">
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-3">Configuration</h2>
				<div class="grid grid-cols-1 md:grid-cols-3 gap-4 text-sm">
					<div>
						<span class="text-gray-500 dark:text-gray-400">Staleness Threshold:</span>
						<span class="ml-2 font-mono text-gray-900 dark:text-white">{status.staleness_threshold}</span>
					</div>
					<div>
						<span class="text-gray-500 dark:text-gray-400">Initial Window:</span>
						<span class="ml-2 font-mono text-gray-900 dark:text-white">
							{status.default_initial_window.start} to {status.default_initial_window.end}
							<span class="text-gray-400">({status.default_initial_window.weeks} weeks)</span>
						</span>
					</div>
					<div>
						<span class="text-gray-500 dark:text-gray-400">Background Window:</span>
						<span class="ml-2 font-mono text-gray-900 dark:text-white">
							{status.default_background_window.start} to {status.default_background_window.end}
							<span class="text-gray-400">({status.default_background_window.weeks} weeks)</span>
						</span>
					</div>
				</div>
			</div>

			<!-- Connections -->
			<div class="mb-6 bg-white dark:bg-zinc-800 border border-gray-200 dark:border-zinc-700 rounded-lg p-4">
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-3">
					Connections ({status.connections.length})
				</h2>
				{#if status.connections.length === 0}
					<p class="text-gray-500 dark:text-gray-400">No calendar connections</p>
				{:else}
					<div class="overflow-x-auto">
						<table class="min-w-full text-sm">
							<thead>
								<tr class="text-left text-gray-500 dark:text-gray-400 border-b border-gray-200 dark:border-zinc-700">
									<th class="pb-2 font-medium">ID</th>
									<th class="pb-2 font-medium">Provider</th>
									<th class="pb-2 font-medium">Last Synced</th>
									<th class="pb-2 font-medium">Status</th>
								</tr>
							</thead>
							<tbody>
								{#each status.connections as conn}
									<tr class="border-b border-gray-100 dark:border-zinc-700/50">
										<td class="py-2 font-mono text-xs text-gray-600 dark:text-gray-300">{conn.id.slice(0, 8)}...</td>
										<td class="py-2 text-gray-900 dark:text-white capitalize">{conn.provider}</td>
										<td class="py-2 text-gray-600 dark:text-gray-300" title={formatDate(conn.last_synced_at)}>
											{formatRelative(conn.last_synced_at)}
										</td>
										<td class="py-2">
											<span class="px-2 py-0.5 rounded text-xs font-medium {conn.is_stale ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300' : 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300'}">
												{conn.is_stale ? 'Stale' : 'Fresh'}
											</span>
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</div>

			<!-- Calendars -->
			<div class="bg-white dark:bg-zinc-800 border border-gray-200 dark:border-zinc-700 rounded-lg p-4">
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-3">
					Calendars ({status.calendars.length})
				</h2>
				{#if status.calendars.length === 0}
					<p class="text-gray-500 dark:text-gray-400">No calendars synced</p>
				{:else}
					<div class="space-y-4">
						{#each status.calendars as cal}
							<div class="border border-gray-200 dark:border-zinc-700 rounded-lg p-3 {getStatusColor(cal)}">
								<div class="flex items-start justify-between mb-2">
									<div>
										<span class="font-medium text-gray-900 dark:text-white">{cal.name}</span>
										{#if cal.is_primary}
											<span class="ml-2 px-1.5 py-0.5 text-xs bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300 rounded">Primary</span>
										{/if}
										{#if !cal.is_selected}
											<span class="ml-2 px-1.5 py-0.5 text-xs bg-gray-100 text-gray-600 dark:bg-zinc-700 dark:text-gray-400 rounded">Not Selected</span>
										{/if}
									</div>
									<span class="px-2 py-0.5 rounded text-xs font-medium {
										cal.needs_reauth ? 'bg-red-200 text-red-800 dark:bg-red-900/50 dark:text-red-300' :
										cal.sync_failure_count > 0 ? 'bg-orange-200 text-orange-800 dark:bg-orange-900/50 dark:text-orange-300' :
										cal.is_stale ? 'bg-yellow-200 text-yellow-800 dark:bg-yellow-900/50 dark:text-yellow-300' :
										'bg-green-200 text-green-800 dark:bg-green-900/50 dark:text-green-300'
									}">
										{getStatusLabel(cal)}
									</span>
								</div>

								<div class="grid grid-cols-2 md:grid-cols-4 gap-2 text-xs">
									<div>
										<span class="text-gray-500 dark:text-gray-400">Water Marks:</span>
										<div class="font-mono text-gray-900 dark:text-white">
											{#if cal.min_synced_date && cal.max_synced_date}
												{cal.min_synced_date.slice(0, 10)} to {cal.max_synced_date.slice(0, 10)}
											{:else}
												<span class="text-gray-400">Not set</span>
											{/if}
										</div>
									</div>
									<div>
										<span class="text-gray-500 dark:text-gray-400">Synced Weeks:</span>
										<div class="font-mono text-gray-900 dark:text-white">{cal.synced_weeks}</div>
									</div>
									<div>
										<span class="text-gray-500 dark:text-gray-400">Last Synced:</span>
										<div class="font-mono text-gray-900 dark:text-white" title={formatDate(cal.last_synced_at)}>
											{formatRelative(cal.last_synced_at)}
										</div>
									</div>
									<div>
										<span class="text-gray-500 dark:text-gray-400">Sync Token:</span>
										<div class="font-mono text-gray-900 dark:text-white">
											{cal.sync_token_set ? 'Set' : 'Not set'}
										</div>
									</div>
								</div>

								<div class="mt-2 text-xs text-gray-500 dark:text-gray-400 font-mono">
									ID: {cal.id}
								</div>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Raw JSON (collapsible) -->
			<details class="mt-6">
				<summary class="cursor-pointer text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300">
					View Raw JSON
				</summary>
				<pre class="mt-2 p-4 bg-gray-100 dark:bg-zinc-900 rounded-lg overflow-x-auto text-xs font-mono text-gray-800 dark:text-gray-200">{JSON.stringify(status, null, 2)}</pre>
			</details>
		{/if}
	</div>
</AppShell>
