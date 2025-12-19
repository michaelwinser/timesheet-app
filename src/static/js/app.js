/**
 * Week view interactions.
 * Note: getLuminance, shouldUseDarkText, applyTextColorClass are defined in api.js
 */

/**
 * Sync events quietly (no alert, for auto-sync on page load).
 * Reloads the page once if new events were fetched.
 */
async function syncEventsQuiet() {
    // Prevent reload loops - only reload once per page load
    if (sessionStorage.getItem('justSynced') === window.location.pathname) {
        sessionStorage.removeItem('justSynced');
        return;
    }

    try {
        const result = await api.syncEvents(window.weekStart, window.weekEnd);
        // Reload if new events were fetched to display them
        if (result.events_new > 0) {
            sessionStorage.setItem('justSynced', window.location.pathname);
            location.reload();
        }
    } catch (error) {
        console.error('Auto-sync failed:', error.message);
    }
}

/**
 * Refresh events (manual sync with reload).
 */
async function refreshEvents() {
    const btn = document.getElementById('sync-btn');
    btn.disabled = true;
    btn.textContent = 'Refreshing...';

    try {
        await api.syncEvents(window.weekStart, window.weekEnd);
        location.reload();
    } catch (error) {
        alert('Refresh failed: ' + error.message);
        btn.disabled = false;
        btn.textContent = '↻ Refresh';
    }
}

/**
 * Classify an event with a project.
 */
async function classifyEvent(eventId, projectId, projectColor) {
    if (!projectId) return;

    const card = document.querySelector(`[data-event-id="${eventId}"]`);
    const eventSide = card.querySelector('.card-event');
    const entrySide = card.querySelector('.card-entry');

    try {
        // Get event times to calculate hours
        const events = await api.getEvents(window.weekStart, window.weekEnd);
        const event = events.find(e => e.id === eventId);

        if (!event) {
            throw new Error('Event not found');
        }

        const start = new Date(event.start_time);
        const end = new Date(event.end_time);
        const hours = (end - start) / (1000 * 60 * 60);

        const entry = await api.classifyEvent(eventId, parseInt(projectId), hours, event.title);

        // Update card UI
        card.classList.remove('unclassified');
        card.classList.add('classified');
        card.dataset.project = entry.project_name;

        // Update entry side
        entrySide.dataset.entryId = entry.id;
        entrySide.style.backgroundColor = projectColor || '#00aa44';
        applyTextColorClass(entrySide, projectColor || '#00aa44');

        // Update project select
        const projectSelect = entrySide.querySelector('.card-row-project select');
        if (projectSelect) {
            projectSelect.value = entry.project_id;
        }

        // Update duration input
        const durationInput = entrySide.querySelector('.duration-input');
        if (durationInput) {
            durationInput.value = entry.hours.toFixed(2);
            durationInput.dataset.entryId = entry.id;
        }

        // Add dog-ear to event side if not present
        if (!eventSide.querySelector('.dog-ear')) {
            const dogEar = document.createElement('div');
            dogEar.className = 'dog-ear';
            dogEar.title = 'Flip to time entry';
            dogEar.onclick = () => flipCard(eventId);
            dogEar.innerHTML = `
                <svg class="flip-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                    <path d="M17 1l4 4-4 4"></path>
                    <path d="M3 11V9a4 4 0 0 1 4-4h14"></path>
                    <path d="M7 23l-4-4 4-4"></path>
                    <path d="M21 13v2a4 4 0 0 1-4 4H3"></path>
                </svg>
            `;
            eventSide.appendChild(dogEar);
        }
        eventSide.dataset.projectColor = projectColor || '#00aa44';

        // Flip to entry side
        eventSide.classList.add('hidden');
        entrySide.classList.remove('hidden');

        // Update sidebar totals
        updateProjectSummary();

    } catch (error) {
        alert('Classification failed: ' + error.message);
        // Reset dropdown
        const select = card.querySelector('.card-row-project select');
        if (select) select.value = '';
    }
}

/**
 * Reclassify or unclassify an event from the time entry side.
 */
async function reclassifyEvent(eventId, entryId, projectId, projectColor) {
    const card = document.querySelector(`[data-event-id="${eventId}"]`);
    const eventSide = card.querySelector('.card-event');
    const entrySide = card.querySelector('.card-entry');

    if (!projectId) {
        // Unclassify
        try {
            await api.unclassify(entryId);

            card.classList.remove('classified');
            card.classList.add('unclassified');
            card.dataset.project = '';

            entrySide.classList.add('hidden');
            eventSide.classList.remove('hidden');

            // Reset event side dropdown
            const eventSelect = eventSide.querySelector('.card-row-project select');
            if (eventSelect) eventSelect.value = '';

            // Remove dog-ear from event side
            const dogEar = eventSide.querySelector('.dog-ear');
            if (dogEar) dogEar.remove();
            eventSide.dataset.projectColor = '';

            // Update sidebar totals
            updateProjectSummary();

        } catch (error) {
            alert('Unclassify failed: ' + error.message);
            // Reload to reset state
            location.reload();
        }
    } else {
        // Reclassify to different project
        try {
            const entry = await api.updateEntry(entryId, { project_id: parseInt(projectId) });
            card.dataset.project = entry.project_name;
            entrySide.style.backgroundColor = projectColor || '#00aa44';
            applyTextColorClass(entrySide, projectColor || '#00aa44');
            eventSide.dataset.projectColor = projectColor || '#00aa44';

            // Update sidebar totals
            updateProjectSummary();
        } catch (error) {
            alert('Reclassify failed: ' + error.message);
            location.reload();
        }
    }
}

/**
 * Flip a card between event and entry views.
 */
function flipCard(eventId) {
    const card = document.querySelector(`[data-event-id="${eventId}"]`);
    const eventSide = card.querySelector('.card-event');
    const entrySide = card.querySelector('.card-entry');

    eventSide.classList.toggle('hidden');
    entrySide.classList.toggle('hidden');
}

/**
 * Round up hours to nearest 15 minutes.
 */
async function roundUp(eventId, entryId) {
    const card = document.querySelector(`[data-event-id="${eventId}"]`);
    const entrySide = card.querySelector('.card-entry');
    const durationInput = entrySide.querySelector('.duration-input');
    const currentHours = parseFloat(durationInput.value);
    const roundedHours = Math.ceil(currentHours * 4) / 4; // Round to nearest 0.25

    if (roundedHours === currentHours) {
        // Already rounded, add 0.25
        const newHours = currentHours + 0.25;
        try {
            await api.updateEntry(entryId, { hours: newHours });
            durationInput.value = newHours.toFixed(2);
            updateProjectSummary();
        } catch (error) {
            alert('Update failed: ' + error.message);
        }
    } else {
        try {
            await api.updateEntry(entryId, { hours: roundedHours });
            durationInput.value = roundedHours.toFixed(2);
            updateProjectSummary();
        } catch (error) {
            alert('Update failed: ' + error.message);
        }
    }
}

/**
 * Update hours directly from duration input.
 */
async function updateHours(entryId, value) {
    const hours = parseFloat(value);
    if (isNaN(hours) || hours < 0) {
        alert('Invalid hours value');
        return;
    }

    try {
        await api.updateEntry(entryId, { hours: hours });
        updateProjectSummary();
    } catch (error) {
        alert('Update failed: ' + error.message);
    }
}

/**
 * Filter events by text, DNA status, and project visibility.
 */
function filterEvents() {
    const filter = document.getElementById('filter-input').value.toLowerCase();
    const showDna = document.getElementById('show-dna-checkbox')?.checked ?? false;
    const cards = document.querySelectorAll('.event-card');

    // Build set of visible project names based on sidebar checkboxes
    const visibleProjects = new Set();
    document.querySelectorAll('.summary-item[data-project-name]').forEach(item => {
        const checkbox = item.querySelector('input[type="checkbox"]');
        if (checkbox && checkbox.checked) {
            visibleProjects.add(item.dataset.projectName);
        }
    });

    cards.forEach(card => {
        const title = (card.dataset.title || '').toLowerCase();
        const project = (card.dataset.project || '').toLowerCase();
        const projectName = card.dataset.project || '';
        const isDna = card.dataset.didNotAttend === 'true';

        // Check text filter
        const matchesText = title.includes(filter) || project.includes(filter);

        // Check DNA filter (hide DNA events unless checkbox is checked)
        const matchesDna = showDna || !isDna;

        // Check project visibility (unclassified events always pass this check)
        const matchesProject = !projectName || visibleProjects.has(projectName);

        card.style.display = (matchesText && matchesDna && matchesProject) ? '' : 'none';
    });
}

/**
 * Update the project summary sidebar with current hours from all cards.
 */
function updateProjectSummary() {
    const projectHours = {};
    let unclassifiedHours = 0;

    // Collect hours from all classified cards (excluding DNA)
    document.querySelectorAll('.event-card.classified:not(.did-not-attend)').forEach(card => {
        const projectName = card.dataset.project;
        if (!projectName) return;

        const durationInput = card.querySelector('.duration-input');
        if (!durationInput) return;

        const hours = parseFloat(durationInput.value) || 0;
        projectHours[projectName] = (projectHours[projectName] || 0) + hours;
    });

    // Collect hours from unclassified cards (excluding DNA)
    document.querySelectorAll('.event-card.unclassified:not(.did-not-attend)').forEach(card => {
        const duration = parseFloat(card.dataset.duration) || 0;
        unclassifiedHours += duration;
    });

    // Calculate total hours (excluding "no hrs" projects)
    let totalHours = 0;

    // Update each project's hours in the sidebar
    document.querySelectorAll('.summary-item').forEach(item => {
        const nameEl = item.querySelector('.summary-name');
        if (!nameEl) return;

        // Get project name from data attribute
        const projectName = item.dataset.projectName;
        if (!projectName) return;
        const hours = projectHours[projectName] || 0;

        // Update hours display
        const hoursEl = item.querySelector('.summary-hours');
        if (hoursEl) {
            hoursEl.textContent = hours.toFixed(1);
        }

        // Check if this project accumulates hours
        const noHrsBadge = nameEl.querySelector('.badge-tiny');
        if (!noHrsBadge) {
            totalHours += hours;
        }

        // Update progress bar if present
        const barFill = item.querySelector('.summary-bar-fill');
        if (barFill) {
            // We'll update the width after calculating total
            barFill.dataset.hours = hours;
        }
    });

    // Update unclassified total
    const unclassifiedEl = document.getElementById('unclassified-total');
    if (unclassifiedEl) {
        unclassifiedEl.textContent = unclassifiedHours.toFixed(1) + ' hrs';
    }

    // Show/hide unclassified section based on whether there are unclassified hours
    const unclassifiedSection = document.getElementById('unclassified-section');
    if (unclassifiedSection) {
        unclassifiedSection.style.display = unclassifiedHours > 0 ? '' : 'none';
    }

    // Update total display
    const totalEl = document.querySelector('.summary-total');
    if (totalEl) {
        totalEl.textContent = totalHours.toFixed(1) + ' hrs total';
    }

    // Update progress bar widths now that we have the total
    document.querySelectorAll('.summary-bar-fill').forEach(barFill => {
        const hours = parseFloat(barFill.dataset.hours) || 0;
        const width = totalHours > 0 ? (hours / totalHours * 100) : 0;
        barFill.style.width = width + '%';
    });
}

/**
 * Toggle project visibility in the week view (client-side filter only).
 * This does NOT affect the database or rule matching - it's purely visual.
 */
function toggleProjectVisibility(projectId, isVisible) {
    // Store visibility preferences in sessionStorage for this week
    const visibilityKey = `projectVisibility_${window.weekStart}`;
    let visibility = JSON.parse(sessionStorage.getItem(visibilityKey) || '{}');
    visibility[projectId] = isVisible;
    sessionStorage.setItem(visibilityKey, JSON.stringify(visibility));

    // Reapply all filters (text, DNA, project visibility)
    filterEvents();
}

/**
 * Initialize project visibility based on checkbox states.
 * Called on page load to hide events for unchecked projects.
 * Now delegates to filterEvents() which handles all filtering.
 */
function initProjectVisibility() {
    // filterEvents() now handles project visibility along with text/DNA filters
    // This function exists for backwards compatibility with restoreProjectVisibility()
}
