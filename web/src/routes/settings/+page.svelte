<script lang="ts">
	import { onMount } from 'svelte';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Modal } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import { auth, theme } from '$lib/stores';
	import type { CalendarConnection, Calendar, ApiKey, ConfigImport } from '$lib/api/types';

	let connections = $state<CalendarConnection[]>([]);
	let loading = $state(true);
	let syncing = $state<string | null>(null);
	let error = $state('');
	let successMessage = $state('');

	// Config export/import
	let exporting = $state(false);
	let importing = $state(false);
	let showImportModal = $state(false);
	let importFile = $state<File | null>(null);
	let importPreview = $state<ConfigImport | null>(null);
	let importError = $state('');

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

	// API Keys
	let apiKeys = $state<ApiKey[]>([]);
	let loadingApiKeys = $state(true);
	let showCreateKeyModal = $state(false);
	let newKeyName = $state('');
	let creatingKey = $state(false);
	let newlyCreatedKey = $state<string | null>(null);
	let showRevokeModal = $state(false);
	let revokingKeyId = $state<string | null>(null);
	let revokingKey = $state(false);
	let keyCopied = $state(false);

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

			// Trigger sync for newly selected calendars (per PRD: additional calendars → background sync)
			if (selectedCalendarIds.size > 0) {
				handleSync(sourcesConnectionId);
			}
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to update calendar sources';
		} finally {
			savingSources = false;
		}
	}

	// API Key functions
	async function loadApiKeys() {
		loadingApiKeys = true;
		try {
			apiKeys = await api.listApiKeys();
		} catch (e) {
			console.error('Failed to load API keys:', e);
		} finally {
			loadingApiKeys = false;
		}
	}

	function openCreateKeyModal() {
		newKeyName = '';
		newlyCreatedKey = null;
		keyCopied = false;
		showCreateKeyModal = true;
	}

	async function handleCreateKey() {
		if (!newKeyName.trim()) return;
		creatingKey = true;
		error = '';

		try {
			const result = await api.createApiKey({ name: newKeyName.trim() });
			newlyCreatedKey = result.key;
			apiKeys = [result, ...apiKeys];
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to create API key';
			showCreateKeyModal = false;
		} finally {
			creatingKey = false;
		}
	}

	async function copyKeyToClipboard() {
		if (newlyCreatedKey) {
			await navigator.clipboard.writeText(newlyCreatedKey);
			keyCopied = true;
			setTimeout(() => (keyCopied = false), 2000);
		}
	}

	function closeCreateKeyModal() {
		showCreateKeyModal = false;
		newlyCreatedKey = null;
		newKeyName = '';
	}

	function openRevokeModal(id: string) {
		revokingKeyId = id;
		showRevokeModal = true;
	}

	async function handleRevokeKey() {
		if (!revokingKeyId) return;
		revokingKey = true;
		error = '';

		try {
			await api.deleteApiKey(revokingKeyId);
			apiKeys = apiKeys.filter((k) => k.id !== revokingKeyId);
			showRevokeModal = false;
			successMessage = 'API key revoked';
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to revoke API key';
		} finally {
			revokingKey = false;
			revokingKeyId = null;
		}
	}

	function formatKeyDate(dateStr: string | null | undefined): string {
		if (!dateStr) return 'Never';
		return new Date(dateStr).toLocaleDateString();
	}

	// Config export/import functions
	async function handleExport() {
		exporting = true;
		error = '';
		try {
			const config = await api.exportConfig(true); // Include archived
			const blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `timesheet-config-${new Date().toISOString().split('T')[0]}.json`;
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			URL.revokeObjectURL(url);
			successMessage = `Exported ${config.projects.length} projects and ${config.rules.length} rules`;
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to export configuration';
		} finally {
			exporting = false;
		}
	}

	function openImportModal() {
		importFile = null;
		importPreview = null;
		importError = '';
		showImportModal = true;
	}

	async function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;

		importFile = file;
		importError = '';

		try {
			const text = await file.text();
			const data = JSON.parse(text) as ConfigImport;

			// Basic validation
			if (!Array.isArray(data.projects) || !Array.isArray(data.rules)) {
				throw new Error('Invalid config file format');
			}

			importPreview = data;
		} catch (e: unknown) {
			importError = e instanceof Error ? e.message : 'Failed to parse file';
			importPreview = null;
		}
	}

	async function handleImport() {
		if (!importPreview) return;

		importing = true;
		importError = '';

		try {
			const result = await api.importConfig(importPreview);
			showImportModal = false;

			const parts = [];
			if (result.projects_created > 0) parts.push(`${result.projects_created} projects created`);
			if (result.projects_updated > 0) parts.push(`${result.projects_updated} projects updated`);
			if (result.rules_created > 0) parts.push(`${result.rules_created} rules created`);
			if (result.rules_updated > 0) parts.push(`${result.rules_updated} rules updated`);
			if (result.rules_skipped && result.rules_skipped > 0) parts.push(`${result.rules_skipped} rules skipped`);

			successMessage = `Import complete: ${parts.join(', ')}`;

			if (result.warnings && result.warnings.length > 0) {
				console.warn('Import warnings:', result.warnings);
			}
		} catch (e: unknown) {
			importError = e instanceof Error ? e.message : 'Failed to import configuration';
		} finally {
			importing = false;
		}
	}

	onMount(async () => {
		await loadConnections();
		loadApiKeys();

		// Check if we just completed OAuth - trigger initial sync for new connection
		const urlParams = new URLSearchParams(window.location.search);
		if (urlParams.get('connected') === 'google') {
			// Clear the query param to prevent re-triggering on refresh
			window.history.replaceState({}, '', window.location.pathname);

			// Find connections that haven't been synced yet and have selected calendars
			// (the primary calendar is auto-selected on connection)
			const unsyncedConnection = connections.find((c) => !c.last_synced_at);
			if (unsyncedConnection) {
				handleSync(unsyncedConnection.id);
			}
		}
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
				<p class="text-sm font-medium text-gray-900 dark:text-white">Dark Mode</p>
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

		<!-- API Keys section -->
		<section class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6 mt-6">
			<div class="flex items-center justify-between mb-4">
				<div>
					<h2 class="text-lg font-semibold text-gray-900 dark:text-white">API Keys</h2>
					<p class="text-sm text-gray-500 dark:text-gray-400">
						Create keys for programmatic access (Claude Code, scripts, integrations)
					</p>
				</div>
				<Button variant="secondary" size="sm" onclick={openCreateKeyModal}>
					Create Key
				</Button>
			</div>

			{#if loadingApiKeys}
				<div class="flex items-center justify-center py-8">
					<div class="animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600"></div>
				</div>
			{:else if apiKeys.length === 0}
				<p class="text-gray-500 dark:text-gray-400 text-sm py-4">
					No API keys yet. Create one to use with Claude Code or other integrations.
				</p>
			{:else}
				<div class="space-y-3">
					{#each apiKeys as key (key.id)}
						<div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
							<div class="flex-1 min-w-0">
								<div class="font-medium text-gray-900 dark:text-white">{key.name}</div>
								<div class="text-sm text-gray-500 dark:text-gray-400 font-mono">
									{key.key_prefix}...
								</div>
								<div class="text-xs text-gray-400 dark:text-gray-500 mt-1">
									Created {formatKeyDate(key.created_at)}
									{#if key.last_used_at}
										&bull; Last used {formatKeyDate(key.last_used_at)}
									{/if}
								</div>
							</div>
							<Button variant="ghost" size="sm" onclick={() => openRevokeModal(key.id)}>
								Revoke
							</Button>
						</div>
					{/each}
				</div>
			{/if}
		</section>

		<!-- Data Management section -->
		<section class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6 mt-6">
			<div class="mb-4">
				<h2 class="text-lg font-semibold text-gray-900 dark:text-white">Data Management</h2>
				<p class="text-sm text-gray-500 dark:text-gray-400">
					Export or import your projects and classification rules
				</p>
			</div>

			<div class="flex flex-col sm:flex-row gap-3">
				<Button variant="secondary" loading={exporting} onclick={handleExport}>
					<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
					</svg>
					Export Config
				</Button>
				<Button variant="secondary" onclick={openImportModal}>
					<svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12"/>
					</svg>
					Import Config
				</Button>
			</div>

			<p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
				Exports include all projects (with settings) and classification rules as JSON.
			</p>
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

	<!-- Create API Key modal -->
	<Modal bind:open={showCreateKeyModal} title={newlyCreatedKey ? 'API Key Created' : 'Create API Key'}>
		{#if newlyCreatedKey}
			<div class="space-y-4">
				<div class="bg-yellow-50 dark:bg-yellow-900/30 border border-yellow-200 dark:border-yellow-800 text-yellow-800 dark:text-yellow-200 px-4 py-3 rounded text-sm">
					Copy this key now. You won't be able to see it again.
				</div>
				<div class="flex items-center gap-2">
					<input
						type="text"
						readonly
						value={newlyCreatedKey}
						class="flex-1 px-3 py-2 bg-gray-100 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md font-mono text-sm text-gray-900 dark:text-white"
					/>
					<Button variant="secondary" onclick={copyKeyToClipboard}>
						{keyCopied ? 'Copied!' : 'Copy'}
					</Button>
				</div>
			</div>
		{:else}
			<div class="space-y-4">
				<div>
					<label for="key-name" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
						Key Name
					</label>
					<input
						id="key-name"
						type="text"
						bind:value={newKeyName}
						placeholder="e.g., Claude Code"
						class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 dark:bg-gray-700 dark:text-white"
					/>
					<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
						A memorable name to identify this key
					</p>
				</div>
			</div>
		{/if}

		{#snippet footer()}
			{#if newlyCreatedKey}
				<Button variant="primary" onclick={closeCreateKeyModal}>
					Done
				</Button>
			{:else}
				<Button variant="secondary" onclick={closeCreateKeyModal}>
					Cancel
				</Button>
				<Button
					variant="primary"
					loading={creatingKey}
					disabled={!newKeyName.trim()}
					onclick={handleCreateKey}
				>
					Create Key
				</Button>
			{/if}
		{/snippet}
	</Modal>

	<!-- Revoke API Key modal -->
	<Modal bind:open={showRevokeModal} title="Revoke API Key">
		<p class="text-gray-600 dark:text-gray-300">
			Are you sure you want to revoke this API key? Any applications using this key will immediately lose access.
		</p>

		{#snippet footer()}
			<Button variant="secondary" onclick={() => (showRevokeModal = false)}>
				Cancel
			</Button>
			<Button variant="danger" loading={revokingKey} onclick={handleRevokeKey}>
				Revoke Key
			</Button>
		{/snippet}
	</Modal>

	<!-- Import Config modal -->
	<Modal bind:open={showImportModal} title="Import Configuration">
		<div class="space-y-4">
			<div>
				<label for="config-file" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
					Select Config File
				</label>
				<input
					id="config-file"
					type="file"
					accept=".json"
					onchange={handleFileSelect}
					class="block w-full text-sm text-gray-500 dark:text-gray-400
						file:mr-4 file:py-2 file:px-4
						file:rounded-md file:border-0
						file:text-sm file:font-medium
						file:bg-gray-100 file:text-gray-700
						dark:file:bg-gray-700 dark:file:text-gray-200
						hover:file:bg-gray-200 dark:hover:file:bg-gray-600
						cursor-pointer"
				/>
			</div>

			{#if importError}
				<div class="bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 px-4 py-3 rounded text-sm">
					{importError}
				</div>
			{/if}

			{#if importPreview}
				<div class="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
					<h4 class="text-sm font-medium text-gray-900 dark:text-white mb-2">Preview</h4>
					<div class="text-sm text-gray-600 dark:text-gray-300 space-y-1">
						<p>{importPreview.projects.length} project(s)</p>
						<p>{importPreview.rules.length} rule(s)</p>
					</div>
					<p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
						Existing projects and rules will be updated by name/query.
					</p>
				</div>
			{/if}
		</div>

		{#snippet footer()}
			<Button variant="secondary" onclick={() => (showImportModal = false)}>
				Cancel
			</Button>
			<Button
				variant="primary"
				loading={importing}
				disabled={!importPreview}
				onclick={handleImport}
			>
				Import
			</Button>
		{/snippet}
	</Modal>
</AppShell>
