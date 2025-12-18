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
        } catch (error) {
            alert('Update failed: ' + error.message);
        }
    } else {
        try {
            await api.updateEntry(entryId, { hours: roundedHours });
            durationInput.value = roundedHours.toFixed(2);
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
    } catch (error) {
        alert('Update failed: ' + error.message);
    }
}

/**
 * Filter events by text.
 */
function filterEvents() {
    const filter = document.getElementById('filter-input').value.toLowerCase();
    const cards = document.querySelectorAll('.event-card');

    cards.forEach(card => {
        const title = (card.dataset.title || '').toLowerCase();
        const project = (card.dataset.project || '').toLowerCase();
        const matches = title.includes(filter) || project.includes(filter);
        card.style.display = matches ? '' : 'none';
    });
}

/**
 * Toggle project visibility in the week view (client-side filter only).
 * This does NOT affect the database or rule matching - it's purely visual.
 */
function toggleProjectVisibility(projectId, isVisible) {
    // Get project name from the sidebar item
    const summaryItem = document.querySelector(`.summary-item[data-project-id="${projectId}"]`);
    const projectName = summaryItem ? summaryItem.querySelector('.summary-name').textContent : '';

    // Update event card visibility based on project
    const cards = document.querySelectorAll('.event-card');
    cards.forEach(card => {
        const cardProject = card.dataset.project || '';
        if (cardProject === projectName) {
            card.style.display = isVisible ? '' : 'none';
        }
    });

    // Store visibility preferences in sessionStorage for this week
    const visibilityKey = `projectVisibility_${window.weekStart}`;
    let visibility = JSON.parse(sessionStorage.getItem(visibilityKey) || '{}');
    visibility[projectId] = isVisible;
    sessionStorage.setItem(visibilityKey, JSON.stringify(visibility));
}
