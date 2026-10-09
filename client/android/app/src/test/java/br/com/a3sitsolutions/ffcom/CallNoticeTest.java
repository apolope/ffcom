package br.com.a3sitsolutions.ffcom;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import android.content.pm.ServiceInfo;
import org.junit.Test;

public class CallNoticeTest {

    private static final int MIC = ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE;
    private static final int MEDIA = ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK;

    @Test
    public void microfoneSoComPermissaoEAndroid11() {
        assertEquals(MEDIA | MIC, CallNotice.foregroundTypes(34, true));
        assertEquals(MEDIA | MIC, CallNotice.foregroundTypes(30, true));
        // Android 10 não tem o tipo microphone (nem a restrição dele).
        assertEquals(MEDIA, CallNotice.foregroundTypes(29, true));
        // Sem RECORD_AUDIO o Android 14 recusa o tipo: fica só a reprodução.
        assertEquals(MEDIA, CallNotice.foregroundTypes(34, false));
    }

    @Test
    public void textosVaziosCaemNoPadrao() {
        CallNotice notice = new CallNotice("", null, null, "  ", "");
        assertEquals("FFCom", notice.channelName);
        assertEquals("FFCom", notice.title);
        assertEquals("", notice.text);
        assertNull(notice.toggleMicLabel);
        assertEquals("Sair", notice.leaveLabel);
    }

    @Test
    public void textosDaPaginaPassamInteiros() {
        CallNotice notice = new CallNotice("Chamada", "Em chamada em Geral · Família", "Microfone aberto", "Mutar", "Sair");
        assertEquals("Chamada", notice.channelName);
        assertEquals("Em chamada em Geral · Família", notice.title);
        assertEquals("Microfone aberto", notice.text);
        assertEquals("Mutar", notice.toggleMicLabel);
        assertEquals("Sair", notice.leaveLabel);
    }
}
