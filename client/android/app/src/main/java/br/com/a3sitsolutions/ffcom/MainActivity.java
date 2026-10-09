package br.com.a3sitsolutions.ffcom;

import android.os.Bundle;
import android.webkit.WebView;
import com.getcapacitor.BridgeActivity;

public class MainActivity extends BridgeActivity {

    @Override
    public void onCreate(Bundle savedInstanceState) {
        // Plugins próprios do app (no Capacitor 8, registrados antes do
        // super.onCreate, que monta a ponte). UpdatePlugin: atualização do
        // APK. CallPlugin: serviço e notificação da chamada (fase 5).
        // PushPlugin: notificações push (fase 6).
        registerPlugin(UpdatePlugin.class);
        registerPlugin(CallPlugin.class);
        registerPlugin(PushPlugin.class);
        super.onCreate(savedInstanceState);

        // Chamada com o app em segundo plano (fase 5): o BridgeActivity não
        // pausa a WebView. No onPause ele só avisa os plugins e o
        // MockCordovaWebViewImpl, que chama pauseTimers() apenas com a
        // preferência KeepRunning em false (o padrão é true, e o app não
        // muda); WebView.onPause() não é chamado em lugar nenhum. O que muda
        // em segundo plano é o que o Chromium faz numa página escondida: sem
        // áudio tocando, ele a congela depois de um minuto, e isso a
        // CallWebView evita durante a chamada mantendo a página visível. O
        // processo de renderização fica preso à importância do app, que o
        // CallService mantém em primeiro plano; deixar explícito que isso
        // vale também com a WebView invisível, para uma mudança de padrão do
        // Chromium não derrubar o renderer (e a chamada) com a tela apagada.
        // Ver docs/architecture.md, "Decisão: chamada em segundo plano no
        // Android (fase 5)".
        WebView webView = getBridge() == null ? null : getBridge().getWebView();
        if (webView != null) {
            webView.setRendererPriorityPolicy(WebView.RENDERER_PRIORITY_IMPORTANT, false);
        }
    }
}
