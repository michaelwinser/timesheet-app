# Web Client Components - Timesheet App v2

This document catalogs the UI components for the **Web Client**. For the overall architecture and how this relates to other layers, see `architecture.md`.

---

## Scope

This document covers:
- **Widgets** - Entity-bound display/edit components
- **Containers** - Composite components with data/state
- **Primitives** - Generic reusable UI elements
- **Pages** - Route-level layouts

It does NOT cover:
- Service layer components (see `architecture.md`)
- Domain entities (see `domain-glossary.md`)
- MCP tools/resources

---

## Design Principles

1. **Consistent naming** - `{Entity}{Presentation}` for widgets
2. **Props-driven** - Data flows in via props, events flow out
3. **Entity-aware** - Widgets know which domain entity they represent
4. **Context-flexible** - Same entity can have multiple presentations
5. **Accessible** - Keyboard navigation, ARIA labels

---

## Widget Catalog

Widgets are entity-bound components. Each entity may have multiple presentations.

### Project Widgets

**`ProjectChip`** - Compact colored label

| Prop | Type | Description |
|------|------|-------------|
| `project` | Project | The project to display |
| `size` | `sm` \| `md` | Size variant (default: `sm`) |

Behavior:
- Shows `short_code` if present, otherwise `name`
- Background from `project.color`
- Auto-selects text color for contrast

---

**`ProjectListItem`** - Row in a project list

| Prop | Type | Description |
|------|------|-------------|
| `project` | Project | The project to display |

| Event | Payload | Description |
|-------|---------|-------------|
| `click` | - | User clicked the row |

Behavior:
- Shows name, chip, client grouping
- Navigates to ProjectEditor on click

---

**`ProjectEditor`** - Full form for editing

| Prop | Type | Description |
|------|------|-------------|
| `project` | Project | The project to edit |

| Event | Payload | Description |
|-------|---------|-------------|
| `save` | Project | User saved changes |
| `delete` | - | User requested deletion |
| `cancel` | - | User cancelled |

Behavior:
- All editable fields (name, code, color, billing, etc.)
- Manages BillingPeriods inline
- Fingerprint pattern editors

---

**`ProjectTooltip`** - Hover detail

| Prop | Type | Description |
|------|------|-------------|
| `project` | Project | The project to display |
| `stats` | { weekHours, monthHours } | Optional stats |

Behavior:
- Shows project name, client, hours summary
- Appears on hover over ProjectChip

---

### TimeEntry Widgets

**`TimeEntryCard`** - Card display with inline editing

| Prop | Type | Description |
|------|------|-------------|
| `entry` | TimeEntry | The entry to display |
| `editable` | boolean | Whether editing allowed |

| Event | Payload | Description |
|-------|---------|-------------|
| `update` | { hours?, description? } | User edited |
| `delete` | - | User requested deletion |

States:
- Default: ProjectChip, hours, description
- Editing: Inline inputs
- Locked: Visual indicator when invoiced
- Orphaned: Visual indicator when events orphaned

---

**`TimeEntryRow`** - Compact row for lists/tables

| Prop | Type | Description |
|------|------|-------------|
| `entry` | TimeEntry | The entry to display |

Behavior:
- Compact single-line display
- Click to expand or navigate

---

**`TimeEntryEditor`** - Modal/form for detailed editing

| Prop | Type | Description |
|------|------|-------------|
| `entry` | TimeEntry? | Entry to edit (null for new) |
| `projects` | Project[] | Available projects |
| `date` | Date | Date for new entries |

| Event | Payload | Description |
|-------|---------|-------------|
| `save` | TimeEntry | User saved |
| `cancel` | - | User cancelled |

---

### CalendarEvent Widgets

**`CalendarEventCard`** - Unclassified event display

| Prop | Type | Description |
|------|------|-------------|
| `event` | CalendarEvent | The event to display |
| `projects` | Project[] | For quick-assign |

| Event | Payload | Description |
|-------|---------|-------------|
| `classify` | { projectId } | User assigned project |
| `skip` | - | User marked DNA |
| `createRule` | - | User wants rule from this |

---

**`CalendarEventPopover`** - Detail view on hover/click

| Prop | Type | Description |
|------|------|-------------|
| `event` | CalendarEvent | The event to display |

Behavior:
- Shows title, time, attendees, description
- Shows classification status and source

---

### Invoice Widgets

**`InvoiceCard`** - Summary card

| Prop | Type | Description |
|------|------|-------------|
| `invoice` | Invoice | The invoice to display |

Behavior:
- Shows period, total hours, total amount, status
- Click to open InvoiceEditor

---

**`InvoiceLineItem`** - Row in invoice detail

| Prop | Type | Description |
|------|------|-------------|
| `entry` | TimeEntry | Entry in the invoice |

---

**`InvoiceEditor`** - Full invoice management

| Prop | Type | Description |
|------|------|-------------|
| `invoice` | Invoice? | Invoice to edit (null for new) |
| `project` | Project | The project being invoiced |

| Event | Payload | Description |
|-------|---------|-------------|
| `save` | Invoice | User saved |
| `send` | - | User marked as sent |
| `cancel` | - | User cancelled |

---

### Rule Widgets

**`RuleCard`** - Display a classification rule

| Prop | Type | Description |
|------|------|-------------|
| `rule` | ClassificationRule | The rule to display |

| Event | Payload | Description |
|-------|---------|-------------|
| `edit` | - | User wants to edit |
| `delete` | - | User wants to delete |
| `toggle` | boolean | User toggled enabled |

---

**`RuleEditor`** - Create/edit rules with preview

| Prop | Type | Description |
|------|------|-------------|
| `rule` | ClassificationRule? | Rule to edit (null for new) |
| `projects` | Project[] | Available targets |

| Event | Payload | Description |
|-------|---------|-------------|
| `save` | ClassificationRule | User saved |
| `cancel` | - | User cancelled |

Behavior:
- Query input with syntax help
- Live preview of matching events
- Debounced preview requests

---

**`RuleMatchExplanation`** - Why a rule matched an event

| Prop | Type | Description |
|------|------|-------------|
| `rule` | ClassificationRule | The rule that matched |
| `event` | CalendarEvent | The event it matched |

Behavior:
- Shows which parts of query matched
- Useful for debugging classification

---

## Containers

Containers are composite components that manage data fetching and state.

### `ProjectSummary`
Sidebar showing hours by project for current view.

| Prop | Type | Description |
|------|------|-------------|
| `entries` | TimeEntry[] | Entries to summarize |
| `projects` | Project[] | Project metadata |

Behavior:
- Groups entries by project
- Shows ProjectChip + hours for each
- Calculates and displays total
- Toggle project visibility (filter)

---

### `TimeEntryList`
List of time entries with filtering/sorting.

| Prop | Type | Description |
|------|------|-------------|
| `entries` | TimeEntry[] | Entries to display |
| `groupBy` | `date` \| `project` | Grouping mode |

---

### `ClassificationPanel`
Panel for managing unclassified events.

| Prop | Type | Description |
|------|------|-------------|
| `events` | CalendarEvent[] | Unclassified events |
| `projects` | Project[] | Available projects |

---

### `InvoiceBuilder`
Interface for creating invoices from uninvoiced entries.

| Prop | Type | Description |
|------|------|-------------|
| `project` | Project | Project to invoice |
| `entries` | TimeEntry[] | Available entries |

---

## Primitives

Generic UI elements, not entity-specific.

| Primitive | Purpose |
|-----------|---------|
| `Button` | Actions |
| `Input` | Text input |
| `Checkbox` | Boolean input |
| `Dropdown` | Selection |
| `TagInput` | Multiple value input (fingerprint patterns) |
| `QueryInput` | Classification query with validation |
| `DatePicker` | Single date selection |
| `DateRangePicker` | Date range selection |
| `Modal` | Dialog overlay |
| `Toast` | Notification messages |
| `Tooltip` | Hover hints |
| `Badge` | Status indicators |

---

## Pages

Route-level components that compose containers and widgets.

| Page | Route | Purpose |
|------|-------|---------|
| `WeekPage` | `/` | Weekly calendar view with time entries |
| `ProjectsPage` | `/projects` | Project list, grouped by client |
| `ProjectDetailPage` | `/projects/:id` | Single project editor |
| `RulesPage` | `/rules` | Classification rule management |
| `InvoicesPage` | `/invoices` | Invoice list and creation |
| `SettingsPage` | `/settings` | User settings, calendar connections |

---

## State Conventions

Widgets receive data via props and emit events. They don't fetch data directly.

Containers:
- Fetch data from API
- Manage loading/error states
- Pass data to widgets
- Handle widget events, update API

Pages:
- Define layout
- Instantiate containers
- Handle navigation

---

## Testing Vocabulary

When writing tests, use widget names precisely:

```
Given a TimeEntry for project "Acme" on 2024-01-15
When I click the TimeEntryCard
Then the TimeEntryEditor opens
And the ProjectChip shows "ACM"
```

This vocabulary maps directly to components, making tests readable and maintainable.
