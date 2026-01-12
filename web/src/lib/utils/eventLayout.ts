/**
 * Event layout algorithm with z-index stacking and horizontal overlap.
 *
 * Features:
 * - Longer events render behind shorter events (z-index stacking)
 * - Shorter events can overlap longer events horizontally if they start
 *   after a "protected" period (configurable)
 * - Overlapping events are indented from the left to show the longer event beneath
 * - Events that can't overlap use standard column-based layout with greedy expansion
 */

// =============================================================================
// Configuration Constants - adjust these to tune the layout behavior
// =============================================================================

/**
 * Protected start period in minutes. A shorter event must start at least this
 * many minutes after a longer event's start to be allowed to overlap it.
 * This ensures the longer event's title/content remains visible.
 */
export const PROTECTED_START_MINUTES = 30;

/**
 * Left indent for overlapping events, in em units.
 * Overlapping events are shifted right by this amount so the longer event
 * beneath remains partially visible on the left.
 */
export const OVERLAP_INDENT_EM = 3;

/**
 * Overlap threshold in minutes. If a shorter event starts less than this
 * many minutes before a longer event ends, treat them as back-to-back
 * (non-overlapping) rather than stacked. This prevents awkward stacking
 * when events barely touch.
 */
export const OVERLAP_THRESHOLD_MINUTES = 9;

// =============================================================================
// Types
// =============================================================================

/**
 * Minimal event interface required for layout calculations.
 * Any event type with id, start_time, and end_time can be used.
 */
export interface LayoutEvent {
	id: string;
	start_time: string;
	end_time: string;
}

/**
 * Layout result for a single event.
 */
export interface EventLayout<T extends LayoutEvent> {
	/** The original event */
	event: T;
	/** Column index (0-based, left to right) - for base events */
	column: number;
	/** Total number of columns in this event's cluster (base events only) */
	totalColumns: number;
	/** Number of columns this event spans (1 = single column, >1 = expanded) */
	span: number;
	/** Whether this event overlays a longer event (uses indent instead of column) */
	isOverlay: boolean;
	/** Z-index for stacking (lower = further back, based on duration) */
	zIndex: number;
}

// =============================================================================
// Helper Functions
// =============================================================================

/**
 * Check if two time ranges collide (overlap in time).
 */
function collide(aStart: number, aEnd: number, bStart: number, bEnd: number): boolean {
	return aStart < bEnd && aEnd > bStart;
}

/**
 * Get event duration in milliseconds.
 */
function getDurationMs(event: LayoutEvent): number {
	return new Date(event.end_time).getTime() - new Date(event.start_time).getTime();
}

/**
 * Maximum z-index for events. Must be below z-40 (modal backdrops) to ensure
 * modals always appear above calendar events.
 */
const MAX_EVENT_ZINDEX = 39;

/**
 * Calculate z-index based on duration. Longer events get lower z-index (render behind).
 * Returns values from 1-39, with shorter events getting higher values.
 * Capped at 39 to stay below modal backdrops (z-40) and popups (z-50).
 */
function calculateZIndex(event: LayoutEvent, allEvents: LayoutEvent[]): number {
	const durations = allEvents.map(getDurationMs);
	const maxDuration = Math.max(...durations);
	const minDuration = Math.min(...durations);
	const eventDuration = getDurationMs(event);

	if (maxDuration === minDuration) return 20; // All same duration

	// Normalize to 1-39 range, shorter = higher z-index
	const normalized = 1 - (eventDuration - minDuration) / (maxDuration - minDuration);
	return Math.round(normalized * (MAX_EVENT_ZINDEX - 1)) + 1;
}

// =============================================================================
// Main Layout Algorithm
// =============================================================================

/**
 * Calculate layout positions for overlapping events with z-index stacking.
 *
 * The algorithm:
 * 1. Groups events into overlapping clusters
 * 2. Within each cluster, identifies "base" events and "overlay" events
 * 3. Base events get column-based layout with greedy expansion
 * 4. Overlay events (shorter events starting 30m+ after a longer event) get
 *    indented positioning and higher z-index
 *
 * @param events - Array of events with id, start_time, and end_time
 * @returns Array of layout results with positions, z-index, and overlay info
 */
export function calculateEventLayout<T extends LayoutEvent>(events: T[]): EventLayout<T>[] {
	if (events.length === 0) return [];

	const protectedMs = PROTECTED_START_MINUTES * 60 * 1000;
	const overlapThresholdMs = OVERLAP_THRESHOLD_MINUTES * 60 * 1000;

	// Sort by start time, then by duration (longest first for better packing)
	const sorted = [...events].sort((a, b) => {
		const aStart = new Date(a.start_time).getTime();
		const bStart = new Date(b.start_time).getTime();
		if (aStart === bStart) {
			return getDurationMs(b) - getDurationMs(a); // Longest first
		}
		return aStart - bStart;
	});

	// Step 1: Group events into overlapping clusters
	const clusters: T[][] = [];
	let currentCluster: T[] = [];
	let clusterEnd = -1;

	for (const event of sorted) {
		const startTime = new Date(event.start_time).getTime();
		const endTime = new Date(event.end_time).getTime();

		if (currentCluster.length === 0) {
			currentCluster.push(event);
			clusterEnd = endTime;
		} else if (startTime < clusterEnd) {
			currentCluster.push(event);
			clusterEnd = Math.max(clusterEnd, endTime);
		} else {
			clusters.push(currentCluster);
			currentCluster = [event];
			clusterEnd = endTime;
		}
	}
	if (currentCluster.length > 0) clusters.push(currentCluster);

	// Step 2: Process each cluster
	const result: EventLayout<T>[] = [];

	for (const cluster of clusters) {
		// Sort cluster by duration (longest first) to determine overlay eligibility
		const byDuration = [...cluster].sort((a, b) => getDurationMs(b) - getDurationMs(a));

		// Separate into base events and overlay events
		const baseEvents: T[] = [];
		const overlayEvents: T[] = [];
		const overlayTargets = new Map<string, T>(); // Maps overlay event id -> the base event it overlays

		for (const event of byDuration) {
			const eventStart = new Date(event.start_time).getTime();
			const eventEnd = new Date(event.end_time).getTime();
			const eventDuration = getDurationMs(event);

			// Check if this event can overlay any existing base event
			let canOverlay = false;
			let targetBase: T | null = null;

			for (const base of baseEvents) {
				const baseStart = new Date(base.start_time).getTime();
				const baseEnd = new Date(base.end_time).getTime();
				const baseDuration = getDurationMs(base);

				// Must be shorter than the base event
				if (eventDuration >= baseDuration) continue;

				// Must temporally overlap
				if (!collide(eventStart, eventEnd, baseStart, baseEnd)) continue;

				// If event starts within threshold of base ending, treat as back-to-back
				if (eventStart >= baseEnd - overlapThresholdMs) continue;

				// Must start after the protected period
				if (eventStart >= baseStart + protectedMs) {
					canOverlay = true;
					targetBase = base;
					break;
				}
			}

			if (canOverlay && targetBase) {
				overlayEvents.push(event);
				overlayTargets.set(event.id, targetBase);
			} else {
				baseEvents.push(event);
			}
		}

		// Step 3: Assign columns to base events only (preserving original start-time order)
		const baseEventsSorted = baseEvents.sort(
			(a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime()
		);

		const columns: T[][] = [];
		const eventColIndex = new Map<string, number>();

		for (const event of baseEventsSorted) {
			const startTime = new Date(event.start_time).getTime();
			let placed = false;

			for (let i = 0; i < columns.length; i++) {
				const colEvents = columns[i];
				const lastEvent = colEvents[colEvents.length - 1];
				const lastEnd = new Date(lastEvent.end_time).getTime();

				if (startTime >= lastEnd) {
					colEvents.push(event);
					eventColIndex.set(event.id, i);
					placed = true;
					break;
				}
			}

			if (!placed) {
				columns.push([event]);
				eventColIndex.set(event.id, columns.length - 1);
			}
		}

		const totalCols = Math.max(columns.length, 1);

		// Step 4: Calculate layout for base events with greedy expansion
		for (const event of baseEventsSorted) {
			const colIndex = eventColIndex.get(event.id) ?? 0;
			const eventStart = new Date(event.start_time).getTime();
			const eventEnd = new Date(event.end_time).getTime();

			let span = 1;
			for (let i = colIndex + 1; i < totalCols; i++) {
				const hasCollision = columns[i]?.some((otherEvt) => {
					const otherStart = new Date(otherEvt.start_time).getTime();
					const otherEnd = new Date(otherEvt.end_time).getTime();
					return collide(eventStart, eventEnd, otherStart, otherEnd);
				});

				if (hasCollision) break;
				span++;
			}

			result.push({
				event,
				column: colIndex,
				totalColumns: totalCols,
				span,
				isOverlay: false,
				zIndex: calculateZIndex(event, cluster)
			});
		}

		// Step 5: Calculate layout for overlay events
		for (const event of overlayEvents) {
			const targetBase = overlayTargets.get(event.id)!;
			const baseColIndex = eventColIndex.get(targetBase.id) ?? 0;

			// Overlay events inherit the base event's column but get marked as overlay
			result.push({
				event,
				column: baseColIndex,
				totalColumns: totalCols,
				span: 1, // Overlays don't expand
				isOverlay: true,
				zIndex: calculateZIndex(event, cluster)
			});
		}
	}

	return result;
}

/**
 * Calculate CSS position values for an event given its layout.
 *
 * @param layout - The event layout result
 * @returns Object with left, width, and zIndex as CSS-ready strings
 */
export function getEventPositionStyle(
	column: number,
	totalColumns: number,
	span: number = 1,
	isOverlay: boolean = false,
	zIndex: number = 1
): { left: string; width: string; zIndex: number } {
	const colWidth = 100 / totalColumns;
	const baseLeft = column * colWidth;
	const width = span * colWidth;

	if (isOverlay) {
		// Overlay events: indent from left edge of their column
		return {
			left: `calc(${baseLeft}% + ${OVERLAP_INDENT_EM}em)`,
			width: `calc(${width}% - ${OVERLAP_INDENT_EM}em)`,
			zIndex
		};
	}

	return {
		left: `${baseLeft}%`,
		width: `${width}%`,
		zIndex
	};
}
