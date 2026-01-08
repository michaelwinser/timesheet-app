# UI Coding Guidelines

This document establishes coding patterns and best practices for the Timesheet web application frontend, built with Svelte 5, SvelteKit 2, and Tailwind CSS.

## Table of Contents

1. [Style System Architecture](#style-system-architecture)
2. [Component Design Principles](#component-design-principles)
3. [Tailwind CSS Patterns](#tailwind-css-patterns)
4. [Svelte 5 Patterns](#svelte-5-patterns)
5. [File Organization](#file-organization)
6. [Common Patterns](#common-patterns)

---

## Style System Architecture

### When to Use the Style System

The style system (`$lib/styles/`) centralizes styling logic that:
- Depends on domain data (e.g., classification status, project colors)
- Is reused across multiple components
- Involves complex conditional styling

**Use the style system when:**
```svelte
<!-- Good: Complex conditional styling based on domain state -->
{@const styles = getClassificationStyles({
  status: event.classification_status,
  needsReview: event.needs_review,
  isSkipped: event.is_skipped,
  projectColor: event.project?.color
})}
<div class={styles.containerClasses} style={styles.containerStyle}>
```

**Use inline Tailwind when:**
```svelte
<!-- Good: Simple, static styling -->
<div class="flex items-center gap-2 p-4 rounded-lg">
```

### Style System Structure

```
lib/styles/
├── index.ts              # Barrel exports
└── classification.ts     # Event classification styles
```

Each style module exports:
1. **A main getter function** that computes all styles from state
2. **Helper functions** for accessing specific style aspects
3. **TypeScript interfaces** for type safety

Example pattern:
```typescript
// classification.ts
export interface ClassificationState {
  status: 'pending' | 'classified' | 'skipped';
  needsReview: boolean;
  isSkipped: boolean;
  projectColor: string | null;
}

export interface ClassificationStyles {
  containerClasses: string;
  containerStyle: string;
  textColors: TextColors | null;
  hasProjectBackground: boolean;
}

export function getClassificationStyles(state: ClassificationState): ClassificationStyles {
  // Compute and return all styles from state
}

// Helper functions for template use
export function getPrimaryTextClasses(styles: ClassificationStyles, isSkipped: boolean): string
export function getPrimaryTextStyle(styles: ClassificationStyles, isSkipped: boolean): string
```

### Guidelines for Style Functions

1. **Pure functions**: Style functions should be pure - same input always produces same output
2. **No side effects**: Never modify state or trigger effects in style functions
3. **Compute once**: Call the main getter once per component render, then use helpers
4. **Type everything**: All inputs and outputs should be typed

---

## Component Design Principles

### When to Create a New Component

Create a new component when:
- The same UI pattern is used in 3+ places
- The UI has its own state or behavior
- The code block exceeds ~50 lines of template
- Testing the UI in isolation would be valuable

### Component Variants

Use a `variant` prop for components with multiple display modes:

```svelte
<script lang="ts">
  type Variant = 'chip' | 'card' | 'compact';

  interface Props {
    variant?: Variant;
  }

  let { variant = 'card' }: Props = $props();

  const variantClasses = {
    chip: 'inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full',
    card: 'p-3 rounded-lg',
    compact: 'p-1.5 text-xs rounded'
  };
</script>

<div class={variantClasses[variant]}>
  <!-- Content adapts to variant -->
</div>
```

### Component Props Interface

Always define a typed `Props` interface:

```svelte
<script lang="ts">
  import type { CalendarEvent, Project } from '$lib/api/types';

  interface Props {
    event: CalendarEvent;
    projects: Project[];
    variant?: 'chip' | 'card' | 'compact';
    showTime?: boolean;
    onclassify?: (projectId: string) => void;
    onskip?: () => void;
    onhover?: (element: HTMLElement | null) => void;
  }

  let {
    event,
    projects,
    variant = 'card',
    showTime = false,
    onclassify,
    onskip,
    onhover
  }: Props = $props();
</script>
```

### Event Handlers

Use lowercase event handler names without the `on` prefix in props, but include `on` when destructuring:

```svelte
<!-- Parent component -->
<CompactEventCard
  onclassify={(id) => handleClassify(id)}
  onskip={() => handleSkip()}
/>

<!-- Child component -->
<script lang="ts">
  let { onclassify, onskip }: Props = $props();
</script>

<button onclick={() => onclassify?.('project-id')}>Classify</button>
```

---

## Tailwind CSS Patterns

### Theme-Aware Colors

Use CSS custom properties for colors that need to adapt to light/dark themes:

```css
/* app.css */
@layer base {
  :root {
    --color-surface: 255 255 255;
    --color-text-primary: 17 24 39;
    --color-border: 229 231 235;
  }

  .dark {
    --color-surface: 24 24 27;
    --color-text-primary: 244 244 245;
    --color-border: 63 63 70;
  }
}
```

```javascript
// tailwind.config.js
theme: {
  extend: {
    colors: {
      surface: {
        DEFAULT: 'rgb(var(--color-surface) / <alpha-value>)',
      },
      'text-primary': 'rgb(var(--color-text-primary) / <alpha-value>)',
    }
  }
}
```

Then use in templates:
```svelte
<div class="bg-surface text-text-primary border-border">
```

### Dynamic Colors from Data

When colors come from data (e.g., project colors), use inline styles:

```svelte
<span
  class="w-3 h-3 rounded-full"
  style="background-color: {project.color}"
></span>
```

For text on dynamic backgrounds, compute contrast:

```typescript
import { getContrastColor } from '$lib/utils/colors';

const textColor = getContrastColor(project.color);
```

### Scoped Styles with @apply

Use Svelte's scoped `<style>` block with `@apply` for component-specific reusable classes:

```svelte
<div class="sidebar">
  <!-- content -->
</div>

<style>
  .sidebar {
    @apply sticky top-4 rounded-lg border bg-surface p-4 border-border;
  }
</style>
```

### Class Composition

Order classes consistently: layout → spacing → typography → colors → effects

```svelte
<div class="flex items-center gap-2 p-4 text-sm text-gray-700 bg-white rounded-lg shadow-sm hover:shadow-md transition-shadow">
```

---

## Svelte 5 Patterns

### Reactive State

Use `$state` for component-local mutable state:

```svelte
<script lang="ts">
  let showMenu = $state(false);
  let selectedId = $state<string | null>(null);
</script>
```

### Derived Values

Use `$derived` for computed values that depend on props or state:

```svelte
<script lang="ts">
  let { projects }: Props = $props();

  const activeProjects = $derived(projects.filter(p => !p.is_archived));

  // For complex computations, use $derived.by
  const projectTotals = $derived.by(() => {
    const totals: Record<string, number> = {};
    for (const entry of entries) {
      totals[entry.project_id] = (totals[entry.project_id] || 0) + entry.hours;
    }
    return totals;
  });
</script>
```

### Template Variables

Use `{@const}` for values computed within template blocks:

```svelte
{#each events as event}
  {@const styles = getClassificationStyles({
    status: event.classification_status,
    needsReview: event.needs_review,
    isSkipped: event.is_skipped,
    projectColor: event.project?.color
  })}

  <div class={styles.containerClasses}>
    {event.title}
  </div>
{/each}
```

### Effects

Use `$effect` sparingly and only for side effects:

```svelte
<script lang="ts">
  // Good: Side effect that responds to state changes
  $effect(() => {
    if (scrollContainer && shouldScroll) {
      scrollContainer.scrollTop = targetPosition;
    }
  });

  // Bad: Computing derived values (use $derived instead)
  // $effect(() => {
  //   filteredItems = items.filter(i => i.active);
  // });
</script>
```

---

## File Organization

### Component Directory Structure

```
lib/components/
├── primitives/           # Basic UI building blocks
│   ├── Button.svelte
│   ├── Input.svelte
│   ├── Modal.svelte
│   └── index.ts
├── widgets/              # Domain-specific components
│   ├── CalendarEventCard.svelte
│   ├── CompactEventCard.svelte
│   ├── TimeGrid.svelte
│   └── index.ts
└── AppShell.svelte       # Layout components
```

### Barrel Exports

Always export components through index.ts:

```typescript
// lib/components/widgets/index.ts
export { default as ProjectChip } from './ProjectChip.svelte';
export { default as TimeEntryCard } from './TimeEntryCard.svelte';
export { default as CompactEventCard } from './CompactEventCard.svelte';
```

Then import from the barrel:

```svelte
<script lang="ts">
  import { ProjectChip, TimeEntryCard, CompactEventCard } from '$lib/components/widgets';
</script>
```

---

## Common Patterns

### Classification State Handling

Events have multiple classification states that affect styling:

```typescript
type ClassificationStatus = 'pending' | 'classified' | 'skipped';

// Full state includes additional flags
interface ClassificationState {
  status: ClassificationStatus;
  needsReview: boolean;      // Auto-classified with medium confidence
  isSkipped: boolean;        // Explicitly marked as not attended
  projectColor: string | null;
}
```

Visual treatments:
- **Pending**: White/neutral background, prominent border, classification buttons visible
- **Classified**: Project color background, text color computed for contrast
- **Needs Review**: Project color border only, verification indicator
- **Skipped**: Dashed border, muted/struck-through text

### Suggested Project Highlighting

When showing project buttons for classification, highlight the suggested project:

```svelte
{#each projects.slice(0, 4) as project, i}
  {@const isBestGuess = event.suggested_project_id === project.id ||
                        (!event.suggested_project_id && i === 0)}
  <button
    class="w-3 h-3 rounded-full transition-shadow {isBestGuess
      ? 'ring-1 ring-black/40 ring-offset-1'
      : 'hover:ring-1 hover:ring-offset-1'}"
    style="background-color: {project.color}"
    title="{project.name}{isBestGuess ? ' (suggested)' : ''}"
    onclick={() => onclassify?.(project.id)}
  ></button>
{/each}
```

### Hover Interactions

For components that show details on hover:

```svelte
<script lang="ts">
  interface Props {
    onhover?: (element: HTMLElement | null) => void;
  }

  let { onhover }: Props = $props();
</script>

<div
  onmouseenter={(e) => onhover?.(e.currentTarget as HTMLElement)}
  onmouseleave={() => onhover?.(null)}
>
  <!-- content -->
</div>
```

Parent handles the hover state and popup positioning:

```svelte
<script lang="ts">
  let hoveredEvent = $state<CalendarEvent | null>(null);
  let hoveredElement = $state<HTMLElement | null>(null);

  function handleEventHover(event: CalendarEvent | null, element: HTMLElement | null) {
    // Add debouncing logic as needed
    hoveredEvent = event;
    hoveredElement = element;
  }
</script>

<CompactEventCard
  {event}
  onhover={(el) => handleEventHover(el ? event : null, el)}
/>

{#if hoveredEvent && hoveredElement}
  <EventPopup event={hoveredEvent} anchorElement={hoveredElement} />
{/if}
```

### Time Formatting

Use consistent time formatting helpers:

```typescript
function formatTimeRange(start: string, end: string): string {
  const startDate = new Date(start);
  const endDate = new Date(end);
  const opts: Intl.DateTimeFormatOptions = { hour: 'numeric', minute: '2-digit' };
  return `${startDate.toLocaleTimeString([], opts)} - ${endDate.toLocaleTimeString([], opts)}`;
}

function formatHourLabel(hour: number): string {
  return hour === 0 ? '12 AM' :
    hour === 12 ? '12 PM' :
    hour > 12 ? `${hour - 12} PM` :
    `${hour} AM`;
}
```

---

## Anti-Patterns to Avoid

### Don't: Duplicate style logic across components

```svelte
<!-- Bad: Same logic in multiple places -->
{@const statusClasses = isSkipped
  ? 'bg-transparent border-dashed'
  : status === 'classified'
    ? 'border-solid'
    : 'bg-white border-2'}
```

```svelte
<!-- Good: Centralized in style system -->
{@const styles = getClassificationStyles(state)}
<div class={styles.containerClasses}>
```

### Don't: Compute styles inline in complex templates

```svelte
<!-- Bad: Complex inline computation -->
<span style="color: {projectColor && getLuminance(projectColor) < 0.5 ? '#fff' : '#000'}">
```

```svelte
<!-- Good: Pre-computed in style system -->
<span class={getPrimaryTextClasses(styles, isSkipped)} style={getPrimaryTextStyle(styles, isSkipped)}>
```

### Don't: Mix concerns in components

```svelte
<!-- Bad: API calls mixed with presentation -->
<script lang="ts">
  async function handleClick() {
    const result = await api.classify(event.id, projectId);
    // Update local state...
  }
</script>
```

```svelte
<!-- Good: Component receives handlers from parent -->
<script lang="ts">
  let { onclassify }: Props = $props();
</script>

<button onclick={() => onclassify?.(projectId)}>
```

### Don't: Use $effect for derived state

```svelte
<!-- Bad: Effect for computed value -->
<script lang="ts">
  let filtered = $state<Item[]>([]);

  $effect(() => {
    filtered = items.filter(i => i.active);
  });
</script>
```

```svelte
<!-- Good: Derived value -->
<script lang="ts">
  const filtered = $derived(items.filter(i => i.active));
</script>
```

---

## Checklist for New UI Code

- [ ] Uses style system for domain-dependent styling
- [ ] Component has typed Props interface
- [ ] Event handlers follow naming convention
- [ ] Uses $derived for computed values (not $effect)
- [ ] Theme-aware colors use CSS custom properties
- [ ] Component is exported through barrel file
- [ ] No duplicate style logic from other components
- [ ] Accessibility: interactive elements are keyboard accessible
