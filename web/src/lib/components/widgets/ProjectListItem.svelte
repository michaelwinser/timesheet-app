<script lang="ts">
	import type { Project } from '$lib/api/types';
	import ProjectChip from './ProjectChip.svelte';

	interface Props {
		project: Project;
		onclick?: () => void;
	}

	let { project, onclick }: Props = $props();
</script>

<button
	type="button"
	class="w-full flex items-center justify-between px-4 py-3 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg hover:shadow-sm dark:hover:bg-gray-700/50 transition-shadow text-left"
	{onclick}
>
	<div class="flex items-center gap-3">
		<ProjectChip {project} size="md" />
		<div>
			<div class="font-medium text-gray-900 dark:text-white">{project.name}</div>
			{#if project.is_archived}
				<span class="text-xs text-gray-500 dark:text-gray-400">Archived</span>
			{/if}
		</div>
	</div>
	<div class="flex items-center gap-2">
		{#if !project.is_billable}
			<span class="text-xs text-gray-400 dark:text-gray-500 bg-gray-100 dark:bg-gray-700 px-2 py-0.5 rounded">Non-billable</span>
		{/if}
		<svg class="w-5 h-5 text-gray-400 dark:text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
		</svg>
	</div>
</button>
