# Project Spreadsheets - Mini PRD

## Overview

Enhance the project-spreadsheet integration to give users explicit control over attached Google Sheets, improve data format for automation, and support yearly archival workflows.

## Goals

1. **Visibility**: Users can see and access their project's spreadsheet directly from the Projects UI
2. **Control**: Users can create, archive, and manage spreadsheet attachments
3. **Automation-friendly**: Data format optimized for pivots, charts, and external tools
4. **Safety**: Warn before overwriting manually-edited sheets

## Non-Goals (Future)

- Attaching existing user spreadsheets (permission complexity)
- Attaching multiple spreadsheets per project
- Exporting time entries (beyond invoice data)
- Automatic archive prompts

## User Stories

### US1: View Attached Spreadsheet
**As a** user with a billable project
**I want to** see if a spreadsheet is attached and access it
**So that** I can quickly open my invoice data in Sheets

### US2: Create Spreadsheet
**As a** user with a billable project without a spreadsheet
**I want to** create a new spreadsheet for the project
**So that** I'm ready to export invoices

### US3: Archive Spreadsheet
**As a** user with a large spreadsheet (many invoices)
**I want to** archive the current spreadsheet and start fresh
**So that** I can organize invoices by year while keeping historical data

### US4: Safe Overwrites
**As a** user who may edit spreadsheets manually
**I want to** be warned before my edits are overwritten
**So that** I don't accidentally lose manual changes

## Functional Requirements

### FR1: Projects UI - Spreadsheet Section

For billable projects, display a "Spreadsheet" section showing:

**If no spreadsheet attached:**
- Message: "No spreadsheet attached"
- Button: "Create Spreadsheet"

**If spreadsheet attached:**
- Link to open spreadsheet in new tab
- "Archive" button/action

### FR2: Create Spreadsheet

When user clicks "Create Spreadsheet":
1. Create new Google Sheet named "{Project Name} - Invoices"
2. Initialize with Summary worksheet (headers + formulas)
3. Store spreadsheet ID and URL on project
4. Show success message with link

### FR3: Archive Spreadsheet

When user clicks "Archive":
1. Prompt for archive name (default: "{Project Name} - Invoices {Year}")
2. Rename current spreadsheet via Sheets API
3. Clear `sheets_spreadsheet_id` and `sheets_spreadsheet_url` from project
4. Next invoice export will create a fresh spreadsheet

### FR4: Invoice Worksheet Format (Updated)

Each invoice worksheet contains **raw data only**:

```
| Date       | Description          | Hours | Rate   | Amount  |
|------------|----------------------|-------|--------|---------|
| 2025-01-15 | Client meeting       | 1.5   | 150.00 | 225.00  |
| 2025-01-16 | Development work     | 4.0   | 150.00 | 600.00  |
| 2025-01-17 | Code review          | 2.0   | 150.00 | 300.00  |
```

No totals row - totals are calculated in Summary sheet.

### FR5: Summary Worksheet Format (Updated)

Summary sheet uses formulas to calculate totals:

```
| Invoice Number | Period                  | Hours   | Amount    | Status    | Exported   |
|----------------|-------------------------|---------|-----------|-----------|------------|
| ABC-2025-001   | 2025-01-01 - 2025-01-31 | =SUM()  | =SUM()    | FINALIZED | 2025-02-01 |
| ABC-2025-002   | 2025-02-01 - 2025-02-28 | =SUM()  | =SUM()    | DRAFT     | 2025-03-01 |
```

Hours and Amount columns use `SUMIF` or direct range references to invoice worksheets.

### FR6: Overwrite Warning

Before updating an existing invoice worksheet:
1. Compare spreadsheet's `modifiedTime` with our `last_exported_at`
2. If spreadsheet was modified after our last export:
   - Show warning: "This spreadsheet was modified since last export. Overwrite anyway?"
   - User can confirm or cancel
3. If not modified, proceed without warning

### FR7: Delete Invoice - Sheet Cleanup

When an invoice is deleted:
1. Remove the corresponding worksheet from the spreadsheet
2. Summary formulas auto-update (broken refs become 0 or #REF which user can clean up)

## UI Mockups

### Projects Page - Spreadsheet Section

```
┌─────────────────────────────────────────────────────────────┐
│ Alpha Omega Consulting                              [Edit]  │
├─────────────────────────────────────────────────────────────┤
│ Client: Acme Corp                                           │
│ Rate: $150/hr                                               │
│ Code: AOC                                                   │
│                                                             │
│ Spreadsheet                                                 │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ 📊 Alpha Omega Consulting - Invoices          [Open] ↗  │ │
│ │                                               [Archive] │ │
│ └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Archive Dialog

```
┌─────────────────────────────────────────────────────────────┐
│ Archive Spreadsheet                                    [X]  │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ This will rename the current spreadsheet and detach it      │
│ from the project. A new spreadsheet will be created on      │
│ the next invoice export.                                    │
│                                                             │
│ Archive name:                                               │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Alpha Omega Consulting - Invoices 2024                  │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ The archived spreadsheet will remain in your Google Drive.  │
│                                                             │
│                              [Cancel]  [Archive]            │
└─────────────────────────────────────────────────────────────┘
```

### Overwrite Warning

```
┌─────────────────────────────────────────────────────────────┐
│ ⚠️ Spreadsheet Modified                               [X]  │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ This spreadsheet was modified after the last export.        │
│ Exporting will overwrite the invoice worksheet.             │
│                                                             │
│ Last exported: 2025-01-15 10:30 AM                          │
│ Last modified: 2025-01-16 2:45 PM                           │
│                                                             │
│                         [Cancel]  [Export Anyway]           │
└─────────────────────────────────────────────────────────────┘
```

## Success Metrics

- Users can manage spreadsheets without leaving the app
- Invoice data is easily consumable for reporting
- No accidental data loss from overwrites

## Open Questions

1. **Archived sheets list**: Should we track archived spreadsheets somewhere, or just let users find them in Drive?
   - **Recommendation**: Don't track - keeps it simple. Users can find in Drive.

2. **Summary formulas complexity**: Cross-sheet references can be fragile. Alternative is to update Summary values directly (not formulas).
   - **Recommendation**: Start with direct values, add formulas later if users request.
