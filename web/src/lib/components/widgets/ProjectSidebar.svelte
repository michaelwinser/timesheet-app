<script lang="ts">
	import type { Project, TimeEntry } from '$lib/api/types';

	type ScopeMode = 'day' | 'week' | 'full-week';

	interface ProjectTotal {
		project: Project;
		hours: number;
	}

	interface Props {
		scopeMode: ScopeMode;
		totalHours: number;
		activeProjects: Project[];
		hiddenProjects: Project[];
		projectTotals: ProjectTotal[];
		archivedTotals: ProjectTotal[];
		visibleProjectIds: Set<string>;
		entries: TimeEntry[];
		ontogglevisibility: (projectId: string) => void;
	}

	let {
		scopeMode,
		totalHours,
		activeProjects,
		hiddenProjects,
		projectTotals,
		archivedTotals,
		visibleProjectIds,
		entries,
		ontogglevisibility
	}: Props = $props();

	let showHiddenSection = $state(false);
</script>

<div class="sidebar">
	<h2 class="mb-4 font-semibold text-text-primary">
		{scopeMode === 'day' ? 'Day' : 'Week'} Summary
	</h2>

	<div class="mb-4 border-b pb-4 border-border">
		<div class="text-3xl font-bold text-text-primary">{totalHours}h</div>
		<div class="text-sm text-text-secondary">Total hours</div>
	</div>

	<!-- Active Projects -->
	{#if activeProjects.length > 0}
		<div class="mb-4">
			<h3 class="mb-2 text-xs font-medium uppercase tracking-wide text-text-secondary">
				Projects
			</h3>
			<div class="space-y-2">
				{#each activeProjects as project}
					{@const hours = projectTotals.find((t) => t.project.id === project.id)?.hours ?? 0}
					<label class="group flex cursor-pointer items-center justify-between">
						<div class="flex items-center gap-2">
							<input
								type="checkbox"
								checked={visibleProjectIds.has(project.id)}
								onchange={() => ontogglevisibility(project.id)}
								class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-zinc-600 dark:bg-zinc-700"
							/>
							<span
								class="h-3 w-3 flex-shrink-0 rounded-full"
								style="background-color: {project.color}"
							></span>
							<span
								class="text-sm text-text-secondary group-hover:text-text-primary"
							>
								{project.name}
							</span>
						</div>
						{#if hours > 0}
							<span class="text-sm font-medium text-text-secondary">{hours}h</span>
						{/if}
					</label>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Hidden Projects (collapsed by default) -->
	{#if hiddenProjects.length > 0}
		<div class="mb-4 border-t pt-4 border-border">
			<button
				type="button"
				class="mb-2 flex items-center gap-1 text-xs font-medium uppercase tracking-wide text-text-secondary hover:text-text-primary"
				onclick={() => (showHiddenSection = !showHiddenSection)}
			>
				<svg
					class="h-3 w-3 transition-transform {showHiddenSection ? 'rotate-90' : ''}"
					fill="none"
					stroke="currentColor"
					viewBox="0 0 24 24"
				>
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M9 5l7 7-7 7"
					/>
				</svg>
				Hidden ({hiddenProjects.length})
			</button>
			{#if showHiddenSection}
				<div class="space-y-2">
					{#each hiddenProjects as project}
						{@const hours = entries
							.filter(
								(e) => e.project_id === project.id && !e.project?.does_not_accumulate_hours
							)
							.reduce((sum, e) => sum + e.hours, 0)}
						<label class="group flex cursor-pointer items-center justify-between">
							<div class="flex items-center gap-2">
								<input
									type="checkbox"
									checked={visibleProjectIds.has(project.id)}
									onchange={() => ontogglevisibility(project.id)}
									class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-zinc-600 dark:bg-zinc-700"
								/>
								<span
									class="h-3 w-3 flex-shrink-0 rounded-full"
									style="background-color: {project.color}"
								></span>
								<span
									class="text-sm text-text-muted group-hover:text-text-secondary"
								>
									{project.name}
								</span>
							</div>
							{#if hours > 0}
								<span class="text-sm font-medium text-text-muted">{hours}h</span>
							{/if}
						</label>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	<!-- Archived Projects (warning if entries exist) -->
	{#if archivedTotals.length > 0}
		<div class="border-t pt-4 border-border">
			<div
				class="mb-2 flex items-center gap-1 text-xs font-medium uppercase tracking-wide text-amber-600 dark:text-amber-500"
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
					/>
				</svg>
				Archived
			</div>
			<div class="space-y-2">
				{#each archivedTotals as { project, hours }}
					<div class="flex items-center justify-between">
						<div class="flex items-center gap-2">
							<span
								class="h-3 w-3 flex-shrink-0 rounded-full opacity-50"
								style="background-color: {project.color}"
							></span>
							<span class="text-sm text-text-secondary">{project.name}</span>
						</div>
						<span class="text-sm font-medium text-amber-600 dark:text-amber-500">{hours}h</span>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	{#if activeProjects.length === 0 && hiddenProjects.length === 0 && archivedTotals.length === 0}
		<p class="text-sm text-text-muted">
			No entries {scopeMode === 'day' ? 'today' : 'this week'}
		</p>
	{/if}
</div>

<style>
	.sidebar {
		@apply sticky top-4 rounded-lg border bg-surface p-4 border-border;
	}
</style>
