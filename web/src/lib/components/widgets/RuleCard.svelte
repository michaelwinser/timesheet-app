<script lang="ts">
	import type { ClassificationRule } from '$lib/api/types';

	interface Props {
		rule: ClassificationRule;
		onedit?: () => void;
		onpreview?: () => void;
		ontoggle?: () => void;
		ondelete?: () => void;
	}

	let { rule, onedit, onpreview, ontoggle, ondelete }: Props = $props();

	let showMenu = $state(false);

	// Determine if this is an attendance rule
	const isAttendanceRule = $derived(rule.attended !== null && rule.attended !== undefined);

	// Check if priority (weight >= 2)
	const isPriority = $derived(rule.weight >= 2);

	// Get display name for target
	const targetDisplay = $derived(() => {
		if (isAttendanceRule) {
			return rule.attended ? 'Attended' : 'Did not attend';
		}
		return rule.project_name || 'Unknown project';
	});

	// Get target color
	const targetColor = $derived(rule.project_color || '#9CA3AF');

	function handleMenuClick(e: Event) {
		e.stopPropagation();
		showMenu = !showMenu;
	}

	function closeMenu() {
		showMenu = false;
	}

	function handleAction(action: (() => void) | undefined) {
		closeMenu();
		action?.();
	}
</script>

<svelte:window onclick={closeMenu} />

<div
	class="border border-gray-200 dark:border-gray-700 rounded-lg p-4 transition-colors {rule.is_enabled
		? 'bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-750'
		: 'bg-gray-50 dark:bg-gray-800/50 opacity-60'}"
>
	<div class="flex items-start justify-between gap-3">
		<div class="flex-1 min-w-0">
			<div class="flex items-center gap-2 mb-1">
				<!-- Enabled/disabled indicator -->
				<div
					class="w-2 h-2 rounded-full flex-shrink-0 {rule.is_enabled
						? 'bg-green-500'
						: 'bg-gray-300 dark:bg-gray-600'}"
				></div>

				<!-- Query -->
				<code class="text-sm font-mono text-gray-900 dark:text-gray-100 truncate">{rule.query}</code>
			</div>

			<div class="flex items-center gap-3 text-sm text-gray-500 dark:text-gray-400">
				<!-- Target -->
				{#if isAttendanceRule}
					<span class="inline-flex items-center gap-1">
						<span
							class="w-4 h-4 rounded-full border-2 border-dashed border-gray-400 flex items-center justify-center text-xs text-gray-400"
							>&#10005;</span
						>
						<span>{targetDisplay()}</span>
					</span>
				{:else}
					<span class="inline-flex items-center gap-1">
						<span class="w-3 h-3 rounded-full" style="background-color: {targetColor}"></span>
						<span>{targetDisplay()}</span>
					</span>
				{/if}

				<!-- Weight -->
				<span>
					Weight: {rule.weight}
					{#if isPriority}
						<span class="ml-1 px-1.5 py-0.5 text-xs bg-yellow-100 dark:bg-yellow-900/50 text-yellow-800 dark:text-yellow-200 rounded"
							>priority</span
						>
					{/if}
				</span>

				<!-- Disabled badge -->
				{#if !rule.is_enabled}
					<span class="px-1.5 py-0.5 text-xs bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-400 rounded">disabled</span>
				{/if}
			</div>
		</div>

		<!-- Menu button -->
		<div class="relative flex-shrink-0">
			<button
				type="button"
				class="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-700 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
				onclick={handleMenuClick}
			>
				<svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
					<path
						d="M10 6a2 2 0 110-4 2 2 0 010 4zM10 12a2 2 0 110-4 2 2 0 010 4zM10 18a2 2 0 110-4 2 2 0 010 4z"
					/>
				</svg>
			</button>

			{#if showMenu}
				<div
					class="absolute right-0 top-8 w-36 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg py-1 z-10"
					onclick={(e) => e.stopPropagation()}
				>
					<button
						type="button"
						class="w-full px-4 py-2 text-left text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
						onclick={() => handleAction(onedit)}
					>
						Edit
					</button>
					<button
						type="button"
						class="w-full px-4 py-2 text-left text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
						onclick={() => handleAction(onpreview)}
					>
						Preview
					</button>
					<div class="border-t border-gray-200 dark:border-gray-700 my-1"></div>
					<button
						type="button"
						class="w-full px-4 py-2 text-left text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
						onclick={() => handleAction(ontoggle)}
					>
						{rule.is_enabled ? 'Disable' : 'Enable'}
					</button>
					<button
						type="button"
						class="w-full px-4 py-2 text-left text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/30"
						onclick={() => handleAction(ondelete)}
					>
						Delete
					</button>
				</div>
			{/if}
		</div>
	</div>
</div>
