ALTER TABLE audit_events ADD COLUMN event_key text;

UPDATE audit_events
SET event_key = 'legacy:' || id::text
WHERE event_key IS NULL;

ALTER TABLE audit_events ALTER COLUMN event_key SET NOT NULL;

CREATE UNIQUE INDEX audit_events_event_key_idx ON audit_events(event_key);
