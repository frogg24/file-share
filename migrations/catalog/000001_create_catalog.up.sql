CREATE TABLE users (
    id UUID PRIMARY KEY,
    display_name TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE files (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL,
    original_name TEXT NOT NULL,
    size BIGINT NOT NULL CHECK (size >= 0),
    created_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ,
    download_count BIGINT NOT NULL DEFAULT 0 CHECK (download_count >= 0)
);

CREATE TABLE downloads (
    id UUID PRIMARY KEY,
    file_id UUID NOT NULL,
    user_id UUID,
    downloaded_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX downloads_file_id_idx ON downloads (file_id);
