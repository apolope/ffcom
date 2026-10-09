package br.com.a3sitsolutions.ffcom;

import android.content.pm.ServiceInfo;
import android.os.Build;

// O que a notificação "Em chamada" mostra e as regras puras do serviço da
// chamada (CallService), separadas para os testes JUnit em app/src/test. Os
// textos vêm prontos e traduzidos da parte web (lib/androidCallService.ts):
// o nativo não tem i18n próprio.
final class CallNotice {

    // Nome do canal de notificação nas configurações do Android.
    final String channelName;
    // "Em chamada em #sala · Servidor".
    final String title;
    // "Microfone aberto", "Microfone fechado" ou "Conectando…".
    final String text;
    // "Mutar" ou "Desmutar"; null esconde a ação (ainda conectando).
    final String toggleMicLabel;
    // "Sair".
    final String leaveLabel;

    CallNotice(String channelName, String title, String text, String toggleMicLabel, String leaveLabel) {
        this.channelName = orDefault(channelName, "FFCom");
        this.title = orDefault(title, "FFCom");
        this.text = text == null ? "" : text;
        this.toggleMicLabel = isBlank(toggleMicLabel) ? null : toggleMicLabel;
        this.leaveLabel = orDefault(leaveLabel, "Sair");
    }

    // Tipos do serviço em primeiro plano. `microphone` mantém o microfone
    // aberto com a tela apagada ou em outro app (Android 11+); o Android 14
    // só aceita esse tipo com RECORD_AUDIO concedida e o app visível, senão
    // startForeground lança SecurityException. `mediaPlayback` vale sempre:
    // segura quem entrou só ouvindo (microfone negado) e é o que sobra até a
    // permissão do microfone sair, porque o serviço começa no clique de
    // entrar, antes do getUserMedia pedir a permissão.
    static int foregroundTypes(int sdkInt, boolean micGranted) {
        int types = ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK;
        if (micGranted && sdkInt >= Build.VERSION_CODES.R) {
            types |= ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE;
        }
        return types;
    }

    private static boolean isBlank(String s) {
        return s == null || s.trim().isEmpty();
    }

    private static String orDefault(String s, String fallback) {
        return isBlank(s) ? fallback : s;
    }
}
