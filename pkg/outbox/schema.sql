-- Transactional outbox (AD-6). A publishing service applies this in its own
-- migrations; pkg/outbox's tests and the service's migration test keep the
-- copies identical.
CREATE TABLE outbox (
    seq         bigserial PRIMARY KEY,
    event_id    uuid NOT NULL UNIQUE,
    topic       text NOT NULL,
    envelope    jsonb NOT NULL,
    traceparent text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    sent_at     timestamptz
);

CREATE INDEX outbox_unsent_idx ON outbox (seq) WHERE sent_at IS NULL;

-- The relay takes this one row's lock each tick, so a second instance skips
-- instead of publishing later rows in parallel.
CREATE TABLE outbox_relay_lock (id int PRIMARY KEY CHECK (id = 1));
INSERT INTO outbox_relay_lock (id) VALUES (1);
