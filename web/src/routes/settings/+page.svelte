<script lang="ts">
	import { onMount } from 'svelte';
	import AppShell from '$lib/components/AppShell.svelte';
	import { auth, theme } from '$lib/stores';
	import {
		CalendarConnectionsSection,
		ApiKeysSection,
		DataManagementSection,
		IngestionFiltersSection
	} from '$lib/components/widgets';

	// Shared error/success state for child components
	let error = $state('');
	let successMessage = $state('');

	// Reference to calendar connections for OAuth callback
	let calendarSection: ReturnType<typeof CalendarConnectionsSection> | undefined = $state();

	onMount(() => {
		// Check if we just completed OAuth - trigger initial sync for new connection
		const urlParams = new URLSearchParams(window.location.search);
		if (urlParams.get('connected') === 'google') {
			// Clear the query param to prevent re-triggering on refresh
			window.history.replaceState({}, '', window.location.pathname);

			// Find connections that haven't been synced yet and have selected calendars
			// (the primary calendar is auto-selected on connection)
			// Wait a tick for the component to load connections
			setTimeout(() => {
				const connections = calendarSection?.getConnections() ?? [];
				const unsyncedConnection = connections.find((c) => !c.last_synced_at);
				if (unsyncedConnection) {
					calendarSection?.handleSync(unsyncedConnection.id);
				}
			}, 500);
		}
	});
</script>

<svelte:head>
	<title>Settings - Timesheet</title>
</svelte:head>

<AppShell>
	<div class="max-w-2xl mx-auto">
		<h1 class="text-2xl font-bold text-gray-900 dark:text-white mb-8">Settings</h1>

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

		<!-- Calendar connections -->
		<CalendarConnectionsSection
			bind:this={calendarSection}
			bind:error
			bind:successMessage
		/>

		<!-- API Keys -->
		<IngestionFiltersSection bind:error bind:successMessage />

		<ApiKeysSection bind:error bind:successMessage />

		<!-- Data Management -->
		<DataManagementSection bind:error bind:successMessage />
	</div>
</AppShell>
