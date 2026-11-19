CREATE TABLE outbox_messages (
    id UUID PRIMARY KEY,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX outbox_messages_pending_idx
    ON outbox_messages (created_at, id)
    WHERE published_at IS NULL;
