<script lang="ts">
	import type { CalendarEvent, Project } from '$lib/api/types';
	import ProjectChip from './ProjectChip.svelte';

	interface Props {
		event: CalendarEvent;
		projects: Project[];
		anchorElement: HTMLElement | null;
		onclassify?: (projectId: string) => void;
		onskip?: () => void;
		onmouseenter?: () => void;
		onmouseleave?: () => void;
	}

	let { event, projects, anchorElement, onclassify, onskip, onmouseenter, onmouseleave }: Props = $props();

	const activeProjects = $derived(projects.filter(p => !p.is_archived));

	// Sanitize HTML - strip dangerous tags, keep basic formatting
	function sanitizeHtml(html: string): string {
		// Create a temporary element to parse HTML
		const temp = document.createElement('div');
		temp.innerHTML = html;

		// Remove script, style, and other dangerous elements
		const dangerous = temp.querySelectorAll('script, style, iframe, object, embed, form, input, button');
		dangerous.forEach(el => el.remove());

		// Remove event handlers from all elements
		temp.querySelectorAll('*').forEach(el => {
			Array.from(el.attributes).forEach(attr => {
				if (attr.name.startsWith('on')) {
					el.removeAttribute(attr.name);
				}
			});
		});

		// Return sanitized HTML
		return temp.innerHTML;
	}

	// Format time range
	function formatTimeRange(start: string, end: string): string {
		const startDate = new Date(start);
		const endDate = new Date(end);
		const options: Intl.DateTimeFormatOptions = { hour: 'numeric', minute: '2-digit' };
		return `${startDate.toLocaleTimeString([], options)} - ${endDate.toLocaleTimeString([], options)}`;
	}

	// Format date
	function formatDate(dateStr: string): string {
		const date = new Date(dateStr);
		return date.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
	}

	// Calculate duration in hours
	function getDuration(start: string, end: string): string {
		const startDate = new Date(start);
		const endDate = new Date(end);
		const hours = (endDate.getTime() - startDate.getTime()) / (1000 * 60 * 60);
		if (hours < 1) {
			return `${Math.round(hours * 60)}m`;
		}
		return hours % 1 === 0 ? `${hours}h` : `${hours.toFixed(1)}h`;
	}

	// Calculate popup position
	const popupPosition = $derived.by(() => {
		if (!anchorElement) return { top: 0, left: 0, placement: 'right' as const };

		const rect = anchorElement.getBoundingClientRect();
		const popupWidth = 420;
		const popupHeight = 400; // Estimated max height
		const gap = 8;
		const viewportWidth = window.innerWidth;
		const viewportHeight = window.innerHeight;

		let left: number;
		let top: number;
		let placement: 'left' | 'right' = 'right';

		// Horizontal positioning: prefer right, fallback to left
		if (rect.right + gap + popupWidth <= viewportWidth) {
			left = rect.right + gap;
			placement = 'right';
		} else if (rect.left - gap - popupWidth >= 0) {
			left = rect.left - gap - popupWidth;
			placement = 'left';
		} else {
			// Center horizontally if neither side works
			left = Math.max(8, (viewportWidth - popupWidth) / 2);
		}

		// Vertical positioning: center on anchor, but keep within viewport
		const anchorCenter = rect.top + rect.height / 2;
		top = anchorCenter - popupHeight / 2;

		// Clamp to viewport
		if (top < 8) top = 8;
		if (top + popupHeight > viewportHeight - 8) {
			top = viewportHeight - popupHeight - 8;
		}

		return { top, left, placement };
	});

</script>

{#if anchorElement}
	<!-- Popup container -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed z-50 bg-white dark:bg-zinc-800 rounded-lg shadow-2xl border border-gray-200 dark:border-white/15 w-[420px] max-h-[400px] overflow-hidden"
		style="top: {popupPosition.top}px; left: {popupPosition.left}px;"
		onmouseenter={onmouseenter}
		onmouseleave={onmouseleave}
	>
		<!-- Header with calendar color -->
		<div
			class="px-4 py-3"
			style="border-left: 4px solid {event.calendar_color || '#9CA3AF'};"
		>
			<h3 class="font-semibold text-gray-900 dark:text-white line-clamp-2">{event.title}</h3>
			<div class="flex items-center gap-2 mt-1 text-sm text-gray-600 dark:text-gray-300">
				<span>{formatDate(event.start_time)}</span>
				<span class="text-gray-400 dark:text-gray-500">·</span>
				<span>{formatTimeRange(event.start_time, event.end_time)}</span>
				<span class="text-gray-400 dark:text-gray-500">·</span>
				<span class="font-medium">{getDuration(event.start_time, event.end_time)}</span>
			</div>
		</div>

		<!-- Classification actions (second row) -->
		<div class="px-4 py-2.5 border-t border-b border-gray-200 dark:border-zinc-700 bg-gray-50 dark:bg-zinc-800/50">
			<div class="flex items-center justify-between">
				<div class="flex flex-wrap gap-2 items-center">
					{#each activeProjects as project}
						<button
							type="button"
							class="w-6 h-6 rounded-full hover:ring-2 hover:ring-offset-1 dark:ring-offset-zinc-800 ring-gray-400 transition-shadow flex items-center justify-center"
							class:ring-2={event.project_id === project.id}
							class:ring-offset-1={event.project_id === project.id}
							style="background-color: {project.color}"
							title={project.name}
							onclick={() => onclassify?.(project.id)}
						></button>
					{/each}
				</div>
				<button
					type="button"
					class="w-6 h-6 rounded border-2 border-dashed border-gray-300 dark:border-zinc-600 text-gray-400 hover:border-gray-500 hover:text-gray-600 dark:hover:border-zinc-400 dark:hover:text-zinc-300 flex items-center justify-center text-xs"
					class:bg-gray-200={event.is_skipped}
					class:dark:bg-zinc-700={event.is_skipped}
					title="Did not attend"
					onclick={() => onskip?.()}
				>
					✕
				</button>
			</div>
		</div>

		<!-- Content -->
		<div class="px-4 py-3 space-y-3 max-h-[220px] overflow-y-auto">
			<!-- Description -->
			{#if event.description}
				<div>
					<span class="text-xs text-gray-500 dark:text-zinc-400 uppercase tracking-wide block mb-1">Description</span>
					<div class="text-sm text-gray-700 dark:text-zinc-300 line-clamp-4 prose prose-sm dark:prose-invert max-w-none">
						{@html sanitizeHtml(event.description)}
					</div>
				</div>
			{/if}

			<!-- Attendees -->
			{#if event.attendees && event.attendees.length > 0}
				<div>
					<span class="text-xs text-gray-500 dark:text-zinc-400 uppercase tracking-wide block mb-1">
						Attendees ({event.attendees.length})
					</span>
					<p class="text-sm text-gray-700 dark:text-zinc-300 line-clamp-2">
						{event.attendees.slice(0, 5).join(', ')}{event.attendees.length > 5 ? `, +${event.attendees.length - 5} more` : ''}
					</p>
				</div>
			{/if}

			<!-- Calendar source with link -->
			{#if event.calendar_name && event.calendar_id}
				<div class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-zinc-400">
					<span
						class="w-2 h-2 rounded-full flex-shrink-0"
						style="background-color: {event.calendar_color || '#9CA3AF'}"
					></span>
					<a
						href="https://calendar.google.com/calendar/event?eid={btoa(event.external_id + ' ' + event.calendar_id)}"
						target="_blank"
						rel="noopener noreferrer"
						class="hover:text-gray-700 dark:hover:text-zinc-200 flex items-center gap-1"
					>
						{event.calendar_name}
						<svg class="w-3 h-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
						</svg>
					</a>
				</div>
			{:else if event.calendar_name}
				<div class="flex items-center gap-1.5 text-xs text-gray-500 dark:text-zinc-400">
					<span
						class="w-2 h-2 rounded-full flex-shrink-0"
						style="background-color: {event.calendar_color || '#9CA3AF'}"
					></span>
					{event.calendar_name}
				</div>
			{/if}
		</div>
	</div>
{/if}
