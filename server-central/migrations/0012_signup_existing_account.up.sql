-- Pedido de acesso de quem já tem conta no Authentik central (a instância é
-- compartilhada com outros projetos da infra A3S). Nesse caso a aprovação
-- não cria usuário: acha a conta pelo e-mail e só a põe em ffcom-users. Ver
-- docs/architecture.md, "Decisão: cadastro com aprovação pelo Telegram".
--
-- existing_account   a pessoa marcou "já tenho conta"; então não informa
--                    nome de usuário nem apelido (a conta já tem os dela)
-- email_accounts     contas do Authentik com o e-mail do pedido, conferidas
--                    no envio só para mostrar a quem aprova (a pessoa não
--                    recebe nada disso); vazio se não havia nenhuma, NULL
--                    se não deu para conferir
-- linked_existing    a aprovação usou uma conta que já existia em vez de
--                    criar uma
-- authentik_username nome de usuário da conta criada ou vinculada
ALTER TABLE signup_requests
    ADD COLUMN existing_account BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN email_accounts TEXT,
    ADD COLUMN linked_existing BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN authentik_username TEXT,
    ALTER COLUMN username DROP NOT NULL,
    ALTER COLUMN nickname DROP NOT NULL,
    ADD CONSTRAINT signup_requests_new_account_fields
        CHECK (existing_account OR (username IS NOT NULL AND nickname IS NOT NULL));
