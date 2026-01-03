<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { auth, isAuthenticated } from '$lib/stores';
	import type { Snippet } from 'svelte';
	import '../app.css';

	let { children }: { children: Snippet } = $props();

	const publicRoutes = ['/login', '/signup'];

	onMount(() => {
		auth.initialize();
	});

	$effect(() => {
		const isPublic = publicRoutes.includes($page.url.pathname);
		if (!$auth.loading) {
			if (!$isAuthenticated && !isPublic) {
				goto('/login');
			} else if ($isAuthenticated && isPublic) {
				goto('/');
			}
		}
	});
</script>

{#if $auth.loading}
	<div class="min-h-screen flex items-center justify-center bg-gray-50">
		<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
	</div>
{:else}
	{@render children()}
{/if}
