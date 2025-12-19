# PRD: Rules System v2

## Overview

Replace the current structured rules system with a text-based query syntax (Gmail-style) featuring live preview, project-centric organization, and LLM-assisted rule generation.

## Problems with Current System

1. **Too cumbersome**: Multiple dropdowns per condition, hard to see what a rule does at a glance
2. **Limited expressiveness**: AND-only logic, no grouping, requires many rules for simple concepts
3. **Multi-value confusion**: "any item matches" vs "list contains" is unclear
4. **Rule sprawl**: Many rules become unmanageable, easy to create duplicates/overlaps
5. **Create-from-event is broken**: Populates comma-separated values that don't match
6. **Wrong mental model**: Rules organized by rule, not by target (project)

## Design Principles

1. **Text-first**: Gmail-style query syntax, learnable through live preview
2. **Project-centric**: Organize rules by what they classify TO, not as a flat list
3. **Live preview**: See matching events as you type/edit
4. **Smart defaults**: Properties use sensible matching (contains for text, domain extraction for emails)
5. **Orthogonal DNA**: "Did Not Attend" rules run separately from project rules (both can match)

---

## Rule Evaluation Model

```
For each event:
  1. Evaluate all DNA rules → if ANY match, set did_not_attend flag
  2. Evaluate project rules in priority order → assign to FIRST matching project

(Both DNA and project classification can apply to the same event)
```

**Priority**: Drag-to-reorder in UI, stored as integer in database for import/export.

---

## Query Syntax

### Basic Structure
```
property:value property2:value2        # Implicit AND
(property:a OR property:b)             # Explicit OR with grouping
property:"value with spaces"           # Quoted strings
-property:value                        # Negation (future)
property:/regex/                       # Regex (future)
```

### Properties

| Property | Type | Matching | Example |
|----------|------|----------|---------|
| `title` | string | contains | `title:standup` |
| `description` | string | contains | `description:agenda` |
| `attendees` | smart | name or email contains | `attendees:alice` |
| `domain` | extracted | domain from attendee emails | `domain:linuxfoundation.org` |
| `email` | exact | exact email in attendees | `email:bob@example.com` |
| `response` | enum | exact match | `response:declined` |
| `recurring` | boolean | yes/no | `recurring:yes` |
| `recurrence-id` | string | exact | (for LLM use) |
| `transparency` | enum | opaque/transparent (busy/free) | `transparency:transparent` |
| `is-all-day` | boolean | yes/no | `is-all-day:yes` |
| `has-attendees` | boolean | has attendees besides owner | `has-attendees:no` |
| `day-of-week` | enum | mon/tue/wed/thu/fri/sat/sun | `day-of-week:sat` |
| `time-of-day` | range | HH:MM format | `time-of-day:>17:00` |
| `color` | string | calendar color ID | `color:11` |
| `visibility` | enum | default/public/private | `visibility:private` |

### Example Rules

```
# Alpha-Omega project: anyone from these orgs
(domain:linuxfoundation.org OR domain:alpha-omega.dev OR email:bob.calloway@google.com)

# Did Not Attend: declined or tentative without response
response:declined OR (response:needsAction has-attendees:yes)

# Personal: weekend or after-hours
day-of-week:sat OR day-of-week:sun OR time-of-day:>18:00

# Noise: free/busy shows free, or all-day events
transparency:transparent OR is-all-day:yes
```

---

## Two Types of Rules

### 1. Project Fingerprint (Generated Rules)

Authored on the Project page as simple lists:
- Domains: `linuxfoundation.org`, `alpha-omega.dev`
- Emails: `bob@example.com`, `alice@example.com`
- Title keywords: `standup`, `weekly sync`

System generates rules from these, shown in rules list as read-only:
```
[Generated from Alpha-Omega settings]
(domain:linuxfoundation.org OR domain:alpha-omega.dev OR email:bob@example.com OR title:standup)
```

### 2. Custom Rules

For complex cases that don't fit the fingerprint model. Full query syntax, manually authored.

---

## Entry Points for Rule Creation

### a) From a Single Event (Live Preview Builder)
1. Click "Create Rule" on an event card
2. See event properties, check boxes to include as conditions
3. Live preview shows all matching events
4. Refine until match set is correct
5. Assign to project → Save

### b) LLM-Assisted (Future)
- "Classify this week based on past classifications"
- "Create rules based on my manual classifications"
- "These 5 events are all Alpha-Omega" → LLM proposes rule
- "These events were manually classified but don't match rules" → suggest rule

### c) Project-Centric
1. Go to Project page
2. Add domains, emails, keywords to fingerprint
3. Rules auto-generated

### d) Direct Authoring
1. Go to Rules page
2. Type query in search box with live preview
3. Assign to project → Save

---

## Migration

**Clean break**: Delete all existing rules, update schema, start fresh. We're early enough that this is acceptable.

---

## Implementation Plan

### Phase 1: Foundation
1. Create Project detail page (currently projects don't have their own page)
2. Design new rules schema with query string storage
3. Build query parser (property:value syntax)
4. Build query evaluator (match events against parsed query)

### Phase 2: Rules Page
1. Rules page with text input + live preview
2. Preview shows matching events (compact, one per line)
3. Assign to project or DNA
4. Drag-to-reorder within sections
5. Rules grouped by project in display

### Phase 3: Project Fingerprint
1. Add fingerprint section to Project page (domains, emails, keywords)
2. Auto-generate rules from fingerprint
3. Show generated rules as read-only in rules list

### Phase 4: Create-from-Event
1. Rebuild modal with property checkboxes
2. Live preview of matching events
3. One-click "use this value" for each property

### Phase 5: LLM Integration (Future)
1. Suggest classifications based on history
2. Propose rules from manual classifications
3. Pattern detection from selected events

---

## Future Considerations

- Rules that set entry properties (e.g., `transparency:transparent → hours:0`)
- Regex support: `title:/^Weekly.*Sync$/`
- Negation: `-domain:personal.com`
- Rules page as main search feature
- Rule conflict detection / overlap warnings

---

# UX Mockups

## Rules Page

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Rules                                                            [+ New Rule]│
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ domain:linuxfoundation.org title:weekly                             🔍  │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ Preview: 7 events match                                                     │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ Dec 16  Alpha-Omega Program Management Weekly    alice@linuxfound...   │ │
│ │ Dec 17  Weekly Security Sync                     bob@linuxfoundati...   │ │
│ │ Dec 18  Weekly Standup                           team@linuxfoundat...   │ │
│ │ Dec 09  Alpha-Omega Program Management Weekly    alice@linuxfound...   │ │
│ │ ...                                                                     │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ Assign to: [Alpha-Omega     ▼]  [Save Rule]  [Cancel]                       │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ ALPHA-OMEGA (3 rules)                                              [Edit ▼] │
│ ┌ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┐ │
│   ≡ (domain:linuxfoundation.org OR domain:alpha-omega.dev)    [Generated]   │
│   ≡ email:michael.scovetta@gmail.com                               [Edit]   │
│   ≡ title:"SLSA Specification"                                     [Edit]   │
│ └ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘ │
│                                                                             │
│ ECLIPSE SECURITY (1 rule)                                          [Edit ▼] │
│ ┌ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┐ │
│   ≡ (domain:eclipse.org OR title:Eclipse)                      [Generated]  │
│ └ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘ │
│                                                                             │
│ DID NOT ATTEND (2 rules)                                           [Edit ▼] │
│ ┌ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┐ │
│   ≡ response:declined                                              [Edit]   │
│   ≡ title:Canceled                                                 [Edit]   │
│ └ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Project Detail Page

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ ← Back to Projects                                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ ■ Alpha-Omega                                                    [Save]     │
│                                                                             │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ Name:        [Alpha-Omega                                    ]          │ │
│ │ Color:       [■ #4A90D9] [Pick]                                         │ │
│ │ Archived:    [ ]                                                        │ │
│ │ Hidden:      [ ]                                                        │ │
│ │ No Hours:    [ ]                                                        │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ ─────────────────────────────────────────────────────────────────────────── │
│                                                                             │
│ MATCHING PATTERNS (Auto-generates rules)                                    │
│                                                                             │
│ Domains:                                                                    │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ linuxfoundation.org                                               [×]   │ │
│ │ alpha-omega.dev                                                   [×]   │ │
│ │ [+ Add domain                                                   ]       │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ Email addresses:                                                            │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ michael.scovetta@gmail.com                                        [×]   │ │
│ │ bob.calloway@google.com                                           [×]   │ │
│ │ [+ Add email                                                    ]       │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ Title keywords:                                                             │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ SLSA                                                              [×]   │ │
│ │ OpenSSF                                                           [×]   │ │
│ │ [+ Add keyword                                                  ]       │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ Generated rule preview:                                                     │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ (domain:linuxfoundation.org OR domain:alpha-omega.dev OR                │ │
│ │  email:michael.scovetta@gmail.com OR email:bob.calloway@google.com OR   │ │
│ │  title:SLSA OR title:OpenSSF)                                           │ │
│ │                                                                         │ │
│ │ Matches 23 events                                              [Preview]│ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ ─────────────────────────────────────────────────────────────────────────── │
│                                                                             │
│ CUSTOM RULES                                                    [+ Add Rule]│
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ ≡ attendees:henriyandell                                         [Edit] │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ ─────────────────────────────────────────────────────────────────────────── │
│                                                                             │
│ STATISTICS                                                                  │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ This week:     8.0 hrs                                                  │ │
│ │ This month:   32.5 hrs                                                  │ │
│ │ All time:    245.0 hrs                                                  │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Create Rule from Event (Modal)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Create Rule from Event                                                  [×] │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ Select properties to match:                                                 │
│                                                                             │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ [✓] title         "Alpha-Omega Program Management Weekly"               │ │
│ │ [ ] description   "Agenda: 1. Status updates 2. Blockers..."            │ │
│ │ [✓] domain        linuxfoundation.org (3 attendees)                     │ │
│ │ [ ] email         alice@linuxfoundation.org                             │ │
│ │ [ ] email         bob@linuxfoundation.org                               │ │
│ │ [ ] email         charlie@linuxfoundation.org                           │ │
│ │ [✓] recurring     yes                                                   │ │
│ │ [ ] response      accepted                                              │ │
│ │ [ ] day-of-week   tuesday                                               │ │
│ │ [ ] time-of-day   10:00                                                 │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ Generated query:                                                            │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ title:"Alpha-Omega Program Management Weekly" domain:linuxfoundation.org│ │
│ │ recurring:yes                                                           │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ Preview: 12 events match                                                    │
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ ✓ Dec 16  Alpha-Omega Program Management Weekly                         │ │
│ │ ✓ Dec 09  Alpha-Omega Program Management Weekly                         │ │
│ │ ✓ Dec 02  Alpha-Omega Program Management Weekly                         │ │
│ │ ✓ Nov 25  Alpha-Omega Program Management Weekly                         │ │
│ │   ...8 more                                                             │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│ Assign to: [Alpha-Omega     ▼]   [○ Project  ● Did Not Attend]              │
│                                                                             │
│                                            [Save Rule]  [Cancel]            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Navigation Update

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ [Week] [Projects] [Rules] [Export]                         user@example.com │
└─────────────────────────────────────────────────────────────────────────────┘
```

Projects page becomes a list linking to individual project detail pages.
