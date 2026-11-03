CREATE TABLE users (
    id         BIGSERIAL PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE CHECK (email = lower(email)),
    name       TEXT NOT NULL,
    password   TEXT NOT NULL, -- bcrypt hash, never the plain password (AC-18)
    role       TEXT NOT NULL CHECK (role IN ('student', 'staff')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id_hash    BYTEA PRIMARY KEY, -- sha256 of the cookie token
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

CREATE TABLE labs (
    id          BIGSERIAL PRIMARY KEY,
    title       TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    starts_at   TIMESTAMPTZ NOT NULL,
    ends_at     TIMESTAMPTZ NOT NULL,
    place_limit INTEGER NOT NULL CHECK (place_limit >= 1),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);

CREATE TABLE bookings (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    lab_id     BIGINT NOT NULL REFERENCES labs (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, lab_id)
);
