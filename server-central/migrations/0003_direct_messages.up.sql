-- Mensagens diretas (DM): server-central medeia diretamente, sem
-- server-channel dedicado por par de amigos (ver docs/architecture.md,
-- "Decisão: modelo de DMs"). sender_id/recipient_id é direcionado (quem
-- mandou vs. quem recebeu), mas a conversa entre duas contas é lida nos
-- dois sentidos — o índice cobre isso via LEAST/GREATEST do par.
CREATE TABLE direct_messages (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sender_id     UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    recipient_id  UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    content       TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at     TIMESTAMPTZ,
    CHECK (sender_id <> recipient_id)
);

CREATE INDEX direct_messages_conversation_idx
    ON direct_messages (LEAST(sender_id, recipient_id), GREATEST(sender_id, recipient_id), created_at DESC);
