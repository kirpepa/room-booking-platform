ALTER TABLE conference_jobs
    ADD COLUMN locked_at TIMESTAMPTZ NULL;

ALTER TABLE conference_jobs
    ADD CONSTRAINT conference_jobs_booking_fk
    FOREIGN KEY (booking_id) REFERENCES bookings(id) ON DELETE CASCADE;

CREATE INDEX idx_conference_jobs_pending_claim
    ON conference_jobs (next_retry_at, created_at)
    WHERE status = 'pending';

CREATE INDEX idx_conference_jobs_processing_lease
    ON conference_jobs (locked_at)
    WHERE status = 'processing';
