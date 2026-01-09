/**
 * Creates a debounced version of a function that delays execution until
 * after a specified wait period has elapsed since the last invocation.
 *
 * @param fn The function to debounce
 * @param wait The number of milliseconds to wait before executing
 * @returns A debounced version of the function
 */
export function debounce<T extends (...args: unknown[]) => void>(
	fn: T,
	wait: number
): (...args: Parameters<T>) => void {
	let timeoutId: ReturnType<typeof setTimeout> | null = null;

	return function (this: unknown, ...args: Parameters<T>) {
		if (timeoutId !== null) {
			clearTimeout(timeoutId);
		}

		timeoutId = setTimeout(() => {
			fn.apply(this, args);
			timeoutId = null;
		}, wait);
	};
}
