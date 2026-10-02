ALTER TABLE users
    DROP CONSTRAINT users_preferred_locale_check;

ALTER TABLE users
    ADD CONSTRAINT users_preferred_locale_check
    CHECK (
        preferred_locale IS NULL
        OR (preferred_locale COLLATE "C") ~ '^[a-z]{2}$'
    );