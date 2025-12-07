-- Add columns for LLM-based classification
-- These columns support improved event classification by tracking
-- response status, calendar visibility, and busy/free time

-- User's RSVP status for the event (accepted, declined, needsAction, tentative)
ALTER TABLE events ADD COLUMN my_response_status TEXT;

-- Event transparency (opaque = busy, transparent = free)
ALTER TABLE events ADD COLUMN transparency TEXT;

-- Event visibility (default, public, private, confidential)
ALTER TABLE events ADD COLUMN visibility TEXT;
