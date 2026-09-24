-- Status escolhido pela pessoa (ver docs/architecture.md, "Decisão: status de
-- presença e avatar nas listas de membros"). O status que os amigos veem é
-- derivado deste mais as conexões abertas: sem conexão ou "invisible" vira
-- offline, e "online" vira "away" quando todas as conexões estão ociosas.
ALTER TABLE accounts
    ADD COLUMN presence_status TEXT NOT NULL DEFAULT 'online'
        CONSTRAINT accounts_presence_status_valid CHECK (presence_status IN ('online', 'busy', 'away', 'invisible'));
