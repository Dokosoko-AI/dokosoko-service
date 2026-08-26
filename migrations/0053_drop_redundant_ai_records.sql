-- AI work executes synchronously and is already represented by durable domain
-- records, audit events, and provider-attempt usage events. The generic
-- analytics stream duplicated the latter and has no remaining producer.

DROP TABLE ai_jobs;
DROP TABLE analytics_events;
DROP FUNCTION prevent_analytics_mutation();
