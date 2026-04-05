CREATE TABLE rooms (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NULL,
    capacity INT NULL CHECK (capacity > 0),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
