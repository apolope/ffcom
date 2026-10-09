package br.com.a3sitsolutions.ffcom;

import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.service.notification.StatusBarNotification;
import android.util.Log;
import androidx.core.app.NotificationCompat;
import androidx.core.app.Person;
import java.util.HashMap;
import java.util.Map;

// Monta e tira as notificações de push (fase 6). Uma notificação por canal
// (servidor + canal), com MessagingStyle: as últimas linhas e, a partir da
// segunda, "N mensagens novas". Uma por conversa de DM, só com o autor (a DM
// é cifrada de ponta a ponta). Uma por pessoa nos avisos de amizade. Três
// canais de notificação, para a pessoa ajustar cada um nas configurações do
// Android. Ver docs/architecture.md, "Decisão: notificações push no app
// Android (fase 6, client)".
final class PushNotifier {

    private static final String TAG = "FfcomPush";

    static final String CHANNEL_MESSAGES = "ffcom_messages";
    static final String CHANNEL_DMS = "ffcom_dms";
    static final String CHANNEL_FRIENDS = "ffcom_friends";

    private static final String TEXTS_PREFS = "ffcom_push_texts";
    private static final String GROUPS_PREFS = "ffcom_push_groups";

    // Toque na notificação: o MainActivity recebe esta ação com os extras, e
    // o PushPlugin passa à página como o evento "open".
    static final String ACTION_OPEN = "br.com.a3sitsolutions.ffcom.PUSH_OPEN";
    static final String EXTRA_TYPE = "ffcomPushType";
    static final String EXTRA_SERVER_ADDRESS = "ffcomPushServerAddress";
    static final String EXTRA_CHANNEL_ID = "ffcomPushChannelId";
    static final String EXTRA_ACCOUNT_ID = "ffcomPushAccountId";

    private PushNotifier() {}

    static PushTexts texts(Context context) {
        SharedPreferences prefs = context.getSharedPreferences(TEXTS_PREFS, Context.MODE_PRIVATE);
        Map<String, String> stored = new HashMap<>();
        for (Map.Entry<String, ?> e : prefs.getAll().entrySet()) {
            if (e.getValue() instanceof String) stored.put(e.getKey(), (String) e.getValue());
        }
        return new PushTexts(stored);
    }

    // Grava os modelos mandados pela página e atualiza o nome dos canais de
    // notificação (troca de idioma).
    static void saveTexts(Context context, Map<String, String> values) {
        SharedPreferences.Editor editor = context.getSharedPreferences(TEXTS_PREFS, Context.MODE_PRIVATE).edit();
        for (String key : PushTexts.DEFAULTS.keySet()) {
            String value = values.get(key);
            if (value == null || value.trim().isEmpty()) {
                editor.remove(key);
            } else {
                editor.putString(key, value);
            }
        }
        editor.apply();
        ensureChannels(context, texts(context));
    }

    // Recriar um canal com o mesmo id só atualiza o nome; a importância que
    // a pessoa escolheu nas configurações continua.
    static void ensureChannels(Context context, PushTexts texts) {
        NotificationManager manager = manager(context);
        if (manager == null) return;
        manager.createNotificationChannel(channel(CHANNEL_MESSAGES, texts.get(PushTexts.CHANNEL_MESSAGES), NotificationManager.IMPORTANCE_HIGH));
        manager.createNotificationChannel(channel(CHANNEL_DMS, texts.get(PushTexts.CHANNEL_DMS), NotificationManager.IMPORTANCE_HIGH));
        manager.createNotificationChannel(channel(CHANNEL_FRIENDS, texts.get(PushTexts.CHANNEL_FRIENDS), NotificationManager.IMPORTANCE_DEFAULT));
    }

    static synchronized void show(Context context, PushPayload payload) {
        NotificationManager manager = manager(context);
        if (manager == null || !NotificationAccess.enabled(context)) return;
        PushTexts texts = texts(context);
        ensureChannels(context, texts);

        String key = payload.groupKey();
        int id = PushPayload.notificationId(key);
        long now = System.currentTimeMillis();
        NotificationCompat.Builder builder;
        if (payload.isFriend()) {
            builder = new NotificationCompat.Builder(context, CHANNEL_FRIENDS)
                .setContentTitle(texts.friendText(payload.type, payload.author))
                .setCategory(NotificationCompat.CATEGORY_SOCIAL);
        } else {
            SharedPreferences groups = context.getSharedPreferences(GROUPS_PREFS, Context.MODE_PRIVATE);
            // Notificação que já saiu da tela (tocada ou dispensada) começa a
            // contagem de novo.
            PushGroup group = isShowing(manager, id) ? PushGroup.decode(groups.getString(key, null)) : new PushGroup();
            if (payload.isChannelMessage()) {
                group.add(texts.author(payload.author), texts.messageLine(payload.text, payload.attachment, payload.threadTitle), now);
                builder = channelNotification(context, texts, payload, group);
            } else {
                group.add(texts.author(payload.author), texts.dmTitle(payload.author), now);
                builder = new NotificationCompat.Builder(context, CHANNEL_DMS)
                    .setContentTitle(texts.dmTitle(payload.author))
                    .setContentText(group.count > 1 ? texts.newMessages(group.count) : null)
                    .setNumber(group.count)
                    .setCategory(NotificationCompat.CATEGORY_MESSAGE);
            }
            groups.edit().putString(key, group.encode()).apply();
        }
        builder
            .setSmallIcon(R.drawable.ic_stat_message)
            .setAutoCancel(true)
            .setShowWhen(true)
            .setWhen(now)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setContentIntent(openIntent(context, payload, id));
        try {
            manager.notify(id, builder.build());
        } catch (RuntimeException e) {
            // SecurityException com a permissão revogada entre a conferência e
            // o notify. Só registra.
            Log.w(TAG, "notificação não saiu", e);
        }
    }

    private static NotificationCompat.Builder channelNotification(Context context, PushTexts texts, PushPayload payload, PushGroup group) {
        // Sem nome na lista (não deveria acontecer), o endereço sem o esquema.
        String serverName = payload.serverName.isEmpty() ? payload.serverAddress.replaceFirst("^https?://", "") : payload.serverName;
        String title = texts.channelTitle(payload.channelName, serverName);
        Person you = new Person.Builder().setName(texts.get(PushTexts.YOU)).build();
        NotificationCompat.MessagingStyle style = new NotificationCompat.MessagingStyle(you)
            .setConversationTitle(title)
            .setGroupConversation(true);
        PushGroup.Line last = null;
        for (PushGroup.Line line : group.lines()) {
            style.addMessage(line.text, line.time, new Person.Builder().setName(line.author).build());
            last = line;
        }
        NotificationCompat.Builder builder = new NotificationCompat.Builder(context, CHANNEL_MESSAGES)
            .setStyle(style)
            .setContentTitle(title)
            .setNumber(group.count)
            .setCategory(NotificationCompat.CATEGORY_MESSAGE);
        if (last != null) builder.setContentText(last.author + ": " + last.text);
        // "5 mensagens novas" quando há mais do que a primeira; o
        // MessagingStyle só mostra as últimas linhas guardadas.
        if (group.count > 1) builder.setSubText(texts.newMessages(group.count));
        return builder;
    }

    // Tira a notificação de um canal ou DM (a pessoa abriu no app) e esquece
    // a contagem dela.
    static synchronized void cancel(Context context, String key) {
        NotificationManager manager = manager(context);
        if (manager != null) manager.cancel(PushPayload.notificationId(key));
        context.getSharedPreferences(GROUPS_PREFS, Context.MODE_PRIVATE).edit().remove(key).apply();
    }

    // Logout: nada da conta que saiu fica na gaveta. A notificação da chamada
    // (CallService) não é desta classe e fica.
    static synchronized void cancelAll(Context context) {
        NotificationManager manager = manager(context);
        if (manager != null) {
            for (StatusBarNotification n : manager.getActiveNotifications()) {
                if (isPushChannel(n.getNotification().getChannelId())) manager.cancel(n.getId());
            }
        }
        context.getSharedPreferences(GROUPS_PREFS, Context.MODE_PRIVATE).edit().clear().apply();
    }

    private static boolean isPushChannel(String channelId) {
        return CHANNEL_MESSAGES.equals(channelId) || CHANNEL_DMS.equals(channelId) || CHANNEL_FRIENDS.equals(channelId);
    }

    private static boolean isShowing(NotificationManager manager, int id) {
        for (StatusBarNotification n : manager.getActiveNotifications()) {
            if (n.getId() == id) return true;
        }
        return false;
    }

    // Abre o app no canal, na DM ou na tela de amigos. O requestCode é o id
    // da notificação, para cada uma ter o próprio PendingIntent.
    private static PendingIntent openIntent(Context context, PushPayload payload, int id) {
        Intent intent = new Intent(context, MainActivity.class)
            .setAction(ACTION_OPEN)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_SINGLE_TOP)
            .putExtra(EXTRA_TYPE, payload.type);
        if (payload.isChannelMessage()) {
            intent.putExtra(EXTRA_SERVER_ADDRESS, payload.serverAddress).putExtra(EXTRA_CHANNEL_ID, payload.channelId);
        } else {
            intent.putExtra(EXTRA_ACCOUNT_ID, payload.accountId);
        }
        return PendingIntent.getActivity(context, id, intent, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    private static NotificationChannel channel(String id, String name, int importance) {
        NotificationChannel channel = new NotificationChannel(id, name, importance);
        channel.setShowBadge(true);
        return channel;
    }

    private static NotificationManager manager(Context context) {
        return (NotificationManager) context.getSystemService(Context.NOTIFICATION_SERVICE);
    }
}
