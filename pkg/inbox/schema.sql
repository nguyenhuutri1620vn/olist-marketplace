-- Idempotent consumer inbox (AD-7). A consuming service applies this in its
-- own migrations; pkg/inbox's tests and the service's migration test keep the
-- copies identical. A row exists for every applied or rejected event, so a
-- redelivery of either is a duplicate.
CREATE TABLE processed_events (
    consumer     text NOT NULL,
    event_id     uuid NOT NULL,
    event_type   text NOT NULL,
    outcome      text NOT NULL CHECK (outcome IN ('applied', 'rejected')),
    reason       text NOT NULL DEFAULT '',
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
