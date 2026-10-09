package br.com.a3sitsolutions.ffcom;

import android.content.Context;
import android.content.SharedPreferences;
import android.os.Build;
import androidx.core.app.NotificationManagerCompat;

// Quem já pediu a permissão de notificação (POST_NOTIFICATIONS, Android 13+),
// num lugar só para o CallPlugin (fase 5) e o PushPlugin (fase 6) não
// pedirem duas vezes. A chamada pede uma vez, sem explicação, se ninguém
// pediu antes; o push mostra a explicação dele uma vez (a fase 5 deixou
// combinado que a fase 6 pede de novo no contexto dela), mesmo que a chamada
// já tenha pedido. Depois disso, só pelas configurações do Android.
final class NotificationAccess {

    // Arquivo e chave da fase 5, mantidos para quem já tinha o app.
    private static final String CALL_PREFS = "ffcom_call";
    private static final String CALL_ASKED = "askedNotifications";
    private static final String PUSH_PREFS = "ffcom_push";
    private static final String PUSH_ASKED = "askedNotifications";

    private NotificationAccess() {}

    static boolean enabled(Context context) {
        return NotificationManagerCompat.from(context).areNotificationsEnabled();
    }

    // Só o Android 13+ tem a permissão em tempo de execução; abaixo dele as
    // notificações vêm ligadas e só a pessoa desliga, nas configurações.
    static boolean hasRuntimePermission() {
        return Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU;
    }

    static boolean askedByCall(Context context) {
        return context.getSharedPreferences(CALL_PREFS, Context.MODE_PRIVATE).getBoolean(CALL_ASKED, false);
    }

    static boolean askedByPush(Context context) {
        return context.getSharedPreferences(PUSH_PREFS, Context.MODE_PRIVATE).getBoolean(PUSH_ASKED, false);
    }

    static void markAskedByCall(Context context) {
        mark(context.getSharedPreferences(CALL_PREFS, Context.MODE_PRIVATE), CALL_ASKED);
    }

    static void markAskedByPush(Context context) {
        mark(context.getSharedPreferences(PUSH_PREFS, Context.MODE_PRIVATE), PUSH_ASKED);
    }

    // A chamada pede só se ninguém pediu antes: depois da explicação do push,
    // um segundo pedido seco no meio de uma chamada seria insistência.
    static boolean callShouldAsk(Context context) {
        if (!hasRuntimePermission() || enabled(context)) return false;
        return !askedByCall(context) && !askedByPush(context);
    }

    private static void mark(SharedPreferences prefs, String key) {
        prefs.edit().putBoolean(key, true).apply();
    }
}
