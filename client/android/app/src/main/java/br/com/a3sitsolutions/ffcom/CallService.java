package br.com.a3sitsolutions.ffcom;

import android.Manifest;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.IBinder;
import android.os.PowerManager;
import android.util.Log;
import androidx.core.app.NotificationCompat;
import androidx.core.app.ServiceCompat;
import androidx.core.content.ContextCompat;

// Serviço em primeiro plano enquanto há chamada (fase 5 do
// docs/android-runbook.md; ver docs/architecture.md, "Decisão: chamada em
// segundo plano no Android (fase 5)"). Sem ele, com a tela apagada ou em
// outro app, o Android corta o microfone (Android 11+) e acaba matando o
// processo, e a chamada cai. A chamada em si continua inteira na WebView
// (LiveKit); o serviço só segura o processo, o microfone e a CPU e mostra a
// notificação "Em chamada" com "Mutar"/"Desmutar" e "Sair".
//
// Quem manda é a parte web, pelo CallPlugin: show() a cada mudança (entrar,
// conectar, mutar, trocar de sala, trocar de idioma) e stop() ao sair. As
// ações da notificação voltam ao serviço e seguem para a página pelo
// CallPlugin.dispatch.
public class CallService extends Service {

    private static final String TAG = "FfcomCall";
    static final String CHANNEL_ID = "ffcom_call";
    private static final int NOTIFICATION_ID = 1001;

    private static final String ACTION_SHOW = "br.com.a3sitsolutions.ffcom.call.SHOW";
    private static final String ACTION_TOGGLE_MIC = "br.com.a3sitsolutions.ffcom.call.TOGGLE_MIC";
    private static final String ACTION_LEAVE = "br.com.a3sitsolutions.ffcom.call.LEAVE";

    private static final String EXTRA_CHANNEL_NAME = "channelName";
    private static final String EXTRA_TITLE = "title";
    private static final String EXTRA_TEXT = "text";
    private static final String EXTRA_TOGGLE_MIC_LABEL = "toggleMicLabel";
    private static final String EXTRA_LEAVE_LABEL = "leaveLabel";

    private CallNotice notice;
    // Tipos com que startForeground já deu certo; 0 antes da primeira vez.
    private int foregroundTypes;
    // Início da chamada, para o cronômetro da notificação não zerar a cada
    // atualização.
    private long startedAt;
    private PowerManager.WakeLock wakeLock;

    // Liga o serviço (ou atualiza a notificação, se já está ligado). Precisa
    // ser chamado com o app visível na primeira vez: startService de um app
    // em segundo plano lança IllegalStateException, e o Android 12+ recusa
    // startForeground (ForegroundServiceStartNotAllowedException). Quem chama
    // trata a exceção.
    static void show(Context context, CallNotice notice) {
        Intent intent = new Intent(context, CallService.class).setAction(ACTION_SHOW);
        intent.putExtra(EXTRA_CHANNEL_NAME, notice.channelName);
        intent.putExtra(EXTRA_TITLE, notice.title);
        intent.putExtra(EXTRA_TEXT, notice.text);
        intent.putExtra(EXTRA_TOGGLE_MIC_LABEL, notice.toggleMicLabel);
        intent.putExtra(EXTRA_LEAVE_LABEL, notice.leaveLabel);
        // startService, não startForegroundService: sem o prazo de 5 s para
        // chamar startForeground, uma recusa dele (permissão, segundo plano)
        // só para o serviço, em vez de derrubar o app.
        context.startService(intent);
    }

    // Para o serviço e tira a notificação. stopService funciona de qualquer
    // estado do app e não faz nada se o serviço já parou.
    static void stop(Context context) {
        context.stopService(new Intent(context, CallService.class));
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        String action = intent == null ? null : intent.getAction();
        if (ACTION_SHOW.equals(action)) {
            showNotice(
                new CallNotice(
                    intent.getStringExtra(EXTRA_CHANNEL_NAME),
                    intent.getStringExtra(EXTRA_TITLE),
                    intent.getStringExtra(EXTRA_TEXT),
                    intent.getStringExtra(EXTRA_TOGGLE_MIC_LABEL),
                    intent.getStringExtra(EXTRA_LEAVE_LABEL)
                )
            );
        } else if (ACTION_TOGGLE_MIC.equals(action)) {
            CallPlugin.dispatch("toggleMic");
        } else if (ACTION_LEAVE.equals(action)) {
            // A página sai da sala e manda stop(). Sem página ouvindo (WebView
            // morta), não há chamada a encerrar: só tira a notificação.
            if (!CallPlugin.dispatch("leave")) stopSelf();
        }
        // Recriado pelo sistema, ou uma ação que chegou sem a página ter
        // pedido a notificação: nada a mostrar nem a manter.
        if (notice == null) stopSelf();
        // A chamada mora na WebView: se o processo morre, ela acabou, e o
        // serviço não deve voltar sozinho.
        return START_NOT_STICKY;
    }

    private void showNotice(CallNotice next) {
        if (startedAt == 0) startedAt = System.currentTimeMillis();
        notice = next;
        ensureChannel(next.channelName);
        Notification notification = buildNotification();
        int wantTypes = CallNotice.foregroundTypes(Build.VERSION.SDK_INT, micGranted());
        if (foregroundTypes == wantTypes) {
            notificationManager().notify(NOTIFICATION_ID, notification);
            return;
        }
        try {
            ServiceCompat.startForeground(this, NOTIFICATION_ID, notification, wantTypes);
            foregroundTypes = wantTypes;
            acquireWakeLock();
        } catch (RuntimeException e) {
            // ForegroundServiceStartNotAllowedException (app em segundo plano
            // no Android 12+) ou SecurityException (tipo microphone sem a
            // permissão ou fora do primeiro plano, Android 14+). Já em
            // primeiro plano com outros tipos, continua como estava e só
            // atualiza a notificação; senão, não há serviço a manter.
            Log.w(TAG, "startForeground recusado (tipos " + wantTypes + ")", e);
            if (foregroundTypes != 0) {
                notificationManager().notify(NOTIFICATION_ID, notification);
            } else {
                stopSelf();
            }
        }
    }

    // Com a tela apagada a CPU pode dormir entre um pacote e outro mesmo com
    // o serviço em primeiro plano; o áudio tocando costuma segurar, mas
    // numa sala em silêncio não há garantia. Solto em onDestroy.
    private void acquireWakeLock() {
        if (wakeLock != null) return;
        PowerManager pm = (PowerManager) getSystemService(Context.POWER_SERVICE);
        if (pm == null) return;
        wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "ffcom:call");
        wakeLock.setReferenceCounted(false);
        wakeLock.acquire();
    }

    private boolean micGranted() {
        return ContextCompat.checkSelfPermission(this, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED;
    }

    private Notification buildNotification() {
        NotificationCompat.Builder builder = new NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_stat_call)
            .setContentTitle(notice.title)
            .setContentText(notice.text)
            .setContentIntent(openAppIntent())
            .setCategory(NotificationCompat.CATEGORY_CALL)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .setShowWhen(true)
            .setWhen(startedAt)
            .setUsesChronometer(true)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            // Android 12+ adia em até 10 s a notificação de um serviço em
            // primeiro plano; numa chamada ela precisa aparecer na hora.
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE);
        if (notice.toggleMicLabel != null) {
            builder.addAction(0, notice.toggleMicLabel, serviceIntent(ACTION_TOGGLE_MIC, 1));
        }
        builder.addAction(0, notice.leaveLabel, serviceIntent(ACTION_LEAVE, 2));
        return builder.build();
    }

    // Tocar na notificação traz o app de volta (singleTask: a mesma
    // Activity, com a chamada na WebView).
    private PendingIntent openAppIntent() {
        Intent intent = new Intent(this, MainActivity.class)
            .setAction(Intent.ACTION_MAIN)
            .addCategory(Intent.CATEGORY_LAUNCHER)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        return PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    private PendingIntent serviceIntent(String action, int requestCode) {
        Intent intent = new Intent(this, CallService.class).setAction(action);
        return PendingIntent.getService(this, requestCode, intent, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    // Importância baixa: aparece na gaveta e na barra, sem som nem vibração.
    // Recriar com o mesmo id só atualiza o nome (troca de idioma).
    private void ensureChannel(String name) {
        NotificationChannel channel = new NotificationChannel(CHANNEL_ID, name, NotificationManager.IMPORTANCE_LOW);
        channel.setShowBadge(false);
        notificationManager().createNotificationChannel(channel);
    }

    private NotificationManager notificationManager() {
        return (NotificationManager) getSystemService(Context.NOTIFICATION_SERVICE);
    }

    // stopWithTask no manifesto já para o serviço quando a pessoa tira o app
    // dos recentes; isto é a reserva, porque a WebView vai junto e a chamada
    // com ela.
    @Override
    public void onTaskRemoved(Intent rootIntent) {
        stopSelf();
    }

    @Override
    public void onDestroy() {
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE);
        // Sem startForeground (recusado), a notificação pode ter saído por
        // notify(); some junto.
        notificationManager().cancel(NOTIFICATION_ID);
        if (wakeLock != null && wakeLock.isHeld()) wakeLock.release();
        wakeLock = null;
        super.onDestroy();
    }
}
