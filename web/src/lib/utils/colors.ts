/**
 * Color utility functions for computing text colors based on background luminance.
 * Centralizes the logic for determining readable text on project-colored backgrounds.
 */

export interface ProjectTextColors {
	/** Primary text color: 'white' for dark backgrounds, 'black' for light */
	text: string;
	/** Muted/secondary text color with appropriate opacity */
	textMuted: string;
	/** Even more subtle text (for tertiary info like attendees) */
	textSubtle: string;
	/** Whether the background is considered dark (for conditional styling) */
	isDark: boolean;
}

/**
 * Calculate relative luminance of a hex color.
 * Uses the standard formula: 0.299*R + 0.587*G + 0.114*B
 * @param hexColor - Color in hex format (with or without #)
 * @returns Luminance value between 0 (black) and 1 (white)
 */
export function getLuminance(hexColor: string): number {
	const hex = hexColor.replace('#', '');
	const r = parseInt(hex.substr(0, 2), 16);
	const g = parseInt(hex.substr(2, 2), 16);
	const b = parseInt(hex.substr(4, 2), 16);
	return (0.299 * r + 0.587 * g + 0.114 * b) / 255;
}

/**
 * Determine if a color is dark enough to need light text.
 * @param hexColor - Color in hex format
 * @param threshold - Luminance threshold (default 0.55, slightly higher for better contrast)
 * @returns true if the color is dark and needs light text
 */
export function isColorDark(hexColor: string, threshold = 0.55): boolean {
	return getLuminance(hexColor) < threshold;
}

/**
 * Get computed text colors for a given background color.
 * Use this for project-colored event backgrounds to ensure readable text.
 *
 * @param hexColor - Background color in hex format
 * @returns Object with text, textMuted, textSubtle colors and isDark flag
 *
 * @example
 * ```svelte
 * {@const colors = getProjectTextColors(project.color)}
 * <div style="background: {project.color}; color: {colors.text}">
 *   <span>{title}</span>
 *   <span style="color: {colors.textMuted}">{subtitle}</span>
 * </div>
 * ```
 */
export function getProjectTextColors(hexColor: string): ProjectTextColors {
	const isDark = isColorDark(hexColor);

	if (isDark) {
		return {
			text: 'white',
			textMuted: 'rgba(255, 255, 255, 0.75)',
			textSubtle: 'rgba(255, 255, 255, 0.6)',
			isDark: true
		};
	} else {
		return {
			text: 'black',
			textMuted: 'rgba(0, 0, 0, 0.65)',
			textSubtle: 'rgba(0, 0, 0, 0.5)',
			isDark: false
		};
	}
}

/**
 * Get text color for "needs verification" state where project color is used as text on white background.
 * For light colors (like yellow), returns a darkened version for better contrast.
 * For dark colors, returns the original color.
 *
 * @param hexColor - Project color in hex format
 * @returns Color string suitable for text on white/light background
 */
export function getVerificationTextColor(hexColor: string): string {
	const luminance = getLuminance(hexColor);

	// If the color is too light for text on white background, darken it
	if (luminance > 0.5) {
		// Darken the color by reducing RGB values
		const hex = hexColor.replace('#', '');
		const r = Math.floor(parseInt(hex.substr(0, 2), 16) * 0.6);
		const g = Math.floor(parseInt(hex.substr(2, 2), 16) * 0.6);
		const b = Math.floor(parseInt(hex.substr(4, 2), 16) * 0.6);
		return `rgb(${r}, ${g}, ${b})`;
	}

	// Dark enough colors can be used directly
	return hexColor;
}
