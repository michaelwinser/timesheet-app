<script lang="ts">
	import type { Project } from '$lib/api/types';

	interface ProjectTotal {
		project: Project;
		hours: number;
	}

	interface Props {
		projectTotals: ProjectTotal[];
		hiddenTotals: ProjectTotal[];
		archivedTotals: ProjectTotal[];
		skippedHours: number;
		unclassifiedHours: number;
		totalHours: number;
		visibleProjectIds: Set<string>;
		showHiddenProjects: boolean;
		showArchivedProjects: boolean;
		showSkippedEvents: boolean;
		ontogglevisibility: (projectId: string) => void;
		ontogglehidden: () => void;
		ontogglearchived: () => void;
		ontoggleskipped: () => void;
		onhover: (target: string | null) => void;
	}

	let {
		projectTotals,
		hiddenTotals,
		archivedTotals,
		skippedHours,
		unclassifiedHours,
		totalHours,
		visibleProjectIds,
		showHiddenProjects,
		showArchivedProjects,
		showSkippedEvents,
		ontogglevisibility,
		ontogglehidden,
		ontogglearchived,
		ontoggleskipped,
		onhover
	}: Props = $props();

	// Calculate totals for pseudo-chips
	const hiddenHours = $derived(
		hiddenTotals.reduce((sum, t) => sum + t.hours, 0)
	);
	const archivedHours = $derived(
		archivedTotals.reduce((sum, t) => sum + t.hours, 0)
	);

	// Format hours for display
	function formatHours(hours: number): string {
		return Math.round(hours * 10) / 10 + 'h';
	}
</script>

<div class="mb-4 flex items-center gap-3 px-3 py-2 bg-gray-100 dark:bg-zinc-800 rounded-lg overflow-x-auto">
	<!-- Active project chips -->
	{#each projectTotals as { project, hours }}
		<label
			class="flex items-center gap-1.5 px-2 py-1 bg-white dark:bg-zinc-700 rounded-full text-sm whitespace-nowrap cursor-pointer transition-all hover:ring-2 hover:ring-primary-400 dark:hover:ring-primary-500"
			onmouseenter={() => onhover(project.id)}
			onmouseleave={() => onhover(null)}
		>
			<input
				type="checkbox"
				checked={visibleProjectIds.has(project.id)}
				onchange={() => ontogglevisibility(project.id)}
				class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-zinc-500 dark:bg-zinc-600 cursor-pointer"
			/>
			<span class="w-2.5 h-2.5 rounded-full flex-shrink-0" style="background-color: {project.color}"></span>
			<span class="text-gray-700 dark:text-gray-300">{project.name}</span>
			<span class="text-gray-500 dark:text-gray-400">({formatHours(hours)})</span>
		</label>
	{/each}

	<!-- Hidden projects pseudo-chip -->
	{#if hiddenTotals.length > 0}
		<label
			class="flex items-center gap-1.5 px-2 py-1 bg-gray-50 dark:bg-zinc-800 border border-dashed border-gray-300 dark:border-zinc-600 rounded-full text-sm whitespace-nowrap cursor-pointer transition-all hover:ring-2 hover:ring-primary-400 dark:hover:ring-primary-500 text-gray-500 dark:text-gray-400"
			onmouseenter={() => onhover('hidden')}
			onmouseleave={() => onhover(null)}
		>
			<input
				type="checkbox"
				checked={showHiddenProjects}
				onchange={ontogglehidden}
				class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-zinc-500 dark:bg-zinc-600 cursor-pointer"
			/>
			<svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
			</svg>
			<span class="text-gray-700 dark:text-gray-300">Hidden</span>
			<span class="text-gray-500 dark:text-gray-400">({formatHours(hiddenHours)})</span>
		</label>
	{/if}

	<!-- Archived projects pseudo-chip -->
	{#if archivedTotals.length > 0}
		<label
			class="flex items-center gap-1.5 px-2 py-1 bg-gray-50 dark:bg-zinc-800 border border-dashed border-gray-300 dark:border-zinc-600 rounded-full text-sm whitespace-nowrap cursor-pointer transition-all hover:ring-2 hover:ring-primary-400 dark:hover:ring-primary-500 text-amber-600 dark:text-amber-500"
			onmouseenter={() => onhover('archived')}
			onmouseleave={() => onhover(null)}
		>
			<input
				type="checkbox"
				checked={showArchivedProjects}
				onchange={ontogglearchived}
				class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-zinc-500 dark:bg-zinc-600 cursor-pointer"
			/>
			<svg class="w-3.5 h-3.5 flex-shrink-0 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
			</svg>
			<span class="text-gray-700 dark:text-gray-300">Archived</span>
			<span class="text-gray-500 dark:text-gray-400">({formatHours(archivedHours)})</span>
		</label>
	{/if}

	<!-- Skipped meetings pseudo-chip -->
	{#if skippedHours > 0}
		<label
			class="flex items-center gap-1.5 px-2 py-1 bg-gray-50 dark:bg-zinc-800 border border-dashed border-gray-300 dark:border-zinc-600 rounded-full text-sm whitespace-nowrap cursor-pointer transition-all hover:ring-2 hover:ring-primary-400 dark:hover:ring-primary-500 text-gray-500 dark:text-gray-400"
			onmouseenter={() => onhover('skipped')}
			onmouseleave={() => onhover(null)}
		>
			<input
				type="checkbox"
				checked={showSkippedEvents}
				onchange={ontoggleskipped}
				class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-zinc-500 dark:bg-zinc-600 cursor-pointer"
			/>
			<svg class="w-3.5 h-3.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
			</svg>
			<span class="text-gray-700 dark:text-gray-300">Skipped</span>
			<span class="text-gray-500 dark:text-gray-400">({formatHours(skippedHours)})</span>
		</label>
	{/if}

	<!-- Unclassified pseudo-chip (no checkbox, just info) -->
	{#if unclassifiedHours > 0}
		<div class="flex items-center gap-1.5 px-2 py-1 border border-dashed border-gray-300 dark:border-zinc-600 rounded-full text-sm whitespace-nowrap bg-white dark:bg-zinc-700">
			<span class="w-2.5 h-2.5 rounded-full flex-shrink-0 bg-gray-400 dark:bg-gray-500 opacity-50"></span>
			<span class="text-gray-700 dark:text-gray-300">Unclassified</span>
			<span class="text-gray-500 dark:text-gray-400">({formatHours(unclassifiedHours)})</span>
		</div>
	{/if}

	<!-- Total hours -->
	<span class="ml-auto text-sm font-medium text-gray-600 dark:text-gray-300 whitespace-nowrap">
		{formatHours(totalHours + unclassifiedHours)} total
	</span>
</div>
