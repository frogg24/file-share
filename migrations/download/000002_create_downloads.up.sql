CREATE TABLE downloads (
    id UUID PRIMARY KEY,
    file_id UUID NOT NULL REFERENCES files(id),
    user_id UUID,
    downloaded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
