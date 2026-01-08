<script lang="ts">
	import type { Project } from '$lib/api/types';
	import { getContrastColor } from '$lib/utils/colors';

	interface Props {
		project: Project;
		size?: 'sm' | 'md';
	}

	let { project, size = 'sm' }: Props = $props();

	const displayText = $derived(project.short_code || project.name);
	const textColor = $derived(getContrastColor(project.color));

	const sizeClasses = {
		sm: 'px-2 py-0.5 text-xs',
		md: 'px-3 py-1 text-sm'
	};
</script>

<span
	class="inline-flex items-center rounded-full font-medium {sizeClasses[size]}"
	style="background-color: {project.color}; color: {textColor};"
>
	{displayText}
</span>
