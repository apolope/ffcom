-- Grant de notificação push de cada membro: o token que o app pediu ao
-- server-central para este servidor e entregou aqui (PUT /api/me/push-grant),
-- junto com o endereço deste servidor como o app o conhece (o mesmo do
-- known_servers da conta, ao qual o grant está preso no server-central).
-- Um por membro: o grant vale para todos os aparelhos da conta. Sai num
-- kick ou ban (MemberStore.Kick). Ver docs/architecture.md, "Decisão:
-- notificações push (fase 6)".
CREATE TABLE member_push_grants (
    member_id      UUID PRIMARY KEY REFERENCES members(id) ON DELETE CASCADE,
    token          TEXT NOT NULL,
    server_address TEXT NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
