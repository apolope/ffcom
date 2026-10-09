package br.com.a3sitsolutions.ffcom;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotEquals;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import java.util.HashMap;
import java.util.Map;
import org.junit.Test;

public class PushPayloadTest {

    private static Map<String, String> channelMessage() {
        Map<String, String> data = new HashMap<>();
        data.put("v", "1");
        data.put("type", "channel_message");
        data.put("sentAt", "2026-10-08T14:03:00Z");
        data.put("serverAddress", "https://chat.exemplo.com");
        data.put("serverName", "Família da Ana");
        data.put("channelId", "c1");
        data.put("channelName", "geral");
        data.put("author", "Marina");
        data.put("text", "Ficaram lindas!");
        data.put("messageId", "m1");
        return data;
    }

    @Test
    public void mensagemDeCanalDoExemploDoProtocolo() {
        PushPayload p = PushPayload.parse(channelMessage());
        assertNotNull(p);
        assertTrue(p.isChannelMessage());
        assertEquals("https://chat.exemplo.com", p.serverAddress);
        assertEquals("Família da Ana", p.serverName);
        assertEquals("geral", p.channelName);
        assertEquals("Marina", p.author);
        assertEquals("Ficaram lindas!", p.text);
        assertEquals("", p.threadId);
        assertEquals("", p.attachment);
    }

    @Test
    public void campoNovoEIgnorado() {
        Map<String, String> data = channelMessage();
        data.put("mention", "true");
        assertNotNull(PushPayload.parse(data));
    }

    @Test
    public void outraVersaoOuTipoNaoMostra() {
        Map<String, String> data = channelMessage();
        data.put("v", "2");
        assertNull(PushPayload.parse(data));
        data = channelMessage();
        data.remove("v");
        assertNull(PushPayload.parse(data));
        data = channelMessage();
        data.put("type", "reaction");
        assertNull(PushPayload.parse(data));
        assertNull(PushPayload.parse(null));
    }

    @Test
    public void semDestinoDoToqueNaoMostra() {
        Map<String, String> data = channelMessage();
        data.remove("channelId");
        assertNull(PushPayload.parse(data));
        Map<String, String> dm = new HashMap<>();
        dm.put("v", "1");
        dm.put("type", "dm");
        dm.put("author", "Marina");
        assertNull(PushPayload.parse(dm));
    }

    @Test
    public void dmEAmizade() {
        Map<String, String> dm = new HashMap<>();
        dm.put("v", "1");
        dm.put("type", "dm");
        dm.put("sentAt", "2026-10-08T14:03:00Z");
        dm.put("accountId", "a1");
        dm.put("author", "");
        PushPayload p = PushPayload.parse(dm);
        assertNotNull(p);
        assertTrue(p.isDm());
        assertFalse(p.isFriend());
        assertEquals(PushPayload.dmKey("a1"), p.groupKey());

        Map<String, String> friend = new HashMap<>(dm);
        friend.put("type", "friend_request");
        friend.put("requestId", "r1");
        PushPayload f = PushPayload.parse(friend);
        assertNotNull(f);
        assertTrue(f.isFriend());
        assertEquals("r1", f.requestId);
        assertNotEquals(p.groupKey(), f.groupKey());
    }

    @Test
    public void agrupaPorServidorECanal() {
        PushPayload a = PushPayload.parse(channelMessage());
        Map<String, String> other = channelMessage();
        other.put("text", "outra");
        other.put("messageId", "m2");
        PushPayload b = PushPayload.parse(other);
        assertEquals(a.groupKey(), b.groupKey());
        assertEquals(PushPayload.channelKey("https://chat.exemplo.com", "c1"), a.groupKey());

        // Mesmo id de canal em outro servidor é outra notificação.
        Map<String, String> otherServer = channelMessage();
        otherServer.put("serverAddress", "https://outro.exemplo.com");
        assertNotEquals(a.groupKey(), PushPayload.parse(otherServer).groupKey());
    }

    @Test
    public void idDaNotificacaoPositivoELongeDaChamada() {
        for (String key : new String[] { "", "dm\na", PushPayload.channelKey("https://x", "y") }) {
            int id = PushPayload.notificationId(key);
            assertTrue(id > 0xFFFF);
            assertEquals(id, PushPayload.notificationId(key));
        }
    }

    @Test
    public void suprimeSoOQueEstaNaTela() {
        PushPayload message = PushPayload.parse(channelMessage());
        String key = message.groupKey();
        // Em segundo plano, tudo notifica.
        assertFalse(PushState.suppress(message, false, key));
        // Na frente, só o canal aberto fica quieto.
        assertTrue(PushState.suppress(message, true, key));
        assertFalse(PushState.suppress(message, true, PushPayload.channelKey("https://chat.exemplo.com", "c2")));
        assertFalse(PushState.suppress(message, true, null));

        Map<String, String> friend = new HashMap<>();
        friend.put("v", "1");
        friend.put("type", "friend_accepted");
        friend.put("accountId", "a1");
        PushPayload f = PushPayload.parse(friend);
        assertTrue(PushState.suppress(f, true, null));
        assertFalse(PushState.suppress(f, false, null));
    }
}
