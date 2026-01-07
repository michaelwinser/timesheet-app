/**
 * QuickDateParser - Natural language date parsing with past-bias
 *
 * Supports inputs like:
 * - Relative: today, yesterday, tomorrow
 * - Shifts: last week, next week, 3 weeks ago, last month, next month, 2 months ago
 * - Days: monday, mon, last tuesday, next friday
 * - Month/day: jan 3, january 3, 3 jan, 3 january
 * - Month only: january, march (first of month)
 * - Month+year: mar 2026, march 2026, next march, last january
 * - Quarters: q1, q2, q3, q4 (first of quarter)
 * - Absolute: 01/03, 2023-01-03, 1/3/2024
 */

const MONTH_NAMES: Record<string, number> = {
	jan: 0, january: 0,
	feb: 1, february: 1,
	mar: 2, march: 2,
	apr: 3, april: 3,
	may: 4,
	jun: 5, june: 5,
	jul: 6, july: 6,
	aug: 7, august: 7,
	sep: 8, sept: 8, september: 8,
	oct: 9, october: 9,
	nov: 10, november: 10,
	dec: 11, december: 11
};

const DAY_NAMES: Record<string, number> = {
	sun: 0, sunday: 0,
	mon: 1, monday: 1,
	tue: 2, tues: 2, tuesday: 2,
	wed: 3, wednesday: 3,
	thu: 4, thur: 4, thurs: 4, thursday: 4,
	fri: 5, friday: 5,
	sat: 6, saturday: 6
};

// Quarter start months (0-indexed): Q1=Jan, Q2=Apr, Q3=Jul, Q4=Sep
const QUARTER_STARTS: Record<string, number> = {
	q1: 0,  // January
	q2: 3,  // April
	q3: 6,  // July
	q4: 8   // September
};

const NOISE_WORDS = new Set(['on', 'at', 'the', 'of', 'in', 'for', 'a', 'an']);

interface ParseResult {
	date: Date;
	confidence: 'high' | 'medium' | 'low';
	interpretation: string;
}

export class QuickDateParser {
	/**
	 * Parse a natural language date string
	 * @param input - The user's input string
	 * @param referenceDate - The reference date (defaults to today)
	 * @returns ParseResult or null if unable to parse
	 */
	static parse(input: string, referenceDate: Date = new Date()): ParseResult | null {
		if (!input || !input.trim()) return null;

		// Normalize input
		const normalized = this.normalize(input);
		if (!normalized) return null;

		// Try parsers in order of specificity
		return (
			this.parseRelativeKeyword(normalized, referenceDate) ||
			this.parseRelativeShift(normalized, referenceDate) ||
			this.parseQuarter(normalized, referenceDate) ||
			this.parseDayOfWeek(normalized, referenceDate) ||
			this.parseMonthDay(normalized, referenceDate) ||
			this.parseAbsoluteDate(normalized, referenceDate)
		);
	}

	/**
	 * Normalize input: lowercase, remove noise words, trim
	 */
	private static normalize(input: string): string {
		return input
			.toLowerCase()
			.trim()
			.split(/\s+/)
			.filter(word => !NOISE_WORDS.has(word))
			.join(' ');
	}

	/**
	 * Parse relative keywords: today, yesterday, tomorrow
	 * Always relative to actual today, not the reference date
	 */
	private static parseRelativeKeyword(input: string, _ref: Date): ParseResult | null {
		const today = this.startOfDay(new Date());

		if (input === 'today' || input === 'now') {
			return { date: today, confidence: 'high', interpretation: 'Today' };
		}

		if (input === 'yesterday') {
			const d = new Date(today);
			d.setDate(d.getDate() - 1);
			return { date: d, confidence: 'high', interpretation: 'Yesterday' };
		}

		if (input === 'tomorrow') {
			const d = new Date(today);
			d.setDate(d.getDate() + 1);
			return { date: d, confidence: 'high', interpretation: 'Tomorrow' };
		}

		return null;
	}

	/**
	 * Parse relative shifts: X days/weeks/months ago, last week, etc.
	 * Always relative to actual today, not the reference date
	 */
	private static parseRelativeShift(input: string, _ref: Date): ParseResult | null {
		const today = this.startOfDay(new Date());

		// Pattern: "X days/weeks/months/years ago"
		const agoMatch = input.match(/^(\d+)\s*(day|week|month|year)s?\s*ago$/);
		if (agoMatch) {
			const quantity = parseInt(agoMatch[1], 10);
			const unit = agoMatch[2];
			const d = this.shiftDate(today, -quantity, unit);
			return {
				date: d,
				confidence: 'high',
				interpretation: `${quantity} ${unit}${quantity > 1 ? 's' : ''} ago`
			};
		}

		// Pattern: "last day/week/month/year"
		const lastMatch = input.match(/^last\s*(day|week|month|year)$/);
		if (lastMatch) {
			const unit = lastMatch[1];
			const d = this.shiftDate(today, -1, unit);
			return { date: d, confidence: 'high', interpretation: `Last ${unit}` };
		}

		// Pattern: "next day/week/month/year"
		const nextMatch = input.match(/^next\s*(day|week|month|year)$/);
		if (nextMatch) {
			const unit = nextMatch[1];
			const d = this.shiftDate(today, 1, unit);
			return { date: d, confidence: 'high', interpretation: `Next ${unit}` };
		}

		// Pattern: "a week/month/year ago"
		const aWeekMatch = input.match(/^a\s*(day|week|month|year)\s*ago$/);
		if (aWeekMatch) {
			const unit = aWeekMatch[1];
			const d = this.shiftDate(today, -1, unit);
			return { date: d, confidence: 'high', interpretation: `A ${unit} ago` };
		}

		// Pattern: "X weeks/months/years" (implicit ago, past-biased)
		const implicitAgoMatch = input.match(/^(\d+)\s*(day|week|month|year)s?$/);
		if (implicitAgoMatch) {
			const quantity = parseInt(implicitAgoMatch[1], 10);
			const unit = implicitAgoMatch[2];
			const d = this.shiftDate(today, -quantity, unit);
			return {
				date: d,
				confidence: 'medium',
				interpretation: `${quantity} ${unit}${quantity > 1 ? 's' : ''} ago`
			};
		}

		return null;
	}

	/**
	 * Parse quarters: q1, q2, q3, q4
	 * Always relative to actual today, not the reference date
	 */
	private static parseQuarter(input: string, _ref: Date): ParseResult | null {
		const today = this.startOfDay(new Date());
		const quarterMonth = QUARTER_STARTS[input];

		if (quarterMonth !== undefined) {
			// Go to first of that quarter, past-biased
			let d = new Date(today.getFullYear(), quarterMonth, 1);
			if (d > today) {
				d = new Date(today.getFullYear() - 1, quarterMonth, 1);
			}
			const quarterNum = input.toUpperCase();
			return {
				date: d,
				confidence: 'high',
				interpretation: `${quarterNum} ${d.getFullYear()}`
			};
		}

		return null;
	}

	/**
	 * Parse day of week: monday, last tuesday, etc.
	 * Always relative to actual today, not the reference date
	 */
	private static parseDayOfWeek(input: string, _ref: Date): ParseResult | null {
		const today = this.startOfDay(new Date());
		const currentDayOfWeek = today.getDay();

		// Check for "last [day]" pattern
		const lastDayMatch = input.match(/^last\s+(\w+)$/);
		if (lastDayMatch) {
			const dayName = lastDayMatch[1];
			const targetDay = DAY_NAMES[dayName];
			if (targetDay !== undefined) {
				// Go back to last occurrence (always in the past week or before)
				let diff = currentDayOfWeek - targetDay;
				if (diff <= 0) diff += 7;
				const d = new Date(today);
				d.setDate(d.getDate() - diff);
				return {
					date: d,
					confidence: 'high',
					interpretation: `Last ${this.capitalize(dayName)}`
				};
			}
		}

		// Check for "next [day]" pattern
		const nextDayMatch = input.match(/^next\s+(\w+)$/);
		if (nextDayMatch) {
			const dayName = nextDayMatch[1];
			const targetDay = DAY_NAMES[dayName];
			if (targetDay !== undefined) {
				// Go forward to next occurrence
				let diff = targetDay - currentDayOfWeek;
				if (diff <= 0) diff += 7;
				const d = new Date(today);
				d.setDate(d.getDate() + diff);
				return {
					date: d,
					confidence: 'high',
					interpretation: `Next ${this.capitalize(dayName)}`
				};
			}
		}

		// Check for plain day name - bias towards past
		const dayIndex = DAY_NAMES[input];
		if (dayIndex !== undefined) {
			let diff = currentDayOfWeek - dayIndex;
			// If diff is 0 (same day) or negative, go back a week
			if (diff <= 0) diff += 7;
			const d = new Date(today);
			d.setDate(d.getDate() - diff);
			return {
				date: d,
				confidence: 'high',
				interpretation: `Last ${this.capitalize(input)}`
			};
		}

		return null;
	}

	/**
	 * Parse month/day combinations: jan 3, 3 jan, january 3rd, etc.
	 * Always relative to actual today, not the reference date
	 */
	private static parseMonthDay(input: string, _ref: Date): ParseResult | null {
		const actualToday = this.startOfDay(new Date());

		// Pattern: "month day" or "day month" with optional ordinal suffix
		const monthFirst = input.match(/^([a-z]+)\s+(\d{1,2})(?:st|nd|rd|th)?$/);
		const dayFirst = input.match(/^(\d{1,2})(?:st|nd|rd|th)?\s+([a-z]+)$/);

		let monthName: string | undefined;
		let day: number | undefined;

		if (monthFirst) {
			monthName = monthFirst[1];
			day = parseInt(monthFirst[2], 10);
		} else if (dayFirst) {
			day = parseInt(dayFirst[1], 10);
			monthName = dayFirst[2];
		}

		// Also try "last [month]" pattern - relative to actual today
		const lastMonthMatch = input.match(/^last\s+([a-z]+)$/);
		if (lastMonthMatch) {
			const month = MONTH_NAMES[lastMonthMatch[1]];
			if (month !== undefined) {
				// Go to first of that month in the past
				const d = new Date(actualToday.getFullYear(), month, 1);
				if (d >= actualToday) {
					d.setFullYear(d.getFullYear() - 1);
				}
				return {
					date: d,
					confidence: 'high',
					interpretation: `Last ${this.capitalize(lastMonthMatch[1])}`
				};
			}
		}

		// Also try "next [month]" pattern - relative to actual today
		const nextMonthMatch = input.match(/^next\s+([a-z]+)$/);
		if (nextMonthMatch) {
			const month = MONTH_NAMES[nextMonthMatch[1]];
			if (month !== undefined) {
				// Go to first of that month in the future
				const d = new Date(actualToday.getFullYear(), month, 1);
				if (d <= actualToday) {
					d.setFullYear(d.getFullYear() + 1);
				}
				return {
					date: d,
					confidence: 'high',
					interpretation: `Next ${this.capitalize(nextMonthMatch[1])}`
				};
			}
		}

		// Month + year pattern (e.g., "mar 2026", "march 2026")
		const monthYearMatch = input.match(/^([a-z]+)\s+(\d{4})$/);
		if (monthYearMatch) {
			const month = MONTH_NAMES[monthYearMatch[1]];
			const year = parseInt(monthYearMatch[2], 10);
			if (month !== undefined) {
				const d = new Date(year, month, 1);
				return {
					date: d,
					confidence: 'high',
					interpretation: `${this.capitalize(monthYearMatch[1])} ${year}`
				};
			}
		}

		// Month only (e.g., "january", "march") - first of that month with past-bias relative to actual today
		const monthOnly = MONTH_NAMES[input];
		if (monthOnly !== undefined) {
			// Go to first of that month, past-biased
			let d = new Date(actualToday.getFullYear(), monthOnly, 1);
			if (d > actualToday) {
				d = new Date(actualToday.getFullYear() - 1, monthOnly, 1);
			}
			return {
				date: d,
				confidence: 'high',
				interpretation: `${this.capitalize(input)} 1`
			};
		}

		if (monthName && day) {
			const month = MONTH_NAMES[monthName];
			if (month !== undefined && day >= 1 && day <= 31) {
				// Try current year first, fall back to last year if in future (past-bias relative to actual today)
				let d = new Date(actualToday.getFullYear(), month, day);

				// If date is in the future, use last year
				if (d > actualToday) {
					d = new Date(actualToday.getFullYear() - 1, month, day);
				}

				return {
					date: d,
					confidence: 'high',
					interpretation: `${this.capitalize(monthName)} ${day}`
				};
			}
		}

		return null;
	}

	/**
	 * Parse absolute dates: YYYY-MM-DD, MM/DD, MM/DD/YYYY, etc.
	 * Uses actual today for past-bias when year is not specified
	 */
	private static parseAbsoluteDate(input: string, _ref: Date): ParseResult | null {
		const today = this.startOfDay(new Date());

		// ISO format: YYYY-MM-DD
		const isoMatch = input.match(/^(\d{4})-(\d{1,2})-(\d{1,2})$/);
		if (isoMatch) {
			const year = parseInt(isoMatch[1], 10);
			const month = parseInt(isoMatch[2], 10) - 1;
			const day = parseInt(isoMatch[3], 10);
			const d = new Date(year, month, day);
			if (!isNaN(d.getTime())) {
				return { date: d, confidence: 'high', interpretation: this.formatDate(d) };
			}
		}

		// US format: MM/DD or MM/DD/YYYY or MM/DD/YY
		const usMatch = input.match(/^(\d{1,2})\/(\d{1,2})(?:\/(\d{2,4}))?$/);
		if (usMatch) {
			const month = parseInt(usMatch[1], 10) - 1;
			const day = parseInt(usMatch[2], 10);
			let year = usMatch[3] ? parseInt(usMatch[3], 10) : today.getFullYear();

			// Handle 2-digit years
			if (year < 100) {
				year += year < 50 ? 2000 : 1900;
			}

			let d = new Date(year, month, day);

			// If no year provided and date is in future, use last year (past-bias)
			if (!usMatch[3] && d > today) {
				d = new Date(today.getFullYear() - 1, month, day);
			}

			if (!isNaN(d.getTime()) && month >= 0 && month <= 11 && day >= 1 && day <= 31) {
				return { date: d, confidence: 'high', interpretation: this.formatDate(d) };
			}
		}

		// European format: DD.MM or DD.MM.YYYY
		const euMatch = input.match(/^(\d{1,2})\.(\d{1,2})(?:\.(\d{2,4}))?$/);
		if (euMatch) {
			const day = parseInt(euMatch[1], 10);
			const month = parseInt(euMatch[2], 10) - 1;
			let year = euMatch[3] ? parseInt(euMatch[3], 10) : today.getFullYear();

			if (year < 100) {
				year += year < 50 ? 2000 : 1900;
			}

			let d = new Date(year, month, day);

			if (!euMatch[3] && d > today) {
				d = new Date(today.getFullYear() - 1, month, day);
			}

			if (!isNaN(d.getTime()) && month >= 0 && month <= 11 && day >= 1 && day <= 31) {
				return { date: d, confidence: 'high', interpretation: this.formatDate(d) };
			}
		}

		return null;
	}

	/**
	 * Helper: Shift a date by a quantity of units
	 */
	private static shiftDate(date: Date, quantity: number, unit: string): Date {
		const d = new Date(date);
		switch (unit) {
			case 'day':
				d.setDate(d.getDate() + quantity);
				break;
			case 'week':
				d.setDate(d.getDate() + quantity * 7);
				break;
			case 'month':
				d.setMonth(d.getMonth() + quantity);
				break;
			case 'year':
				d.setFullYear(d.getFullYear() + quantity);
				break;
		}
		return d;
	}

	/**
	 * Helper: Get start of day (midnight)
	 */
	private static startOfDay(date: Date): Date {
		const d = new Date(date);
		d.setHours(0, 0, 0, 0);
		return d;
	}

	/**
	 * Helper: Capitalize first letter
	 */
	private static capitalize(str: string): string {
		return str.charAt(0).toUpperCase() + str.slice(1);
	}

	/**
	 * Helper: Format date for display
	 */
	private static formatDate(date: Date): string {
		return date.toLocaleDateString('en-US', {
			weekday: 'short',
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		});
	}
}
