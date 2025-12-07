/**
 * Client-side API library for Timesheet app.
 * Thin wrapper around fetch providing domain methods.
 */

/**
 * Show a toast notification.
 * @param {string} message - The message to display
 * @param {string} type - 'success', 'error', or 'info'
 * @param {number} duration - Duration in ms (default 3000)
 */
function showToast(message, type = 'info', duration = 3000) {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);

    setTimeout(() => {
        toast.style.animation = 'fadeOut 0.3s ease-out forwards';
        setTimeout(() => toast.remove(), 300);
    }, duration);
}

const api = {
    /**
     * Make a GET request.
     */
    async get(path) {
        const response = await fetch(path);
        if (!response.ok) {
            const error = await response.json().catch(() => ({ detail: 'Request failed' }));
            throw new Error(error.detail || 'Request failed');
        }
        return response.json();
    },

    /**
     * Make a POST request with JSON body.
     */
    async post(path, data) {
        const response = await fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data),
        });
        if (!response.ok) {
            const error = await response.json().catch(() => ({ detail: 'Request failed' }));
            throw new Error(error.detail || 'Request failed');
        }
        return response.json();
    },

    /**
     * Make a PUT request with JSON body.
     */
    async put(path, data) {
        const response = await fetch(path, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data),
        });
        if (!response.ok) {
            const error = await response.json().catch(() => ({ detail: 'Request failed' }));
            throw new Error(error.detail || 'Request failed');
        }
        return response.json();
    },

    /**
     * Make a DELETE request.
     */
    async delete(path) {
        const response = await fetch(path, { method: 'DELETE' });
        if (!response.ok) {
            const error = await response.json().catch(() => ({ detail: 'Request failed' }));
            throw new Error(error.detail || 'Request failed');
        }
        return response.json();
    },

    // --- Domain methods ---

    /**
     * Sync calendar events for a date range.
     */
    async syncEvents(startDate, endDate) {
        return this.post('/api/sync', { start_date: startDate, end_date: endDate });
    },

    /**
     * Get events for a date range.
     */
    async getEvents(startDate, endDate) {
        return this.get(`/api/events?start_date=${startDate}&end_date=${endDate}`);
    },

    /**
     * Classify an event (create time entry).
     */
    async classifyEvent(eventId, projectId, hours, description) {
        return this.post('/api/entries', {
            event_id: eventId,
            project_id: projectId,
            hours: hours,
            description: description,
        });
    },

    /**
     * Update a time entry.
     */
    async updateEntry(entryId, updates) {
        return this.put(`/api/entries/${entryId}`, updates);
    },

    /**
     * Unclassify an event (delete time entry).
     */
    async unclassify(entryId) {
        return this.delete(`/api/entries/${entryId}`);
    },

    /**
     * Bulk classify events.
     */
    async bulkClassify(eventIds, projectId) {
        return this.post('/api/entries/bulk', {
            event_ids: eventIds,
            project_id: projectId,
        });
    },

    /**
     * Get all projects.
     */
    async getProjects() {
        return this.get('/api/projects');
    },

    /**
     * Create a project.
     */
    async createProject(name, client) {
        return this.post('/api/projects', { name, client });
    },

    /**
     * Update project visibility.
     */
    async setProjectVisibility(projectId, isVisible) {
        return this.put(`/api/projects/${projectId}/visibility`, { is_visible: isVisible });
    },
};
