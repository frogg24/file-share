CREATE TABLE files(
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL,
    original_name TEXT NOT NULL,
    size BIGINT NOT NULL,
    content BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,

    CHECK (size >= 0),
    CHECK (size = octet_length(content))
);