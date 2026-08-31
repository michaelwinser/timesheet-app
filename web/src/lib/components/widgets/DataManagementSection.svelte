<script lang="ts">
	import { Button, Modal } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import type { ConfigImport } from '$lib/api/types';

	interface Props {
		error: string;
		successMessage: string;
	}

	let { error = $bindable(''), successMessage = $bindable('') }: Props = $props();

	// Export/import state
	let exporting = $state(false);
	let importing = $state(false);
	let showImportModal = $state(false);
	let importFile = $state<File | null>(null);
	let importPreview = $state<ConfigImport | null>(null);
	let importError = $state('');

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
</script>

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
