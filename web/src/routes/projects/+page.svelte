<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Modal, Input } from '$lib/components/primitives';
	import { ProjectListItem } from '$lib/components/widgets';
	import { api } from '$lib/api/client';
	import type { Project } from '$lib/api/types';

	let projects = $state<Project[]>([]);
	let loading = $state(true);
	let showArchived = $state(false);

	// Create modal state
	let showCreateModal = $state(false);
	let createName = $state('');
	let createCode = $state('');
	let createColor = $state('#3B82F6');
	let createBillable = $state(true);
	let createSubmitting = $state(false);

	const activeProjects = $derived(projects.filter((p) => !p.is_archived));
	const archivedProjects = $derived(projects.filter((p) => p.is_archived));

	async function loadProjects() {
		loading = true;
		try {
			projects = await api.listProjects(true);
		} catch (e) {
			console.error('Failed to load projects:', e);
		} finally {
			loading = false;
		}
	}

	function openCreateModal() {
		createName = '';
		createCode = '';
		createColor = '#3B82F6';
		createBillable = true;
		showCreateModal = true;
	}

	async function handleCreate() {
		if (!createName.trim()) return;
		createSubmitting = true;
		try {
			const newProject = await api.createProject({
				name: createName.trim(),
				short_code: createCode.trim() || undefined,
				color: createColor,
				is_billable: createBillable
			});
			projects = [...projects, newProject];
			showCreateModal = false;
		} catch (e) {
			console.error('Failed to create project:', e);
		} finally {
			createSubmitting = false;
		}
	}

	onMount(() => {
		loadProjects();
	});
</script>

<svelte:head>
	<title>Projects - Timesheet</title>
</svelte:head>

<AppShell>
	<div class="max-w-3xl mx-auto">
		<div class="flex items-center justify-between mb-6">
			<h1 class="text-2xl font-bold text-gray-900">Projects</h1>
			<Button onclick={openCreateModal}>
				<svg class="w-5 h-5 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
				</svg>
				New Project
			</Button>
		</div>

		{#if loading}
			<div class="flex items-center justify-center py-12">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		{:else}
			<div class="space-y-3">
				{#each activeProjects as project (project.id)}
					<ProjectListItem {project} onclick={() => goto(`/projects/${project.id}`)} />
				{/each}

				{#if activeProjects.length === 0}
					<div class="text-center py-12 text-gray-500">
						<p>No projects yet.</p>
						<button
							type="button"
							class="text-primary-600 hover:text-primary-700 mt-2"
							onclick={openCreateModal}
						>
							Create your first project
						</button>
					</div>
				{/if}

				{#if archivedProjects.length > 0}
					<div class="pt-6">
						<button
							type="button"
							class="flex items-center gap-2 text-sm text-gray-500 hover:text-gray-700"
							onclick={() => (showArchived = !showArchived)}
						>
							<svg
								class="w-4 h-4 transition-transform {showArchived ? 'rotate-90' : ''}"
								fill="none"
								stroke="currentColor"
								viewBox="0 0 24 24"
							>
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
							</svg>
							Archived ({archivedProjects.length})
						</button>

						{#if showArchived}
							<div class="mt-3 space-y-3">
								{#each archivedProjects as project (project.id)}
									<ProjectListItem {project} onclick={() => goto(`/projects/${project.id}`)} />
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<!-- Create modal -->
	<Modal bind:open={showCreateModal} title="New Project">
		<form class="space-y-4" onsubmit={(e) => { e.preventDefault(); handleCreate(); }}>
			<Input
				type="text"
				label="Project name"
				bind:value={createName}
				required
				placeholder="e.g., Acme Corp Website"
			/>

			<Input
				type="text"
				label="Short code (optional)"
				bind:value={createCode}
				placeholder="e.g., ACM"
			/>

			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">Color</label>
				<div class="flex items-center gap-3">
					<input
						type="color"
						bind:value={createColor}
						class="h-10 w-20 rounded border cursor-pointer"
					/>
					<span class="text-sm text-gray-500">{createColor}</span>
				</div>
			</div>

			<div class="flex items-center gap-2">
				<input
					type="checkbox"
					id="billable"
					bind:checked={createBillable}
					class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
				/>
				<label for="billable" class="text-sm text-gray-700">Billable project</label>
			</div>

			<div class="flex justify-end gap-3 pt-4">
				<Button variant="secondary" onclick={() => (showCreateModal = false)}>
					Cancel
				</Button>
				<Button type="submit" loading={createSubmitting}>
					Create Project
				</Button>
			</div>
		</form>
	</Modal>
</AppShell>
