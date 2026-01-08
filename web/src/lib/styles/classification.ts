/**
 * Classification style system for calendar events.
 * Separates styling logic from component rendering to reduce coupling.
 *
 * Classification states:
 * - pending: Event not yet assigned to a project
 * - classified: Event assigned to a project (may need review)
 * - skipped: Event marked as not work time
 */

import { getProjectTextColors, getVerificationTextColor, type ProjectTextColors } from '$lib/utils/colors';

export type ClassificationStatus = 'pending' | 'classified' | 'skipped';

export interface ClassificationState {
	status: ClassificationStatus;
	needsReview: boolean;
	isSkipped: boolean;
	projectColor: string | null;
}

export interface ClassificationStyles {
	/** Tailwind classes for the container element */
	containerClasses: string;
	/** Inline style string for dynamic colors */
	containerStyle: string;
	/** Text color classes/values for content */
	textColors: ProjectTextColors | null;
	/** Verification text color (for outlined style) */
	verificationTextColor: string | null;
	/** Whether the background is the project color (for text contrast) */
	hasProjectBackground: boolean;
}

/**
 * Base container classes shared across all classification states.
 */
const BASE_CONTAINER_CLASSES = 'rounded-lg transition-shadow';

/**
 * Get Tailwind classes for classification container based on state.
 */
function getContainerClasses(state: ClassificationState): string {
	const { status, needsReview, isSkipped } = state;

	if (isSkipped) {
		// Skipped: dashed border, transparent background
		return `${BASE_CONTAINER_CLASSES} bg-transparent border border-dashed border-gray-400 dark:border-gray-500`;
	}

	if (status === 'classified' && needsReview) {
		// Needs verification: outlined style with thick border
		return `${BASE_CONTAINER_CLASSES} bg-white dark:bg-zinc-900 border-2 border-solid`;
	}

	if (status === 'classified') {
		// Confirmed: solid project color background (set via inline style)
		return `${BASE_CONTAINER_CLASSES} border border-solid`;
	}

	// Pending: white/dark background with prominent border
	return `${BASE_CONTAINER_CLASSES} bg-white dark:bg-zinc-900 border-2 border-solid border-black/30 dark:border-white/50`;
}

/**
 * Get inline style for classification container.
 */
function getContainerStyle(state: ClassificationState): string {
	const { status, needsReview, isSkipped, projectColor } = state;

	if (isSkipped || !projectColor) {
		return '';
	}

	if (status === 'classified' && !needsReview) {
		// Confirmed: solid project color background and border
		return `background-color: ${projectColor}; border-color: ${projectColor};`;
	}

	if (status === 'classified' && needsReview) {
		// Needs verification: outlined style with project color border
		return `border-color: ${projectColor};`;
	}

	return '';
}

/**
 * Compute all classification styles for a given state.
 * Use this in components to get pre-computed style values.
 *
 * @example
 * ```svelte
 * const styles = $derived(getClassificationStyles({
 *   status: event.classification_status,
 *   needsReview: event.needs_review,
 *   isSkipped: event.is_skipped,
 *   projectColor: event.project?.color
 * }));
 *
 * <div class={styles.containerClasses} style={styles.containerStyle}>
 * ```
 */
export function getClassificationStyles(state: ClassificationState): ClassificationStyles {
	const { status, needsReview, isSkipped, projectColor } = state;

	const hasProjectBackground = status === 'classified' && !needsReview && !isSkipped && !!projectColor;

	return {
		containerClasses: getContainerClasses(state),
		containerStyle: getContainerStyle(state),
		textColors: hasProjectBackground && projectColor ? getProjectTextColors(projectColor) : null,
		verificationTextColor:
			status === 'classified' && needsReview && !isSkipped && projectColor
				? getVerificationTextColor(projectColor)
				: null,
		hasProjectBackground
	};
}

/**
 * Get text style for primary content (title).
 */
export function getPrimaryTextStyle(styles: ClassificationStyles, isSkipped: boolean): string {
	if (isSkipped) {
		return '';
	}

	if (styles.verificationTextColor) {
		return `color: ${styles.verificationTextColor}`;
	}

	if (styles.textColors) {
		return `color: ${styles.textColors.text}`;
	}

	return '';
}

/**
 * Get text classes for primary content (title).
 */
export function getPrimaryTextClasses(styles: ClassificationStyles, isSkipped: boolean): string {
	if (isSkipped) {
		return 'line-through text-gray-400';
	}

	if (!styles.hasProjectBackground && !styles.verificationTextColor) {
		return 'text-gray-900 dark:text-gray-100';
	}

	return '';
}

/**
 * Get text style for secondary content (time, duration).
 */
export function getSecondaryTextStyle(styles: ClassificationStyles, isSkipped: boolean): string {
	if (isSkipped) {
		return '';
	}

	if (styles.verificationTextColor) {
		return `color: ${styles.verificationTextColor}; opacity: 0.8`;
	}

	if (styles.textColors) {
		return `color: ${styles.textColors.textMuted}`;
	}

	return '';
}

/**
 * Get text classes for secondary content (time, duration).
 */
export function getSecondaryTextClasses(styles: ClassificationStyles, isSkipped: boolean): string {
	if (isSkipped) {
		return 'text-gray-400';
	}

	if (!styles.hasProjectBackground && !styles.verificationTextColor) {
		return 'text-gray-500 dark:text-gray-400';
	}

	return '';
}

/**
 * Get text style for tertiary content (attendees).
 */
export function getTertiaryTextStyle(styles: ClassificationStyles, isSkipped: boolean): string {
	if (isSkipped) {
		return '';
	}

	if (styles.verificationTextColor) {
		return `color: ${styles.verificationTextColor}; opacity: 0.6`;
	}

	if (styles.textColors) {
		return `color: ${styles.textColors.textSubtle}`;
	}

	return '';
}

/**
 * Get text classes for tertiary content (attendees).
 */
export function getTertiaryTextClasses(styles: ClassificationStyles, isSkipped: boolean): string {
	if (isSkipped) {
		return 'text-gray-400';
	}

	if (!styles.hasProjectBackground && !styles.verificationTextColor) {
		return 'text-gray-400';
	}

	return '';
}

/**
 * Format confidence tooltip text.
 */
export function formatConfidenceTitle(
	projectName: string,
	confidence: number | null | undefined,
	source: string | null | undefined
): string {
	if (source === 'manual') return projectName;
	if (confidence != null) {
		return `${projectName} (confidence: ${Math.round(confidence * 100)}%)`;
	}
	return projectName;
}
