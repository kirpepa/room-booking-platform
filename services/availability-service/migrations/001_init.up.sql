CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE schedules (
    id UUID PRIMARY KEY,
    room_id UUID NOT NULL UNIQUE,
    days_of_week SMALLINT[] NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    slot_duration_minutes SMALLINT NOT NULL DEFAULT 30 CHECK (slot_duration_minutes = 30),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (end_time > start_time)
);

CREATE TABLE slots (
    id UUID PRIMARY KEY,
    room_id UUID NOT NULL,
    schedule_id UUID NOT NULL REFERENCES schedules(id),
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'free' CHECK (status IN ('free', 'booked')),
    current_booking_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (end_at > start_at),
    CHECK (end_at = start_at + INTERVAL '30 minutes'),
    UNIQUE (room_id, start_at),
    EXCLUDE USING gist (
        room_id WITH =,
        tstzrange(start_at, end_at) WITH &&
    )
);

CREATE INDEX idx_slots_room_start ON slots (room_id, start_at);
CREATE INDEX idx_slots_free ON slots (room_id, start_at) WHERE status = 'free';

CREATE TABLE slot_generation_state (
    room_id UUID PRIMARY KEY,
    generated_until DATE NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
