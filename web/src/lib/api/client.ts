import type {
	User,
	AuthResponse,
	Project,
	ProjectCreate,
	ProjectUpdate,
	TimeEntry,
	TimeEntryCreate,
	TimeEntryUpdate,
	CalendarConnection,
	Calendar,
	CalendarEvent,
	SyncResult,
	OAuthAuthorizeResponse,
	ClassifyEventRequest,
	ClassifyEventResponse,
	UpdateCalendarSourcesRequest,
	ApiError,
	ClassificationRule,
	RuleCreate,
	RuleUpdate,
	RulePreviewRequest,
	RulePreviewResponse,
	ApplyRulesRequest,
	ApplyRulesResponse,
	BulkClassifyRequest,
	BulkClassifyResponse,
	ApiKey,
	ApiKeyCreate,
	ApiKeyWithSecret,
	BillingPeriod,
	BillingPeriodCreate,
	BillingPeriodUpdate,
	Invoice,
	InvoiceCreate,
	InvoiceStatusUpdate,
	InvoiceStatus,
	ConfigExport,
	ConfigImport,
	ConfigImportResult
} from './types';

const API_BASE = '/api';

class ApiClient {
	private token: string | null = null;

	setToken(token: string | null) {
		this.token = token;
	}

	private async request<T>(
		method: string,
		path: string,
		body?: unknown
	): Promise<T> {
		const headers: Record<string, string> = {
			'Content-Type': 'application/json'
		};

		if (this.token) {
			headers['Authorization'] = `Bearer ${this.token}`;
		}

		const response = await fetch(`${API_BASE}${path}`, {
			method,
			headers,
			body: body ? JSON.stringify(body) : undefined
		});

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'unknown',
				message: response.statusText
			}));
			throw new ApiClientError(response.status, error);
		}

		if (response.status === 204) {
			return undefined as T;
		}

		return response.json();
	}

	// Auth
	async signup(email: string, password: string, name: string): Promise<AuthResponse> {
		return this.request('POST', '/auth/signup', { email, password, name });
	}

	async login(email: string, password: string): Promise<AuthResponse> {
		return this.request('POST', '/auth/login', { email, password });
	}

	async logout(): Promise<void> {
		return this.request('POST', '/auth/logout');
	}

	async getCurrentUser(): Promise<User> {
		return this.request('GET', '/auth/me');
	}

	// Projects
	async listProjects(includeArchived = false): Promise<Project[]> {
		const query = includeArchived ? '?include_archived=true' : '';
		return this.request('GET', `/projects${query}`);
	}

	async getProject(id: string): Promise<Project> {
		return this.request('GET', `/projects/${id}`);
	}

	async createProject(data: ProjectCreate): Promise<Project> {
		return this.request('POST', '/projects', data);
	}

	async updateProject(id: string, data: ProjectUpdate): Promise<Project> {
		return this.request('PUT', `/projects/${id}`, data);
	}

	async deleteProject(id: string): Promise<void> {
		return this.request('DELETE', `/projects/${id}`);
	}

	// Time Entries
	async listTimeEntries(params?: {
		start_date?: string;
		end_date?: string;
		project_id?: string;
	}): Promise<TimeEntry[]> {
		const searchParams = new URLSearchParams();
		if (params?.start_date) searchParams.set('start_date', params.start_date);
		if (params?.end_date) searchParams.set('end_date', params.end_date);
		if (params?.project_id) searchParams.set('project_id', params.project_id);
		const query = searchParams.toString();
		return this.request('GET', `/time-entries${query ? `?${query}` : ''}`);
	}

	async getTimeEntry(id: string): Promise<TimeEntry> {
		return this.request('GET', `/time-entries/${id}`);
	}

	async createTimeEntry(data: TimeEntryCreate): Promise<TimeEntry> {
		return this.request('POST', '/time-entries', data);
	}

	async updateTimeEntry(id: string, data: TimeEntryUpdate): Promise<TimeEntry> {
		return this.request('PUT', `/time-entries/${id}`, data);
	}

	async deleteTimeEntry(id: string): Promise<void> {
		return this.request('DELETE', `/time-entries/${id}`);
	}

	async refreshTimeEntry(id: string): Promise<TimeEntry> {
		return this.request('POST', `/time-entries/${id}/refresh`);
	}

	// Calendars
	async googleAuthorize(): Promise<OAuthAuthorizeResponse> {
		return this.request('GET', '/auth/google/authorize');
	}

	async listCalendarConnections(): Promise<CalendarConnection[]> {
		return this.request('GET', '/calendars');
	}

	async deleteCalendarConnection(id: string): Promise<void> {
		return this.request('DELETE', `/calendars/${id}`);
	}

	async syncCalendar(id: string, params?: { start_date?: string; end_date?: string }): Promise<SyncResult> {
		const searchParams = new URLSearchParams();
		if (params?.start_date) searchParams.set('start_date', params.start_date);
		if (params?.end_date) searchParams.set('end_date', params.end_date);
		const query = searchParams.toString();
		return this.request('POST', `/calendars/${id}/sync${query ? `?${query}` : ''}`);
	}

	async listCalendarSources(id: string): Promise<Calendar[]> {
		return this.request('GET', `/calendars/${id}/sources`);
	}

	async updateCalendarSources(id: string, data: UpdateCalendarSourcesRequest): Promise<Calendar[]> {
		return this.request('PUT', `/calendars/${id}/sources`, data);
	}

	async listCalendarEvents(params?: {
		start_date?: string;
		end_date?: string;
		classification_status?: 'pending' | 'classified';
		connection_id?: string;
	}): Promise<CalendarEvent[]> {
		const searchParams = new URLSearchParams();
		if (params?.start_date) searchParams.set('start_date', params.start_date);
		if (params?.end_date) searchParams.set('end_date', params.end_date);
		if (params?.classification_status) searchParams.set('classification_status', params.classification_status);
		if (params?.connection_id) searchParams.set('connection_id', params.connection_id);
		const query = searchParams.toString();
		return this.request('GET', `/calendar-events${query ? `?${query}` : ''}`);
	}

	async classifyCalendarEvent(id: string, data: ClassifyEventRequest): Promise<ClassifyEventResponse> {
		return this.request('PUT', `/calendar-events/${id}/classify`, data);
	}

	// Classification Rules
	async listRules(includeDisabled = false): Promise<ClassificationRule[]> {
		const query = includeDisabled ? '?include_disabled=true' : '';
		return this.request('GET', `/rules${query}`);
	}

	async getRule(id: string): Promise<ClassificationRule> {
		return this.request('GET', `/rules/${id}`);
	}

	async createRule(data: RuleCreate): Promise<ClassificationRule> {
		return this.request('POST', '/rules', data);
	}

	async updateRule(id: string, data: RuleUpdate): Promise<ClassificationRule> {
		return this.request('PUT', `/rules/${id}`, data);
	}

	async deleteRule(id: string): Promise<void> {
		return this.request('DELETE', `/rules/${id}`);
	}

	async previewRule(data: RulePreviewRequest): Promise<RulePreviewResponse> {
		return this.request('POST', '/rules/preview', data);
	}

	async applyRules(data: ApplyRulesRequest = {}): Promise<ApplyRulesResponse> {
		return this.request('POST', '/rules/apply', data);
	}

	async bulkClassifyEvents(data: BulkClassifyRequest): Promise<BulkClassifyResponse> {
		return this.request('POST', '/calendar-events/bulk-classify', data);
	}

	// API Keys
	async listApiKeys(): Promise<ApiKey[]> {
		return this.request('GET', '/api-keys');
	}

	async createApiKey(data: ApiKeyCreate): Promise<ApiKeyWithSecret> {
		return this.request('POST', '/api-keys', data);
	}

	async deleteApiKey(id: string): Promise<void> {
		return this.request('DELETE', `/api-keys/${id}`);
	}

	// Billing Periods
	async listBillingPeriods(projectId: string): Promise<BillingPeriod[]> {
		return this.request('GET', `/billing-periods?project_id=${projectId}`);
	}

	async createBillingPeriod(data: BillingPeriodCreate): Promise<BillingPeriod> {
		return this.request('POST', '/billing-periods', data);
	}

	async updateBillingPeriod(id: string, data: BillingPeriodUpdate): Promise<BillingPeriod> {
		return this.request('PUT', `/billing-periods/${id}`, data);
	}

	async deleteBillingPeriod(id: string): Promise<void> {
		return this.request('DELETE', `/billing-periods/${id}`);
	}

	// Invoices
	async listInvoices(params?: { projectId?: string; status?: InvoiceStatus }): Promise<Invoice[]> {
		const query = new URLSearchParams();
		if (params?.projectId) query.set('project_id', params.projectId);
		if (params?.status) query.set('status', params.status);
		const queryString = query.toString();
		return this.request('GET', `/invoices${queryString ? `?${queryString}` : ''}`);
	}

	async getInvoice(id: string): Promise<Invoice> {
		return this.request('GET', `/invoices/${id}`);
	}

	async createInvoice(data: InvoiceCreate): Promise<Invoice> {
		return this.request('POST', '/invoices', data);
	}

	async deleteInvoice(id: string): Promise<void> {
		return this.request('DELETE', `/invoices/${id}`);
	}

	async updateInvoiceStatus(id: string, data: InvoiceStatusUpdate): Promise<Invoice> {
		return this.request('PUT', `/invoices/${id}/status`, data);
	}

	async exportInvoiceCSV(id: string): Promise<Blob> {
		const headers: Record<string, string> = {};
		if (this.token) {
			headers['Authorization'] = `Bearer ${this.token}`;
		}

		const response = await fetch(`${API_BASE}/invoices/${id}/export/csv`, {
			method: 'GET',
			headers
		});

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({
				code: 'unknown',
				message: response.statusText
			}));
			throw new ApiClientError(response.status, error);
		}

		return response.blob();
	}

	async exportInvoiceSheets(id: string): Promise<{ spreadsheet_id?: string; spreadsheet_url?: string; worksheet_id?: number }> {
		return this.request('POST', `/invoices/${id}/export/sheets`);
	}

	// Configuration Export/Import
	async exportConfig(includeArchived = false): Promise<ConfigExport> {
		const query = includeArchived ? '?include_archived=true' : '';
		return this.request('GET', `/config/export${query}`);
	}

	async importConfig(data: ConfigImport): Promise<ConfigImportResult> {
		return this.request('POST', '/config/import', data);
	}
}

export class ApiClientError extends Error {
	constructor(
		public status: number,
		public error: ApiError
	) {
		super(error.message);
		this.name = 'ApiClientError';
	}
}

export const api = new ApiClient();
