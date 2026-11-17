-- Back to one time period per lab. Only possible while every lab has exactly one session: otherwise
-- going down would lose sessions or leave a lab without a time, so it stops and changes nothing.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM labs l WHERE (SELECT count(*) FROM lab_sessions s WHERE s.lab_id = l.id) <> 1) THEN
        RAISE EXCEPTION 'cannot migrate down: every lab must have exactly one session';
    END IF;
END $$;

ALTER TABLE labs ADD COLUMN starts_at TIMESTAMPTZ, ADD COLUMN ends_at TIMESTAMPTZ;
UPDATE labs l SET starts_at = s.starts_at, ends_at = s.ends_at FROM lab_sessions s WHERE s.lab_id = l.id;
ALTER TABLE labs ALTER COLUMN starts_at SET NOT NULL, ALTER COLUMN ends_at SET NOT NULL;
ALTER TABLE labs ADD CHECK (ends_at > starts_at);

ALTER TABLE bookings ADD COLUMN lab_id BIGINT REFERENCES labs (id) ON DELETE CASCADE;
UPDATE bookings b SET lab_id = s.lab_id FROM lab_sessions s WHERE s.id = b.session_id;
ALTER TABLE bookings ALTER COLUMN lab_id SET NOT NULL;
ALTER TABLE bookings DROP CONSTRAINT bookings_user_id_session_id_key;
ALTER TABLE bookings ADD CONSTRAINT bookings_user_id_lab_id_key UNIQUE (user_id, lab_id);
DROP INDEX bookings_session_id_idx;
CREATE INDEX bookings_lab_id_idx ON bookings (lab_id);
ALTER TABLE bookings DROP COLUMN session_id;

DROP TABLE lab_sessions;
-- btree_gist stays installed: other objects may use it, and it is harmless.
