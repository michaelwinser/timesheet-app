<script lang="ts">
	import { fly, fade } from 'svelte/transition';

	export type ToastType = 'success' | 'error' | 'info';

	interface Props {
		message: string;
		type?: ToastType;
		duration?: number;
		ondismiss?: () => void;
	}

	let { message, type = 'info', duration = 4000, ondismiss }: Props = $props();

	// Auto-dismiss after duration
	$effect(() => {
		if (duration > 0 && ondismiss) {
			const timer = setTimeout(ondismiss, duration);
			return () => clearTimeout(timer);
		}
	});

	const iconPaths: Record<ToastType, string> = {
		success: 'M5 13l4 4L19 7',
		error: 'M6 18L18 6M6 6l12 12',
		info: 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z'
	};

	const colors: Record<ToastType, { bg: string; text: string; icon: string }> = {
		success: {
			bg: 'bg-green-50 dark:bg-green-900/30',
			text: 'text-green-800 dark:text-green-200',
			icon: 'text-green-500 dark:text-green-400'
		},
		error: {
			bg: 'bg-red-50 dark:bg-red-900/30',
			text: 'text-red-800 dark:text-red-200',
			icon: 'text-red-500 dark:text-red-400'
		},
		info: {
			bg: 'bg-blue-50 dark:bg-blue-900/30',
			text: 'text-blue-800 dark:text-blue-200',
			icon: 'text-blue-500 dark:text-blue-400'
		}
	};
</script>

<div
	class="pointer-events-auto flex items-center gap-3 rounded-lg border px-4 py-3 shadow-lg {colors[type].bg} border-gray-200 dark:border-zinc-700"
	in:fly={{ y: -20, duration: 200 }}
	out:fade={{ duration: 150 }}
	role="alert"
>
	<svg class="h-5 w-5 flex-shrink-0 {colors[type].icon}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
		<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={iconPaths[type]} />
	</svg>
	<span class="text-sm {colors[type].text}">{message}</span>
	{#if ondismiss}
		<button
			type="button"
			class="ml-auto -mr-1 rounded p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
			onclick={ondismiss}
			aria-label="Dismiss"
		>
			<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
			</svg>
		</button>
	{/if}
</div>
