-- Idioma da interface escolhido pela pessoa (PATCH /api/me), para valer em
-- todos os dispositivos. NULL é "nunca escolheu": o client segue o
-- localStorage e o idioma do navegador. Os valores são os arquivos de
-- locales/ na raiz; idioma novo precisa entrar neste CHECK e em
-- store.SupportedLanguages. Ver docs/architecture.md, "Decisão:
-- internacionalização".
ALTER TABLE accounts
    ADD COLUMN language TEXT
        CONSTRAINT accounts_language_valid CHECK (language IN ('pt-BR', 'en'));
