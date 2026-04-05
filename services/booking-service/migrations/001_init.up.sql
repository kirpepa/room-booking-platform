CREATE TABLE bookings (
    id UUID PRIMARY KEY,
    slot_id UUID NOT NULL,
    user_id UUID NOT NULL,
    room_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cancelled')),
    conference_link TEXT NULL,
    conference_requested BOOLEAN NOT NULL DEFAULT FALSE,
    conference_status TEXT NOT NULL DEFAULT 'not_requested' CHECK (conference_status IN ('not_requested', 'pending', 'ready', 'failed')),
    slot_start_at TIMESTAMPTZ NOT NULL,
    slot_end_at TIMESTAMPTZ NOT NULL,
    cancelled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_bookings_active_slot ON bookings (slot_id) WHERE status = 'active';
CREATE INDEX idx_bookings_user_start ON bookings (user_id, slot_start_at);
CREATE INDEX idx_bookings_status_created ON bookings (status, created_at);
CREATE INDEX idx_bookings_room_start ON bookings (room_id, slot_start_at);

CREATE TABLE conference_jobs (
    booking_id UUID PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'done', 'failed')),
    attempts INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NULL,
    last_error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
