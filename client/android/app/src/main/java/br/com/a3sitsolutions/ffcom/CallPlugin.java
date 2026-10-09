package br.com.a3sitsolutions.ffcom;

import android.Manifest;
import android.content.Context;
import android.content.SharedPreferences;
import android.os.Build;
import android.util.Log;
import android.webkit.WebView;
import androidx.core.app.NotificationManagerCompat;
import com.getcapacitor.JSObject;
import com.getcapacitor.PermissionState;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.WebViewListener;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.getcapacitor.annotation.Permission;
import com.getcapacitor.annotation.PermissionCallback;

// Ponte entre a chamada na WebView e o CallService (fase 5 do
// docs/android-runbook.md; ver docs/architecture.md, "Decisão: chamada em
// segundo plano no Android (fase 5)"). A parte web
// (client/src/lib/androidCallService.ts) chama show() ao entrar na sala e a
// cada mudança do que a notificação mostra, e stop() ao sair; as ações da
// notificação voltam como o evento "action" ({action: "toggleMic" | "leave"}).
//
// O serviço também para sem pedido da página quando ela não pode mais estar
// numa chamada: página recarregada ou trocada (onPageStarted) e Activity
// destruída (handleOnDestroy). Assim nunca sobra notificação de uma chamada
// que já acabou.
@CapacitorPlugin(
    name = "FfcomCall",
    permissions = { @Permission(strings = { Manifest.permission.POST_NOTIFICATIONS }, alias = CallPlugin.NOTIFICATIONS) }
)
public class CallPlugin extends Plugin {

    private static final String TAG = "FfcomCall";
    static final String NOTIFICATIONS = "notifications";
    private static final String PREFS = "ffcom_call";
    private static final String PREF_ASKED_NOTIFICATIONS = "askedNotifications";

    // O plugin da ponte viva, para o CallService entregar as ações da
    // notificação. Só existe uma Activity (singleTask).
    private static volatile CallPlugin instance;

    // Última notificação pedida pela página, reenviada se a permissão de
    // notificação sair depois do serviço já ligado.
    private CallNotice lastNotice;

    private final WebViewListener pageListener = new WebViewListener() {
        @Override
        public void onPageStarted(WebView webView) {
            // Recarregar (botão verde, F5 do service worker) ou navegar desfaz
            // a sala do LiveKit junto com a página.
            stopService();
        }
    };

    @Override
    public void load() {
        instance = this;
        getBridge().addWebViewListener(pageListener);
    }

    @Override
    protected void handleOnDestroy() {
        getBridge().removeWebViewListener(pageListener);
        if (instance == this) instance = null;
        // A WebView morre com a Activity, e a chamada com ela.
        stopService();
    }

    // Entrega uma ação da notificação à página. false se não há página
    // ouvindo (ponte destruída).
    static boolean dispatch(String action) {
        CallPlugin plugin = instance;
        if (plugin == null || !plugin.hasListeners("action")) return false;
        JSObject data = new JSObject();
        data.put("action", action);
        plugin.notifyListeners("action", data);
        return true;
    }

    // {channelName, title, text, toggleMicLabel?, leaveLabel}, todos já
    // traduzidos. Liga o serviço na primeira vez e atualiza a notificação nas
    // seguintes. Rejeita com "not-allowed" se o Android recusou ligar (app já
    // em segundo plano); a chamada segue, só sem a proteção do serviço.
    @PluginMethod
    public void show(PluginCall call) {
        CallNotice notice = new CallNotice(
            call.getString("channelName"),
            call.getString("title"),
            call.getString("text"),
            call.getString("toggleMicLabel"),
            call.getString("leaveLabel")
        );
        lastNotice = notice;
        try {
            CallService.show(getContext(), notice);
        } catch (RuntimeException e) {
            // IllegalStateException (startService com o app em segundo plano)
            // ou ForegroundServiceStartNotAllowedException. Só registra.
            Log.w(TAG, "serviço da chamada não ligou", e);
            call.reject("O Android não deixou ligar o serviço da chamada", "not-allowed", e);
            return;
        }
        if (shouldAskNotifications()) {
            markAskedNotifications();
            requestPermissionForAlias(NOTIFICATIONS, call, "notificationsResult");
            return;
        }
        call.resolve();
    }

    @PluginMethod
    public void stop(PluginCall call) {
        lastNotice = null;
        stopService();
        call.resolve();
    }

    // Com a permissão concedida, a notificação do serviço já ligado precisa
    // sair de novo para aparecer. Negada, o serviço continua igual (ver
    // shouldAskNotifications).
    @PermissionCallback
    private void notificationsResult(PluginCall call) {
        CallNotice notice = lastNotice;
        if (getPermissionState(NOTIFICATIONS) == PermissionState.GRANTED && notice != null) {
            try {
                CallService.show(getContext(), notice);
            } catch (RuntimeException e) {
                Log.w(TAG, "notificação não atualizou", e);
            }
        }
        call.resolve();
    }

    // Android 13+: a notificação "Em chamada" depende de POST_NOTIFICATIONS.
    // Sem ela, o serviço em primeiro plano roda igual (o microfone e o
    // processo seguem protegidos), mas a notificação só aparece no
    // "Gerenciador de tarefas" da gaveta, sem as ações. Pede uma vez só, na
    // primeira chamada; depois disso o caminho é a tela de configurações do
    // app (e a fase 6, das notificações push, pede de novo no contexto dela).
    private boolean shouldAskNotifications() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return false;
        if (NotificationManagerCompat.from(getContext()).areNotificationsEnabled()) return false;
        return !prefs().getBoolean(PREF_ASKED_NOTIFICATIONS, false);
    }

    private void markAskedNotifications() {
        prefs().edit().putBoolean(PREF_ASKED_NOTIFICATIONS, true).apply();
    }

    private SharedPreferences prefs() {
        return getContext().getSharedPreferences(PREFS, Context.MODE_PRIVATE);
    }

    private void stopService() {
        try {
            CallService.stop(getContext());
        } catch (RuntimeException e) {
            Log.w(TAG, "serviço da chamada não parou", e);
        }
    }
}
