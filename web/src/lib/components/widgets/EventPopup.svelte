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
		const popupWidth = 320;
		const popupHeight = 280; // Estimated max height
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
		class="fixed z-50 bg-white rounded-lg shadow-xl border border-gray-200 w-80 max-h-[320px] overflow-hidden"
		style="top: {popupPosition.top}px; left: {popupPosition.left}px;"
		onmouseenter={onmouseenter}
		onmouseleave={onmouseleave}
	>
		<!-- Header with calendar color -->
		<div
			class="px-4 py-3"
			style="border-left: 4px solid {event.calendar_color || '#9CA3AF'};"
		>
			<h3 class="font-semibold text-gray-900 line-clamp-2">{event.title}</h3>
			<div class="flex items-center gap-2 mt-1 text-sm text-gray-600">
				<span>{formatDate(event.start_time)}</span>
				<span class="text-gray-400">·</span>
				<span>{formatTimeRange(event.start_time, event.end_time)}</span>
				<span class="text-gray-400">·</span>
				<span class="font-medium">{getDuration(event.start_time, event.end_time)}</span>
			</div>
		</div>

		<!-- Classification actions (second row) -->
		<div class="px-4 py-2 border-t border-b bg-gray-50">
			<div class="flex flex-wrap gap-2 items-center">
				{#each activeProjects as project}
					<button
						type="button"
						class="w-6 h-6 rounded-full hover:ring-2 hover:ring-offset-1 ring-gray-400 transition-shadow flex items-center justify-center"
						class:ring-2={event.project_id === project.id}
						class:ring-offset-1={event.project_id === project.id}
						style="background-color: {project.color}"
						title={project.name}
						onclick={() => onclassify?.(project.id)}
					></button>
				{/each}
				<button
					type="button"
					class="w-6 h-6 rounded-full border-2 border-dashed border-gray-300 text-gray-400 hover:border-gray-500 hover:text-gray-600 flex items-center justify-center text-xs"
					class:bg-gray-200={event.classification_status === 'skipped'}
					title="Skip - did not attend"
					onclick={() => onskip?.()}
				>
					✕
				</button>
			</div>
		</div>

		<!-- Content -->
		<div class="px-4 py-3 space-y-3 max-h-[160px] overflow-y-auto">
			<!-- Description -->
			{#if event.description}
				<div>
					<span class="text-xs text-gray-500 uppercase tracking-wide block mb-1">Description</span>
					<div class="text-sm text-gray-700 line-clamp-3 prose prose-sm max-w-none">
						{@html sanitizeHtml(event.description)}
					</div>
				</div>
			{/if}

			<!-- Attendees -->
			{#if event.attendees && event.attendees.length > 0}
				<div>
					<span class="text-xs text-gray-500 uppercase tracking-wide block mb-1">
						Attendees ({event.attendees.length})
					</span>
					<p class="text-sm text-gray-700 line-clamp-2">
						{event.attendees.slice(0, 5).join(', ')}{event.attendees.length > 5 ? `, +${event.attendees.length - 5} more` : ''}
					</p>
				</div>
			{/if}

			<!-- Calendar source -->
			{#if event.calendar_name}
				<div class="flex items-center gap-2 text-xs text-gray-500">
					<span
						class="w-2 h-2 rounded-full"
						style="background-color: {event.calendar_color || '#9CA3AF'}"
					></span>
					{event.calendar_name}
				</div>
			{/if}
		</div>
	</div>
{/if}
