CREATE TABLE dialogs (
    id         bigserial PRIMARY KEY,
    user_id    bigint NOT NULL,
    title      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_dialogs_user_id ON dialogs (user_id);
