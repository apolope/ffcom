package br.com.a3sitsolutions.ffcom;

import android.content.Context;
import android.util.AttributeSet;
import android.view.View;
import com.getcapacitor.CapacitorWebView;

// A WebView do app (trocada no layout capacitor_bridge_layout_main, que
// sobrescreve o do Capacitor) com uma única diferença: durante uma chamada a
// página continua "visível" para o Chromium mesmo com a tela apagada ou com
// outro app na frente.
//
// Motivo: no Android o Blink congela a página escondida e sem áudio tocando
// um minuto depois de ela sumir (feature "stop-in-background", ligada por
// padrão no Android; PageSchedulerImpl::UpdateFrozenState). Congelada, nenhum
// timer, evento de WebSocket ou de RTCPeerConnection roda; numa sala em
// silêncio com a tela apagada, a troca de Wi-Fi para dados móveis passava
// sem o livekit-client perceber, o servidor derrubava a pessoa e nada
// reconectava até a tela acender. Ter WebRTC ativo não impede o
// congelamento (só a limitação intensiva de timers). Quem decide que a página
// está escondida é o AwContents, a partir de onWindowVisibilityChanged; aqui
// ele recebe VISIBLE enquanto houver chamada. Ver docs/architecture.md,
// "Decisão: chamada em segundo plano no Android (fase 5)".
public class CallWebView extends CapacitorWebView {

    private boolean inCall;

    public CallWebView(Context context, AttributeSet attrs) {
        super(context, attrs);
    }

    // Liga e desliga a visibilidade forçada. Desligar devolve ao Chromium a
    // visibilidade real da janela (tela apagada: GONE, e a página volta a
    // poder congelar como antes). Só na thread principal.
    void setInCall(boolean inCall) {
        if (this.inCall == inCall) return;
        this.inCall = inCall;
        super.onWindowVisibilityChanged(inCall ? View.VISIBLE : getWindowVisibility());
    }

    @Override
    protected void onWindowVisibilityChanged(int visibility) {
        super.onWindowVisibilityChanged(inCall ? View.VISIBLE : visibility);
    }
}
