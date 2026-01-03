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
	class="w-full flex items-center justify-between px-4 py-3 bg-white border rounded-lg hover:shadow-sm transition-shadow text-left"
	{onclick}
>
	<div class="flex items-center gap-3">
		<ProjectChip {project} size="md" />
		<div>
			<div class="font-medium text-gray-900">{project.name}</div>
			{#if project.is_archived}
				<span class="text-xs text-gray-500">Archived</span>
			{/if}
		</div>
	</div>
	<div class="flex items-center gap-2">
		{#if !project.is_billable}
			<span class="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded">Non-billable</span>
		{/if}
		<svg class="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
		</svg>
	</div>
</button>
