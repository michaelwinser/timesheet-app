Here is a complete implementation in **TypeScript** and **Svelte**.

This solution uses a "Greedy Expansion" algorithm. It packs events into columns and then expands them to the right if there is free space, exactly mimicking the Google Calendar behavior.

### 1. The Algorithm (`eventLayout.ts`)

First, we create a pure TypeScript utility to handle the math. This keeps your Svelte components clean.

```typescript
// types.ts
export interface CalendarEvent {
  id: string;
  start: number; // Minutes from midnight (e.g., 0 = 00:00, 60 = 01:00)
  end: number;
  title?: string;
}

export interface LaidOutEvent extends CalendarEvent {
  // CSS values in percentages
  top: number;
  height: number;
  left: number;
  width: number;
}

```

```typescript
// eventLayout.ts
import type { CalendarEvent, LaidOutEvent } from './types';

export function calculateLayout(events: CalendarEvent[]): LaidOutEvent[] {
  // 1. Sort events by start time, then by duration (longest first) for better packing
  const sortedEvents = [...events].sort((a, b) => {
    if (a.start === b.start) return (b.end - b.start) - (a.end - a.start);
    return a.start - b.start;
  });

  // 2. Group events into colliding clusters
  const clusters: CalendarEvent[][] = [];
  let currentCluster: CalendarEvent[] = [];
  let clusterEnd = -1;

  for (const event of sortedEvents) {
    if (currentCluster.length === 0) {
      currentCluster.push(event);
      clusterEnd = event.end;
    } else if (event.start < clusterEnd) {
      // Overlap detected: add to current cluster
      currentCluster.push(event);
      clusterEnd = max(clusterEnd, event.end);
    } else {
      // No overlap: seal the current cluster and start a new one
      clusters.push(currentCluster);
      currentCluster = [event];
      clusterEnd = event.end;
    }
  }
  if (currentCluster.length > 0) clusters.push(currentCluster);

  // 3. Process each cluster to assign columns and layout
  const layoutEvents: LaidOutEvent[] = [];
  
  for (const cluster of clusters) {
    layoutEvents.push(...layoutCluster(cluster));
  }

  return layoutEvents;
}

function layoutCluster(cluster: CalendarEvent[]): LaidOutEvent[] {
  // Columns is an array of "end times" for the last event in that column
  const columns: CalendarEvent[][] = []; 
  
  // A map to store which column an event was assigned to
  const eventColIndex = new Map<string, number>();

  // A. Pack events into columns
  for (const event of cluster) {
    let placed = false;
    
    // Try to fit in existing columns
    for (let i = 0; i < columns.length; i++) {
      const colEvents = columns[i];
      const lastEvent = colEvents[colEvents.length - 1];
      
      // If the event starts after the last event in this column ends
      if (event.start >= lastEvent.end) {
        colEvents.push(event);
        eventColIndex.set(event.id, i);
        placed = true;
        break;
      }
    }
    
    // If it didn't fit, start a new column
    if (!placed) {
      columns.push([event]);
      eventColIndex.set(event.id, columns.length - 1);
    }
  }

  // B. Calculate dimensions with "Greedy Expansion"
  const totalCols = columns.length;
  
  return cluster.map(event => {
    const colIndex = eventColIndex.get(event.id)!;
    
    // Default span is 1
    let span = 1;

    // Check columns to the right. If they are empty during this event's time, expand.
    for (let i = colIndex + 1; i < totalCols; i++) {
      // Check if any event in column `i` overlaps with our current `event`
      const hasCollision = columns[i].some(otherEvt => 
        collide(event, otherEvt)
      );
      
      if (hasCollision) break;
      span++;
    }

    return {
      ...event,
      top: event.start,             // In minutes (CSS can convert to px)
      height: event.end - event.start,
      left: (colIndex / totalCols) * 100,
      width: (span / totalCols) * 100
    };
  });
}

// Helper: Simple collision check
function collide(a: CalendarEvent, b: CalendarEvent) {
  return a.start < b.end && a.end > b.start;
}

// Helper: Math.max alias
function max(a: number, b: number) { return a > b ? a : b; }

```

---

### 2. The Components

#### `CalendarEventCard.svelte`

This component is purely presentational. It positions itself absolutely based on the props passed to it.

```svelte
<script lang="ts">
  import type { LaidOutEvent } from './types';

  export let event: LaidOutEvent;
  
  // Configuration: How many pixels per minute?
  const PIXELS_PER_MINUTE = 2; 
</script>

<div 
  class="event-card"
  style:top="{event.top * PIXELS_PER_MINUTE}px"
  style:height="{event.height * PIXELS_PER_MINUTE}px"
  style:left="{event.left}%"
  style:width="{event.width}%"
>
  <div class="inner-content">
    <strong>{event.title || 'Untitled'}</strong>
    <span class="time">{event.start} - {event.end}</span>
  </div>
</div>

<style>
  .event-card {
    position: absolute;
    padding: 0 4px; /* Gutter between events */
    box-sizing: border-box;
    transition: all 0.2s ease;
  }

  .inner-content {
    background-color: #039be5;
    color: white;
    height: 100%;
    border-radius: 4px;
    padding: 4px;
    font-size: 12px;
    overflow: hidden;
    border: 1px solid white; /* Adds visual separation when overlapping */
    box-shadow: 0 1px 3px rgba(0,0,0,0.2);
  }
  
  .time {
    display: block;
    font-size: 0.85em;
    opacity: 0.9;
  }
</style>

```

#### `TimeGrid.svelte`

This component manages the data flow. It takes raw events, runs the layout algorithm, and renders the container.

```svelte
<script lang="ts">
  import { calculateLayout } from './eventLayout';
  import type { CalendarEvent } from './types';
  import CalendarEventCard from './CalendarEventCard.svelte';

  // 1. Input: Raw events
  export let events: CalendarEvent[] = [];

  // 2. Output: Laid out events (Computed)
  // Re-run layout whenever `events` changes
  $: layout = calculateLayout(events);
  
  // Total height of the grid (24 hours * 60 mins * 2px/min)
  const TOTAL_HEIGHT = 24 * 60 * 2; 
</script>

<div class="time-grid-container" style:height="{TOTAL_HEIGHT}px">
  {#each Array(24) as _, i}
    <div class="hour-marker" style:top="{i * 60 * 2}px">
      <span class="label">{i}:00</span>
    </div>
  {/each}

  {#each layout as event (event.id)}
    <CalendarEventCard {event} />
  {/each}
</div>

<style>
  .time-grid-container {
    position: relative;
    width: 100%;
    background-color: #fafafa;
    border-left: 1px solid #ddd;
    /* Ensure the container has width for percentage-based children */
    min-width: 300px; 
  }

  .hour-marker {
    position: absolute;
    width: 100%;
    border-top: 1px solid #eee;
    color: #888;
    font-size: 10px;
    pointer-events: none; /* Let clicks pass through to events */
  }
  
  .label {
    position: absolute;
    top: -8px;
    left: -35px; /* Pull label outside grid */
    width: 30px;
    text-align: right;
  }
</style>

```

### 3. Usage Example

Here is how you would use it in a parent page (e.g., `App.svelte`):

```svelte
<script lang="ts">
  import TimeGrid from './TimeGrid.svelte';
  import type { CalendarEvent } from './types';

  const myEvents: CalendarEvent[] = [
    { id: '1', start: 30, end: 150, title: 'Meeting A' },  // 00:30 - 02:30
    { id: '2', start: 540, end: 600, title: 'Lunch' },     // 09:00 - 10:00
    { id: '3', start: 560, end: 620, title: 'Call' },      // 09:20 - 10:20 (Overlap)
    { id: '4', start: 610, end: 670, title: 'Coding' },    // 10:10 - 11:10 (Overlap with Call)
  ];
</script>

<div style="padding-left: 50px; width: 600px;">
  <TimeGrid events={myEvents} />
</div>

```

### Key Implementation Details

1. **Reactive Statements (`$:`)**: In `TimeGrid.svelte`, the line `$: layout = calculateLayout(events);` ensures that if you fetch new events from an API or drag-and-drop an event (updating its start/end time), the layout automatically recalculates.
2. **Absolute Positioning**: We use `left` and `width` in percentages. This makes the grid responsive. If the user resizes the window, the columns scale automatically.
3. **Visual Polish**: In `CalendarEventCard`, I added a `padding: 0 4px` to the wrapper div. This creates a small transparent gap between columns (the "gutter"), while the `border: 1px solid white` on the inner content makes overlapping cards visually distinct, exactly like Google Calendar.

Would you like me to add logic for handling click/drag events to modify the `start` and `end` times?
