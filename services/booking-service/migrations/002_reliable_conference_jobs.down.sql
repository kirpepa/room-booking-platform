DROP INDEX IF EXISTS idx_conference_jobs_processing_lease;
DROP INDEX IF EXISTS idx_conference_jobs_pending_claim;

ALTER TABLE conference_jobs
    DROP CONSTRAINT IF EXISTS conference_jobs_booking_fk;

ALTER TABLE conference_jobs
    DROP COLUMN IF EXISTS locked_at;
