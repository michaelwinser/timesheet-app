<script lang="ts">
	import { Button, Modal } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import type { ApiKey } from '$lib/api/types';

	interface Props {
		error: string;
		successMessage: string;
	}

	let { error = $bindable(''), successMessage = $bindable('') }: Props = $props();

	// API Keys state
	let apiKeys = $state<ApiKey[]>([]);
	let loading = $state(true);
	let showCreateKeyModal = $state(false);
	let newKeyName = $state('');
	let creatingKey = $state(false);
	let newlyCreatedKey = $state<string | null>(null);
	let showRevokeModal = $state(false);
	let revokingKeyId = $state<string | null>(null);
	let revokingKey = $state(false);
	let keyCopied = $state(false);

	async function loadApiKeys() {
		loading = true;
		try {
			apiKeys = await api.listApiKeys();
		} catch (e) {
			console.error('Failed to load API keys:', e);
		} finally {
			loading = false;
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

	// Load on mount
	$effect(() => {
		loadApiKeys();
	});
</script>

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

	{#if loading}
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
