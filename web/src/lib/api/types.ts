// API types matching the OpenAPI spec

export interface User {
	id: string;
	email: string;
	name: string;
	created_at: string;
}

export interface AuthResponse {
	token: string;
	user: User;
}

export interface Project {
	id: string;
	user_id: string;
	name: string;
	short_code?: string;
	client?: string;
	color: string;
	is_billable: boolean;
	is_archived: boolean;
	is_hidden_by_default?: boolean;
	does_not_accumulate_hours?: boolean;
	fingerprint_domains?: string[];
	fingerprint_emails?: string[];
	fingerprint_keywords?: string[];
	created_at: string;
	updated_at?: string;
}

export interface ProjectCreate {
	name: string;
	short_code?: string;
	client?: string;
	color?: string;
	is_billable?: boolean;
	is_hidden_by_default?: boolean;
	does_not_accumulate_hours?: boolean;
	fingerprint_domains?: string[];
	fingerprint_emails?: string[];
	fingerprint_keywords?: string[];
}

export interface ProjectUpdate {
	name?: string;
	short_code?: string;
	client?: string;
	color?: string;
	is_billable?: boolean;
	is_archived?: boolean;
	is_hidden_by_default?: boolean;
	does_not_accumulate_hours?: boolean;
	fingerprint_domains?: string[];
	fingerprint_emails?: string[];
	fingerprint_keywords?: string[];
}

export interface TimeEntry {
	id: string;
	user_id: string;
	project_id: string;
	project?: Project;
	date: string;
	hours: number;
	title?: string;
	description?: string;
	source: 'manual' | 'calendar' | 'import';
	invoice_id?: string | null;
	has_user_edits?: boolean;
	// Protection model fields
	is_pinned?: boolean;
	is_locked?: boolean;
	is_stale?: boolean;
	// Computed fields (from analyzer)
	computed_hours?: number;
	computed_title?: string;
	computed_description?: string;
	calculation_details?: CalculationDetails;
	contributing_events?: string[];
	created_at: string;
	updated_at?: string;
}

export interface CalculationDetails {
	events: Array<{
		id: string;
		title: string;
		start: string;
		end: string;
		raw_minutes: number;
		is_all_day?: boolean;
	}>;
	time_ranges: Array<{
		start: string;
		end: string;
		minutes: number;
	}>;
	union_minutes: number;
	rounding_applied: string;
	final_minutes: number;
}

export interface TimeEntryCreate {
	project_id: string;
	date: string;
	hours: number;
	description?: string;
}

export interface TimeEntryUpdate {
	hours?: number;
	description?: string;
}

export interface CalendarConnection {
	id: string;
	user_id: string;
	provider: 'google';
	last_synced_at?: string | null;
	created_at: string;
	updated_at?: string;
}

export interface Calendar {
	id: string;
	connection_id: string;
	external_id: string;
	name: string;
	color?: string;
	is_primary: boolean;
	is_selected: boolean;
	last_synced_at?: string | null;
	created_at: string;
	updated_at?: string;
}

export interface UpdateCalendarSourcesRequest {
	calendar_ids: string[];
}

export interface CalendarEvent {
	id: string;
	connection_id: string;
	user_id: string;
	external_id: string;
	title: string;
	description?: string | null;
	start_time: string;
	end_time: string;
	attendees?: string[];
	is_recurring?: boolean;
	response_status?: string | null;
	transparency?: string | null;
	is_orphaned?: boolean;
	is_suppressed?: boolean;
	classification_status: 'pending' | 'classified' | 'skipped';
	classification_source?: 'rule' | 'fingerprint' | 'manual' | 'llm' | null;
	classification_confidence?: number | null;
	needs_review?: boolean;
	project_id?: string | null;
	project?: Project;
	calendar_id?: string | null; // Google Calendar ID (typically the calendar email)
	calendar_name?: string | null;
	calendar_color?: string | null;
	created_at: string;
	updated_at?: string;
}

export interface SyncResult {
	events_created: number;
	events_updated: number;
	events_orphaned: number;
}

export interface OAuthAuthorizeResponse {
	url: string;
	state: string;
}

export interface ClassifyEventRequest {
	project_id?: string;
	skip?: boolean;
}

export interface ClassifyEventResponse {
	event: CalendarEvent;
	time_entry?: TimeEntry;
}

export interface ApiError {
	code: string;
	message: string;
	details?: Record<string, unknown>;
}

// Classification Rules
export interface ClassificationRule {
	id: string;
	user_id: string;
	query: string;
	project_id?: string | null;
	project_name?: string | null;
	project_color?: string | null;
	attended?: boolean | null;
	weight: number;
	is_enabled: boolean;
	created_at: string;
	updated_at?: string;
}

export interface RuleCreate {
	query: string;
	project_id?: string;
	attended?: boolean;
	weight?: number;
	is_enabled?: boolean;
}

export interface RuleUpdate {
	query?: string;
	project_id?: string | null;
	attended?: boolean | null;
	weight?: number;
	is_enabled?: boolean;
}

export interface RulePreviewRequest {
	query: string;
	project_id?: string;
	start_date?: string;
	end_date?: string;
}

export interface RulePreviewResponse {
	matches: MatchedEvent[];
	conflicts: RuleConflict[];
	stats: PreviewStats;
}

export interface MatchedEvent {
	event_id: string;
	title: string;
	start_time: string;
}

export interface RuleConflict {
	event_id: string;
	current_project_id?: string | null;
	current_source?: string;
	proposed_project_id?: string | null;
}

export interface PreviewStats {
	total_matches: number;
	already_correct: number;
	would_change: number;
	manual_conflicts: number;
}

export interface ApplyRulesRequest {
	start_date?: string;
	end_date?: string;
	dry_run?: boolean;
}

export interface ApplyRulesResponse {
	classified: ClassifiedEvent[];
	skipped: number;
}

export interface ClassifiedEvent {
	event_id: string;
	project_id: string;
	confidence: number;
	needs_review: boolean;
}

export interface BulkClassifyRequest {
	query: string;
	project_id?: string;
	skip?: boolean;
}

export interface BulkClassifyResponse {
	classified_count: number;
	skipped_count: number;
	time_entries_created?: number;
}

// API Keys
export interface ApiKey {
	id: string;
	user_id: string;
	name: string;
	key_prefix: string;
	last_used_at?: string | null;
	created_at: string;
}

export interface ApiKeyCreate {
	name: string;
}

export interface ApiKeyWithSecret extends ApiKey {
	key: string;
}

// Billing Periods
export interface BillingPeriod {
	id: string;
	user_id: string;
	project_id: string;
	starts_on: string;
	ends_on?: string | null;
	hourly_rate: number;
	created_at: string;
	updated_at?: string;
}

export interface BillingPeriodCreate {
	project_id: string;
	starts_on: string;
	ends_on?: string;
	hourly_rate: number;
}

export interface BillingPeriodUpdate {
	starts_on?: string;
	ends_on?: string;
	hourly_rate?: number;
}

// Invoices
export type InvoiceStatus = 'draft' | 'sent' | 'paid';

export interface InvoiceLineItem {
	id: string;
	invoice_id: string;
	time_entry_id: string;
	date: string;
	description: string;
	hours: number;
	hourly_rate: number;
	amount: number;
}

export interface Invoice {
	id: string;
	user_id: string;
	project_id: string;
	billing_period_id?: string | null;
	invoice_number: string;
	period_start: string;
	period_end: string;
	invoice_date: string;
	status: InvoiceStatus;
	total_hours: number;
	total_amount: number;
	project?: Project;
	line_items?: InvoiceLineItem[];
	spreadsheet_id?: string | null;
	spreadsheet_url?: string | null;
	worksheet_id?: number | null;
	created_at: string;
	updated_at?: string;
}

export interface InvoiceCreate {
	project_id: string;
	period_start: string;
	period_end: string;
	invoice_date?: string;
}

export interface InvoiceStatusUpdate {
	status: InvoiceStatus;
}
