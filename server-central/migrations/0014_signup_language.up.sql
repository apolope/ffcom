-- Idioma em que a pessoa fez o pedido de cadastro no site (pt-BR ou en).
-- Na aprovação vira o settings.locale do usuário criado no Authentik, para
-- o e-mail de definir senha e as telas do Authentik saírem nesse idioma.
-- NULL nos pedidos antigos e quando o site não mandou um idioma suportado.
-- Mesmos valores de accounts.language (0013). Ver docs/architecture.md,
-- "Decisão: internacionalização".
ALTER TABLE signup_requests
    ADD COLUMN language TEXT
        CONSTRAINT signup_requests_language_valid CHECK (language IN ('pt-BR', 'en'));
