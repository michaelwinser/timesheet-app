/** @type {import('tailwindcss').Config} */
export default {
	content: ['./src/**/*.{html,js,svelte,ts}'],
	darkMode: 'class',
	theme: {
		extend: {
			colors: {
				primary: {
					50: '#eff6ff',
					100: '#dbeafe',
					200: '#bfdbfe',
					300: '#93c5fd',
					400: '#60a5fa',
					500: '#3b82f6',
					600: '#2563eb',
					700: '#1d4ed8',
					800: '#1e40af',
					900: '#1e3a8a'
				},
				// Theme-aware colors using CSS variables
				surface: {
					DEFAULT: 'rgb(var(--color-surface) / <alpha-value>)',
					secondary: 'rgb(var(--color-surface-secondary) / <alpha-value>)',
					elevated: 'rgb(var(--color-surface-elevated) / <alpha-value>)'
				},
				border: {
					DEFAULT: 'rgb(var(--color-border) / <alpha-value>)',
					strong: 'rgb(var(--color-border-strong) / <alpha-value>)'
				},
				text: {
					primary: 'rgb(var(--color-text-primary) / <alpha-value>)',
					secondary: 'rgb(var(--color-text-secondary) / <alpha-value>)',
					tertiary: 'rgb(var(--color-text-tertiary) / <alpha-value>)',
					muted: 'rgb(var(--color-text-muted) / <alpha-value>)'
				}
			}
		}
	},
	plugins: []
};
