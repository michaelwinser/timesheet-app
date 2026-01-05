<script lang="ts">
	interface Props {
		type?: 'text' | 'email' | 'password' | 'number' | 'date';
		value?: string | number;
		placeholder?: string;
		label?: string;
		error?: string;
		disabled?: boolean;
		required?: boolean;
		name?: string;
		id?: string;
		oninput?: (e: Event) => void;
		onchange?: (e: Event) => void;
	}

	let {
		type = 'text',
		value = $bindable(''),
		placeholder = '',
		label,
		error,
		disabled = false,
		required = false,
		name,
		id,
		oninput,
		onchange
	}: Props = $props();

	const inputId = id || name || crypto.randomUUID();
</script>

<div class="w-full">
	{#if label}
		<label for={inputId} class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
			{label}
			{#if required}<span class="text-red-500">*</span>{/if}
		</label>
	{/if}
	<input
		{type}
		id={inputId}
		{name}
		bind:value
		{placeholder}
		{disabled}
		{required}
		{oninput}
		{onchange}
		class="block w-full rounded-md shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm
			bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100
			placeholder-gray-400 dark:placeholder-gray-500
			{error ? 'border-red-500' : 'border-gray-300 dark:border-gray-600'}
			{disabled ? 'bg-gray-100 dark:bg-gray-800 cursor-not-allowed' : ''}"
		class:border-red-500={error}
		style="padding: 0.5rem 0.75rem; border-width: 1px;"
	/>
	{#if error}
		<p class="mt-1 text-sm text-red-600">{error}</p>
	{/if}
</div>
