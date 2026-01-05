<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores';
	import { Button, Input } from '$lib/components/primitives';

	let email = $state('');
	let password = $state('');
	let submitting = $state(false);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		submitting = true;
		const success = await auth.login(email, password);
		submitting = false;
		if (success) {
			goto('/');
		}
	}
</script>

<svelte:head>
	<title>Login - Timesheet</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-900 py-12 px-4 sm:px-6 lg:px-8">
	<div class="max-w-md w-full space-y-8">
		<div>
			<h2 class="mt-6 text-center text-3xl font-bold text-gray-900 dark:text-white">
				Sign in to Timesheet
			</h2>
			<p class="mt-2 text-center text-sm text-gray-600 dark:text-gray-400">
				Or
				<a href="/signup" class="font-medium text-primary-600 dark:text-primary-400 hover:text-primary-500">
					create a new account
				</a>
			</p>
		</div>

		<form class="mt-8 space-y-6" onsubmit={handleSubmit}>
			{#if $auth.error}
				<div class="bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 px-4 py-3 rounded">
					{$auth.error}
				</div>
			{/if}

			<div class="space-y-4">
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
					placeholder="Your password"
				/>
			</div>

			<Button type="submit" size="lg" loading={submitting} disabled={submitting}>
				Sign in
			</Button>
		</form>
	</div>
</div>

<style>
	form :global(button[type="submit"]) {
		width: 100%;
	}
</style>
