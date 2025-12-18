/**
 * Client-side API library for Timesheet app.
 * Thin wrapper around fetch providing domain methods.
 */

/**
 * Calculate relative luminance of a hex color.
 * Returns a value between 0 (darkest) and 1 (lightest).
 */
function getLuminance(hexColor) {
    // Remove # if present
    const hex = hexColor.replace('#', '');

    // Parse RGB values
    const r = parseInt(hex.substr(0, 2), 16) / 255;
    const g = parseInt(hex.substr(2, 2), 16) / 255;
    const b = parseInt(hex.substr(4, 2), 16) / 255;

    // Apply sRGB to linear conversion
    const toLinear = (c) => c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);

    const rLin = toLinear(r);
    const gLin = toLinear(g);
    const bLin = toLinear(b);

    // Calculate relative luminance (WCAG formula)
    return 0.2126 * rLin + 0.7152 * gLin + 0.0722 * bLin;
}

/**
 * Determine if text should be dark based on background color luminance.
 * Returns true if dark text should be used (for light backgrounds).
 */
function shouldUseDarkText(hexColor) {
    if (!hexColor) return false;
    const luminance = getLuminance(hexColor);
    // Use dark text if luminance is above 0.5 (light background)
    return luminance > 0.5;
}

/**
 * Apply appropriate text color class to an element based on its background.
 */
function applyTextColorClass(element, bgColor) {
    if (shouldUseDarkText(bgColor)) {
        element.classList.add('dark-text');
    } else {
        element.classList.remove('dark-text');
    }
}

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

/**
 * Handle API response, redirecting to login on 401.
 */
async function handleResponse(response) {
    if (response.status === 401) {
        // Not authenticated - redirect to login
        window.location.href = '/login?next=' + encodeURIComponent(window.location.pathname);
        throw new Error('Not authenticated');
    }
    if (!response.ok) {
        const error = await response.json().catch(() => ({ detail: 'Request failed' }));
        throw new Error(error.detail || 'Request failed');
    }
    return response.json();
}

const api = {
    /**
     * Make a GET request.
     */
    async get(path) {
        const response = await fetch(path);
        return handleResponse(response);
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
        return handleResponse(response);
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
        return handleResponse(response);
    },

    /**
     * Make a DELETE request.
     */
    async delete(path) {
        const response = await fetch(path, { method: 'DELETE' });
        return handleResponse(response);
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

    /**
     * Apply rules to unclassified events.
     */
    async applyRules(startDate, endDate) {
        return this.post('/api/rules/apply', {
            start_date: startDate,
            end_date: endDate,
        });
    },
};
