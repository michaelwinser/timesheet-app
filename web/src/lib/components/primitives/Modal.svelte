<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		open: boolean;
		title?: string;
		onclose?: () => void;
		children: Snippet;
		footer?: Snippet;
	}

	let { open = $bindable(false), title, onclose, children, footer }: Props = $props();

	function handleClose() {
		open = false;
		onclose?.();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			handleClose();
		}
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			handleClose();
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 overflow-y-auto"
		aria-labelledby="modal-title"
		role="dialog"
		aria-modal="true"
	>
		<div class="flex min-h-full items-center justify-center p-4">
			<div
				class="fixed inset-0 bg-gray-500 dark:bg-gray-900 bg-opacity-75 dark:bg-opacity-80 transition-opacity"
				onclick={handleBackdropClick}
			></div>

			<div class="relative bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-lg w-full">
				{#if title}
					<div class="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-700">
						<h3 class="text-lg font-semibold text-gray-900 dark:text-white" id="modal-title">
							{title}
						</h3>
						<button
							type="button"
							class="text-gray-400 hover:text-gray-500 dark:hover:text-gray-300"
							onclick={handleClose}
						>
							<span class="sr-only">Close</span>
							<svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
							</svg>
						</button>
					</div>
				{/if}

				<div class="px-6 py-4 text-gray-900 dark:text-gray-100">
					{@render children()}
				</div>

				{#if footer}
					<div class="px-6 py-4 border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50 flex justify-end gap-3 rounded-b-lg">
						{@render footer()}
					</div>
				{/if}
			</div>
		</div>
	</div>
{/if}
