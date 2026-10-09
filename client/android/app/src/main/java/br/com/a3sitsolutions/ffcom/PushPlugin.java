package br.com.a3sitsolutions.ffcom;

import android.Manifest;
import android.content.Intent;
import android.provider.Settings;
import android.util.Log;
import com.getcapacitor.JSObject;
import com.getcapacitor.PermissionState;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.getcapacitor.annotation.Permission;
import com.getcapacitor.annotation.PermissionCallback;
import com.google.firebase.FirebaseApp;
import com.google.firebase.messaging.FirebaseMessaging;
import java.util.HashMap;
import java.util.Map;

// Ponte entre a página e o push do Android (fase 6 do
// docs/android-runbook.md; ver docs/architecture.md, "Decisão: notificações
// push no app Android (fase 6, client)"). A parte web
// (client/src/lib/androidPush.ts) pede a permissão depois da explicação dela,
// pega o token FCM para registrar no server-central, manda os textos
// traduzidos, diz o que está na tela e recebe o toque numa notificação como o
// evento "open" ({type, serverAddress?, channelId?, accountId?}) e a troca de
// token como "token" ({token}).
//
// Sem google-services.json no build, o FirebaseApp não sobe
// (FirebaseInitProvider sem configuração): getStatus devolve available
// false, e a página esconde tudo do push.
@CapacitorPlugin(
    name = "FfcomPush",
    permissions = { @Permission(strings = { Manifest.permission.POST_NOTIFICATIONS }, alias = PushPlugin.NOTIFICATIONS) }
)
public class PushPlugin extends Plugin {

    private static final String TAG = "FfcomPush";
    static final String NOTIFICATIONS = "notifications";

    // O plugin da ponte viva, para o PushMessagingService entregar o token
    // novo. Só existe uma Activity (singleTask).
    private static volatile PushPlugin instance;

    @Override
    public void load() {
        instance = this;
        PushState.foreground = true;
        // Abertura a frio pelo toque numa notificação: o evento fica retido
        // até a página (já logada) ouvir.
        dispatchOpen(getActivity() == null ? null : getActivity().getIntent());
    }

    @Override
    protected void handleOnNewIntent(Intent intent) {
        dispatchOpen(intent);
    }

    @Override
    protected void handleOnResume() {
        PushState.foreground = true;
    }

    @Override
    protected void handleOnPause() {
        PushState.foreground = false;
    }

    @Override
    protected void handleOnDestroy() {
        PushState.foreground = false;
        PushState.activeKey = null;
        if (instance == this) instance = null;
    }

    static void onNewToken(String token) {
        PushPlugin plugin = instance;
        if (plugin == null) return;
        JSObject data = new JSObject();
        data.put("token", token);
        plugin.notifyListeners("token", data);
    }

    // {available, enabled, asked}: Firebase configurado neste APK,
    // notificações do app ligadas e se a explicação do push já apareceu.
    @PluginMethod
    public void getStatus(PluginCall call) {
        JSObject result = new JSObject();
        result.put("available", available());
        result.put("enabled", NotificationAccess.enabled(getContext()));
        result.put("asked", NotificationAccess.askedByPush(getContext()));
        call.resolve(result);
    }

    // Depois do "Ativar" da explicação. Android 13+ sem a permissão: o pedido
    // do sistema. Permissão já dada, mas notificações desligadas pela pessoa
    // (ou Android abaixo do 13): abre as configurações de notificação do app.
    // Resolve {enabled}.
    @PluginMethod
    public void requestPermission(PluginCall call) {
        NotificationAccess.markAskedByPush(getContext());
        if (NotificationAccess.enabled(getContext())) {
            resolveEnabled(call);
            return;
        }
        if (NotificationAccess.hasRuntimePermission() && getPermissionState(NOTIFICATIONS) != PermissionState.GRANTED) {
            requestPermissionForAlias(NOTIFICATIONS, call, "permissionResult");
            return;
        }
        try {
            Intent intent = new Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS)
                .putExtra(Settings.EXTRA_APP_PACKAGE, getContext().getPackageName())
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            getContext().startActivity(intent);
        } catch (RuntimeException e) {
            Log.w(TAG, "configurações de notificação não abriram", e);
        }
        resolveEnabled(call);
    }

    @PermissionCallback
    private void permissionResult(PluginCall call) {
        resolveEnabled(call);
    }

    // "Agora não" na explicação: não pergunta de novo.
    @PluginMethod
    public void dismissPermission(PluginCall call) {
        NotificationAccess.markAskedByPush(getContext());
        call.resolve();
    }

    @PluginMethod
    public void getToken(PluginCall call) {
        if (!available()) {
            call.reject("Push indisponível neste APK", "unavailable");
            return;
        }
        FirebaseMessaging.getInstance().getToken().addOnCompleteListener(task -> {
            if (!task.isSuccessful() || task.getResult() == null) {
                Log.w(TAG, "token FCM não veio", task.getException());
                call.reject("Token FCM indisponível", "token-failed", task.getException());
                return;
            }
            JSObject result = new JSObject();
            result.put("token", task.getResult());
            call.resolve(result);
        });
    }

    // Logout: o token deixa de valer no FCM, e a próxima conta que entrar
    // neste aparelho recebe outro.
    @PluginMethod
    public void deleteToken(PluginCall call) {
        if (!available()) {
            call.resolve();
            return;
        }
        FirebaseMessaging.getInstance().deleteToken().addOnCompleteListener(task -> {
            if (!task.isSuccessful()) Log.w(TAG, "token FCM não foi apagado", task.getException());
            call.resolve();
        });
    }

    // Modelos traduzidos (ver PushTexts): um campo por chave de
    // PushTexts.DEFAULTS; ausente ou vazio volta ao padrão.
    @PluginMethod
    public void setTexts(PluginCall call) {
        Map<String, String> values = new HashMap<>();
        for (String key : PushTexts.DEFAULTS.keySet()) {
            String value = call.getString(key);
            if (value != null) values.put(key, value);
        }
        PushNotifier.saveTexts(getContext(), values);
        call.resolve();
    }

    // O que está na tela: {serverAddress, channelId} de um canal,
    // {dmAccountId} de uma DM, ou nada. Tira a notificação do que abriu.
    @PluginMethod
    public void setActive(PluginCall call) {
        String serverAddress = call.getString("serverAddress");
        String channelId = call.getString("channelId");
        String dmAccountId = call.getString("dmAccountId");
        String key = null;
        if (notBlank(serverAddress) && notBlank(channelId)) {
            key = PushPayload.channelKey(serverAddress, channelId);
        } else if (notBlank(dmAccountId)) {
            key = PushPayload.dmKey(dmAccountId);
        }
        PushState.activeKey = key;
        if (key != null) PushNotifier.cancel(getContext(), key);
        call.resolve();
    }

    // Logout: tira todas as notificações de push da gaveta.
    @PluginMethod
    public void clearAll(PluginCall call) {
        PushState.activeKey = null;
        PushNotifier.cancelAll(getContext());
        call.resolve();
    }

    private void resolveEnabled(PluginCall call) {
        JSObject result = new JSObject();
        result.put("enabled", NotificationAccess.enabled(getContext()));
        call.resolve(result);
    }

    // Passa o toque à página e tira os extras do Intent: o getIntent() da
    // Activity continua o mesmo a cada recarga da página, e o mesmo toque
    // não pode abrir o canal de novo.
    private void dispatchOpen(Intent intent) {
        if (intent == null || !PushNotifier.ACTION_OPEN.equals(intent.getAction())) return;
        JSObject data = new JSObject();
        data.put("type", intent.getStringExtra(PushNotifier.EXTRA_TYPE));
        putIfPresent(data, "serverAddress", intent.getStringExtra(PushNotifier.EXTRA_SERVER_ADDRESS));
        putIfPresent(data, "channelId", intent.getStringExtra(PushNotifier.EXTRA_CHANNEL_ID));
        putIfPresent(data, "accountId", intent.getStringExtra(PushNotifier.EXTRA_ACCOUNT_ID));
        intent.setAction(Intent.ACTION_MAIN);
        intent.removeExtra(PushNotifier.EXTRA_TYPE);
        intent.removeExtra(PushNotifier.EXTRA_SERVER_ADDRESS);
        intent.removeExtra(PushNotifier.EXTRA_CHANNEL_ID);
        intent.removeExtra(PushNotifier.EXTRA_ACCOUNT_ID);
        notifyListeners("open", data, true);
    }

    private boolean available() {
        return !FirebaseApp.getApps(getContext()).isEmpty();
    }

    private static void putIfPresent(JSObject data, String key, String value) {
        if (value != null && !value.isEmpty()) data.put(key, value);
    }

    private static boolean notBlank(String s) {
        return s != null && !s.trim().isEmpty();
    }
}
