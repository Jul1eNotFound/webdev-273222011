-- Every lab has an owner: the staff member who added it (Slice 3a, AC-60).
-- That the owner is staff is checked by the service, which lets only staff add labs.
ALTER TABLE labs ADD COLUMN owner_id BIGINT REFERENCES users (id) ON DELETE RESTRICT;

-- Labs from before owners existed go to the first staff account.
UPDATE labs SET owner_id = (SELECT id FROM users WHERE role = 'staff' ORDER BY id LIMIT 1);

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM labs WHERE owner_id IS NULL) THEN
        RAISE EXCEPTION 'labs exist but there is no staff account to own them: add a staff account first';
    END IF;
END $$;

ALTER TABLE labs ALTER COLUMN owner_id SET NOT NULL;

-- "My labs" looks labs up by owner (AC-62).
CREATE INDEX labs_owner_id_idx ON labs (owner_id);
