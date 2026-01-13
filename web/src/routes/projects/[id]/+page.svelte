<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Input, Modal } from '$lib/components/primitives';
	import { ProjectChip, FingerprintsEditor, BillingPeriodsSection } from '$lib/components/widgets';
	import { api } from '$lib/api/client';
	import type { Project } from '$lib/api/types';

	// Fetch state for the project (grouped to make it clear this is loaded data, not a copy from a parent array)
	let projectState = $state<{
		loading: boolean;
		error: string;
		data: Project | null;
	}>({ loading: true, error: '', data: null });

	// Convenience accessors
	const project = $derived(projectState.data);
	const loading = $derived(projectState.loading);
	const error = $derived(projectState.error);

	let saving = $state(false);

	// Form state
	let name = $state('');
	let shortCode = $state('');
	let client = $state('');
	let color = $state('');
	let isBillable = $state(true);
	let isArchived = $state(false);
	let isHiddenByDefault = $state(false);
	let doesNotAccumulateHours = $state(false);

	// Fingerprint fields
	let fingerprintDomains = $state<string[]>([]);
	let fingerprintEmails = $state<string[]>([]);
	let fingerprintKeywords = $state<string[]>([]);

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deleting = $state(false);

	const projectId = $derived($page.params.id);

	// Preview project with current form values
	const previewProject = $derived<Project | null>(
		project
			? {
					...project,
					name,
					short_code: shortCode || undefined,
					color
				}
			: null
	);

	async function loadProject() {
		if (!projectId) return;
		projectState.loading = true;
		projectState.error = '';
		try {
			const loadedProject = await api.getProject(projectId);
			projectState.data = loadedProject;
			// Initialize form state
			name = loadedProject.name;
			shortCode = loadedProject.short_code || '';
			client = loadedProject.client || '';
			color = loadedProject.color;
			isBillable = loadedProject.is_billable;
			isArchived = loadedProject.is_archived;
			isHiddenByDefault = loadedProject.is_hidden_by_default || false;
			doesNotAccumulateHours = loadedProject.does_not_accumulate_hours || false;
			fingerprintDomains = loadedProject.fingerprint_domains || [];
			fingerprintEmails = loadedProject.fingerprint_emails || [];
			fingerprintKeywords = loadedProject.fingerprint_keywords || [];
		} catch (e) {
			projectState.error = 'Failed to load project';
			console.error(e);
		} finally {
			projectState.loading = false;
		}
	}

	async function handleSave() {
		if (!projectId) return;
		saving = true;
		projectState.error = '';
		try {
			projectState.data = await api.updateProject(projectId, {
				name,
				short_code: shortCode || undefined,
				client: client || undefined,
				color,
				is_billable: isBillable,
				is_archived: isArchived,
				is_hidden_by_default: isHiddenByDefault,
				does_not_accumulate_hours: doesNotAccumulateHours,
				fingerprint_domains: fingerprintDomains,
				fingerprint_emails: fingerprintEmails,
				fingerprint_keywords: fingerprintKeywords
			});
			goto('/projects');
		} catch (e) {
			projectState.error = 'Failed to save project';
			console.error(e);
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!projectId) return;
		deleting = true;
		try {
			await api.deleteProject(projectId);
			goto('/projects');
		} catch (e: unknown) {
			projectState.error = e instanceof Error ? e.message : 'Failed to delete project';
			showDeleteModal = false;
		} finally {
			deleting = false;
		}
	}

	onMount(() => {
		loadProject();
	});
</script>

<svelte:head>
	<title>{project?.name || 'Project'} - Timesheet</title>
</svelte:head>

<AppShell>
	<div class="max-w-2xl mx-auto">
		<div class="mb-6">
			<a href="/projects" class="text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300 flex items-center gap-1">
				<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
				</svg>
				Back to Projects
			</a>
		</div>

		{#if loading}
			<div class="flex items-center justify-center py-12">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		{:else if error && !project}
			<div class="text-center py-12">
				<p class="text-red-600">{error}</p>
				<Button variant="secondary" onclick={loadProject}>Try again</Button>
			</div>
		{:else if project}
			<div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6">
				<div class="flex items-center justify-between mb-6">
					<div class="flex items-center gap-3">
						{#if previewProject}
							<ProjectChip project={previewProject} size="md" />
						{/if}
						<h1 class="text-xl font-semibold text-gray-900 dark:text-white">{project.name}</h1>
					</div>
					<Button variant="danger" size="sm" onclick={() => (showDeleteModal = true)}>
						Delete
					</Button>
				</div>

				{#if error}
					<div class="mb-4 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 px-4 py-3 rounded">
						{error}
					</div>
				{/if}

				<form class="space-y-6" onsubmit={(e) => { e.preventDefault(); handleSave(); }}>
					<Input
						type="text"
						label="Project name"
						bind:value={name}
						required
					/>

					<Input
						type="text"
						label="Short code"
						bind:value={shortCode}
						placeholder="e.g., ACM"
					/>

					<Input
						type="text"
						label="Client"
						bind:value={client}
						placeholder="e.g., Acme Corp"
					/>

					<div>
						<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Color</label>
						<div class="flex items-center gap-3">
							<input
								type="color"
								bind:value={color}
								class="h-10 w-20 rounded border border-gray-300 dark:border-gray-600 cursor-pointer"
							/>
							<span class="text-sm text-gray-500 dark:text-gray-400">{color}</span>
						</div>
					</div>

					<div class="space-y-3">
						<div class="flex items-center gap-2">
							<input
								type="checkbox"
								id="billable"
								bind:checked={isBillable}
								class="rounded border-gray-300 dark:border-gray-600 text-primary-600 focus:ring-primary-500 dark:bg-gray-700"
							/>
							<label for="billable" class="text-sm text-gray-700 dark:text-gray-300">Billable project</label>
						</div>

						<div class="flex items-center gap-2">
							<input
								type="checkbox"
								id="archived"
								bind:checked={isArchived}
								class="rounded border-gray-300 dark:border-gray-600 text-primary-600 focus:ring-primary-500 dark:bg-gray-700"
							/>
							<label for="archived" class="text-sm text-gray-700 dark:text-gray-300">Archived</label>
						</div>

						<div class="flex items-center gap-2">
							<input
								type="checkbox"
								id="hidden"
								bind:checked={isHiddenByDefault}
								class="rounded border-gray-300 dark:border-gray-600 text-primary-600 focus:ring-primary-500 dark:bg-gray-700"
							/>
							<label for="hidden" class="text-sm text-gray-700 dark:text-gray-300">Hidden by default</label>
						</div>

						<div class="flex items-center gap-2">
							<input
								type="checkbox"
								id="noAccumulate"
								bind:checked={doesNotAccumulateHours}
								class="rounded border-gray-300 dark:border-gray-600 text-primary-600 focus:ring-primary-500 dark:bg-gray-700"
							/>
							<label for="noAccumulate" class="text-sm text-gray-700 dark:text-gray-300">
								Does not accumulate hours
								<span class="text-gray-400 dark:text-gray-500">(e.g., lunch, PTO)</span>
							</label>
						</div>
					</div>

					<FingerprintsEditor
						bind:domains={fingerprintDomains}
						bind:emails={fingerprintEmails}
						bind:keywords={fingerprintKeywords}
					/>

					{#if project.is_billable}
						<BillingPeriodsSection projectId={projectId} />
					{/if}

					<div class="flex justify-end pt-4">
						<Button type="submit" loading={saving}>
							Save Changes
						</Button>
					</div>
				</form>
			</div>
		{/if}
	</div>

	<!-- Delete confirmation modal -->
	<Modal bind:open={showDeleteModal} title="Delete Project">
		<p class="text-gray-600 dark:text-gray-300">
			Are you sure you want to delete <strong class="dark:text-white">{project?.name}</strong>? This cannot be undone.
		</p>
		<p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
			If this project has time entries or invoices, you won't be able to delete it.
		</p>

		{#snippet footer()}
			<Button variant="secondary" onclick={() => (showDeleteModal = false)}>
				Cancel
			</Button>
			<Button variant="danger" loading={deleting} onclick={handleDelete}>
				Delete Project
			</Button>
		{/snippet}
	</Modal>
</AppShell>
