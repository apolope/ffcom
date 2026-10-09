package br.com.a3sitsolutions.ffcom;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import java.util.HashMap;
import java.util.List;
import java.util.Map;
import org.junit.Test;

public class PushTextsTest {

    @Test
    public void padroesEmPortugues() {
        PushTexts texts = new PushTexts(null);
        assertEquals("#geral · Família da Ana", texts.channelTitle("geral", "Família da Ana"));
        assertEquals("1 mensagem nova", texts.newMessages(1));
        assertEquals("5 mensagens novas", texts.newMessages(5));
        assertEquals("Nova mensagem de Marina", texts.dmTitle("Marina"));
        assertEquals("Nova mensagem de Alguém", texts.dmTitle(""));
        assertEquals("Marina mandou um pedido de amizade", texts.friendText("friend_request", "Marina"));
        assertEquals("Marina aceitou seu pedido de amizade", texts.friendText("friend_accepted", "Marina"));
    }

    @Test
    public void modelosDaPaginaSubstituemOsPadroes() {
        Map<String, String> stored = new HashMap<>();
        stored.put(PushTexts.NEW_MESSAGES_MANY, "{n} new messages");
        stored.put(PushTexts.DM_TITLE, "New message from {author}");
        stored.put(PushTexts.SOMEONE, "  ");
        stored.put("desconhecida", "x");
        PushTexts texts = new PushTexts(stored);
        assertEquals("12 new messages", texts.newMessages(12));
        assertEquals("New message from Ana", texts.dmTitle("Ana"));
        // Vazio volta ao padrão.
        assertEquals("Alguém", texts.author(null));
    }

    @Test
    public void linhaDaMensagem() {
        PushTexts texts = new PushTexts(null);
        assertEquals("Oi", texts.messageLine("Oi", "", ""));
        assertEquals("Anexo: foto.jpg", texts.messageLine("", "foto.jpg", ""));
        assertEquals("Oi", texts.messageLine("Oi", "foto.jpg", ""));
        assertEquals("Viagem · Oi", texts.messageLine("Oi", "", "Viagem"));
        assertEquals("Viagem", texts.messageLine("", "", "Viagem"));
    }

    @Test
    public void grupoGuardaAsUltimasLinhasEContaTodas() {
        PushGroup group = new PushGroup();
        for (int i = 1; i <= 8; i++) group.add("Marina", "msg " + i, 1000L + i);
        assertEquals(8, group.count);
        List<PushGroup.Line> lines = group.lines();
        assertEquals(PushGroup.MAX_LINES, lines.size());
        assertEquals("msg 4", lines.get(0).text);
        assertEquals("msg 8", lines.get(lines.size() - 1).text);

        PushGroup back = PushGroup.decode(group.encode());
        assertEquals(8, back.count);
        assertEquals(PushGroup.MAX_LINES, back.lines().size());
        assertEquals("msg 8", back.lines().get(PushGroup.MAX_LINES - 1).text);
        assertEquals(1008L, back.lines().get(PushGroup.MAX_LINES - 1).time);
        assertEquals("8 mensagens novas", new PushTexts(null).newMessages(back.count));
    }

    @Test
    public void separadoresNoTextoNaoEstragamOGrupo() {
        PushGroup group = new PushGroup();
        group.add("A\u001Fb", "linha\u001Ecom\u001Fseparador", 5L);
        PushGroup back = PushGroup.decode(group.encode());
        assertEquals(1, back.count);
        assertEquals("A b", back.lines().get(0).author);
        assertEquals("linha com separador", back.lines().get(0).text);
    }

    @Test
    public void grupoEstragadoRecomeca() {
        assertEquals(0, PushGroup.decode(null).count);
        assertEquals(0, PushGroup.decode("abc").count);
        assertTrue(PushGroup.decode("3\u001Equebrado").lines().isEmpty());
    }
}
