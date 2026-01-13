<script lang="ts">
	import { Button } from '$lib/components/primitives';

	interface Props {
		domains: string[];
		emails: string[];
		keywords: string[];
	}

	let { domains = $bindable([]), emails = $bindable([]), keywords = $bindable([]) }: Props = $props();

	// Input state
	let newDomain = $state('');
	let newEmail = $state('');
	let newKeyword = $state('');

	// Parse email from formats like "Name <email@domain.com>" or plain "email@domain.com"
	function parseEmail(input: string): string | null {
		const trimmed = input.trim().toLowerCase();
		if (!trimmed) return null;

		// Match "Name <email@domain.com>" format
		const angleMatch = trimmed.match(/<([^>]+@[^>]+)>/);
		if (angleMatch) {
			return angleMatch[1];
		}

		// Check if it's a plain email address
		if (trimmed.includes('@') && !trimmed.includes(' ')) {
			return trimmed;
		}

		return null;
	}

	// Extract domain from email address
	function extractDomain(email: string): string | null {
		const atIndex = email.lastIndexOf('@');
		if (atIndex > 0 && atIndex < email.length - 1) {
			return email.substring(atIndex + 1).toLowerCase();
		}
		return null;
	}

	// Parse input that might contain emails and extract domains
	function parseDomains(input: string): string[] {
		const result: string[] = [];
		const parts = input.split(',');

		for (const part of parts) {
			const trimmed = part.trim().toLowerCase();
			if (!trimmed) continue;

			// Try to extract email first (handles "Name <email>" format)
			const email = parseEmail(part);
			if (email) {
				const domain = extractDomain(email);
				if (domain && !result.includes(domain)) {
					result.push(domain);
				}
			} else if (!trimmed.includes(' ') && !trimmed.includes('<')) {
				// Plain domain or potential domain
				// Remove any @ prefix if someone types @domain.com
				const cleaned = trimmed.replace(/^@/, '');
				if (cleaned && !result.includes(cleaned)) {
					result.push(cleaned);
				}
			}
		}

		return result;
	}

	// Parse comma-separated input for emails, with smart extraction
	function parseEmails(input: string): string[] {
		const result: string[] = [];
		const parts = input.split(',');

		for (const part of parts) {
			const email = parseEmail(part);
			if (email && !result.includes(email)) {
				result.push(email);
			}
		}

		return result;
	}

	function addDomain() {
		const parsed = parseDomains(newDomain);
		const newOnes = parsed.filter(d => !domains.includes(d));
		if (newOnes.length > 0) {
			domains = [...domains, ...newOnes];
		}
		newDomain = '';
	}

	function removeDomain(domain: string) {
		domains = domains.filter(d => d !== domain);
	}

	function addEmail() {
		const parsed = parseEmails(newEmail);
		const newOnes = parsed.filter(e => !emails.includes(e));
		if (newOnes.length > 0) {
			emails = [...emails, ...newOnes];
		}
		newEmail = '';
	}

	function removeEmail(email: string) {
		emails = emails.filter(e => e !== email);
	}

	function addKeyword() {
		const parts = newKeyword.split(',');
		const newOnes: string[] = [];

		for (const part of parts) {
			const trimmed = part.trim();
			if (trimmed && !keywords.includes(trimmed) && !newOnes.includes(trimmed)) {
				newOnes.push(trimmed);
			}
		}

		if (newOnes.length > 0) {
			keywords = [...keywords, ...newOnes];
		}
		newKeyword = '';
	}

	function removeKeyword(keyword: string) {
		keywords = keywords.filter(k => k !== keyword);
	}
</script>

<div class="border-t border-gray-200 dark:border-gray-700 pt-6">
	<h3 class="text-sm font-medium text-gray-900 dark:text-white mb-4">
		Classification Fingerprints
		<span class="font-normal text-gray-500 dark:text-gray-400 ml-1">(for auto-classification)</span>
	</h3>

	<!-- Domains -->
	<div class="mb-4">
		<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Domains</label>
		<p class="text-xs text-gray-500 dark:text-gray-400 mb-2">Match attendee email domains. Paste comma-separated values or email lists.</p>
		<div class="flex gap-2 mb-2">
			<input
				type="text"
				bind:value={newDomain}
				placeholder="acme.com, contoso.com or paste attendee list"
				class="flex-1 rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm text-sm focus:border-primary-500 focus:ring-primary-500"
				onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addDomain())}
			/>
			<Button type="button" variant="secondary" size="sm" onclick={addDomain}>Add</Button>
		</div>
		{#if domains.length > 0}
			<div class="flex flex-wrap gap-1">
				{#each domains as domain}
					<span class="inline-flex items-center gap-1 px-2 py-1 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 text-xs rounded">
						{domain}
						<button type="button" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" onclick={() => removeDomain(domain)}>×</button>
					</span>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Emails -->
	<div class="mb-4">
		<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Email Addresses</label>
		<p class="text-xs text-gray-500 dark:text-gray-400 mb-2">Match specific attendee emails. Paste from meeting invites.</p>
		<div class="flex gap-2 mb-2">
			<input
				type="text"
				bind:value={newEmail}
				placeholder="Name <email@example.com>, other@example.com"
				class="flex-1 rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm text-sm focus:border-primary-500 focus:ring-primary-500"
				onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addEmail())}
			/>
			<Button type="button" variant="secondary" size="sm" onclick={addEmail}>Add</Button>
		</div>
		{#if emails.length > 0}
			<div class="flex flex-wrap gap-1">
				{#each emails as email}
					<span class="inline-flex items-center gap-1 px-2 py-1 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 text-xs rounded">
						{email}
						<button type="button" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" onclick={() => removeEmail(email)}>×</button>
					</span>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Keywords -->
	<div class="mb-4">
		<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Keywords</label>
		<p class="text-xs text-gray-500 dark:text-gray-400 mb-2">Match words in event titles or descriptions. Comma-separated.</p>
		<div class="flex gap-2 mb-2">
			<input
				type="text"
				bind:value={newKeyword}
				placeholder="Weekly Sync, Standup, Planning"
				class="flex-1 rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm text-sm focus:border-primary-500 focus:ring-primary-500"
				onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addKeyword())}
			/>
			<Button type="button" variant="secondary" size="sm" onclick={addKeyword}>Add</Button>
		</div>
		{#if keywords.length > 0}
			<div class="flex flex-wrap gap-1">
				{#each keywords as keyword}
					<span class="inline-flex items-center gap-1 px-2 py-1 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 text-xs rounded">
						{keyword}
						<button type="button" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" onclick={() => removeKeyword(keyword)}>×</button>
					</span>
				{/each}
			</div>
		{/if}
	</div>
</div>
