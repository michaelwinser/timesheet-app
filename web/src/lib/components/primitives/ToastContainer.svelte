<script lang="ts">
	import Toast, { type ToastType } from './Toast.svelte';

	interface ToastItem {
		id: number;
		message: string;
		type: ToastType;
	}

	let toasts = $state<ToastItem[]>([]);
	let nextId = 0;

	export function show(message: string, type: ToastType = 'info') {
		const id = nextId++;
		toasts = [...toasts, { id, message, type }];
	}

	export function success(message: string) {
		show(message, 'success');
	}

	export function error(message: string) {
		show(message, 'error');
	}

	export function info(message: string) {
		show(message, 'info');
	}

	function dismiss(id: number) {
		toasts = toasts.filter((t) => t.id !== id);
	}
</script>

<div class="pointer-events-none fixed right-4 top-4 z-50 flex flex-col gap-2">
	{#each toasts as toast (toast.id)}
		<Toast message={toast.message} type={toast.type} ondismiss={() => dismiss(toast.id)} />
	{/each}
</div>
