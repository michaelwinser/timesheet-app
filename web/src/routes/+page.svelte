<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { debounce } from '$lib/utils/debounce';
	import AppShell from '$lib/components/AppShell.svelte';
	import { Button, Modal, Input, ToastContainer } from '$lib/components/primitives';
	import {
		ProjectChip,
		TimeEntryCard,
		CalendarEventCard,
		CompactEventCard,
		TimeGrid,
		EventPopup,
		GoToDateModal,
		DateNavigator,
		ProjectSidebar,
		ReclassifyWeekModal
	} from '$lib/components/widgets';
	import { api, type CalendarSyncStatus } from '$lib/api/client';
	import type { Project, TimeEntry, CalendarEvent, CalendarConnection, SyncResult, ClassifiedEvent } from '$lib/api/types';
	import {
		getClassificationStyles,
		getPrimaryTextClasses,
		getPrimaryTextStyle,
		getSecondaryTextClasses,
		getSecondaryTextStyle,
		formatConfidenceTitle,
		getClassificationSourceBadge,
		type ClassificationStatus,
		type ClassificationSource
	} from '$lib/styles';

	// Scope: how many days to show
	type ScopeMode = 'day' | 'week' | 'full-week';
	// Display: how to render events
	type DisplayMode = 'calendar' | 'list';

	// State
	let projects = $state<Project[]>([]);
	let entries = $state<TimeEntry[]>([]);
	let calendarEvents = $state<CalendarEvent[]>([]);
	let calendarConnections = $state<CalendarConnection[]>([]);
	let loading = $state(true);
	let currentDate = $state(getDateFromUrl());
	let scopeMode = $state<ScopeMode>(getScopeModeFromUrl());
	let displayMode = $state<DisplayMode>(getDisplayModeFromUrl());
	let showClassificationPanel = $state(true);
	let classifyingId = $state<string | null>(null);
	let syncing = $state(false);

	// Toast container reference
	let toastContainer: ToastContainer;

	// Derived: most recent sync time across all connections
	const lastSyncedAt = $derived.by(() => {
		let latest: Date | null = null;
		for (const conn of calendarConnections) {
			if (conn.last_synced_at) {
				const syncDate = new Date(conn.last_synced_at);
				if (!latest || syncDate > latest) {
					latest = syncDate;
				}
			}
		}
		return latest;
	});

	// Hover popup state
	let hoveredEvent = $state<CalendarEvent | null>(null);
	let hoveredElement = $state<HTMLElement | null>(null);
	let hoverShowTimeout: ReturnType<typeof setTimeout>;
	let hoverHideTimeout: ReturnType<typeof setTimeout>;

	// Project visibility filtering
	let visibleProjectIds = $state<Set<string>>(new Set());

	// Track date ranges that have been synced on-demand
	let syncedDateRanges = $state<Set<string>>(new Set());

	// Track actual water marks from sync status (for determining if on-demand sync is needed)
	let calendarWaterMarks = $state<{ minDate: Date | null; maxDate: Date | null }>({
		minDate: null,
		maxDate: null
	});

	// Navigation debounce delay (ms) - prevents rapid API calls during quick navigation
	const NAVIGATION_DEBOUNCE_MS = 250;

	// Go to date modal
	let showGoToDateModal = $state(false);

	// Reclassify week modal
	let showReclassifyModal = $state(false);
	let reclassifyLoading = $state(false);
	let reclassifyPreviewResults = $state<ClassifiedEvent[] | null>(null);

	// Global keyboard shortcuts
	function handleGlobalKeydown(e: KeyboardEvent) {
		// Ignore if typing in an input/textarea or if modal is open
		const target = e.target as HTMLElement;
		if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) {
			return;
		}

		// 'g' opens go-to-date modal
		if (e.key === 'g' && !e.ctrlKey && !e.metaKey && !e.altKey) {
			e.preventDefault();
			showGoToDateModal = true;
		}
	}

	function handleGoToDate(date: Date) {
		currentDate = date;
		updateUrl(currentDate, scopeMode, displayMode);
	}

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

	// Computed: check if there are classified events for the selected date/project
	let availableEventsForAdd = $derived.by(() => {
		if (!addDate || !addProjectId) return [];
		const targetDate = new Date(addDate + 'T00:00:00');
		const dateStr = formatDate(targetDate);
		return calendarEvents.filter(
			(e) =>
				e.project_id === addProjectId &&
				e.classification_status === 'classified' &&
				formatDate(new Date(e.start_time)) === dateStr
		);
	});

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
		// Use local date components to avoid timezone conversion issues
		const year = date.getFullYear();
		const month = String(date.getMonth() + 1).padStart(2, '0');
		const day = String(date.getDate()).padStart(2, '0');
		return `${year}-${month}-${day}`;
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

	// Format day header as "5 Mon" (number first, then short day name)
	function formatDayHeaderCompact(date: Date): { num: number; day: string } {
		return {
			num: date.getDate(),
			day: date.toLocaleDateString('en-US', { weekday: 'short' })
		};
	}

	// Calculate hours and project count for a specific day
	function getDayStats(dateStr: string): { hours: number; projectCount: number } {
		const dayEntries = entriesByDate[dateStr] || [];
		const hours = dayEntries.reduce((sum, e) => sum + e.hours, 0);
		const projectIds = new Set(dayEntries.map(e => e.project_id));
		return { hours, projectCount: projectIds.size };
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

	// Filter entries by visible projects
	const filteredEntries = $derived(
		entries.filter((e) => visibleProjectIds.has(e.project_id))
	);

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
			if (e.is_skipped) return true;
			if (e.project_id && visibleProjectIds.has(e.project_id)) return true;
			return false;
		})
	);

	// Weekend events (for warning when in week mode)
	// Uses filteredCalendarEvents so count matches what would be displayed in full-week view
	const weekendEvents = $derived.by(() => {
		if (scopeMode !== 'week') return [];
		return filteredCalendarEvents.filter(e => {
			const eventDate = new Date(e.start_time);
			const day = eventDate.getDay();
			return day === 0 || day === 6; // Sunday or Saturday
		});
	});

	// Helper to format hour label
	function formatHourLabel(hour: number): string {
		return hour === 0 ? '12 AM' :
			hour === 12 ? '12 PM' :
			hour > 12 ? `${hour - 12} PM` :
			`${hour} AM`;
	}

	// Group events by hour for a specific day (excludes all-day events)
	function getEventsByHourForDay(dayEvents: CalendarEvent[]): { hour: number; label: string; events: CalendarEvent[] }[] {
		// Filter out all-day events - they're shown separately
		const timedEvents = dayEvents.filter(e => !isAllDayEvent(e));
		const sorted = [...timedEvents].sort(
			(a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime()
		);

		const groups: { hour: number; label: string; events: CalendarEvent[] }[] = [];
		let currentHour = -1;

		for (const event of sorted) {
			const startHour = new Date(event.start_time).getHours();
			if (startHour !== currentHour) {
				currentHour = startHour;
				groups.push({ hour: startHour, label: formatHourLabel(startHour), events: [] });
			}
			groups[groups.length - 1].events.push(event);
		}

		return groups;
	}

	// Get all-day events for a specific day
	function getAllDayEventsForDay(dayEvents: CalendarEvent[]): CalendarEvent[] {
		return dayEvents.filter(e => isAllDayEvent(e));
	}

	// Group events by date (filtered)
	const eventsByDate = $derived.by(() => {
		const byDate: Record<string, CalendarEvent[]> = {};
		for (const event of filteredCalendarEvents) {
			// Parse the event time and use local date to match formatDate()
			const eventDate = new Date(event.start_time);
			const dateStr = formatDate(eventDate);
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

	// Time grid configuration for unified week view
	const gridStartHour = 0;
	const gridEndHour = 24;
	const hourHeight = 60;
	const viewportHours = 15; // 15 hours visible (50% taller than original 10)
	const gridHeight = (gridEndHour - gridStartHour) * hourHeight;
	const hours = Array.from({ length: gridEndHour - gridStartHour }, (_, i) => gridStartHour + i);

	// Scroll container reference for unified week grid
	let weekScrollContainer: HTMLDivElement;

	// Track when we should scroll (incremented on navigation/view changes, not on event updates)
	let scrollTrigger = $state(0);

	// Calculate first event hour across all visible days for auto-scroll
	function getFirstEventHour(events: Record<string, CalendarEvent[]>, days: Date[]): number {
		let minHour = 8; // Default to 8 AM if no events
		for (const day of days) {
			const dateStr = formatDate(day);
			const dayEvents = events[dateStr] || [];
			for (const event of dayEvents) {
				if (!isAllDayEvent(event)) {
					const hour = new Date(event.start_time).getHours();
					if (hour < minHour) minHour = hour;
				}
			}
		}
		return Math.max(0, minHour - 1); // Scroll to 1 hour before first event
	}

	// Scroll calendar view to first event
	function scrollToFirstEvent() {
		if (!weekScrollContainer || scopeMode === 'day' || displayMode !== 'calendar') return;
		const firstHour = getFirstEventHour(eventsByDate, visibleDays);
		const scrollTop = firstHour * hourHeight;
		weekScrollContainer.scrollTop = scrollTop;
	}

	// Auto-scroll only when scrollTrigger changes (navigation/view changes)
	$effect(() => {
		// Only track the scroll trigger, not event data
		const _trigger = scrollTrigger;

		if (weekScrollContainer && scopeMode !== 'day' && displayMode === 'calendar') {
			requestAnimationFrame(() => scrollToFirstEvent());
		}
	});

	// Detect all-day events
	function isAllDayEvent(event: CalendarEvent): boolean {
		const start = new Date(event.start_time);
		const end = new Date(event.end_time);
		const durationHours = (end.getTime() - start.getTime()) / (1000 * 60 * 60);
		if (durationHours >= 23) return true;
		if (start.getHours() === 0 && start.getMinutes() === 0) {
			if ((end.getHours() === 0 && end.getMinutes() === 0) ||
				(end.getHours() === 23 && end.getMinutes() >= 59)) {
				return true;
			}
		}
		return false;
	}

	// Calculate position and height for an event
	function getEventStyle(event: CalendarEvent): { top: number; height: number } {
		const start = new Date(event.start_time);
		const end = new Date(event.end_time);
		const startMinutes = start.getHours() * 60 + start.getMinutes();
		const endMinutes = end.getHours() * 60 + end.getMinutes();
		const top = (startMinutes / 60) * hourHeight;
		const height = Math.max(((endMinutes - startMinutes) / 60) * hourHeight, 20);
		return { top, height };
	}

	// Calculate overlapping events and assign columns
	function getEventsWithColumns(events: CalendarEvent[]): Array<{ event: CalendarEvent; column: number; totalColumns: number }> {
		if (events.length === 0) return [];
		const sorted = [...events].sort((a, b) =>
			new Date(a.start_time).getTime() - new Date(b.start_time).getTime()
		);
		const result: Array<{ event: CalendarEvent; column: number; totalColumns: number; endTime: number }> = [];
		const columns: number[] = [];
		for (const event of sorted) {
			const startTime = new Date(event.start_time).getTime();
			const endTime = new Date(event.end_time).getTime();
			let column = 0;
			while (column < columns.length && columns[column] > startTime) {
				column++;
			}
			columns[column] = endTime;
			result.push({ event, column, totalColumns: 1, endTime });
		}
		for (let i = 0; i < result.length; i++) {
			const current = result[i];
			const currentStart = new Date(current.event.start_time).getTime();
			const currentEnd = current.endTime;
			let maxColumn = current.column;
			for (let j = 0; j < result.length; j++) {
				const other = result[j];
				const otherStart = new Date(other.event.start_time).getTime();
				const otherEnd = other.endTime;
				if (currentStart < otherEnd && currentEnd > otherStart) {
					maxColumn = Math.max(maxColumn, other.column);
				}
			}
			current.totalColumns = maxColumn + 1;
		}
		return result;
	}

	// Project categories for sidebar
	const activeProjects = $derived(
		projects.filter((p) => !p.is_archived && !p.is_hidden_by_default)
	);
	const hiddenProjects = $derived(
		projects.filter((p) => !p.is_archived && p.is_hidden_by_default)
	);
	const archivedProjects = $derived(projects.filter((p) => p.is_archived));

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
		return Object.values(totals).sort((a, b) => a.project.name.localeCompare(b.project.name));
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
			const [projectsData, entriesData, eventsData, connectionsData] = await Promise.all([
				api.listProjects(),
				api.listTimeEntries({
					start_date: formatDate(startDate),
					end_date: formatDate(endDate)
				}),
				api.listCalendarEvents({
					start_date: formatDate(startDate),
					end_date: formatDate(endDate)
				}),
				api.listCalendarConnections()
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
			calendarConnections = connectionsData;

			// Trigger scroll after data loads (for initial load and date range changes)
			scrollTrigger++;

			// Fetch water marks for on-demand sync decisions
			fetchWaterMarks();
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
			// Populate the project object from local data (API only returns project_id)
			if (result.event.project_id) {
				result.event.project = projects.find((p) => p.id === result.event.project_id);
			}
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

	async function handleUnskip(eventId: string) {
		classifyingId = eventId;
		try {
			const result = await api.classifyCalendarEvent(eventId, { skip: false });
			// Populate the project object from local data if needed
			if (result.event.project_id) {
				result.event.project = projects.find((p) => p.id === result.event.project_id);
			}
			// Update the event in place
			calendarEvents = calendarEvents.map((e) =>
				e.id === eventId ? result.event : e
			);
			// Add or update time entry if returned
			if (result.time_entry) {
				result.time_entry.project = projects.find((p) => p.id === result.time_entry?.project_id);
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
			console.error('Failed to unskip event:', e);
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
			// Hide popup after longer delay (allows moving to popup)
			hoverHideTimeout = setTimeout(() => {
				hoveredEvent = null;
				hoveredElement = null;
			}, 300);
		}
	}

	function handlePopupMouseEnter() {
		// Cancel any pending hide when entering popup
		clearTimeout(hoverHideTimeout);
	}

	function handlePopupMouseLeave() {
		// Hide after delay when leaving popup
		hoverHideTimeout = setTimeout(() => {
			hoveredEvent = null;
			hoveredElement = null;
		}, 100);
	}

	function handlePopupClose() {
		clearTimeout(hoverHideTimeout);
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

	function handlePopupUnskip() {
		if (hoveredEvent) {
			handleUnskip(hoveredEvent.id);
		}
	}

	function navigatePrevious() {
		const d = new Date(currentDate);
		d.setDate(d.getDate() - (scopeMode === 'day' ? 1 : 7));
		currentDate = d;
		updateUrl(d, scopeMode, displayMode);
		scrollTrigger++;
	}

	function navigateNext() {
		const d = new Date(currentDate);
		d.setDate(d.getDate() + (scopeMode === 'day' ? 1 : 7));
		currentDate = d;
		updateUrl(d, scopeMode, displayMode);
		scrollTrigger++;
	}

	function goToToday() {
		const today = getToday();
		currentDate = today;
		updateUrl(today, scopeMode, displayMode);
		scrollTrigger++;
	}

	function setScopeMode(mode: ScopeMode) {
		scopeMode = mode;
		updateUrl(currentDate, mode, displayMode);
		scrollTrigger++;
	}

	function setDisplayMode(mode: DisplayMode) {
		displayMode = mode;
		updateUrl(currentDate, scopeMode, mode);
		scrollTrigger++;
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
			// Navigation
			case 'k':
				navigatePrevious();
				break;
			case 'j':
				navigateNext();
				break;
			case 't':
				goToToday();
				break;
			// Scope modes
			case 'd':
				setScopeMode('day');
				break;
			case 'w':
				setScopeMode('week');
				break;
			case 'f':
				setScopeMode('full-week');
				break;
			// Display modes
			case 'c':
				setDisplayMode('calendar');
				break;
			case 'a':
			case 'l':
				setDisplayMode('list');
				break;
			// Sync
			case 'r':
				handleManualSync();
				break;
		}
	}

	function openAddModal(date: Date) {
		addDate = formatDate(date);
		addProjectId = projects[0]?.id || '';
		addHours = 0; // Set to 0 initially - will be updated based on events
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

	async function handleRefreshEntry(entryId: string) {
		try {
			const refreshed = await api.refreshTimeEntry(entryId);
			refreshed.project = entries.find((e) => e.id === entryId)?.project;
			entries = entries.map((e) => (e.id === entryId ? refreshed : e));
		} catch (e) {
			console.error('Failed to refresh entry:', e);
		}
	}

	// Check if a connection is stale (last synced > 24 hours ago or never synced)
	function isConnectionStale(connection: CalendarConnection): boolean {
		if (!connection.last_synced_at) return true;
		const lastSynced = new Date(connection.last_synced_at);
		const hoursSinceSync = (Date.now() - lastSynced.getTime()) / (1000 * 60 * 60);
		return hoursSinceSync > 24;
	}

	// Fetch water marks from sync status API
	async function fetchWaterMarks() {
		try {
			const status = await api.getSyncStatus();
			// Find the combined min/max across all selected calendars
			let minDate: Date | null = null;
			let maxDate: Date | null = null;

			for (const cal of status.calendars) {
				if (!cal.is_selected) continue;

				if (cal.min_synced_date) {
					const min = new Date(cal.min_synced_date);
					if (!minDate || min < minDate) minDate = min;
				}
				if (cal.max_synced_date) {
					const max = new Date(cal.max_synced_date);
					if (!maxDate || max > maxDate) maxDate = max;
				}
			}

			calendarWaterMarks = { minDate, maxDate };
		} catch (e) {
			console.error('Failed to fetch water marks:', e);
		}
	}

	// Check if a date range is outside the synced water marks
	function isOutsideSyncedWindow(start: Date, end: Date): boolean {
		const { minDate, maxDate } = calendarWaterMarks;

		// If we don't have water marks yet, we can't determine - don't trigger sync
		if (!minDate || !maxDate) return false;

		// If any part of the requested range is outside the synced window, return true
		return start < minDate || end > maxDate;
	}

	// Generate a key for tracking synced date ranges (week granularity)
	function getDateRangeKey(start: Date, end: Date): string {
		return `${formatDate(start)}:${formatDate(end)}`;
	}

	// Trigger on-demand sync for dates outside the synced window
	// Per PRD: fetch only the viewed week, not a large range
	async function onDemandSync(viewedStart: Date, viewedEnd: Date) {
		const rangeKey = getDateRangeKey(viewedStart, viewedEnd);

		// Skip if we've already synced this exact range
		if (syncedDateRanges.has(rangeKey)) {
			return;
		}

		try {
			const connections = await api.listCalendarConnections();
			if (connections.length === 0) {
				return;
			}

			syncing = true;
			console.log(`[SYNC] on-demand: range=${formatDate(viewedStart)} to ${formatDate(viewedEnd)}`);

			// Sync all connections with just the viewed date range
			await Promise.all(
				connections.map((conn) =>
					api.syncCalendar(conn.id, {
						start_date: formatDate(viewedStart),
						end_date: formatDate(viewedEnd)
					})
				)
			);

			// Mark this range as synced
			syncedDateRanges = new Set([...syncedDateRanges, rangeKey]);

			// Update water marks after sync
			await fetchWaterMarks();

			// Reload events for the viewed range
			const eventsData = await api.listCalendarEvents({
				start_date: formatDate(viewedStart),
				end_date: formatDate(viewedEnd)
			});
			calendarEvents = eventsData;
		} catch (e) {
			console.error('[SYNC] on-demand failed:', e);
		} finally {
			syncing = false;
		}
	}

	// Auto-sync stale connections in the background
	async function autoSyncStaleConnections() {
		try {
			const connections = await api.listCalendarConnections();
			calendarConnections = connections;
			const staleConnections = connections.filter(isConnectionStale);

			if (staleConnections.length > 0) {
				syncing = true;
				// Sync all stale connections in parallel
				await Promise.all(staleConnections.map((conn) => api.syncCalendar(conn.id)));
				// Reload events and connections after sync
				const [eventsData, updatedConnections] = await Promise.all([
					api.listCalendarEvents({
						start_date: formatDate(startDate),
						end_date: formatDate(endDate)
					}),
					api.listCalendarConnections()
				]);
				calendarEvents = eventsData;
				calendarConnections = updatedConnections;
			}
		} catch (e) {
			console.error('Auto-sync failed:', e);
		} finally {
			syncing = false;
		}
	}

	// Manual sync triggered by user
	async function handleManualSync() {
		if (syncing || calendarConnections.length === 0) return;

		syncing = true;
		try {
			// Sync all connections in parallel
			const results = await Promise.all(
				calendarConnections.map((conn) => api.syncCalendar(conn.id))
			);

			// Calculate totals
			const totals = results.reduce(
				(acc, r) => ({
					created: acc.created + r.events_created,
					updated: acc.updated + r.events_updated,
					orphaned: acc.orphaned + r.events_orphaned
				}),
				{ created: 0, updated: 0, orphaned: 0 }
			);

			// Reload events and connections
			const [eventsData, updatedConnections] = await Promise.all([
				api.listCalendarEvents({
					start_date: formatDate(startDate),
					end_date: formatDate(endDate)
				}),
				api.listCalendarConnections()
			]);
			calendarEvents = eventsData;
			calendarConnections = updatedConnections;

			// Show success toast
			if (totals.created > 0 || totals.updated > 0) {
				const parts = [];
				if (totals.created > 0) parts.push(`${totals.created} new`);
				if (totals.updated > 0) parts.push(`${totals.updated} updated`);
				toastContainer?.success(`Sync complete: ${parts.join(', ')} events`);
			} else {
				toastContainer?.info('Sync complete: Calendar is up to date');
			}
		} catch (e) {
			console.error('Manual sync failed:', e);
			toastContainer?.error('Sync failed. Please try again.');
		} finally {
			syncing = false;
		}
	}

	// Reclassify week handlers
	function openReclassifyModal() {
		showReclassifyModal = true;
		reclassifyPreviewResults = null;
	}

	function closeReclassifyModal() {
		showReclassifyModal = false;
		reclassifyPreviewResults = null;
		reclassifyLoading = false;
	}

	async function handleReclassifyPreview() {
		reclassifyLoading = true;
		try {
			const result = await api.applyRules({
				start_date: formatDate(weekStart),
				end_date: formatDate(endDate),
				dry_run: true
			});
			reclassifyPreviewResults = result.classified;
		} catch (e) {
			console.error('Failed to preview reclassification:', e);
			toastContainer?.error('Failed to preview changes. Please try again.');
		} finally {
			reclassifyLoading = false;
		}
	}

	async function handleReclassifyConfirm() {
		reclassifyLoading = true;
		try {
			const result = await api.applyRules({
				start_date: formatDate(weekStart),
				end_date: formatDate(endDate),
				dry_run: false
			});

			// Close modal
			showReclassifyModal = false;
			reclassifyPreviewResults = null;

			// Reload data to reflect changes
			await loadData();

			// Show success toast
			const count = result.classified.length;
			if (count > 0) {
				toastContainer?.success(`Reclassified ${count} event${count === 1 ? '' : 's'}`);
			} else {
				toastContainer?.info('No events were reclassified');
			}
		} catch (e) {
			console.error('Failed to reclassify events:', e);
			toastContainer?.error('Reclassification failed. Please try again.');
		} finally {
			reclassifyLoading = false;
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

	// Debounced data loading - prevents rapid API calls during quick navigation
	const debouncedLoadData = debounce(() => {
		loadData();
	}, NAVIGATION_DEBOUNCE_MS);

	// Debounced on-demand sync
	const debouncedOnDemandSync = debounce((start: Date, end: Date) => {
		if (isOutsideSyncedWindow(start, end)) {
			onDemandSync(start, end);
		}
	}, NAVIGATION_DEBOUNCE_MS);

	// Visibility change handler - refresh when tab becomes visible
	function handleVisibilityChange() {
		if (document.visibilityState === 'visible') {
			// Refresh data when user returns to this tab
			loadData();
			// Also check for stale connections
			autoSyncStaleConnections();
		}
	}

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

		// Add visibility change listener for multi-tab sync
		document.addEventListener('visibilitychange', handleVisibilityChange);

		return () => {
			window.removeEventListener('keydown', handleKeydown);
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});

	// Reload when date range changes (debounced to prevent rapid API calls)
	$effect(() => {
		// Track the date range
		const start = startDate;
		const end = endDate;
		debouncedLoadData();

		// Trigger on-demand sync if viewing dates outside the default window (debounced)
		debouncedOnDemandSync(start, end);
	});
</script>

<svelte:head>
	<title>{scopeMode === 'day' ? 'Day' : scopeMode === 'week' ? 'Week' : 'Full Week'} View - Timesheet</title>
</svelte:head>

<svelte:window onkeydown={handleGlobalKeydown} />

<AppShell wide>
	<!-- Consolidated Header -->
	<DateNavigator
		{currentDate}
		{scopeMode}
		{displayMode}
		{weekStart}
		weekdaysEnd={weekdaysOnly[4]}
		weekEnd={endDate}
		weekendEventCount={weekendEvents.length}
		{lastSyncedAt}
		{syncing}
		hasCalendarConnections={calendarConnections.length > 0}
		reclassifying={reclassifyLoading}
		onnavigateprevious={navigatePrevious}
		onnavigatenext={navigateNext}
		ongototoday={goToToday}
		onscopechange={setScopeMode}
		ondisplaychange={setDisplayMode}
		onsync={handleManualSync}
		onreclassify={openReclassifyModal}
	/>

	<!-- Project Summary Bar -->
	{@const unclassifiedHours = calendarEvents
		.filter(e => e.classification_status === 'pending')
		.reduce((sum, e) => {
			const start = new Date(e.start_time);
			const end = new Date(e.end_time);
			return sum + (end.getTime() - start.getTime()) / (1000 * 60 * 60);
		}, 0)}
	<div class="mb-4 flex items-center gap-3 px-3 py-2 bg-gray-100 dark:bg-zinc-800 rounded-lg overflow-x-auto">
		{#each projectTotals as { project, hours }}
			<div class="flex items-center gap-1.5 px-2 py-1 bg-white dark:bg-zinc-700 rounded-full text-sm whitespace-nowrap">
				<span class="w-2.5 h-2.5 rounded-full flex-shrink-0" style="background-color: {project.color}"></span>
				<span class="text-gray-700 dark:text-gray-300">{project.name}</span>
				<span class="text-gray-500 dark:text-gray-400">({hours}h)</span>
			</div>
		{/each}
		{#if unclassifiedHours > 0}
			<div class="flex items-center gap-1.5 px-2 py-1 border border-dashed border-gray-300 dark:border-zinc-600 rounded-full text-sm whitespace-nowrap">
				<span class="w-2.5 h-2.5 rounded-full flex-shrink-0 bg-gray-400 dark:bg-gray-500 opacity-50"></span>
				<span class="text-gray-500 dark:text-gray-400">Unclassified</span>
				<span class="text-gray-500 dark:text-gray-400">({Math.round(unclassifiedHours * 10) / 10}h)</span>
			</div>
		{/if}
		<span class="ml-auto text-sm font-medium text-gray-600 dark:text-gray-300 whitespace-nowrap">
			{totalHours + Math.round(unclassifiedHours * 10) / 10}h total
		</span>
	</div>

	<!-- Calendar Panel -->
	<div class="mb-6 bg-white dark:bg-zinc-800 border border-gray-200 dark:border-zinc-700 rounded-lg p-4 relative">
			<!-- Sync loading overlay -->
			{#if syncing}
				<div class="absolute inset-0 bg-white/80 dark:bg-zinc-800/80 rounded-lg flex items-center justify-center z-10">
					<div class="flex items-center gap-3 bg-white dark:bg-zinc-700 px-4 py-3 rounded-lg shadow-lg border border-gray-200 dark:border-zinc-600">
						<div class="animate-spin rounded-full h-5 w-5 border-b-2 border-primary-600"></div>
						<span class="text-sm font-medium text-gray-700 dark:text-gray-200">Syncing calendar...</span>
					</div>
				</div>
			{/if}
			{#if displayMode === 'calendar'}
				<!-- Time Grid View -->
				{#if scopeMode === 'day'}
					<!-- Single day - full width time grid -->
					{@const dateStr = formatDate(currentDate)}
					{@const dayEvents = eventsByDate[dateStr] || []}
					<div class="bg-gray-50 dark:bg-zinc-900 rounded-lg p-4">
						<TimeGrid
							events={dayEvents}
							{projects}
							date={currentDate}
							{scrollTrigger}
							onclassify={(eventId, projectId) => handleClassify(eventId, projectId)}
							onskip={(eventId) => handleSkip(eventId)}
							onunskip={(eventId) => handleUnskip(eventId)}
							onhover={handleEventHover}
						/>
					</div>
				{:else}
					<!-- Week view - unified single scroller -->
					{@const allDayEventsForWeek = visibleDays.flatMap(day => {
						const dateStr = formatDate(day);
						return (eventsByDate[dateStr] || []).filter(e => isAllDayEvent(e)).map(e => ({ ...e, dayDate: day }));
					})}

					<!-- Day headers (outside scroll, above all-day events) -->
					<div class="flex mb-1">
						<div class="w-12 flex-shrink-0"></div>
						<div class="flex-1 grid" style="grid-template-columns: repeat({visibleDays.length}, minmax(0, 1fr));">
							{#each visibleDays as day}
								{@const dateStr = formatDate(day)}
								{@const isToday = formatDate(new Date()) === dateStr}
								{@const header = formatDayHeaderCompact(day)}
								{@const stats = getDayStats(dateStr)}
								<div class="text-center py-1 px-1 {isToday ? 'bg-zinc-100 dark:bg-zinc-800 border-b-2 border-primary-500' : 'border-b border-transparent'}">
									<div class="flex items-baseline justify-center gap-1">
										<span class="text-lg font-semibold {isToday ? 'text-gray-900 dark:text-white' : 'text-gray-700 dark:text-gray-300'}">{header.num}</span>
										<span class="text-xs uppercase {isToday ? 'text-gray-600 dark:text-gray-400' : 'text-gray-500 dark:text-gray-500'}">{header.day}</span>
									</div>
									<div class="text-xs text-gray-400 dark:text-gray-500 mt-0.5">
										{#if stats.hours > 0}
											{stats.hours}h · {stats.projectCount} project{stats.projectCount !== 1 ? 's' : ''}
										{:else}
											—
										{/if}
									</div>
								</div>
							{/each}
						</div>
					</div>

					<!-- All-day events row (below headers) -->
					{#if allDayEventsForWeek.length > 0}
						<div class="mb-2 border-b border-gray-200 dark:border-zinc-700 pb-2 flex">
							<div class="w-12 flex-shrink-0 text-xs text-gray-400 dark:text-gray-500 text-right pr-2 pt-0.5">All day</div>
							<div class="flex-1 grid gap-2" style="grid-template-columns: repeat({visibleDays.length}, minmax(0, 1fr));">
								{#each visibleDays as day}
									{@const dateStr = formatDate(day)}
									{@const dayAllDayEvents = (eventsByDate[dateStr] || []).filter(e => isAllDayEvent(e))}
									<div class="flex flex-wrap gap-1">
										{#each dayAllDayEvents as event (event.id)}
											<CompactEventCard
												{event}
												{projects}
												variant="chip"
												maxProjectButtons={3}
												onclassify={(projectId) => handleClassify(event.id, projectId)}
												onskip={() => handleSkip(event.id)}
												onunskip={() => handleUnskip(event.id)}
												onhover={(el) => handleEventHover(el ? event : null, el)}
											/>
										{/each}
									</div>
								{/each}
							</div>
						</div>
					{/if}

					<!-- Unified scroll container -->
					<div
						class="overflow-y-auto overflow-x-hidden bg-gray-50 dark:bg-zinc-900 rounded-lg"
						style="height: {viewportHours * hourHeight}px"
						bind:this={weekScrollContainer}
					>
						<div class="flex" style="height: {gridHeight}px">
							<!-- Time legend -->
							<div class="w-12 flex-shrink-0 text-right pr-2 relative">
								{#each hours as hour, i}
									<div class="absolute text-xs text-gray-400 dark:text-gray-500" style="top: {i * hourHeight}px">
										{hour === 0 ? '12 AM' : hour === 12 ? '12 PM' : hour > 12 ? `${hour - 12} PM` : `${hour} AM`}
									</div>
								{/each}
							</div>

							<!-- Day columns grid -->
							<div class="flex-1 grid gap-2 relative" style="grid-template-columns: repeat({visibleDays.length}, minmax(0, 1fr));">
								<!-- Hour lines (spanning all columns) -->
								{#each hours as hour, i}
									<div
										class="absolute w-full border-t border-gray-200 dark:border-zinc-700 pointer-events-none"
										style="top: {i * hourHeight}px; left: 0; right: 0;"
									></div>
								{/each}

								<!-- Day columns with events -->
								{#each visibleDays as day}
									{@const dateStr = formatDate(day)}
									{@const dayEvents = (eventsByDate[dateStr] || []).filter(e => !isAllDayEvent(e))}
									{@const isToday = formatDate(new Date()) === dateStr}
									{@const eventsWithCols = getEventsWithColumns(dayEvents)}
									{@const activeProjectsList = projects.filter(p => !p.is_archived)}

									<div class="relative border-l border-gray-200 dark:border-zinc-700 {isToday ? 'bg-zinc-100/50 dark:bg-zinc-800/50' : ''}">
										{#each eventsWithCols as { event, column, totalColumns } (event.id)}
											{@const style = getEventStyle(event)}
											{@const width = 100 / totalColumns}
											{@const left = column * width}
											{@const isPending = event.classification_status === 'pending'}
											{@const isClassified = event.classification_status === 'classified'}
											{@const isSkipped = event.is_skipped === true}
											{@const needsReview = event.needs_review === true}
											{@const projectColor = isSkipped ? null : (event.project?.color ?? null)}
											{@const styles = getClassificationStyles({
												status: event.classification_status as 'pending' | 'classified' | 'skipped',
												needsReview,
												isSkipped,
												projectColor
											})}

											<!-- svelte-ignore a11y_no_static_element_interactions -->
											<div
												class="absolute rounded-md overflow-hidden text-xs {styles.containerClasses} hover:shadow-md transition-shadow cursor-pointer"
												style="
													top: {style.top}px;
													height: calc({style.height}px - 1px);
													left: calc({left}% + 2px);
													width: calc({width}% - 4px);
													{styles.containerStyle}
												"
												onmouseenter={(e) => handleEventHover(event, e.currentTarget as HTMLElement)}
												onmouseleave={() => handleEventHover(null, null)}
											>
												<div class="p-1 h-full flex flex-col">
													<!-- Title row -->
													<div class="flex items-start justify-between gap-1 min-w-0">
														<span
															class="font-medium truncate flex-1 {getPrimaryTextClasses(styles, isSkipped)}"
															style={getPrimaryTextStyle(styles, isSkipped)}
														>{event.title}</span>
														<!-- Project dot with source badge: only for confirmed classified events -->
														{#if isClassified && !needsReview && event.project}
															{@const sourceBadge = getClassificationSourceBadge(event.classification_source as ClassificationSource, event.project.name)}
															<div class="flex items-center gap-0.5 flex-shrink-0 mt-0.5">
																<span
																	class="w-3 h-3 rounded-full {styles.textColors?.isDark ? 'border border-white/50' : ''}"
																	style="background-color: {event.project.color}"
																	title={formatConfidenceTitle(event.project.name, event.classification_confidence, event.classification_source)}
																></span>
																{#if sourceBadge}
																	<span
																		class="text-[7px] font-medium {styles.textColors?.isDark ? 'text-white/70' : 'text-gray-500'}"
																		title={sourceBadge.tooltip}
																	>{sourceBadge.label}</span>
																{/if}
															</div>
														{/if}
													</div>

													<!-- Bottom row: project buttons (left) and skip button (right) -->
													{#if isPending && style.height >= 40}
														<div class="mt-auto pt-1 flex items-center justify-between">
															<div class="flex items-center gap-0.5">
																{#each activeProjectsList.slice(0, 4) as project, i}
																	{@const isBestGuess = event.suggested_project_id === project.id || (!event.suggested_project_id && i === 0)}
																	<button
																		type="button"
																		class="w-3.5 h-3.5 rounded-full transition-shadow {isBestGuess ? 'ring-1 ring-black/40 ring-offset-1 ring-offset-white dark:ring-white/60 dark:ring-offset-zinc-900' : 'ring-gray-400 hover:ring-2 hover:ring-offset-1'}"
																		style="background-color: {project.color}"
																		title="{project.name}{isBestGuess ? ' (suggested)' : ''}"
																		onclick={(e) => { e.stopPropagation(); handleClassify(event.id, project.id); }}
																	></button>
																{/each}
															</div>
															<button
																type="button"
																class="w-3.5 h-3.5 rounded border border-dashed border-gray-400 text-gray-400 hover:border-gray-600 hover:text-gray-600 flex items-center justify-center text-[7px]"
																title="Did not attend"
																onclick={(e) => { e.stopPropagation(); handleSkip(event.id); }}
															>✕</button>
														</div>
													{:else if isSkipped}
														<!-- Skip indicator in bottom right - clickable to unskip -->
														<div class="mt-auto flex justify-end">
															<button
																type="button"
																class="w-3.5 h-3.5 rounded border border-dashed border-gray-400 text-gray-400 hover:border-gray-600 hover:text-gray-600 flex items-center justify-center text-[7px]"
																title="Click to mark as attended"
																onclick={(e) => { e.stopPropagation(); handleUnskip(event.id); }}
															>✕</button>
														</div>
													{/if}
												</div>
											</div>
										{/each}
									</div>
								{/each}
							</div>
						</div>
					</div>
				{/if}
			{:else}
				<!-- List View with day columns -->
				{#if scopeMode === 'day'}
					<!-- Single day list -->
					{@const dateStr = formatDate(currentDate)}
					{@const dayEvents = eventsByDate[dateStr] || []}
					{@const allDayEvents = getAllDayEventsForDay(dayEvents)}
					{@const hourGroups = getEventsByHourForDay(dayEvents)}
					<div class="bg-gray-50 dark:bg-zinc-900 rounded-lg p-4 max-h-[32rem] overflow-y-auto">
						{#if allDayEvents.length > 0 || hourGroups.length > 0}
							<div class="space-y-3">
								<!-- All-day events section -->
								{#if allDayEvents.length > 0}
									<div>
										<div class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">All day</div>
										<div class="space-y-px">
											{#each allDayEvents as event (event.id)}
												<div
													class:opacity-50={classifyingId === event.id}
													class:pointer-events-none={classifyingId === event.id}
												>
													<CompactEventCard
														{event}
														{projects}
														variant="card"
														onclassify={(projectId) => handleClassify(event.id, projectId)}
														onskip={() => handleSkip(event.id)}
														onunskip={() => handleUnskip(event.id)}
														onhover={(el) => handleEventHover(el ? event : null, el)}
													/>
												</div>
											{/each}
										</div>
									</div>
								{/if}
								<!-- Timed events by hour -->
								{#each hourGroups as group}
									<div>
										<div class="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">{group.label}</div>
										<div class="space-y-px">
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
							<p class="text-sm text-gray-400 py-8 text-center">No calendar events for this day</p>
						{/if}
					</div>
				{:else}
					<!-- Week/Full week - day columns -->
					<div class="overflow-x-auto">
						<div class="grid gap-2" style="grid-template-columns: repeat({visibleDays.length}, minmax(0, 1fr));">
							{#each visibleDays as day}
								{@const dateStr = formatDate(day)}
								{@const dayEvents = eventsByDate[dateStr] || []}
								{@const allDayEvents = getAllDayEventsForDay(dayEvents)}
								{@const hourGroups = getEventsByHourForDay(dayEvents)}
								{@const isToday = formatDate(new Date()) === dateStr}

								{@const header = formatDayHeaderCompact(day)}
								{@const stats = getDayStats(dateStr)}
								<div class="bg-gray-50 dark:bg-zinc-900 rounded-lg p-2 max-h-[32rem] overflow-y-auto {isToday ? 'ring-2 ring-primary-500' : ''}">
									<div class="text-center mb-2 pb-1 {isToday ? 'border-b-2 border-primary-500' : 'border-b border-gray-200 dark:border-zinc-700'}">
										<div class="flex items-baseline justify-center gap-1">
											<span class="text-lg font-semibold {isToday ? 'text-gray-900 dark:text-white' : 'text-gray-700 dark:text-gray-300'}">{header.num}</span>
											<span class="text-xs uppercase {isToday ? 'text-gray-600 dark:text-gray-400' : 'text-gray-500 dark:text-gray-500'}">{header.day}</span>
										</div>
										<div class="text-xs text-gray-400 dark:text-gray-500 mt-0.5">
											{#if stats.hours > 0}
												{stats.hours}h · {stats.projectCount} project{stats.projectCount !== 1 ? 's' : ''}
											{:else}
												—
											{/if}
										</div>
									</div>
									{#if allDayEvents.length > 0 || hourGroups.length > 0}
										<div class="space-y-2">
											<!-- All-day events -->
											{#if allDayEvents.length > 0}
												<div>
													<div class="text-xs font-medium text-gray-400 dark:text-gray-500 mb-1">All day</div>
													<div class="space-y-px">
														{#each allDayEvents as event (event.id)}
															<CompactEventCard
																{event}
																{projects}
																variant="compact"
																maxProjectButtons={3}
																onclassify={(projectId) => handleClassify(event.id, projectId)}
																onskip={() => handleSkip(event.id)}
																onunskip={() => handleUnskip(event.id)}
																onhover={(el) => handleEventHover(el ? event : null, el)}
															/>
														{/each}
													</div>
												</div>
											{/if}
											<!-- Timed events by hour -->
											{#each hourGroups as group}
												<div>
													<div class="text-xs font-medium text-gray-400 dark:text-gray-500 mb-1">{group.label}</div>
													<div class="space-y-px">
														{#each group.events as event (event.id)}
															<CompactEventCard
																{event}
																{projects}
																variant="compact"
																showTime={true}
																onclassify={(projectId) => handleClassify(event.id, projectId)}
																onskip={() => handleSkip(event.id)}
																onunskip={() => handleUnskip(event.id)}
																onhover={(el) => handleEventHover(el ? event : null, el)}
															/>
														{/each}
													</div>
												</div>
											{/each}
										</div>
									{:else}
										<p class="text-xs text-gray-400 py-4 text-center">No events</p>
									{/if}
								</div>
							{/each}
						</div>
					</div>
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

						<div class="bg-white dark:bg-zinc-800 border border-gray-200 dark:border-zinc-700 rounded-lg p-4 {isToday ? 'ring-2 ring-primary-500' : ''}">
							<div class="flex items-center justify-between mb-3">
								<div class="flex items-center gap-3">
									<span class="font-medium text-gray-900 dark:text-white">
										{scopeMode === 'day' ? 'Time Entries' : formatDayLabel(day)}
									</span>
									{#if dayTotal > 0}
										<span class="text-sm text-gray-500 dark:text-gray-400">{dayTotal}h</span>
									{/if}
								</div>
								<button
									type="button"
									class="text-primary-600 dark:text-primary-400 hover:text-primary-700 dark:hover:text-primary-300 text-sm font-medium"
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
											onrefresh={() => handleRefreshEntry(entry.id)}
										/>
									{/each}
								</div>
							{:else}
								<p class="text-sm text-gray-400 dark:text-gray-500">No entries</p>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Sidebar with project totals -->
		<div class="lg:w-72">
			<ProjectSidebar
				{scopeMode}
				{totalHours}
				{activeProjects}
				{hiddenProjects}
				{projectTotals}
				{archivedTotals}
				{visibleProjectIds}
				{entries}
				ontogglevisibility={toggleProjectVisibility}
			/>
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
			onmouseenter={handlePopupMouseEnter}
			onmouseleave={handlePopupMouseLeave}
		/>
	{/if}

	<!-- Add entry modal -->
	<Modal bind:open={showAddModal} title="Add Time Entry">
		<form class="space-y-4" onsubmit={(e) => { e.preventDefault(); handleAddEntry(); }}>
			<div>
				<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Project</label>
				<select
					bind:value={addProjectId}
					class="block w-full rounded-md border-gray-300 dark:border-zinc-600 dark:bg-zinc-700 dark:text-white shadow-sm focus:border-primary-500 focus:ring-primary-500 sm:text-sm"
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

			<div>
				<Input
					type="number"
					label="Hours"
					bind:value={addHours}
					step="0.25"
					min="0"
					required
				/>
				{#if availableEventsForAdd.length > 0}
					<p class="mt-1 text-xs text-blue-600 dark:text-blue-400">
						{availableEventsForAdd.length} calendar event{availableEventsForAdd.length > 1 ? 's' : ''} found for this date and project.
						{#if addHours === 0}
							Hours will be automatically calculated from these events.
						{:else}
							Set hours to 0 to auto-calculate from events.
						{/if}
					</p>
				{/if}
			</div>

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

<!-- Go to Date Modal (opened with 'g' key) -->
<GoToDateModal
	bind:open={showGoToDateModal}
	referenceDate={currentDate}
	onselect={handleGoToDate}
/>

<!-- Reclassify Week Modal -->
<ReclassifyWeekModal
	bind:open={showReclassifyModal}
	{weekStart}
	weekEnd={endDate}
	events={calendarEvents}
	{projects}
	loading={reclassifyLoading}
	previewResults={reclassifyPreviewResults}
	onclose={closeReclassifyModal}
	onpreview={handleReclassifyPreview}
	onconfirm={handleReclassifyConfirm}
/>

<!-- Toast notifications -->
<ToastContainer bind:this={toastContainer} />
