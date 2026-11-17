-- A lab is scheduled as sessions (Slice 3b). A lab keeps its title, description, place limit and
-- owner; each session has its own time period, and bookings belong to a session.

-- The overlap rule below needs = on a bigint inside a GiST index.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE lab_sessions (
    id           BIGSERIAL PRIMARY KEY,
    lab_id       BIGINT NOT NULL REFERENCES labs (id) ON DELETE CASCADE,
    starts_at    TIMESTAMPTZ NOT NULL,
    ends_at      TIMESTAMPTZ NOT NULL,
    -- Set when the owner cancels the session (Slice 3c). A cancelled session no longer blocks its time.
    cancelled_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    -- Two live sessions of one lab may not overlap (AC-69). The database decides, not a prior lookup,
    -- so two owners adding sessions at once cannot both succeed.
    CONSTRAINT lab_sessions_no_overlap
        EXCLUDE USING gist (lab_id WITH =, tstzrange(starts_at, ends_at) WITH &&) WHERE (cancelled_at IS NULL)
);

CREATE INDEX lab_sessions_lab_id_idx ON lab_sessions (lab_id);
-- The lab list looks sessions up by day.
CREATE INDEX lab_sessions_starts_at_idx ON lab_sessions (starts_at);

-- Each existing lab becomes one session with its old time period (AC-66).
INSERT INTO lab_sessions (lab_id, starts_at, ends_at)
SELECT id, starts_at, ends_at FROM labs;

-- Bookings move from the lab to its session. RESTRICT: a session with bookings cannot be deleted, so
-- deleting a booked lab fails instead of silently dropping bookings (Slice 3c, AC-78).
ALTER TABLE bookings ADD COLUMN session_id BIGINT REFERENCES lab_sessions (id) ON DELETE RESTRICT;
UPDATE bookings b SET session_id = s.id FROM lab_sessions s WHERE s.lab_id = b.lab_id;
ALTER TABLE bookings ALTER COLUMN session_id SET NOT NULL;

ALTER TABLE bookings DROP CONSTRAINT bookings_user_id_lab_id_key;
ALTER TABLE bookings ADD CONSTRAINT bookings_user_id_session_id_key UNIQUE (user_id, session_id);
DROP INDEX bookings_lab_id_idx;
CREATE INDEX bookings_session_id_idx ON bookings (session_id);
ALTER TABLE bookings DROP COLUMN lab_id;

-- The time period now lives on the sessions. Dropping the columns drops their CHECK too.
ALTER TABLE labs DROP COLUMN starts_at, DROP COLUMN ends_at;
