<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/stores';
	import { auth } from '$lib/stores';

	interface Props {
		children: Snippet;
		wide?: boolean;
	}

	let { children, wide = false }: Props = $props();

	const navItems = [
		{ href: '/', label: 'Week', icon: 'calendar' },
		{ href: '/projects', label: 'Projects', icon: 'folder' },
		{ href: '/rules', label: 'Rules', icon: 'rules' },
		{ href: '/settings', label: 'Settings', icon: 'cog' }
	];

	function handleLogout() {
		auth.logout();
	}
</script>

<div class="min-h-screen bg-gray-50">
	<!-- Top nav -->
	<nav class="bg-white border-b">
		<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
			<div class="flex justify-between h-16">
				<div class="flex">
					<div class="flex-shrink-0 flex items-center">
						<span class="text-xl font-bold text-primary-600">Timesheet</span>
					</div>
					<div class="hidden sm:ml-8 sm:flex sm:space-x-4">
						{#each navItems as item}
							<a
								href={item.href}
								class="inline-flex items-center px-3 py-2 text-sm font-medium rounded-md
									{$page.url.pathname === item.href
										? 'text-primary-600 bg-primary-50'
										: 'text-gray-600 hover:text-gray-900 hover:bg-gray-50'}"
							>
								{item.label}
							</a>
						{/each}
					</div>
				</div>

				<div class="flex items-center gap-4">
					{#if $auth.user}
						<span class="text-sm text-gray-600">{$auth.user.name}</span>
						<button
							type="button"
							class="text-sm text-gray-500 hover:text-gray-700"
							onclick={handleLogout}
						>
							Logout
						</button>
					{/if}
				</div>
			</div>
		</div>
	</nav>

	<!-- Mobile nav -->
	<nav class="sm:hidden fixed bottom-0 left-0 right-0 bg-white border-t z-40">
		<div class="flex justify-around py-2">
			{#each navItems as item}
				<a
					href={item.href}
					class="flex flex-col items-center px-3 py-2 text-xs
						{$page.url.pathname === item.href
							? 'text-primary-600'
							: 'text-gray-500'}"
				>
					{#if item.icon === 'calendar'}
						<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
						</svg>
					{:else if item.icon === 'folder'}
						<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
						</svg>
					{:else if item.icon === 'rules'}
						<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v10a2 2 0 002 2h8a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
						</svg>
					{:else if item.icon === 'cog'}
						<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
						</svg>
					{/if}
					<span class="mt-1">{item.label}</span>
				</a>
			{/each}
		</div>
	</nav>

	<!-- Main content -->
	<main class="{wide ? '' : 'max-w-7xl mx-auto'} px-4 sm:px-6 lg:px-8 py-8 pb-24 sm:pb-8">
		{@render children()}
	</main>
</div>
