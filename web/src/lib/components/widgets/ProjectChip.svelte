<script lang="ts">
	import type { Project } from '$lib/api/types';

	interface Props {
		project: Project;
		size?: 'sm' | 'md';
	}

	let { project, size = 'sm' }: Props = $props();

	// Calculate contrasting text color
	function getContrastColor(hexColor: string): string {
		const hex = hexColor.replace('#', '');
		const r = parseInt(hex.slice(0, 2), 16);
		const g = parseInt(hex.slice(2, 4), 16);
		const b = parseInt(hex.slice(4, 6), 16);
		const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
		return luminance > 0.5 ? '#000000' : '#ffffff';
	}

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
