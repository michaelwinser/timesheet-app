<script lang="ts">
	import { QuickDateParser } from '$lib/utils/dateParser';

	interface Props {
		open: boolean;
		referenceDate?: Date;
		onselect?: (date: Date) => void;
		onclose?: () => void;
	}

	let { open = $bindable(false), referenceDate = new Date(), onselect, onclose }: Props = $props();

	let inputValue = $state('');
	let inputElement: HTMLInputElement | undefined = $state();

	// Reactive parsed result
	const parsedResult = $derived.by(() => {
		if (!inputValue.trim()) return null;
		return QuickDateParser.parse(inputValue, referenceDate);
	});

	// Format date for preview
	function formatPreviewDate(date: Date): string {
		const today = new Date();
		today.setHours(0, 0, 0, 0);
		const target = new Date(date);
		target.setHours(0, 0, 0, 0);

		const diffDays = Math.round((today.getTime() - target.getTime()) / (1000 * 60 * 60 * 24));

		const dateStr = date.toLocaleDateString('en-US', {
			weekday: 'long',
			month: 'long',
			day: 'numeric',
			year: 'numeric'
		});

		if (diffDays === 0) return `${dateStr} (today)`;
		if (diffDays === 1) return `${dateStr} (yesterday)`;
		if (diffDays === -1) return `${dateStr} (tomorrow)`;
		if (diffDays > 0) return `${dateStr} (${diffDays} days ago)`;
		return `${dateStr} (in ${Math.abs(diffDays)} days)`;
	}

	function handleClose() {
		inputValue = '';
		open = false;
		onclose?.();
	}

	function handleSubmit() {
		if (parsedResult) {
			onselect?.(parsedResult.date);
			inputValue = '';
			open = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			handleClose();
		} else if (e.key === 'Enter' && parsedResult) {
			e.preventDefault();
			handleSubmit();
		}
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			handleClose();
		}
	}

	// Focus input when modal opens
	$effect(() => {
		if (open && inputElement) {
			// Small delay to ensure DOM is ready
			setTimeout(() => inputElement?.focus(), 50);
		}
	});
</script>

<svelte:window onkeydown={open ? handleKeydown : undefined} />

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 overflow-y-auto"
		aria-labelledby="goto-date-title"
		role="dialog"
		aria-modal="true"
	>
		<div class="flex min-h-full items-start justify-center pt-[15vh] px-4">
			<div
				class="fixed inset-0 bg-gray-500 dark:bg-gray-900 bg-opacity-75 dark:bg-opacity-80 transition-opacity"
				onclick={handleBackdropClick}
			></div>

			<div class="relative bg-white dark:bg-zinc-800 rounded-lg shadow-xl w-full max-w-md border border-gray-200 dark:border-white/10">
				<!-- Header -->
				<div class="flex items-center justify-between px-4 py-3 border-b border-gray-200 dark:border-zinc-700">
					<h3 class="text-base font-medium text-gray-900 dark:text-white" id="goto-date-title">
						Go to date
					</h3>
					<button
						type="button"
						class="text-gray-400 hover:text-gray-500 dark:hover:text-gray-300 p-1"
						onclick={handleClose}
					>
						<span class="sr-only">Close</span>
						<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
						</svg>
					</button>
				</div>

				<!-- Content -->
				<div class="px-4 py-4">
					<input
						bind:this={inputElement}
						type="text"
						bind:value={inputValue}
						placeholder="e.g., last monday, 3 weeks ago, jan 15"
						class="w-full px-3 py-2.5 text-base border rounded-lg outline-none transition-colors
							bg-white dark:bg-zinc-900
							border-gray-300 dark:border-zinc-600
							text-gray-900 dark:text-white
							placeholder-gray-400 dark:placeholder-zinc-500
							focus:border-primary-500 dark:focus:border-primary-400
							focus:ring-2 focus:ring-primary-500/20 dark:focus:ring-primary-400/20"
						class:border-green-500={parsedResult}
						class:dark:border-green-400={parsedResult}
					/>

					<!-- Preview area -->
					<div class="mt-3 min-h-[60px] px-3 py-2 rounded-lg bg-gray-50 dark:bg-zinc-900/50">
						{#if parsedResult}
							<div class="flex items-start gap-2">
								<svg class="w-5 h-5 text-green-500 dark:text-green-400 flex-shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
								</svg>
								<div class="flex-1 min-w-0">
									<p class="text-sm font-medium text-gray-900 dark:text-white">
										{formatPreviewDate(parsedResult.date)}
									</p>
									<p class="text-xs text-gray-500 dark:text-zinc-400 mt-0.5">
										Press Enter to navigate
									</p>
								</div>
							</div>
						{:else if inputValue.trim()}
							<p class="text-sm text-gray-500 dark:text-zinc-400 italic">
								Keep typing to parse date...
							</p>
						{:else}
							<div class="text-xs text-gray-400 dark:text-zinc-500 space-y-1">
								<p class="font-medium">Examples:</p>
								<div class="flex flex-wrap gap-x-4 gap-y-0.5">
									<span>yesterday</span>
									<span>last week</span>
									<span>3 weeks ago</span>
									<span>monday</span>
									<span>jan 15</span>
									<span>last month</span>
								</div>
							</div>
						{/if}
					</div>
				</div>

				<!-- Footer -->
				<div class="px-4 py-3 border-t border-gray-200 dark:border-zinc-700 bg-gray-50 dark:bg-zinc-800/50 flex justify-end gap-2 rounded-b-lg">
					<button
						type="button"
						class="px-3 py-1.5 text-sm font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-zinc-700 rounded-md transition-colors"
						onclick={handleClose}
					>
						Cancel
					</button>
					<button
						type="button"
						class="px-3 py-1.5 text-sm font-medium text-white bg-primary-600 hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed rounded-md transition-colors"
						disabled={!parsedResult}
						onclick={handleSubmit}
					>
						Go
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}
