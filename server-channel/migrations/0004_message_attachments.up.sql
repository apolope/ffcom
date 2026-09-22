-- Upload de anexo/imagem em mensagem de canal de texto (ver
-- docs/architecture.md, "Decisão: upload de anexo em mensagem"). Modelada
-- como 1:N desde já (message_id não é UNIQUE) mesmo a v1 só permitindo um
-- anexo por mensagem, para não exigir migration nova se "vários anexos por
-- mensagem" virar demanda real depois.
CREATE TABLE attachments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id    UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    filename      TEXT NOT NULL,
    content_type  TEXT NOT NULL,
    size_bytes    BIGINT NOT NULL,
    -- storage_key identifica o arquivo em internal/storage.FileStore
    -- (disco local do host, ver docs/architecture.md) -- nunca é o filename
    -- original enviado pelo client.
    storage_key   TEXT NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX attachments_message_idx ON attachments(message_id);
