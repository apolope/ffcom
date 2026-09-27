# Prompt para o agente do a3s-network-monitor: repassar os botões de cadastro do FFCom

Cole o texto abaixo numa sessão aberta em `D:\Dev\a3s-network`.

---

O FFCom (`D:\Dev\ffcom`) passou a mandar pedidos de cadastro ao grupo "Rede" do Telegram usando o mesmo bot do `a3s-network-monitor` (`@Monitoracao_Internet_bot`). O FFCom só chama `sendMessage` e `editMessageText`; o webhook continua sendo do monitor, e é por ele que chegam os cliques nos botões. Preciso que o monitor repasse ao FFCom os cliques dos botões dele, sem mexer no webhook nem nos fluxos atuais.

**O que chega:** `callback_query` com `callback_data` no formato `ffcom-signup:approve:<uuid>` ou `ffcom-signup:reject:<uuid>` (até 57 bytes). A mensagem está no tópico "FFCom" do mesmo `chat_id` já configurado.

**O que fazer em `TelegramWebhookController#handleCallbackQuery`:**

1. Novo prefixo `FFCOM_SIGNUP_PREFIX = "ffcom-signup:"`, despachado como os outros (`router-update:`, `router-script-apply:`, `wol:`).
2. Só repassar se `callback_query.message.chat.id` for o `chat_id` configurado (`a3s.monitor.telegram.chat-id`); fora disso, logar e responder o clique com "Não autorizado".
3. Responder o clique **na hora** com `answerCallbackQuery(id, "Processando...")`, antes de chamar o FFCom. A aprovação leva alguns segundos (cria o usuário no Authentik e dispara e-mail) e o Telegram reenvia o webhook que demora.
4. Em segundo plano (executor assíncrono, sem segurar a resposta 200 do webhook), fazer:
   ```
   POST {FFCOM_SIGNUP_DECISION_URL}/internal/signup-requests/{uuid}/decision
   X-FFCom-Secret: {FFCOM_SIGNUP_DECISION_SECRET}
   Content-Type: application/json

   {"decision": "approve" | "reject", "by": "<quem clicou>"}
   ```
   `by` vem de `callback_query.from`: `@username` quando houver, senão `first_name` (+ `last_name`). Timeout de 60 s.
5. **Não editar a mensagem nem tirar os botões:** o FFCom edita a própria mensagem com o resultado (aprovado/reprovado sem botões, ou o motivo da falha mantendo os botões para tentar de novo). A resposta é `200 {"ok": bool, "message": "..."}`; basta logar. Se a chamada falhar (rede, 401, 5xx), logar em `warn`; a mensagem continua com os botões e dá para clicar de novo, porque o FFCom é idempotente (um segundo clique num pedido já decidido não faz nada).

**Configuração nova** (no `.env` do monitor, fora do git):

- `FFCOM_SIGNUP_DECISION_URL=http://ffcom-central-app:8090` (o container `ffcom-central-app` está na rede overlay `a3s-services`, a mesma do monitor; a porta 8090 é o listener interno do FFCom, que o NPM não encaminha. Já conferido em 2026-09-27: `docker exec a3s-network-monitor curl http://ffcom-central-app:8090/internal/signup-requests/<id>/decision` resolve o nome e responde.)
- `FFCOM_SIGNUP_DECISION_SECRET=<o mesmo valor de SIGNUP_DECISION_SECRET em /opt/ffcom/envs/ffcom-central.env>` (já gerado e gravado no env do FFCom, no mesmo host; copie de lá sem imprimir no log). O cadastro já está ligado no FFCom e os pedidos já chegam ao tópico "FFCom" (`message_thread_id` 2448); só falta este repasse para os botões funcionarem.

Sem `FFCOM_SIGNUP_DECISION_URL` configurado, o prefixo deve responder o clique com "Cadastro do FFCom não configurado" e não quebrar nada.

**Não fazer:** `setWebhook`, `deleteWebhook` ou `getUpdates`; mudar o comportamento dos prefixos existentes.

**Teste antes do deploy:** um teste do controller com `callback_data` `ffcom-signup:approve:<uuid>` verificando que o `answerCallbackQuery` sai antes da chamada ao FFCom e que o POST leva o header e o corpo acima; e um com `chat.id` diferente, que não deve repassar. Contrato completo em `D:\Dev\ffcom\docs\protocol.md` (seção da rota de decisão) e o porquê em `D:\Dev\ffcom\docs\architecture.md`, "Decisão: cadastro com aprovação pelo Telegram".
