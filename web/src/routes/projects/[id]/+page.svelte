<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Input, Modal } from '$lib/components/primitives';
	import { ProjectChip } from '$lib/components/widgets';
	import { api } from '$lib/api/client';
	import type { Project } from '$lib/api/types';

	let project = $state<Project | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');

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
	let newDomain = $state('');
	let newEmail = $state('');
	let newKeyword = $state('');

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
		loading = true;
		error = '';
		try {
			project = await api.getProject(projectId);
			// Initialize form state
			name = project.name;
			shortCode = project.short_code || '';
			client = project.client || '';
			color = project.color;
			isBillable = project.is_billable;
			isArchived = project.is_archived;
			isHiddenByDefault = project.is_hidden_by_default || false;
			doesNotAccumulateHours = project.does_not_accumulate_hours || false;
			fingerprintDomains = project.fingerprint_domains || [];
			fingerprintEmails = project.fingerprint_emails || [];
			fingerprintKeywords = project.fingerprint_keywords || [];
		} catch (e) {
			error = 'Failed to load project';
			console.error(e);
		} finally {
			loading = false;
		}
	}

	async function handleSave() {
		if (!projectId) return;
		saving = true;
		error = '';
		try {
			project = await api.updateProject(projectId, {
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
		} catch (e) {
			error = 'Failed to save project';
			console.error(e);
		} finally {
			saving = false;
		}
	}

	// Parse email from formats like "Name <email@domain.com>" or plain "email@domain.com"
	function parseEmail(input: string): string | null {
		const trimmed = input.trim().toLowerCase();
		if (!trimmed) return null;

		// Match "Name <email@domain.com>" format
		const angleMatch = trimmed.match(/<([^>]+@[^>]+)>/);
		if (angleMatch) {
			return angleMatch[1];
		}

		// Check if it's a plain email address
		if (trimmed.includes('@') && !trimmed.includes(' ')) {
			return trimmed;
		}

		return null;
	}

	// Extract domain from email address
	function extractDomain(email: string): string | null {
		const atIndex = email.lastIndexOf('@');
		if (atIndex > 0 && atIndex < email.length - 1) {
			return email.substring(atIndex + 1).toLowerCase();
		}
		return null;
	}

	// Parse input that might contain emails and extract domains
	function parseDomains(input: string): string[] {
		const domains: string[] = [];
		const parts = input.split(',');

		for (const part of parts) {
			const trimmed = part.trim().toLowerCase();
			if (!trimmed) continue;

			// Try to extract email first (handles "Name <email>" format)
			const email = parseEmail(part);
			if (email) {
				const domain = extractDomain(email);
				if (domain && !domains.includes(domain)) {
					domains.push(domain);
				}
			} else if (!trimmed.includes(' ') && !trimmed.includes('<')) {
				// Plain domain or potential domain
				// Remove any @ prefix if someone types @domain.com
				const cleaned = trimmed.replace(/^@/, '');
				if (cleaned && !domains.includes(cleaned)) {
					domains.push(cleaned);
				}
			}
		}

		return domains;
	}

	// Parse comma-separated input for emails, with smart extraction
	function parseEmails(input: string): string[] {
		const emails: string[] = [];
		const parts = input.split(',');

		for (const part of parts) {
			const email = parseEmail(part);
			if (email && !emails.includes(email)) {
				emails.push(email);
			}
		}

		return emails;
	}

	function addDomain() {
		const domains = parseDomains(newDomain);
		const newDomains = domains.filter(d => !fingerprintDomains.includes(d));
		if (newDomains.length > 0) {
			fingerprintDomains = [...fingerprintDomains, ...newDomains];
		}
		newDomain = '';
	}

	function removeDomain(domain: string) {
		fingerprintDomains = fingerprintDomains.filter(d => d !== domain);
	}

	function addEmail() {
		const emails = parseEmails(newEmail);
		const newEmails = emails.filter(e => !fingerprintEmails.includes(e));
		if (newEmails.length > 0) {
			fingerprintEmails = [...fingerprintEmails, ...newEmails];
		}
		newEmail = '';
	}

	function removeEmail(email: string) {
		fingerprintEmails = fingerprintEmails.filter(e => e !== email);
	}

	function addKeyword() {
		const parts = newKeyword.split(',');
		const newKeywords: string[] = [];

		for (const part of parts) {
			const trimmed = part.trim();
			if (trimmed && !fingerprintKeywords.includes(trimmed) && !newKeywords.includes(trimmed)) {
				newKeywords.push(trimmed);
			}
		}

		if (newKeywords.length > 0) {
			fingerprintKeywords = [...fingerprintKeywords, ...newKeywords];
		}
		newKeyword = '';
	}

	function removeKeyword(keyword: string) {
		fingerprintKeywords = fingerprintKeywords.filter(k => k !== keyword);
	}

	async function handleDelete() {
		if (!projectId) return;
		deleting = true;
		try {
			await api.deleteProject(projectId);
			goto('/projects');
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Failed to delete project';
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

					<!-- Fingerprints Section -->
					<div class="border-t border-gray-200 dark:border-gray-700 pt-6">
						<h3 class="text-sm font-medium text-gray-900 dark:text-white mb-4">
							Classification Fingerprints
							<span class="font-normal text-gray-500 dark:text-gray-400 ml-1">(for auto-classification)</span>
						</h3>

						<!-- Domains -->
						<div class="mb-4">
							<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Domains</label>
							<p class="text-xs text-gray-500 dark:text-gray-400 mb-2">Match attendee email domains. Paste comma-separated values or email lists.</p>
							<div class="flex gap-2 mb-2">
								<input
									type="text"
									bind:value={newDomain}
									placeholder="acme.com, contoso.com or paste attendee list"
									class="flex-1 rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm text-sm focus:border-primary-500 focus:ring-primary-500"
									onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addDomain())}
								/>
								<Button type="button" variant="secondary" size="sm" onclick={addDomain}>Add</Button>
							</div>
							{#if fingerprintDomains.length > 0}
								<div class="flex flex-wrap gap-1">
									{#each fingerprintDomains as domain}
										<span class="inline-flex items-center gap-1 px-2 py-1 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 text-xs rounded">
											{domain}
											<button type="button" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" onclick={() => removeDomain(domain)}>×</button>
										</span>
									{/each}
								</div>
							{/if}
						</div>

						<!-- Emails -->
						<div class="mb-4">
							<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Email Addresses</label>
							<p class="text-xs text-gray-500 dark:text-gray-400 mb-2">Match specific attendee emails. Paste from meeting invites.</p>
							<div class="flex gap-2 mb-2">
								<input
									type="text"
									bind:value={newEmail}
									placeholder="Name <email@example.com>, other@example.com"
									class="flex-1 rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm text-sm focus:border-primary-500 focus:ring-primary-500"
									onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addEmail())}
								/>
								<Button type="button" variant="secondary" size="sm" onclick={addEmail}>Add</Button>
							</div>
							{#if fingerprintEmails.length > 0}
								<div class="flex flex-wrap gap-1">
									{#each fingerprintEmails as email}
										<span class="inline-flex items-center gap-1 px-2 py-1 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 text-xs rounded">
											{email}
											<button type="button" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" onclick={() => removeEmail(email)}>×</button>
										</span>
									{/each}
								</div>
							{/if}
						</div>

						<!-- Keywords -->
						<div class="mb-4">
							<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Keywords</label>
							<p class="text-xs text-gray-500 dark:text-gray-400 mb-2">Match words in event titles or descriptions. Comma-separated.</p>
							<div class="flex gap-2 mb-2">
								<input
									type="text"
									bind:value={newKeyword}
									placeholder="Weekly Sync, Standup, Planning"
									class="flex-1 rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm text-sm focus:border-primary-500 focus:ring-primary-500"
									onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addKeyword())}
								/>
								<Button type="button" variant="secondary" size="sm" onclick={addKeyword}>Add</Button>
							</div>
							{#if fingerprintKeywords.length > 0}
								<div class="flex flex-wrap gap-1">
									{#each fingerprintKeywords as keyword}
										<span class="inline-flex items-center gap-1 px-2 py-1 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 text-xs rounded">
											{keyword}
											<button type="button" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" onclick={() => removeKeyword(keyword)}>×</button>
										</span>
									{/each}
								</div>
							{/if}
						</div>
					</div>

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
