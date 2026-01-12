<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Modal } from '$lib/components/primitives';
	import { api, ApiClientError } from '$lib/api/client';
	import type { Invoice, InvoiceStatus } from '$lib/api/types';

	let invoice = $state<Invoice | null>(null);
	let loading = $state(true);
	let error = $state('');

	// Action states
	let statusUpdating = $state(false);
	let exporting = $state(false);
	let deleting = $state(false);

	// Delete confirmation modal
	let showDeleteModal = $state(false);

	// Toggle for showing 0h time entries (hidden by default)
	let showZeroHourEntries = $state(false);

	const invoiceId = $derived(() => {
		return $page.params.id;
	});

	// Filter line items based on showZeroHourEntries toggle
	const filteredLineItems = $derived(() => {
		if (!invoice?.line_items) return [];
		if (showZeroHourEntries) return invoice.line_items;
		return invoice.line_items.filter(item => item.hours > 0);
	});

	// Count of hidden 0h entries for display
	const zeroHourCount = $derived(() => {
		if (!invoice?.line_items) return 0;
		return invoice.line_items.filter(item => item.hours === 0).length;
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

	async function loadInvoice() {
		loading = true;
		error = '';
		const id = invoiceId();
		if (!id) {
			error = 'Invalid invoice ID.';
			loading = false;
			return;
		}
		try {
			invoice = await api.getInvoice(id);
		} catch (e) {
			console.error('Failed to load invoice:', e);
			if (e instanceof ApiClientError && e.status === 404) {
				error = 'Invoice not found.';
			} else {
				error = 'Failed to load invoice. Please try again.';
			}
		} finally {
			loading = false;
		}
	}

	async function updateStatus(newStatus: InvoiceStatus) {
		if (!invoice) return;

		statusUpdating = true;
		error = '';

		try {
			const updated = await api.updateInvoiceStatus(invoice.id, { status: newStatus });
			invoice = updated;
		} catch (e) {
			console.error('Failed to update status:', e);
			if (e instanceof ApiClientError) {
				error = e.error.message || 'Failed to update invoice status.';
			} else {
				error = 'Failed to update invoice status. Please try again.';
			}
		} finally {
			statusUpdating = false;
		}
	}

	async function exportCSV() {
		if (!invoice) return;

		exporting = true;
		error = '';

		try {
			const blob = await api.exportInvoiceCSV(invoice.id);
			const url = window.URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `${invoice.invoice_number}.csv`;
			document.body.appendChild(a);
			a.click();
			window.URL.revokeObjectURL(url);
			document.body.removeChild(a);
		} catch (e) {
			console.error('Failed to export CSV:', e);
			error = 'Failed to export CSV. Please try again.';
		} finally {
			exporting = false;
		}
	}

	async function exportSheets() {
		if (!invoice) return;

		exporting = true;
		error = '';

		try {
			const result = await api.exportInvoiceSheets(invoice.id);

			// Reload invoice to get updated spreadsheet info
			await loadInvoice();

			// Open spreadsheet in new tab
			if (result.spreadsheet_url) {
				window.open(result.spreadsheet_url, '_blank');
			}
		} catch (e) {
			console.error('Failed to export to Sheets:', e);
			if (e instanceof ApiClientError) {
				if (e.error.code === 'no_connection') {
					error = 'Please connect your Google Calendar first to enable Sheets export.';
				} else {
					error = e.error.message || 'Failed to export to Google Sheets.';
				}
			} else {
				error = 'Failed to export to Google Sheets. Please try again.';
			}
		} finally {
			exporting = false;
		}
	}

	async function handleDelete() {
		if (!invoice) return;

		deleting = true;
		error = '';

		try {
			await api.deleteInvoice(invoice.id);
			goto('/invoices');
		} catch (e) {
			console.error('Failed to delete invoice:', e);
			if (e instanceof ApiClientError) {
				error = e.error.message || 'Failed to delete invoice.';
			} else {
				error = 'Failed to delete invoice. Please try again.';
			}
			showDeleteModal = false;
		} finally {
			deleting = false;
		}
	}

	onMount(() => {
		loadInvoice();
	});
</script>

<svelte:head>
	<title>{invoice ? `Invoice ${invoice.invoice_number}` : 'Invoice'} - Timesheet</title>
</svelte:head>

<AppShell>
	<div class="max-w-5xl mx-auto">
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
		</div>

		{#if loading}
			<div class="flex items-center justify-center py-12">
				<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		{:else if error && !invoice}
			<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-8 text-center">
				<div class="text-red-600 dark:text-red-400 mb-4">
					{error}
				</div>
				<Button onclick={() => goto('/invoices')}>
					Back to Invoices
				</Button>
			</div>
		{:else if invoice}
			<div class="space-y-6">
				{#if error}
					<div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 px-4 py-3 rounded">
						{error}
					</div>
				{/if}

				<!-- Header -->
				<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
					<div class="flex items-start justify-between">
						<div>
							<div class="flex items-center gap-3 mb-2">
								<h1 class="text-2xl font-bold text-gray-900 dark:text-white">
									{invoice.invoice_number}
								</h1>
								<span class="px-3 py-1 text-sm font-semibold rounded-full {getStatusColor(invoice.status)}">
									{invoice.status.charAt(0).toUpperCase() + invoice.status.slice(1)}
								</span>
							</div>
							<div class="space-y-1">
								<div class="text-lg text-gray-900 dark:text-white">
									{invoice.project?.name || 'Unknown Project'}
								</div>
								{#if invoice.project?.client}
									<div class="text-gray-500 dark:text-gray-400">
										{invoice.project.client}
									</div>
								{/if}
							</div>
						</div>

						<div class="text-right">
							<div class="text-sm text-gray-500 dark:text-gray-400 mb-1">Invoice Date</div>
							<div class="text-lg font-medium text-gray-900 dark:text-white">
								{formatDate(invoice.invoice_date)}
							</div>
						</div>
					</div>
				</div>

				<!-- Period Info -->
				<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
					<h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Billing Period</h2>
					<div class="grid grid-cols-1 md:grid-cols-3 gap-6">
						<div>
							<div class="text-sm text-gray-500 dark:text-gray-400 mb-1">Period Start</div>
							<div class="text-gray-900 dark:text-white">{formatDate(invoice.period_start)}</div>
						</div>
						<div>
							<div class="text-sm text-gray-500 dark:text-gray-400 mb-1">Period End</div>
							<div class="text-gray-900 dark:text-white">{formatDate(invoice.period_end)}</div>
						</div>
						<div>
							<div class="text-sm text-gray-500 dark:text-gray-400 mb-1">Total Days</div>
							<div class="text-gray-900 dark:text-white">
								{Math.ceil((new Date(invoice.period_end).getTime() - new Date(invoice.period_start).getTime()) / (1000 * 60 * 60 * 24)) + 1}
							</div>
						</div>
					</div>
				</div>

				<!-- Line Items -->
				<div class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
					<div class="p-6 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
						<h2 class="text-lg font-semibold text-gray-900 dark:text-white">Line Items</h2>
						{#if zeroHourCount() > 0}
							<label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400 cursor-pointer">
								<input
									type="checkbox"
									bind:checked={showZeroHourEntries}
									class="rounded border-gray-300 dark:border-gray-600 text-primary-600 focus:ring-primary-500"
								/>
								Show 0h entries ({zeroHourCount()})
							</label>
						{/if}
					</div>

					{#if filteredLineItems().length > 0}
						<div class="overflow-x-auto">
							<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
								<thead class="bg-gray-50 dark:bg-gray-900">
									<tr>
										<th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
											Date
										</th>
										<th scope="col" class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
											Description
										</th>
										<th scope="col" class="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
											Hours
										</th>
										<th scope="col" class="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
											Rate
										</th>
										<th scope="col" class="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
											Amount
										</th>
									</tr>
								</thead>
								<tbody class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
									{#each filteredLineItems() as item (item.id)}
										<tr>
											<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white">
												{formatDate(item.date)}
											</td>
											<td class="px-6 py-4 text-sm text-gray-900 dark:text-white">
												{item.description || '-'}
											</td>
											<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white text-right">
												{item.hours.toFixed(2)}
											</td>
											<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white text-right">
												{formatCurrency(item.hourly_rate)}
											</td>
											<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white text-right">
												{formatCurrency(item.amount)}
											</td>
										</tr>
									{/each}
								</tbody>
								<tfoot class="bg-gray-50 dark:bg-gray-900">
									<tr>
										<td colspan="2" class="px-6 py-4 text-sm font-semibold text-gray-900 dark:text-white">
											Total
										</td>
										<td class="px-6 py-4 text-sm font-semibold text-gray-900 dark:text-white text-right">
											{invoice.total_hours.toFixed(2)}
										</td>
										<td class="px-6 py-4"></td>
										<td class="px-6 py-4 text-sm font-semibold text-gray-900 dark:text-white text-right">
											{formatCurrency(invoice.total_amount)}
										</td>
									</tr>
								</tfoot>
							</table>
						</div>
					{:else}
						<div class="p-6 text-center text-gray-500 dark:text-gray-400">
							No line items
						</div>
					{/if}
				</div>

				<!-- Actions -->
				<div class="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
					<div class="flex flex-wrap gap-3">
						<!-- Status Actions -->
						{#if invoice.status === 'draft'}
							<Button onclick={() => updateStatus('sent')} loading={statusUpdating}>
								<svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
								</svg>
								Mark as Sent
							</Button>

							<Button variant="danger" onclick={() => (showDeleteModal = true)}>
								<svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
								</svg>
								Delete
							</Button>
						{:else if invoice.status === 'sent'}
							<Button onclick={() => updateStatus('draft')} loading={statusUpdating} variant="secondary">
								Revert to Draft
							</Button>

							<Button onclick={() => updateStatus('paid')} loading={statusUpdating}>
								<svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
								</svg>
								Mark as Paid
							</Button>
						{/if}

						<div class="flex-grow"></div>

						<!-- Export Actions -->
						<Button variant="secondary" onclick={exportCSV} loading={exporting}>
							<svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
							</svg>
							Export CSV
						</Button>

						<Button variant="secondary" onclick={exportSheets} loading={exporting}>
							<svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
							</svg>
							{invoice.spreadsheet_id ? 'Re-export to' : 'Export to'} Sheets
						</Button>

						{#if invoice.spreadsheet_url}
							<Button variant="secondary" onclick={() => invoice?.spreadsheet_url && window.open(invoice.spreadsheet_url, '_blank')}>
								<svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
								</svg>
								View Spreadsheet
							</Button>
						{/if}
					</div>
				</div>
			</div>
		{/if}
	</div>

	<!-- Delete Confirmation Modal -->
	<Modal bind:open={showDeleteModal} title="Delete Invoice">
		<div class="space-y-4">
			<p class="text-gray-700 dark:text-gray-300">
				Are you sure you want to delete this invoice? This action cannot be undone.
			</p>

			<div class="flex justify-end gap-3 pt-4">
				<Button variant="secondary" onclick={() => (showDeleteModal = false)}>
					Cancel
				</Button>
				<Button variant="danger" onclick={handleDelete} loading={deleting}>
					Delete Invoice
				</Button>
			</div>
		</div>
	</Modal>
</AppShell>
