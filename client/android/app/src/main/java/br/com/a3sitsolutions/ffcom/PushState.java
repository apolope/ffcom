package br.com.a3sitsolutions.ffcom;

// O que está na tela agora, para o push não avisar do que a pessoa já está
// vendo. O server-channel já não notifica quem está com o WebSocket do canal
// aberto, mas a DM e a amizade vêm do server-central, que não sabe qual
// aparelho está em uso (docs/architecture.md, "Decisão: notificações push
// (fase 6)", escopo aceito). foreground muda com o onResume/onPause da
// Activity (PushPlugin); activeKey vem da página (FfcomPush.setActive). Sem
// a Activity, o processo começa com foreground false e tudo notifica.
final class PushState {

    static volatile boolean foreground;
    // PushPayload.groupKey() do canal ou da DM aberta, ou null.
    static volatile String activeKey;

    private PushState() {}

    // Com o app na frente: o canal ou a DM aberta não notificam, e amizade
    // também não, porque a página já mostra o aviso no canto (vem pelo
    // WebSocket de presença).
    static boolean suppress(PushPayload payload, boolean foreground, String activeKey) {
        if (!foreground) return false;
        if (payload.isFriend()) return true;
        return activeKey != null && activeKey.equals(payload.groupKey());
    }

    static boolean suppress(PushPayload payload) {
        return suppress(payload, foreground, activeKey);
    }
}
