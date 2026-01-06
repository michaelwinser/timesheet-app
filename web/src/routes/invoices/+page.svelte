<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import type { Invoice, Project, InvoiceStatus } from '$lib/api/types';

	let invoices = $state<Invoice[]>([]);
	let projects = $state<Project[]>([]);
	let loading = $state(true);

	// Filters
	let selectedProject = $state<string>('');
	let selectedStatus = $state<InvoiceStatus | ''>('');

	const filteredInvoices = $derived(() => {
		let result = invoices;

		if (selectedProject) {
			result = result.filter(inv => inv.project_id === selectedProject);
		}

		if (selectedStatus) {
			result = result.filter(inv => inv.status === selectedStatus);
		}

		// Sort by invoice date descending (use toSorted to avoid mutating state)
		return result.toSorted((a, b) => new Date(b.invoice_date).getTime() - new Date(a.invoice_date).getTime());
	});

	function getStatusColor(status: InvoiceStatus): string {
		switch (status) {
			case 'draft':
				return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300';
			case 'sent':
				return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-300';
			case 'paid':
				return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300';
		}
	}

	function formatDate(dateStr: string): string {
		const date = new Date(dateStr);
		return date.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
	}

	function formatCurrency(amount: number): string {
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: 'USD'
		}).format(amount);
	}

	async function loadData() {
		loading = true;
		try {
			const [invoicesData, projectsData] = await Promise.all([
				api.listInvoices(),
				api.listProjects(false)
			]);
			invoices = invoicesData;
			projects = projectsData.filter(p => p.is_billable);
		} catch (e) {
			console.error('Failed to load data:', e);
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		loadData();
	});
</script>

<svelte:head>
	<title>Invoices - Timesheet</title>
</svelte:head>

<AppShell>
	<div class="max-w-6xl mx-auto">
		<div class="flex items-center justify-between mb-6">
			<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Invoices</h1>
			<Button onclick={() => goto('/invoices/new')}>
				<svg class="w-5 h-5 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
				</svg>
				New Invoice
			</Button>
		</div>

		<!-- Filters -->
		<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-4 mb-6">
			<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
				<div>
					<label for="project-filter" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
						Project
					</label>
					<select
						id="project-filter"
						bind:value={selectedProject}
						class="w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-primary-500 focus:ring-primary-500"
					>
						<option value="">All projects</option>
						{#each projects as project}
							<option value={project.id}>{project.name}</option>
						{/each}
					</select>
				</div>

				<div>
					<label for="status-filter" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
						Status
					</label>
					<select
						id="status-filter"
						bind:value={selectedStatus}
						class="w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-primary-500 focus:ring-primary-500"
					>
						<option value="">All statuses</option>
						<option value="draft">Draft</option>
						<option value="sent">Sent</option>
						<option value="paid">Paid</option>
					</select>
				</div>
			</div>
		</div>

		{#if loading}
			<div class="flex items-center justify-center py-12">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		{:else if filteredInvoices().length === 0}
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-12 text-center">
				<svg class="w-16 h-16 mx-auto text-gray-400 dark:text-gray-600 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
				</svg>
				<p class="text-gray-500 dark:text-gray-400 mb-4">
					{#if selectedProject || selectedStatus}
						No invoices match your filters.
					{:else}
						No invoices yet. Create your first invoice to get started.
					{/if}
				</p>
				{#if !selectedProject && !selectedStatus}
					<Button onclick={() => goto('/invoices/new')}>
						Create your first invoice
					</Button>
				{/if}
			</div>
		{:else}
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
				<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
					<thead class="bg-gray-50 dark:bg-gray-900">
						<tr>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
								Invoice #
							</th>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
								Project
							</th>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
								Period
							</th>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
								Date
							</th>
							<th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
								Status
							</th>
							<th scope="col" class="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
								Hours
							</th>
							<th scope="col" class="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
								Amount
							</th>
						</tr>
					</thead>
					<tbody class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
						{#each filteredInvoices() as invoice (invoice.id)}
							<tr
								class="hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer"
								onclick={() => goto(`/invoices/${invoice.id}`)}
							>
								<td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-white">
									{invoice.invoice_number}
								</td>
								<td class="px-6 py-4 whitespace-nowrap">
									<div class="text-sm text-gray-900 dark:text-white">
										{invoice.project?.name || 'Unknown Project'}
									</div>
									{#if invoice.project?.client}
										<div class="text-sm text-gray-500 dark:text-gray-400">
											{invoice.project.client}
										</div>
									{/if}
								</td>
								<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
									{formatDate(invoice.period_start)} - {formatDate(invoice.period_end)}
								</td>
								<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
									{formatDate(invoice.invoice_date)}
								</td>
								<td class="px-6 py-4 whitespace-nowrap">
									<span class="px-2 py-1 inline-flex text-xs leading-5 font-semibold rounded-full {getStatusColor(invoice.status)}">
										{invoice.status.charAt(0).toUpperCase() + invoice.status.slice(1)}
									</span>
								</td>
								<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white text-right">
									{invoice.total_hours.toFixed(2)}h
								</td>
								<td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-white text-right">
									{formatCurrency(invoice.total_amount)}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			<!-- Summary -->
			<div class="mt-4 text-sm text-gray-500 dark:text-gray-400">
				Showing {filteredInvoices().length} invoice{filteredInvoices().length !== 1 ? 's' : ''}
			</div>
		{/if}
	</div>
</AppShell>
