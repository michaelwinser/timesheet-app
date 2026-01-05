import { writable } from 'svelte/store';
import { browser } from '$app/environment';

type Theme = 'light' | 'dark';

function createThemeStore() {
	const initialTheme: Theme = browser
		? (localStorage.getItem('theme') as Theme) || 'dark'
		: 'dark';

	const { subscribe, set } = writable<Theme>(initialTheme);

	return {
		subscribe,
		toggle: () => {
			const html = document.documentElement;
			const isDark = html.classList.contains('dark');
			const newTheme: Theme = isDark ? 'light' : 'dark';

			if (newTheme === 'dark') {
				html.classList.add('dark');
			} else {
				html.classList.remove('dark');
			}

			localStorage.setItem('theme', newTheme);
			set(newTheme);
		},
		set: (theme: Theme) => {
			if (!browser) return;

			const html = document.documentElement;
			if (theme === 'dark') {
				html.classList.add('dark');
			} else {
				html.classList.remove('dark');
			}

			localStorage.setItem('theme', theme);
			set(theme);
		}
	};
}

export const theme = createThemeStore();
