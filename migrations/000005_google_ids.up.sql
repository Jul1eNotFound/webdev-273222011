-- A user may link one Google account (Slice 6). Matched on Google's account id, never on the email
-- alone (AC-20, AC-53). The unique constraint lets one Google account link to at most one row, so two
-- sign-ins racing cannot both link it.
ALTER TABLE users ADD COLUMN google_id TEXT UNIQUE;
