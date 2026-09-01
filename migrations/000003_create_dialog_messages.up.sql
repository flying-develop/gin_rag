CREATE TABLE dialog_messages (
    id         bigserial PRIMARY KEY,
    dialog_id  bigint NOT NULL REFERENCES dialogs (id) ON DELETE CASCADE,
    role       text NOT NULL,
    content    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_dialog_messages_dialog_id ON dialog_messages (dialog_id, id);
