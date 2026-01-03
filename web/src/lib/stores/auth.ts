import { writable, derived } from 'svelte/store';
import { api, ApiClientError } from '$lib/api/client';
import type { User } from '$lib/api/types';
import { browser } from '$app/environment';

const TOKEN_KEY = 'timesheet_token';

interface AuthState {
	user: User | null;
	token: string | null;
	loading: boolean;
	error: string | null;
}

function createAuthStore() {
	const initialToken = browser ? localStorage.getItem(TOKEN_KEY) : null;
	if (initialToken) {
		api.setToken(initialToken);
	}

	const { subscribe, set, update } = writable<AuthState>({
		user: null,
		token: initialToken,
		loading: !!initialToken,
		error: null
	});

	async function initialize() {
		const state = await new Promise<AuthState>((resolve) => {
			subscribe((s) => resolve(s))();
		});

		if (state.token && !state.user) {
			try {
				const user = await api.getCurrentUser();
				update((s) => ({ ...s, user, loading: false }));
			} catch (e) {
				// Token invalid, clear it
				logout();
			}
		} else {
			update((s) => ({ ...s, loading: false }));
		}
	}

	async function login(email: string, password: string) {
		update((s) => ({ ...s, loading: true, error: null }));
		try {
			const response = await api.login(email, password);
			api.setToken(response.token);
			if (browser) {
				localStorage.setItem(TOKEN_KEY, response.token);
			}
			set({ user: response.user, token: response.token, loading: false, error: null });
			return true;
		} catch (e) {
			const message = e instanceof ApiClientError ? e.error.message : 'Login failed';
			update((s) => ({ ...s, loading: false, error: message }));
			return false;
		}
	}

	async function signup(email: string, password: string, name: string) {
		update((s) => ({ ...s, loading: true, error: null }));
		try {
			const response = await api.signup(email, password, name);
			api.setToken(response.token);
			if (browser) {
				localStorage.setItem(TOKEN_KEY, response.token);
			}
			set({ user: response.user, token: response.token, loading: false, error: null });
			return true;
		} catch (e) {
			const message = e instanceof ApiClientError ? e.error.message : 'Signup failed';
			update((s) => ({ ...s, loading: false, error: message }));
			return false;
		}
	}

	function logout() {
		api.setToken(null);
		if (browser) {
			localStorage.removeItem(TOKEN_KEY);
		}
		set({ user: null, token: null, loading: false, error: null });
	}

	return {
		subscribe,
		initialize,
		login,
		signup,
		logout
	};
}

export const auth = createAuthStore();
export const isAuthenticated = derived(auth, ($auth) => !!$auth.user);
