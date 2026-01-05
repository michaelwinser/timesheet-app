<script lang="ts">
	import { onMount } from 'svelte';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Modal } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import { auth, theme } from '$lib/stores';
	import type { CalendarConnection, Calendar } from '$lib/api/types';

	let connections = $state<CalendarConnection[]>([]);
	let loading = $state(true);
	let syncing = $state<string | null>(null);
	let error = $state('');
	let successMessage = $state('');

	// Disconnect modal
	let showDisconnectModal = $state(false);
	let disconnectingId = $state<string | null>(null);
	let disconnecting = $state(false);

	// Calendar sources modal
	let showSourcesModal = $state(false);
	let sourcesConnectionId = $state<string | null>(null);
	let calendars = $state<Calendar[]>([]);
	let selectedCalendarIds = $state<Set<string>>(new Set());
	let loadingSources = $state(false);
	let savingSources = $state(false);

	async function loadConnections() {
		loading = true;
		try {
			connections = await api.listCalendarConnections();
		} catch (e) {
			console.error('Failed to load connections:', e);
		} finally {
			loading = false;
		}
	}

	async function handleConnectGoogle() {
		try {
			const { url } = await api.googleAuthorize();
			// Store current URL for redirect back
			sessionStorage.setItem('oauth_return', window.location.href);
			window.location.href = url;
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to start OAuth flow';
		}
	}

	async function handleSync(connectionId: string) {
		syncing = connectionId;
		error = '';
		successMessage = '';
		try {
			const result = await api.syncCalendar(connectionId);
			successMessage = `Synced: ${result.events_created} new, ${result.events_updated} updated, ${result.events_orphaned} orphaned`;
			// Update last_synced_at
			connections = connections.map((c) =>
				c.id === connectionId ? { ...c, last_synced_at: new Date().toISOString() } : c
			);
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to sync calendar';
		} finally {
			syncing = null;
		}
	}

	function openDisconnectModal(id: string) {
		disconnectingId = id;
		showDisconnectModal = true;
	}

	async function handleDisconnect() {
		if (!disconnectingId) return;
		disconnecting = true;
		try {
			await api.deleteCalendarConnection(disconnectingId);
			connections = connections.filter((c) => c.id !== disconnectingId);
			showDisconnectModal = false;
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to disconnect calendar';
		} finally {
			disconnecting = false;
			disconnectingId = null;
		}
	}

	function formatDate(dateStr: string | null | undefined): string {
		if (!dateStr) return 'Never';
		return new Date(dateStr).toLocaleString();
	}

	async function openSourcesModal(connectionId: string) {
		sourcesConnectionId = connectionId;
		showSourcesModal = true;
		loadingSources = true;
		error = '';

		try {
			calendars = await api.listCalendarSources(connectionId);
			selectedCalendarIds = new Set(calendars.filter((c) => c.is_selected).map((c) => c.id));
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to load calendar sources';
			showSourcesModal = false;
		} finally {
			loadingSources = false;
		}
	}

	function toggleCalendar(id: string) {
		const newSet = new Set(selectedCalendarIds);
		if (newSet.has(id)) {
			newSet.delete(id);
		} else {
			newSet.add(id);
		}
		selectedCalendarIds = newSet;
	}

	async function saveCalendarSources() {
		if (!sourcesConnectionId) return;
		savingSources = true;
		error = '';

		try {
			calendars = await api.updateCalendarSources(sourcesConnectionId, {
				calendar_ids: Array.from(selectedCalendarIds)
			});
			selectedCalendarIds = new Set(calendars.filter((c) => c.is_selected).map((c) => c.id));
			showSourcesModal = false;
			successMessage = 'Calendar sources updated';
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to update calendar sources';
		} finally {
			savingSources = false;
		}
	}

	onMount(() => {
		loadConnections();
	});
</script>

<svelte:head>
	<title>Settings - Timesheet</title>
</svelte:head>

<AppShell>
	<div class="max-w-2xl mx-auto">
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white mb-8">Settings</h1>

		<!-- Appearance section -->
		<section class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6 mb-6">
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Appearance</h2>
			<div class="flex items-center justify-between">
				<div>
					<p class="text-sm font-medium text-gray-900 dark:text-white">Theme</p>
					<p class="text-sm text-gray-500 dark:text-gray-400">Choose between light and dark mode</p>
				</div>
				<button
					type="button"
					onclick={() => theme.toggle()}
					class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:focus:ring-offset-gray-800
						{$theme === 'dark' ? 'bg-primary-600' : 'bg-gray-200'}"
				>
					<span class="sr-only">Toggle theme</span>
					<span
						class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform
							{$theme === 'dark' ? 'translate-x-6' : 'translate-x-1'}"
					></span>
				</button>
			</div>
		</section>

		<!-- Profile section -->
		<section class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6 mb-6">
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Profile</h2>

			{#if $auth.user}
				<div class="space-y-2">
					<p class="text-sm">
						<span class="text-gray-500 dark:text-gray-400">Name:</span>
						<span class="ml-2 text-gray-900 dark:text-white">{$auth.user.name}</span>
					</p>
					<p class="text-sm">
						<span class="text-gray-500 dark:text-gray-400">Email:</span>
						<span class="ml-2 text-gray-900 dark:text-white">{$auth.user.email}</span>
					</p>
				</div>
			{/if}
		</section>

		<!-- Calendar connections section -->
		<section class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6">
			<div class="flex items-center justify-between mb-4">
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white">Calendar Connections</h2>
			</div>

			{#if error}
				<div class="mb-4 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 px-4 py-3 rounded text-sm">
					{error}
				</div>
			{/if}

			{#if successMessage}
				<div class="mb-4 bg-green-50 dark:bg-green-900/30 border border-green-200 dark:border-green-800 text-green-700 dark:text-green-300 px-4 py-3 rounded text-sm">
					{successMessage}
				</div>
			{/if}

			{#if loading}
				<div class="flex items-center justify-center py-8">
					<div class="animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600"></div>
				</div>
			{:else}
				{#if connections.length > 0}
					<div class="space-y-4 mb-6">
						{#each connections as connection (connection.id)}
							<div class="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
								<div class="flex items-center gap-3">
									{#if connection.provider === 'google'}
										<svg class="w-6 h-6" viewBox="0 0 24 24">
											<path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
											<path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
											<path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
											<path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
										</svg>
									{/if}
									<div>
										<div class="font-medium text-gray-900 dark:text-white capitalize">{connection.provider} Calendar</div>
										<div class="text-sm text-gray-500 dark:text-gray-400">
											Last synced: {formatDate(connection.last_synced_at)}
										</div>
									</div>
								</div>
								<div class="flex items-center gap-2">
									<Button
										variant="secondary"
										size="sm"
										onclick={() => openSourcesModal(connection.id)}
									>
										Calendars
									</Button>
									<Button
										variant="secondary"
										size="sm"
										loading={syncing === connection.id}
										onclick={() => handleSync(connection.id)}
									>
										Sync
									</Button>
									<Button
										variant="ghost"
										size="sm"
										onclick={() => openDisconnectModal(connection.id)}
									>
										Disconnect
									</Button>
								</div>
							</div>
						{/each}
					</div>
				{:else}
					<p class="text-gray-500 dark:text-gray-400 mb-6">No calendars connected.</p>
				{/if}

				<Button variant="secondary" onclick={handleConnectGoogle}>
					<svg class="w-5 h-5 mr-2" viewBox="0 0 24 24">
						<path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
						<path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
						<path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
						<path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
					</svg>
					Connect Google Calendar
				</Button>
			{/if}
		</section>
	</div>

	<!-- Disconnect confirmation modal -->
	<Modal bind:open={showDisconnectModal} title="Disconnect Calendar">
		<p class="text-gray-600 dark:text-gray-300">
			Are you sure you want to disconnect this calendar? Synced events will remain but no new events will be imported.
		</p>

		{#snippet footer()}
			<Button variant="secondary" onclick={() => (showDisconnectModal = false)}>
				Cancel
			</Button>
			<Button variant="danger" loading={disconnecting} onclick={handleDisconnect}>
				Disconnect
			</Button>
		{/snippet}
	</Modal>

	<!-- Calendar sources modal -->
	<Modal bind:open={showSourcesModal} title="Select Calendars to Sync">
		{#if loadingSources}
			<div class="flex items-center justify-center py-8">
				<div class="animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600"></div>
			</div>
		{:else if calendars.length === 0}
			<p class="text-gray-500 dark:text-gray-400 py-4">No calendars found.</p>
		{:else}
			<div class="space-y-2 max-h-80 overflow-y-auto">
				{#each calendars as calendar (calendar.id)}
					<label
						class="flex items-center gap-3 p-3 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer"
					>
						<input
							type="checkbox"
							checked={selectedCalendarIds.has(calendar.id)}
							onchange={() => toggleCalendar(calendar.id)}
							class="h-4 w-4 rounded border-gray-300 dark:border-gray-600 text-primary-600 focus:ring-primary-500 dark:bg-gray-700"
						/>
						<div
							class="w-3 h-3 rounded-full flex-shrink-0"
							style="background-color: {calendar.color || '#9CA3AF'}"
						></div>
						<div class="flex-1 min-w-0">
							<div class="font-medium text-gray-900 dark:text-white truncate">
								{calendar.name}
								{#if calendar.is_primary}
									<span class="ml-1 text-xs text-gray-500 dark:text-gray-400">(primary)</span>
								{/if}
							</div>
						</div>
					</label>
				{/each}
			</div>
		{/if}

		{#snippet footer()}
			<Button variant="secondary" onclick={() => (showSourcesModal = false)}>
				Cancel
			</Button>
			<Button
				variant="primary"
				loading={savingSources}
				disabled={loadingSources}
				onclick={saveCalendarSources}
			>
				Save Changes
			</Button>
		{/snippet}
	</Modal>
</AppShell>
