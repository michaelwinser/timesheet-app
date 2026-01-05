<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores';
	import { Button, Input } from '$lib/components/primitives';

	let name = $state('');
	let email = $state('');
	let password = $state('');
	let confirmPassword = $state('');
	let submitting = $state(false);
	let localError = $state('');

	async function handleSubmit(e: Event) {
		e.preventDefault();
		localError = '';

		if (password !== confirmPassword) {
			localError = 'Passwords do not match';
			return;
		}

		if (password.length < 8) {
			localError = 'Password must be at least 8 characters';
			return;
		}

		submitting = true;
		const success = await auth.signup(email, password, name);
		submitting = false;
		if (success) {
			goto('/');
		}
	}

	const error = $derived(localError || $auth.error);
</script>

<svelte:head>
	<title>Sign Up - Timesheet</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-900 py-12 px-4 sm:px-6 lg:px-8">
	<div class="max-w-md w-full space-y-8">
		<div>
			<h2 class="mt-6 text-center text-3xl font-bold text-gray-900 dark:text-white">
				Create your account
			</h2>
			<p class="mt-2 text-center text-sm text-gray-600 dark:text-gray-400">
				Already have an account?
				<a href="/login" class="font-medium text-primary-600 dark:text-primary-400 hover:text-primary-500">
					Sign in
				</a>
			</p>
		</div>

		<form class="mt-8 space-y-6" onsubmit={handleSubmit}>
			{#if error}
				<div class="bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 px-4 py-3 rounded">
					{error}
				</div>
			{/if}

			<div class="space-y-4">
				<Input
					type="text"
					label="Full name"
					bind:value={name}
					required
					placeholder="John Doe"
				/>
				<Input
					type="email"
					label="Email address"
					bind:value={email}
					required
					placeholder="you@example.com"
				/>
				<Input
					type="password"
					label="Password"
					bind:value={password}
					required
					placeholder="At least 8 characters"
				/>
				<Input
					type="password"
					label="Confirm password"
					bind:value={confirmPassword}
					required
					placeholder="Confirm your password"
				/>
			</div>

			<Button type="submit" size="lg" loading={submitting} disabled={submitting}>
				Create account
			</Button>
		</form>
	</div>
</div>

<style>
	form :global(button[type="submit"]) {
		width: 100%;
	}
</style>
