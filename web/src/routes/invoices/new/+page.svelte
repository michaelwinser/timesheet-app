<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Input } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import { ApiClientError } from '$lib/api/client';
	import type { Project } from '$lib/api/types';

	let projects = $state<Project[]>([]);
	let loading = $state(true);
	let submitting = $state(false);
	let error = $state('');

	// Form fields
	let projectId = $state('');
	let periodStart = $state('');
	let periodEnd = $state('');
	let invoiceDate = $state('');

	// Preview data
	let previewLoading = $state(false);
	let previewData = $state<{ hours: number; amount: number } | null>(null);

	const selectedProject = $derived(() => {
		return projects.find(p => p.id === projectId);
	});

	const canPreview = $derived(() => {
		return projectId && periodStart && periodEnd && new Date(periodStart) <= new Date(periodEnd);
	});

	// Initialize invoice date to today
	function initializeDates() {
		const today = new Date();
		invoiceDate = today.toISOString().split('T')[0];

		// Default period to last month
		const lastMonthEnd = new Date(today.getFullYear(), today.getMonth(), 0);
		const lastMonthStart = new Date(today.getFullYear(), today.getMonth() - 1, 1);
		periodEnd = lastMonthEnd.toISOString().split('T')[0];
		periodStart = lastMonthStart.toISOString().split('T')[0];
	}

	async function loadProjects() {
		loading = true;
		try {
			const allProjects = await api.listProjects(false);
			projects = allProjects.filter(p => p.is_billable && !p.is_archived);
		} catch (e) {
			console.error('Failed to load projects:', e);
			error = 'Failed to load projects. Please try again.';
		} finally {
			loading = false;
		}
	}

	async function loadPreview() {
		if (!canPreview()) return;

		previewLoading = true;
		previewData = null;

		try {
			// Fetch time entries for the selected period
			const entries = await api.listTimeEntries({
				project_id: projectId,
				start_date: periodStart,
				end_date: periodEnd
			});

			// Filter for unbilled entries only
			const unbilledEntries = entries.filter(e => !e.invoice_id);

			// Calculate totals
			const hours = unbilledEntries.reduce((sum, e) => sum + e.hours, 0);

			// For preview, use a simple calculation - actual rates will be from billing periods
			const amount = hours * 100; // Placeholder - actual amount calculated server-side

			previewData = { hours, amount };
		} catch (e) {
			console.error('Failed to load preview:', e);
		} finally {
			previewLoading = false;
		}
	}

	async function handleCreate() {
		if (!projectId || !periodStart || !periodEnd || !invoiceDate) {
			error = 'Please fill in all required fields.';
			return;
		}

		if (new Date(periodStart) > new Date(periodEnd)) {
			error = 'Period start date must be before or equal to end date.';
			return;
		}

		submitting = true;
		error = '';

		try {
			const invoice = await api.createInvoice({
				project_id: projectId,
				period_start: periodStart,
				period_end: periodEnd,
				invoice_date: invoiceDate
			});

			// Navigate to the invoice detail page
			goto(`/invoices/${invoice.id}`);
		} catch (e) {
			console.error('Failed to create invoice:', e);

			if (e instanceof ApiClientError) {
				if (e.error.code === 'no_entries') {
					error = 'No unbilled time entries found for the specified date range.';
				} else if (e.error.code === 'not_found') {
					error = 'Project not found.';
				} else {
					error = e.error.message || 'Failed to create invoice. Please try again.';
				}
			} else {
				error = 'Failed to create invoice. Please try again.';
			}
		} finally {
			submitting = false;
		}
	}

	// Load preview when inputs change
	$effect(() => {
		if (canPreview()) {
			loadPreview();
		} else {
			previewData = null;
		}
	});

	onMount(() => {
		initializeDates();
		loadProjects();
	});
</script>

<svelte:head>
	<title>New Invoice - Timesheet</title>
</svelte:head>

<AppShell>
	<div class="max-w-2xl mx-auto">
		<div class="mb-6">
			<button
				type="button"
				onclick={() => goto('/invoices')}
				class="flex items-center text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300 mb-4"
			>
				<svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
				</svg>
				Back to Invoices
			</button>
			<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Create New Invoice</h1>
		</div>

		{#if loading}
			<div class="flex items-center justify-center py-12">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		{:else if projects.length === 0}
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-8 text-center">
				<p class="text-gray-500 dark:text-gray-400 mb-4">
					No billable projects found. Please create a billable project first.
				</p>
				<Button onclick={() => goto('/projects')}>
					Go to Projects
				</Button>
			</div>
		{:else}
			<form class="bg-white dark:bg-gray-800 rounded-lg shadow p-6 space-y-6" onsubmit={(e) => { e.preventDefault(); handleCreate(); }}>
				{#if error}
					<div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 px-4 py-3 rounded">
						{error}
					</div>
				{/if}

				<div>
					<label for="project" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
						Project <span class="text-red-500">*</span>
					</label>
					<select
						id="project"
						bind:value={projectId}
						required
						class="w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-primary-500 focus:ring-primary-500"
					>
						<option value="">Select a project</option>
						{#each projects as project}
							<option value={project.id}>
								{project.name}{project.client ? ` - ${project.client}` : ''}
							</option>
						{/each}
					</select>
				</div>

				<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
					<Input
						type="date"
						label="Period Start"
						bind:value={periodStart}
						required
					/>

					<Input
						type="date"
						label="Period End"
						bind:value={periodEnd}
						required
					/>
				</div>

				<Input
					type="date"
					label="Invoice Date"
					bind:value={invoiceDate}
					required
				/>

				<!-- Preview Card -->
				{#if canPreview()}
					<div class="bg-gray-50 dark:bg-gray-900 rounded-lg p-4 border border-gray-200 dark:border-gray-700">
						<h3 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Preview</h3>

						{#if previewLoading}
							<div class="flex items-center justify-center py-4">
								<div class="animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600"></div>
							</div>
						{:else if previewData}
							{#if previewData.hours === 0}
								<div class="text-amber-600 dark:text-amber-400 text-sm">
									<svg class="w-5 h-5 inline mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
									</svg>
									No unbilled time entries found for this period
								</div>
							{:else}
								<div class="grid grid-cols-2 gap-4">
									<div>
										<div class="text-sm text-gray-500 dark:text-gray-400">Total Hours</div>
										<div class="text-2xl font-bold text-gray-900 dark:text-white">
											{previewData.hours.toFixed(2)}
										</div>
									</div>
									<div>
										<div class="text-sm text-gray-500 dark:text-gray-400">Unbilled Entries</div>
										<div class="text-2xl font-bold text-gray-900 dark:text-white">
											{previewData.hours > 0 ? Math.ceil(previewData.hours) : 0}
										</div>
									</div>
								</div>
								<p class="text-xs text-gray-500 dark:text-gray-400 mt-3">
									Final amount will be calculated using billing period rates
								</p>
							{/if}
						{/if}
					</div>
				{/if}

				<div class="flex justify-end gap-3 pt-4">
					<Button variant="secondary" onclick={() => goto('/invoices')}>
						Cancel
					</Button>
					<Button type="submit" loading={submitting} disabled={!canPreview() || (previewData?.hours === 0)}>
						Create Invoice
					</Button>
				</div>
			</form>
		{/if}
	</div>
</AppShell>
