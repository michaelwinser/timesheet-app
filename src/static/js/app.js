/**
 * Week view interactions.
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
        const projectSelect = entrySide.querySelector('.entry-project-select');
        projectSelect.value = entry.project_id;
        entrySide.querySelector('.hours-value').textContent = entry.hours.toFixed(2);
        entrySide.querySelector('.entry-description').textContent = entry.description || event.title;

        // Add flip button to event side if not present
        const eventActions = eventSide.querySelector('.event-actions');
        if (!eventActions.querySelector('.btn-small')) {
            const flipBtn = document.createElement('button');
            flipBtn.className = 'btn-small';
            flipBtn.textContent = 'Flip';
            flipBtn.onclick = () => flipCard(eventId);
            eventActions.appendChild(flipBtn);
        }

        // Flip to entry side
        eventSide.classList.add('hidden');
        entrySide.classList.remove('hidden');

    } catch (error) {
        alert('Classification failed: ' + error.message);
        // Reset dropdown
        card.querySelector('.project-select').value = '';
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
            eventSide.querySelector('.project-select').value = '';

            // Remove flip button from event side
            const flipBtn = eventSide.querySelector('.event-actions .btn-small');
            if (flipBtn) flipBtn.remove();

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
    const hoursSpan = entrySide.querySelector('.hours-value');
    const currentHours = parseFloat(hoursSpan.textContent);
    const roundedHours = Math.ceil(currentHours * 4) / 4; // Round to nearest 0.25

    if (roundedHours === currentHours) {
        // Already rounded, add 0.25
        const newHours = currentHours + 0.25;
        try {
            await api.updateEntry(entryId, { hours: newHours });
            hoursSpan.textContent = newHours.toFixed(2);
        } catch (error) {
            alert('Update failed: ' + error.message);
        }
    } else {
        try {
            await api.updateEntry(entryId, { hours: roundedHours });
            hoursSpan.textContent = roundedHours.toFixed(2);
        } catch (error) {
            alert('Update failed: ' + error.message);
        }
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
