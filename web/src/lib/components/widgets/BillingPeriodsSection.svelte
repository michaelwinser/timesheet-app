<script lang="ts">
	import { Button, Input, Modal } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import type { BillingPeriod } from '$lib/api/types';

	interface Props {
		projectId: string;
	}

	let { projectId }: Props = $props();

	// State
	let billingPeriods = $state<BillingPeriod[]>([]);
	let loading = $state(false);
	let showModal = $state(false);
	let submitting = $state(false);
	let error = $state('');

	// Form state
	let startsOn = $state('');
	let endsOn = $state('');
	let hourlyRate = $state('100.00');

	function formatDate(dateStr: string): string {
		const date = new Date(dateStr);
		return date.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
	}

	async function loadPeriods() {
		if (!projectId) return;
		loading = true;
		try {
			billingPeriods = await api.listBillingPeriods(projectId);
			// Sort by starts_on descending
			billingPeriods.sort((a, b) => new Date(b.starts_on).getTime() - new Date(a.starts_on).getTime());
		} catch (e) {
			console.error('Failed to load billing periods:', e);
		} finally {
			loading = false;
		}
	}

	function openModal() {
		startsOn = '';
		endsOn = '';
		hourlyRate = '100.00';
		error = '';
		showModal = true;
	}

	async function handleCreate() {
		if (!projectId || !startsOn || !hourlyRate) {
			error = 'Please fill in required fields.';
			return;
		}

		const rate = parseFloat(hourlyRate);
		if (isNaN(rate) || rate < 0) {
			error = 'Please enter a valid hourly rate.';
			return;
		}

		if (endsOn && new Date(startsOn) > new Date(endsOn)) {
			error = 'Start date must be before or equal to end date.';
			return;
		}

		submitting = true;
		error = '';

		try {
			const newPeriod = await api.createBillingPeriod({
				project_id: projectId,
				starts_on: startsOn,
				ends_on: endsOn || undefined,
				hourly_rate: rate
			});

			billingPeriods = [newPeriod, ...billingPeriods];
			showModal = false;
		} catch (e: unknown) {
			console.error('Failed to create billing period:', e);
			error = e instanceof Error ? e.message : 'Failed to create billing period. Check for overlapping periods.';
		} finally {
			submitting = false;
		}
	}

	async function deletePeriod(periodId: string) {
		if (!confirm('Are you sure you want to delete this billing period?')) return;

		try {
			await api.deleteBillingPeriod(periodId);
			billingPeriods = billingPeriods.filter(p => p.id !== periodId);
		} catch (e) {
			console.error('Failed to delete billing period:', e);
			alert('Failed to delete billing period. It may be in use by an invoice.');
		}
	}

	// Load periods on mount
	$effect(() => {
		loadPeriods();
	});
</script>

<div class="border-t border-gray-200 dark:border-gray-700 pt-6">
	<div class="flex items-center justify-between mb-4">
		<div>
			<h3 class="text-sm font-medium text-gray-900 dark:text-white">
				Billing Periods
			</h3>
			<p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
				Define hourly rates for different time periods
			</p>
		</div>
		<Button type="button" variant="secondary" size="sm" onclick={openModal}>
			Add Period
		</Button>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-8">
			<div class="animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600"></div>
		</div>
	{:else if billingPeriods.length === 0}
		<div class="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">
			No billing periods defined yet.
		</div>
	{:else}
		<div class="overflow-x-auto">
			<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
				<thead class="bg-gray-50 dark:bg-gray-900">
					<tr>
						<th scope="col" class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
							Start Date
						</th>
						<th scope="col" class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
							End Date
						</th>
						<th scope="col" class="px-3 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
							Hourly Rate
						</th>
						<th scope="col" class="px-3 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
							Actions
						</th>
					</tr>
				</thead>
				<tbody class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
					{#each billingPeriods as period (period.id)}
						<tr>
							<td class="px-3 py-2 whitespace-nowrap text-sm text-gray-900 dark:text-white">
								{formatDate(period.starts_on)}
							</td>
							<td class="px-3 py-2 whitespace-nowrap text-sm text-gray-900 dark:text-white">
								{period.ends_on ? formatDate(period.ends_on) : 'Ongoing'}
							</td>
							<td class="px-3 py-2 whitespace-nowrap text-sm text-gray-900 dark:text-white text-right">
								${period.hourly_rate.toFixed(2)}/hr
							</td>
							<td class="px-3 py-2 whitespace-nowrap text-sm text-right">
								<button
									type="button"
									onclick={() => deletePeriod(period.id)}
									class="text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300"
								>
									Delete
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<!-- Create Billing Period Modal -->
<Modal bind:open={showModal} title="New Billing Period">
	<form class="space-y-4" onsubmit={(e) => { e.preventDefault(); handleCreate(); }}>
		{#if error}
			<div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 px-4 py-3 rounded text-sm">
				{error}
			</div>
		{/if}

		<Input
			type="date"
			label="Start Date"
			bind:value={startsOn}
			required
		/>

		<Input
			type="date"
			label="End Date (optional)"
			bind:value={endsOn}
		/>

		<p class="text-xs text-gray-500 dark:text-gray-400">
			Leave end date empty for an ongoing period
		</p>

		<Input
			type="number"
			label="Hourly Rate"
			bind:value={hourlyRate}
			required
			step="0.01"
			min="0"
		/>

		<div class="flex justify-end gap-3 pt-4">
			<Button variant="secondary" onclick={() => (showModal = false)}>
				Cancel
			</Button>
			<Button type="submit" loading={submitting}>
				Create Period
			</Button>
		</div>
	</form>
</Modal>
