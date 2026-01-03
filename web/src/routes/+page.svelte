<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Modal, Input } from '$lib/components/primitives';
	import { ProjectChip, TimeEntryCard, CalendarEventCard, TimeGrid, EventPopup } from '$lib/components/widgets';
	import { api } from '$lib/api/client';
	import type { Project, TimeEntry, CalendarEvent, CalendarConnection } from '$lib/api/types';

	// Scope: how many days to show
	type ScopeMode = 'day' | 'week' | 'full-week';
	// Display: how to render events
	type DisplayMode = 'calendar' | 'list';

	// State
	let projects = $state<Project[]>([]);
	let entries = $state<TimeEntry[]>([]);
	let calendarEvents = $state<CalendarEvent[]>([]);
	let loading = $state(true);
	let currentDate = $state(getDateFromUrl());
	let scopeMode = $state<ScopeMode>(getScopeModeFromUrl());
	let displayMode = $state<DisplayMode>(getDisplayModeFromUrl());
	let showClassificationPanel = $state(true);
	let classifyingId = $state<string | null>(null);
	let syncing = $state(false);

	// Hover popup state
	let hoveredEvent = $state<CalendarEvent | null>(null);
	let hoveredElement = $state<HTMLElement | null>(null);
	let hoverShowTimeout: ReturnType<typeof setTimeout>;
	let hoverHideTimeout: ReturnType<typeof setTimeout>;

	// Project visibility filtering
	let visibleProjectIds = $state<Set<string>>(new Set());
	let showHiddenSection = $state(false);

	// Get date from URL or default to today
	function getDateFromUrl(): Date {
		if (typeof window !== 'undefined') {
			const params = new URLSearchParams(window.location.search);
			const dateParam = params.get('date');
			if (dateParam) {
				const parsed = new Date(dateParam + 'T00:00:00');
				if (!isNaN(parsed.getTime())) {
					return parsed;
				}
			}
		}
		return getToday();
	}

	// Get scope mode from URL or default to 'week'
	function getScopeModeFromUrl(): ScopeMode {
		if (typeof window !== 'undefined') {
			const params = new URLSearchParams(window.location.search);
			const scopeParam = params.get('scope');
			if (scopeParam === 'day' || scopeParam === 'week' || scopeParam === 'full-week') {
				return scopeParam;
			}
			// Legacy support: map old 'view' param
			const viewParam = params.get('view');
			if (viewParam === 'day') return 'day';
			if (viewParam === 'week') return 'week';
		}
		return 'week';
	}

	// Get display mode from URL or default to 'calendar'
	function getDisplayModeFromUrl(): DisplayMode {
		if (typeof window !== 'undefined') {
			const params = new URLSearchParams(window.location.search);
			const displayParam = params.get('display');
			if (displayParam === 'calendar' || displayParam === 'list') {
				return displayParam;
			}
		}
		return 'calendar';
	}

	// Update URL when date or modes change
	function updateUrl(date: Date, scope: ScopeMode, display: DisplayMode) {
		const dateStr = formatDate(date);
		const url = new URL(window.location.href);
		url.searchParams.set('date', dateStr);
		url.searchParams.set('scope', scope);
		url.searchParams.set('display', display);
		// Clean up legacy param
		url.searchParams.delete('view');
		goto(url.pathname + url.search, { replaceState: true, keepFocus: true });
	}

	// Add entry modal state
	let showAddModal = $state(false);
	let addDate = $state('');
	let addProjectId = $state('');
	let addHours = $state(1);
	let addDescription = $state('');
	let addSubmitting = $state(false);

	function getToday(): Date {
		const d = new Date();
		d.setHours(0, 0, 0, 0);
		return d;
	}

	function getWeekStart(date: Date): Date {
		const d = new Date(date);
		const day = d.getDay();
		const diff = d.getDate() - day + (day === 0 ? -6 : 1);
		d.setDate(diff);
		d.setHours(0, 0, 0, 0);
		return d;
	}

	function formatDate(date: Date): string {
		return date.toISOString().split('T')[0];
	}

	function getWeekDays(weekStart: Date): Date[] {
		const days = [];
		for (let i = 0; i < 7; i++) {
			const d = new Date(weekStart);
			d.setDate(d.getDate() + i);
			days.push(d);
		}
		return days;
	}

	function formatDayLabel(date: Date): string {
		return date.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' });
	}

	function formatFullDayLabel(date: Date): string {
		return date.toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' });
	}

	function formatShortDay(date: Date): string {
		return date.toLocaleDateString('en-US', { weekday: 'short', day: 'numeric' });
	}

	function formatTimeRange(start: string, end: string): string {
		const startDate = new Date(start);
		const endDate = new Date(end);
		const options: Intl.DateTimeFormatOptions = { hour: 'numeric', minute: '2-digit' };
		return `${startDate.toLocaleTimeString([], options)} - ${endDate.toLocaleTimeString([], options)}`;
	}

	// Derived date range based on scope mode
	const weekStart = $derived(getWeekStart(currentDate));
	const weekDays = $derived(getWeekDays(weekStart));
	const weekdaysOnly = $derived(weekDays.slice(0, 5)); // Mon-Fri

	// The visible days depend on the scope mode
	const visibleDays = $derived.by(() => {
		if (scopeMode === 'day') return [currentDate];
		if (scopeMode === 'week') return weekdaysOnly;
		return weekDays; // full-week
	});

	// Date range for API calls - always fetch full week to detect weekend events
	const startDate = $derived(scopeMode === 'day' ? currentDate : weekStart);
	const endDate = $derived.by(() => {
		if (scopeMode === 'day') {
			return currentDate;
		} else {
			// Always fetch through Sunday for weekend detection
			return new Date(weekStart.getTime() + 6 * 24 * 60 * 60 * 1000);
		}
	});

	// Weekend events (for warning when in week mode)
	const weekendEvents = $derived.by(() => {
		if (scopeMode !== 'week') return [];
		return calendarEvents.filter(e => {
			const eventDate = new Date(e.start_time);
			const day = eventDate.getDay();
			return day === 0 || day === 6; // Sunday or Saturday
		});
	});

	// Group entries by date (filtered by visible projects)
	const entriesByDate = $derived.by(() => {
		const byDate: Record<string, TimeEntry[]> = {};
		for (const entry of filteredEntries) {
			if (!byDate[entry.date]) {
				byDate[entry.date] = [];
			}
			byDate[entry.date].push(entry);
		}
		return byDate;
	});

	// Pending events (for the count)
	const pendingEvents = $derived(calendarEvents.filter(e => e.classification_status === 'pending'));

	// Events that need review (auto-classified with medium confidence)
	const reviewEvents = $derived(calendarEvents.filter(e => e.needs_review === true));

	// Filter calendar events by visible projects
	// Show: pending events (need classification), skipped events, and events classified to visible projects
	const filteredCalendarEvents = $derived(
		calendarEvents.filter(e => {
			if (e.classification_status === 'pending') return true;
			if (e.classification_status === 'skipped') return true;
			if (e.project_id && visibleProjectIds.has(e.project_id)) return true;
			return false;
		})
	);

	// Group filtered events by hour for list view
	const eventsByHour = $derived.by(() => {
		// First sort by start time
		const sorted = [...filteredCalendarEvents].sort(
			(a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime()
		);

		// Group by hour
		const groups: { hour: number; label: string; events: CalendarEvent[] }[] = [];
		let currentHour = -1;

		for (const event of sorted) {
			const startHour = new Date(event.start_time).getHours();
			if (startHour !== currentHour) {
				currentHour = startHour;
				const label = startHour === 0 ? '12 AM' :
					startHour === 12 ? '12 PM' :
					startHour > 12 ? `${startHour - 12} PM` :
					`${startHour} AM`;
				groups.push({ hour: startHour, label, events: [] });
			}
			groups[groups.length - 1].events.push(event);
		}

		return groups;
	});

	// Group events by date (filtered)
	const eventsByDate = $derived.by(() => {
		const byDate: Record<string, CalendarEvent[]> = {};
		for (const event of filteredCalendarEvents) {
			const dateStr = event.start_time.split('T')[0];
			if (!byDate[dateStr]) {
				byDate[dateStr] = [];
			}
			byDate[dateStr].push(event);
		}
		// Sort events within each day by start time
		for (const date in byDate) {
			byDate[date].sort((a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime());
		}
		return byDate;
	});

	// Project categories for sidebar
	const activeProjects = $derived(
		projects.filter((p) => !p.is_archived && !p.is_hidden_by_default)
	);
	const hiddenProjects = $derived(
		projects.filter((p) => !p.is_archived && p.is_hidden_by_default)
	);
	const archivedProjects = $derived(projects.filter((p) => p.is_archived));

	// Filter entries by visible projects
	const filteredEntries = $derived(
		entries.filter((e) => visibleProjectIds.has(e.project_id))
	);

	// Entries from archived projects (for warning)
	const archivedEntries = $derived(
		entries.filter((e) => e.project?.is_archived)
	);

	// Calculate project totals (from filtered entries only)
	const projectTotals = $derived.by(() => {
		const totals: Record<string, { project: Project; hours: number }> = {};
		for (const entry of filteredEntries) {
			if (entry.project && !entry.project.does_not_accumulate_hours) {
				if (!totals[entry.project_id]) {
					totals[entry.project_id] = { project: entry.project, hours: 0 };
				}
				totals[entry.project_id].hours += entry.hours;
			}
		}
		return Object.values(totals).sort((a, b) => b.hours - a.hours);
	});

	// Totals for archived entries (shown in warning)
	const archivedTotals = $derived.by(() => {
		const totals: Record<string, { project: Project; hours: number }> = {};
		for (const entry of archivedEntries) {
			if (entry.project && !entry.project.does_not_accumulate_hours) {
				if (!totals[entry.project_id]) {
					totals[entry.project_id] = { project: entry.project, hours: 0 };
				}
				totals[entry.project_id].hours += entry.hours;
			}
		}
		return Object.values(totals).sort((a, b) => b.hours - a.hours);
	});

	const totalHours = $derived(
		filteredEntries
			.filter((e) => !e.project?.does_not_accumulate_hours)
			.reduce((sum, e) => sum + e.hours, 0)
	);

	async function loadData() {
		loading = true;
		try {
			const [projectsData, entriesData, eventsData] = await Promise.all([
				api.listProjects(),
				api.listTimeEntries({
					start_date: formatDate(startDate),
					end_date: formatDate(endDate)
				}),
				api.listCalendarEvents({
					start_date: formatDate(startDate),
					end_date: formatDate(endDate)
				})
			]);
			// Initialize visible projects BEFORE setting events to ensure correct filtering
			const initialVisible = new Set<string>();
			for (const p of projectsData) {
				if (!p.is_archived && !p.is_hidden_by_default) {
					initialVisible.add(p.id);
				}
			}
			visibleProjectIds = initialVisible;

			// Now set the data - filtering will use the correct visibility
			projects = projectsData;
			entries = entriesData;
			calendarEvents = eventsData;
		} catch (e) {
			console.error('Failed to load data:', e);
		} finally {
			loading = false;
		}
	}

	function toggleProjectVisibility(projectId: string) {
		const newSet = new Set(visibleProjectIds);
		if (newSet.has(projectId)) {
			newSet.delete(projectId);
		} else {
			newSet.add(projectId);
		}
		visibleProjectIds = newSet;
	}

	async function handleClassify(eventId: string, projectId: string) {
		classifyingId = eventId;
		try {
			const result = await api.classifyCalendarEvent(eventId, { project_id: projectId });
			// Update the event in place
			calendarEvents = calendarEvents.map((e) =>
				e.id === eventId ? result.event : e
			);
			// Add or update time entry if returned
			if (result.time_entry) {
				result.time_entry.project = projects.find((p) => p.id === result.time_entry?.project_id);
				// Check if entry for same date/project exists
				const existingIdx = entries.findIndex(
					(e) => e.project_id === result.time_entry?.project_id && e.date === result.time_entry?.date
				);
				if (existingIdx >= 0) {
					entries = entries.map((e, i) => (i === existingIdx ? result.time_entry! : e));
				} else {
					entries = [...entries, result.time_entry];
				}
			}
		} catch (e) {
			console.error('Failed to classify event:', e);
		} finally {
			classifyingId = null;
		}
	}

	async function handleSkip(eventId: string) {
		classifyingId = eventId;
		try {
			const result = await api.classifyCalendarEvent(eventId, { skip: true });
			// Update the event in place
			calendarEvents = calendarEvents.map((e) =>
				e.id === eventId ? result.event : e
			);
		} catch (e) {
			console.error('Failed to skip event:', e);
		} finally {
			classifyingId = null;
		}
	}

	// Handle hover events for popup
	function handleEventHover(event: CalendarEvent | null, element: HTMLElement | null) {
		clearTimeout(hoverShowTimeout);
		clearTimeout(hoverHideTimeout);

		if (event && element) {
			// Show popup after short delay
			hoverShowTimeout = setTimeout(() => {
				hoveredEvent = event;
				hoveredElement = element;
			}, 150);
		} else {
			// Hide popup after short delay (allows moving to popup)
			hoverHideTimeout = setTimeout(() => {
				hoveredEvent = null;
				hoveredElement = null;
			}, 100);
		}
	}

	function handlePopupClose() {
		hoveredEvent = null;
		hoveredElement = null;
	}

	function handlePopupClassify(projectId: string) {
		if (hoveredEvent) {
			handleClassify(hoveredEvent.id, projectId);
		}
	}

	function handlePopupSkip() {
		if (hoveredEvent) {
			handleSkip(hoveredEvent.id);
		}
	}

	function navigatePrevious() {
		const d = new Date(currentDate);
		d.setDate(d.getDate() - (scopeMode === 'day' ? 1 : 7));
		currentDate = d;
		updateUrl(d, scopeMode, displayMode);
	}

	function navigateNext() {
		const d = new Date(currentDate);
		d.setDate(d.getDate() + (scopeMode === 'day' ? 1 : 7));
		currentDate = d;
		updateUrl(d, scopeMode, displayMode);
	}

	function goToToday() {
		const today = getToday();
		currentDate = today;
		updateUrl(today, scopeMode, displayMode);
	}

	function setScopeMode(mode: ScopeMode) {
		scopeMode = mode;
		updateUrl(currentDate, mode, displayMode);
	}

	function setDisplayMode(mode: DisplayMode) {
		displayMode = mode;
		updateUrl(currentDate, scopeMode, mode);
	}

	// Keyboard shortcuts
	function handleKeydown(event: KeyboardEvent) {
		// Ignore if typing in an input field
		const target = event.target as HTMLElement;
		if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT') {
			return;
		}
		// Ignore if modifier keys are pressed
		if (event.metaKey || event.ctrlKey || event.altKey) {
			return;
		}

		switch (event.key.toLowerCase()) {
			case 'd':
				setScopeMode('day');
				break;
			case 'w':
				setScopeMode('week');
				break;
			case 'f':
				setScopeMode('full-week');
				break;
			case 'c':
				setDisplayMode('calendar');
				break;
			case 'a':
				setDisplayMode('list');
				break;
		}
	}

	function openAddModal(date: Date) {
		addDate = formatDate(date);
		addProjectId = projects[0]?.id || '';
		addHours = 1;
		addDescription = '';
		showAddModal = true;
	}

	async function handleAddEntry() {
		if (!addProjectId) return;
		addSubmitting = true;
		try {
			const newEntry = await api.createTimeEntry({
				project_id: addProjectId,
				date: addDate,
				hours: addHours,
				description: addDescription || undefined
			});
			// Find project and attach
			newEntry.project = projects.find((p) => p.id === newEntry.project_id);
			entries = [...entries, newEntry];
			showAddModal = false;
		} catch (e) {
			console.error('Failed to add entry:', e);
		} finally {
			addSubmitting = false;
		}
	}

	async function handleUpdateEntry(entryId: string, data: { hours?: number; description?: string }) {
		try {
			const updated = await api.updateTimeEntry(entryId, data);
			updated.project = entries.find((e) => e.id === entryId)?.project;
			entries = entries.map((e) => (e.id === entryId ? updated : e));
		} catch (e) {
			console.error('Failed to update entry:', e);
		}
	}

	async function handleDeleteEntry(entryId: string) {
		try {
			await api.deleteTimeEntry(entryId);
			entries = entries.filter((e) => e.id !== entryId);
		} catch (e) {
			console.error('Failed to delete entry:', e);
		}
	}

	// Check if a connection is stale (last synced > 24 hours ago or never synced)
	function isConnectionStale(connection: CalendarConnection): boolean {
		if (!connection.last_synced_at) return true;
		const lastSynced = new Date(connection.last_synced_at);
		const hoursSinceSync = (Date.now() - lastSynced.getTime()) / (1000 * 60 * 60);
		return hoursSinceSync > 24;
	}

	// Auto-sync stale connections in the background
	async function autoSyncStaleConnections() {
		try {
			const connections = await api.listCalendarConnections();
			const staleConnections = connections.filter(isConnectionStale);

			if (staleConnections.length > 0) {
				syncing = true;
				// Sync all stale connections in parallel
				await Promise.all(staleConnections.map((conn) => api.syncCalendar(conn.id)));
				// Reload events after sync
				const eventsData = await api.listCalendarEvents({
					start_date: formatDate(startDate),
					end_date: formatDate(endDate)
				});
				calendarEvents = eventsData;
			}
		} catch (e) {
			console.error('Auto-sync failed:', e);
		} finally {
			syncing = false;
		}
	}

	// Handle URL changes (browser back/forward)
	$effect(() => {
		const dateParam = $page.url.searchParams.get('date');
		const scopeParam = $page.url.searchParams.get('scope');
		const displayParam = $page.url.searchParams.get('display');

		if (dateParam) {
			const parsed = new Date(dateParam + 'T00:00:00');
			if (!isNaN(parsed.getTime()) && formatDate(parsed) !== formatDate(currentDate)) {
				currentDate = parsed;
			}
		}

		if (scopeParam === 'day' || scopeParam === 'week' || scopeParam === 'full-week') {
			if (scopeParam !== scopeMode) {
				scopeMode = scopeParam;
			}
		}

		if (displayParam === 'calendar' || displayParam === 'list') {
			if (displayParam !== displayMode) {
				displayMode = displayParam;
			}
		}
	});

	// Load on mount
	onMount(() => {
		// Set URL if not already set
		if (!window.location.search.includes('date=')) {
			updateUrl(currentDate, scopeMode, displayMode);
		}
		loadData();
		// Trigger auto-sync for stale connections (runs in background)
		autoSyncStaleConnections();

		// Add keyboard listener
		window.addEventListener('keydown', handleKeydown);
		return () => window.removeEventListener('keydown', handleKeydown);
	});

	// Reload when date range changes
	$effect(() => {
		// Track the date range
		startDate;
		endDate;
		loadData();
	});
</script>

<svelte:head>
	<title>{scopeMode === 'day' ? 'Day' : scopeMode === 'week' ? 'Week' : 'Full Week'} View - Timesheet</title>
</svelte:head>

<AppShell wide>
	<!-- Navigation (Global) -->
	<div class="flex items-center justify-between mb-6">
		<div class="flex items-center gap-2">
			<Button variant="ghost" onclick={navigatePrevious}>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
				</svg>
			</Button>
			<h1 class="text-lg font-semibold text-gray-900">
				{#if scopeMode === 'day'}
					{formatFullDayLabel(currentDate)}
				{:else if scopeMode === 'week'}
					{weekStart.toLocaleDateString('en-US', { month: 'long', day: 'numeric' })} -
					{weekdaysOnly[4].toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })}
				{:else}
					{weekStart.toLocaleDateString('en-US', { month: 'long', day: 'numeric' })} -
					{endDate.toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })}
				{/if}
			</h1>
			<Button variant="ghost" onclick={navigateNext}>
				<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
				</svg>
			</Button>
		</div>
		<div class="flex items-center gap-3">
			<!-- Scope mode toggle -->
			<div class="flex bg-gray-100 rounded-lg p-0.5">
				<button
					type="button"
					class="px-3 py-1 text-sm rounded-md transition-colors {scopeMode === 'day' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
					onclick={() => setScopeMode('day')}
					title="Day (D)"
				>
					Day
				</button>
				<button
					type="button"
					class="px-3 py-1 text-sm rounded-md transition-colors {scopeMode === 'week' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
					onclick={() => setScopeMode('week')}
					title="Week Mon-Fri (W)"
				>
					Week
				</button>
				<button
					type="button"
					class="px-3 py-1 text-sm rounded-md transition-colors {scopeMode === 'full-week' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
					onclick={() => setScopeMode('full-week')}
					title="Full Week Mon-Sun (F)"
				>
					Full
				</button>
			</div>
			<!-- Weekend warning -->
			{#if weekendEvents.length > 0}
				<button
					type="button"
					class="flex items-center gap-1 px-2 py-1 text-xs bg-amber-100 text-amber-700 rounded-md hover:bg-amber-200 transition-colors"
					onclick={() => setScopeMode('full-week')}
					title="Click to show full week"
				>
					<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					</svg>
					{weekendEvents.length} weekend
				</button>
			{/if}
			<Button variant="secondary" size="sm" onclick={goToToday}>
				Today
			</Button>
		</div>
	</div>

	<!-- Sync indicator -->
	{#if syncing}
		<div class="mb-4 flex items-center gap-2 text-sm text-gray-600">
			<svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
				<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
				<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
			</svg>
			Syncing calendar events...
		</div>
	{/if}

	<!-- Calendar Events Panel -->
	<div class="mb-6 bg-white border rounded-lg p-4">
		<div class="flex items-center justify-between mb-3">
			<div class="flex items-center gap-2">
				<svg class="w-5 h-5 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
				</svg>
				<h2 class="font-semibold text-gray-900">
					Calendar Events
					{#if pendingEvents.length > 0}
						<span class="ml-2 px-2 py-0.5 text-xs bg-amber-100 text-amber-700 rounded-full">
							{pendingEvents.length} to classify
						</span>
					{/if}
					{#if reviewEvents.length > 0}
						<span class="ml-2 px-2 py-0.5 text-xs bg-yellow-100 text-yellow-700 rounded-full">
							{reviewEvents.length} to review
						</span>
					{/if}
				</h2>
			</div>
			<div class="flex items-center gap-2">
				<!-- Display mode toggle -->
				<div class="flex bg-gray-100 rounded-lg p-0.5">
					<button
						type="button"
						class="px-2 py-1 text-xs rounded-md transition-colors {displayMode === 'calendar' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
						onclick={() => setDisplayMode('calendar')}
						title="Calendar view (C)"
					>
						Calendar
					</button>
					<button
						type="button"
						class="px-2 py-1 text-xs rounded-md transition-colors {displayMode === 'list' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'}"
						onclick={() => setDisplayMode('list')}
						title="List view (A)"
					>
						List
					</button>
				</div>
				<button
					type="button"
					class="text-gray-500 hover:text-gray-700 text-sm"
					onclick={() => showClassificationPanel = !showClassificationPanel}
				>
					{showClassificationPanel ? 'Hide' : 'Show'}
				</button>
			</div>
		</div>

		{#if showClassificationPanel}
			{#if displayMode === 'calendar'}
				<!-- Time Grid View -->
				{#if scopeMode === 'day'}
					<!-- Single day - full width time grid -->
					{@const dateStr = formatDate(currentDate)}
					{@const dayEvents = eventsByDate[dateStr] || []}
					<div class="bg-gray-50 rounded-lg p-4">
						<TimeGrid
							events={dayEvents}
							{projects}
							date={currentDate}
							onclassify={(eventId, projectId) => handleClassify(eventId, projectId)}
							onskip={(eventId) => handleSkip(eventId)}
							onhover={handleEventHover}
						/>
					</div>
				{:else}
					<!-- Week view - 7 columns -->
					<div class="overflow-x-auto">
						<div class="grid grid-cols-7 gap-2">
							{#each visibleDays as day}
								{@const dateStr = formatDate(day)}
								{@const dayEvents = eventsByDate[dateStr] || []}
								{@const isToday = formatDate(new Date()) === dateStr}

								<div class="bg-gray-50 rounded-lg p-2 {isToday ? 'ring-2 ring-primary-500' : ''}">
									<h3 class="font-medium text-sm text-center mb-2 pb-1 border-b {isToday ? 'text-primary-600' : 'text-gray-700'}">
										{formatShortDay(day)}
									</h3>
									<TimeGrid
										events={dayEvents}
										{projects}
										date={day}
										onclassify={(eventId, projectId) => handleClassify(eventId, projectId)}
										onskip={(eventId) => handleSkip(eventId)}
										onhover={handleEventHover}
									/>
								</div>
							{/each}
						</div>
					</div>
				{/if}
			{:else}
				<!-- List View with hour groups -->
				{#if eventsByHour.length > 0}
					<div class="space-y-4 max-h-[32rem] overflow-y-auto">
						{#each eventsByHour as group}
							<div>
								<!-- Hour header -->
								<div class="flex items-center gap-2 mb-2">
									<span class="text-xs font-medium text-gray-500 uppercase tracking-wide">{group.label}</span>
									<div class="flex-1 border-t border-gray-200"></div>
									<span class="text-xs text-gray-400">{group.events.length} event{group.events.length !== 1 ? 's' : ''}</span>
								</div>
								<!-- Events in this hour -->
								<div class="space-y-2 pl-2">
									{#each group.events as event (event.id)}
										<div class={classifyingId === event.id ? 'opacity-50 pointer-events-none' : ''}>
											<CalendarEventCard
												{event}
												{projects}
												onclassify={(projectId) => handleClassify(event.id, projectId)}
												onskip={() => handleSkip(event.id)}
											/>
										</div>
									{/each}
								</div>
							</div>
						{/each}
					</div>
				{:else}
					<p class="text-sm text-gray-400 py-8 text-center">No calendar events for this {scopeMode === 'day' ? 'day' : 'week'}</p>
				{/if}
			{/if}
		{/if}
	</div>

	<div class="flex flex-col lg:flex-row gap-6">
		<!-- Time entries view -->
		<div class="flex-1">
			{#if loading}
				<div class="flex items-center justify-center py-12">
					<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
				</div>
			{:else}
				<!-- Day columns -->
				<div class="space-y-4">
					{#each visibleDays as day}
						{@const dateStr = formatDate(day)}
						{@const dayEntries = entriesByDate[dateStr] || []}
						{@const dayTotal = dayEntries.reduce((sum, e) => sum + e.hours, 0)}
						{@const isToday = formatDate(new Date()) === dateStr}

						<div class="bg-white border rounded-lg p-4 {isToday ? 'ring-2 ring-primary-500' : ''}">
							<div class="flex items-center justify-between mb-3">
								<div class="flex items-center gap-3">
									<span class="font-medium {isToday ? 'text-primary-600' : 'text-gray-900'}">
										{scopeMode === 'day' ? 'Time Entries' : formatDayLabel(day)}
									</span>
									{#if dayTotal > 0}
										<span class="text-sm text-gray-500">{dayTotal}h</span>
									{/if}
								</div>
								<button
									type="button"
									class="text-primary-600 hover:text-primary-700 text-sm font-medium"
									onclick={() => openAddModal(day)}
								>
									+ Add
								</button>
							</div>

							{#if dayEntries.length > 0}
								<div class="space-y-2">
									{#each dayEntries as entry (entry.id)}
										<TimeEntryCard
											{entry}
											onupdate={(data) => handleUpdateEntry(entry.id, data)}
											ondelete={() => handleDeleteEntry(entry.id)}
										/>
									{/each}
								</div>
							{:else}
								<p class="text-sm text-gray-400">No entries</p>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Sidebar with project totals -->
		<div class="lg:w-72">
			<div class="bg-white border rounded-lg p-4 sticky top-4">
				<h2 class="font-semibold text-gray-900 mb-4">{scopeMode === 'day' ? 'Day' : 'Week'} Summary</h2>

				<div class="mb-4 pb-4 border-b">
					<div class="text-3xl font-bold text-gray-900">{totalHours}h</div>
					<div class="text-sm text-gray-500">Total hours</div>
				</div>

				<!-- Active Projects -->
				{#if activeProjects.length > 0}
					<div class="mb-4">
						<h3 class="text-xs font-medium text-gray-500 uppercase tracking-wide mb-2">Projects</h3>
						<div class="space-y-2">
							{#each activeProjects as project}
								{@const hours = projectTotals.find(t => t.project.id === project.id)?.hours ?? 0}
								<label class="flex items-center justify-between cursor-pointer group">
									<div class="flex items-center gap-2">
										<input
											type="checkbox"
											checked={visibleProjectIds.has(project.id)}
											onchange={() => toggleProjectVisibility(project.id)}
											class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
										/>
										<span
											class="w-3 h-3 rounded-full flex-shrink-0"
											style="background-color: {project.color}"
										></span>
										<span class="text-sm text-gray-700 group-hover:text-gray-900">{project.name}</span>
									</div>
									{#if hours > 0}
										<span class="text-sm font-medium text-gray-500">{hours}h</span>
									{/if}
								</label>
							{/each}
						</div>
					</div>
				{/if}

				<!-- Hidden Projects (collapsed by default) -->
				{#if hiddenProjects.length > 0}
					<div class="mb-4 border-t pt-4">
						<button
							type="button"
							class="flex items-center gap-1 text-xs font-medium text-gray-500 uppercase tracking-wide mb-2 hover:text-gray-700"
							onclick={() => showHiddenSection = !showHiddenSection}
						>
							<svg
								class="w-3 h-3 transition-transform {showHiddenSection ? 'rotate-90' : ''}"
								fill="none"
								stroke="currentColor"
								viewBox="0 0 24 24"
							>
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
							</svg>
							Hidden ({hiddenProjects.length})
						</button>
						{#if showHiddenSection}
							<div class="space-y-2">
								{#each hiddenProjects as project}
									{@const hours = entries.filter(e => e.project_id === project.id && !e.project?.does_not_accumulate_hours).reduce((sum, e) => sum + e.hours, 0)}
									<label class="flex items-center justify-between cursor-pointer group">
										<div class="flex items-center gap-2">
											<input
												type="checkbox"
												checked={visibleProjectIds.has(project.id)}
												onchange={() => toggleProjectVisibility(project.id)}
												class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
											/>
											<span
												class="w-3 h-3 rounded-full flex-shrink-0"
												style="background-color: {project.color}"
											></span>
											<span class="text-sm text-gray-500 group-hover:text-gray-700">{project.name}</span>
										</div>
										{#if hours > 0}
											<span class="text-sm font-medium text-gray-400">{hours}h</span>
										{/if}
									</label>
								{/each}
							</div>
						{/if}
					</div>
				{/if}

				<!-- Archived Projects (warning if entries exist) -->
				{#if archivedTotals.length > 0}
					<div class="border-t pt-4">
						<div class="flex items-center gap-1 text-xs font-medium text-amber-600 uppercase tracking-wide mb-2">
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
							</svg>
							Archived
						</div>
						<div class="space-y-2">
							{#each archivedTotals as { project, hours }}
								<div class="flex items-center justify-between">
									<div class="flex items-center gap-2">
										<span
											class="w-3 h-3 rounded-full flex-shrink-0 opacity-50"
											style="background-color: {project.color}"
										></span>
										<span class="text-sm text-gray-500">{project.name}</span>
									</div>
									<span class="text-sm font-medium text-amber-600">{hours}h</span>
								</div>
							{/each}
						</div>
					</div>
				{/if}

				{#if activeProjects.length === 0 && hiddenProjects.length === 0 && archivedTotals.length === 0}
					<p class="text-sm text-gray-400">No entries {scopeMode === 'day' ? 'today' : 'this week'}</p>
				{/if}
			</div>
		</div>
	</div>

	<!-- Event hover popup -->
	{#if hoveredEvent}
		<EventPopup
			event={hoveredEvent}
			{projects}
			anchorElement={hoveredElement}
			onclassify={handlePopupClassify}
			onskip={handlePopupSkip}
			onclose={handlePopupClose}
		/>
	{/if}

	<!-- Add entry modal -->
	<Modal bind:open={showAddModal} title="Add Time Entry">
		<form class="space-y-4" onsubmit={(e) => { e.preventDefault(); handleAddEntry(); }}>
			<div>
				<label class="block text-sm font-medium text-gray-700 mb-1">Project</label>
				<select
					bind:value={addProjectId}
					class="block w-full rounded-md border-gray-300 shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
					style="padding: 0.5rem 0.75rem; border-width: 1px;"
				>
					{#each projects.filter((p) => !p.is_archived) as project}
						<option value={project.id}>{project.name}</option>
					{/each}
				</select>
			</div>

			<Input
				type="date"
				label="Date"
				bind:value={addDate}
				required
			/>

			<Input
				type="number"
				label="Hours"
				bind:value={addHours}
				required
			/>

			<Input
				type="text"
				label="Description (optional)"
				bind:value={addDescription}
				placeholder="What did you work on?"
			/>

			<div class="flex justify-end gap-3 pt-4">
				<Button variant="secondary" onclick={() => (showAddModal = false)}>
					Cancel
				</Button>
				<Button type="submit" loading={addSubmitting}>
					Add Entry
				</Button>
			</div>
		</form>
	</Modal>
</AppShell>
