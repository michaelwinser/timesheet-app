<script lang="ts">
	import { Button, Modal } from '$lib/components/primitives';
	import { api } from '$lib/api/client';
	import type { ClassificationRule, Project, RuleCreate, RuleUpdate } from '$lib/api/types';

	interface Props {
		open: boolean;
		rule: ClassificationRule | null;
		projects: Project[];
		initialQuery?: string;
		initialProjectId?: string | null;
		onsave?: () => void;
		onpreview?: (query: string, projectId: string | null, isAttendance: boolean, isPriority: boolean) => void;
	}

	let {
		open = $bindable(false),
		rule,
		projects,
		initialQuery = '',
		initialProjectId = null,
		onsave,
		onpreview
	}: Props = $props();

	// Editor state
	let editorQuery = $state('');
	let editorProjectId = $state<string | null>(null);
	let editorIsAttendance = $state(false);
	let editorIsPriority = $state(false);
	let editorError = $state('');
	let saving = $state(false);

	// Reset state when modal opens or rule changes
	$effect(() => {
		if (open) {
			if (rule) {
				editorQuery = rule.query;
				editorProjectId = rule.project_id || null;
				editorIsAttendance = rule.attended !== null && rule.attended !== undefined;
				editorIsPriority = rule.weight >= 2;
			} else {
				editorQuery = initialQuery;
				editorProjectId = initialProjectId;
				editorIsAttendance = false;
				editorIsPriority = false;
			}
			editorError = '';
		}
	});

	async function handleSave() {
		editorError = '';

		if (!editorQuery.trim()) {
			editorError = 'Query is required';
			return;
		}

		if (!editorIsAttendance && !editorProjectId) {
			editorError = 'Please select a project or choose "Did not attend"';
			return;
		}

		saving = true;

		try {
			if (rule) {
				// Update existing rule
				const update: RuleUpdate = {
					query: editorQuery,
					weight: editorIsPriority ? 2.0 : 1.0
				};

				if (editorIsAttendance) {
					update.project_id = null;
					update.attended = false;
				} else {
					update.project_id = editorProjectId;
					update.attended = null;
				}

				await api.updateRule(rule.id, update);
			} else {
				// Create new rule
				const create: RuleCreate = {
					query: editorQuery,
					weight: editorIsPriority ? 2.0 : 1.0
				};

				if (editorIsAttendance) {
					create.attended = false;
				} else {
					create.project_id = editorProjectId!;
				}

				await api.createRule(create);
			}

			open = false;
			onsave?.();
		} catch (e) {
			console.error('Failed to save rule:', e);
			editorError = e instanceof Error ? e.message : 'Failed to save rule';
		} finally {
			saving = false;
		}
	}

	function handlePreviewBeforeSave() {
		editorError = '';

		if (!editorQuery.trim()) {
			editorError = 'Query is required';
			return;
		}

		open = false;
		onpreview?.(editorQuery, editorIsAttendance ? null : editorProjectId, editorIsAttendance, editorIsPriority);
	}
</script>

<Modal bind:open title={rule ? 'Edit Rule' : 'New Rule'}>
	<div class="space-y-4">
		{#if editorError}
			<div class="bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 px-3 py-2 rounded text-sm">
				{editorError}
			</div>
		{/if}

		<div>
			<label for="query" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Query</label>
			<input
				id="query"
				type="text"
				bind:value={editorQuery}
				placeholder="domain:acme.com title:sync"
				class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 font-mono text-sm"
			/>
			<p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
				e.g. <code class="bg-gray-100 dark:bg-gray-700 px-1 rounded">standup domain:acme.com -response:declined</code>
			</p>
		</div>

		<div class="border-t border-gray-200 dark:border-gray-700 pt-4">
			<div class="flex items-center gap-4 mb-3">
				<label class="flex items-center gap-2 cursor-pointer">
					<input
						type="radio"
						name="ruleType"
						checked={!editorIsAttendance}
						onchange={() => (editorIsAttendance = false)}
						class="h-4 w-4 text-primary-600 focus:ring-primary-500"
					/>
					<span class="text-sm text-gray-700 dark:text-gray-300">Assign to project</span>
				</label>
				<label class="flex items-center gap-2 cursor-pointer">
					<input
						type="radio"
						name="ruleType"
						checked={editorIsAttendance}
						onchange={() => (editorIsAttendance = true)}
						class="h-4 w-4 text-primary-600 focus:ring-primary-500"
					/>
					<span class="text-sm text-gray-700 dark:text-gray-300">Did not attend</span>
				</label>
			</div>

			{#if !editorIsAttendance}
				<div>
					<label for="project" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Project</label>
					<select
						id="project"
						bind:value={editorProjectId}
						class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
					>
						<option value={null}>Select project...</option>
						{#each projects.filter((p) => !p.is_archived) as project}
							<option value={project.id}>
								{project.name}
							</option>
						{/each}
					</select>
				</div>
			{/if}
		</div>

		<div class="border-t border-gray-200 dark:border-gray-700 pt-4">
			<label class="flex items-center gap-2 cursor-pointer">
				<input
					type="checkbox"
					bind:checked={editorIsPriority}
					class="h-4 w-4 rounded text-primary-600 focus:ring-primary-500 dark:bg-gray-700 border-gray-300 dark:border-gray-600"
				/>
				<span class="text-sm text-gray-700 dark:text-gray-300">Priority rule (counts twice in scoring)</span>
			</label>
		</div>
	</div>

	{#snippet footer()}
		<Button variant="secondary" onclick={() => (open = false)}>Cancel</Button>
		<Button variant="primary" loading={saving} onclick={handlePreviewBeforeSave}>
			Preview & Save
		</Button>
	{/snippet}
</Modal>
