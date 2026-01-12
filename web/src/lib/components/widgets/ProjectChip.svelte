<script lang="ts">
	import type { Project } from '$lib/api/types';
	import { getContrastColor, getVerificationTextColor } from '$lib/utils/colors';

	interface Props {
		project: Project;
		size?: 'sm' | 'md';
		/** Use outlined style (transparent background, project color border and text) */
		outlined?: boolean;
	}

	let { project, size = 'sm', outlined = false }: Props = $props();

	const displayText = $derived(project.short_code || project.name);
	const textColor = $derived(getContrastColor(project.color));
	const outlinedTextColor = $derived(getVerificationTextColor(project.color));

	const sizeClasses = {
		sm: 'px-2 py-0.5 text-xs',
		md: 'px-3 py-1 text-sm'
	};
</script>

{#if outlined}
	<span
		class="inline-flex items-center rounded-full font-medium border {sizeClasses[size]} bg-white dark:bg-zinc-900"
		style="border-color: {project.color}; color: {outlinedTextColor};"
	>
		{displayText}
	</span>
{:else}
	<span
		class="inline-flex items-center rounded-full font-medium {sizeClasses[size]}"
		style="background-color: {project.color}; color: {textColor};"
	>
		{displayText}
	</span>
{/if}
