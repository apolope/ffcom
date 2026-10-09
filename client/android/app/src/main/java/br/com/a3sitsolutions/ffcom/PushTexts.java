package br.com.a3sitsolutions.ffcom;

import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

// Textos das notificações de push. O FirebaseMessagingService roda sem a
// WebView (app fechado), então não tem o i18n do client: a parte web
// (client/src/lib/androidPush.ts) manda os modelos já traduzidos no idioma
// escolhido, com marcadores {author}, {channel}, {server}, {name} e {n}, e o
// PushPlugin os guarda em SharedPreferences. Enquanto a página não mandou
// nada (primeiro push antes de abrir o app), valem os padrões em pt-BR
// abaixo. Classe pura, sem Android, para os testes JUnit (PushTextsTest).
final class PushTexts {

    // Nomes dos canais de notificação nas configurações do Android.
    static final String CHANNEL_MESSAGES = "channelMessages";
    static final String CHANNEL_DMS = "channelDms";
    static final String CHANNEL_FRIENDS = "channelFriends";
    // "#{channel} · {server}": título da notificação de um canal.
    static final String CHANNEL_TITLE = "channelTitle";
    // "{n} mensagens novas" (2 ou mais) e "1 mensagem nova".
    static final String NEW_MESSAGES_ONE = "newMessagesOne";
    static final String NEW_MESSAGES_MANY = "newMessagesMany";
    // "Nova mensagem de {author}": a DM é cifrada, o texto não vem.
    static final String DM_TITLE = "dmTitle";
    static final String FRIEND_REQUEST = "friendRequest";
    static final String FRIEND_ACCEPTED = "friendAccepted";
    // "Anexo: {name}": mensagem só com anexo.
    static final String ATTACHMENT = "attachment";
    // Autor vazio.
    static final String SOMEONE = "someone";
    // A própria pessoa no MessagingStyle (o Android exige um "eu").
    static final String YOU = "you";

    static final Map<String, String> DEFAULTS;

    static {
        Map<String, String> d = new HashMap<>();
        d.put(CHANNEL_MESSAGES, "Mensagens dos canais");
        d.put(CHANNEL_DMS, "Mensagens diretas");
        d.put(CHANNEL_FRIENDS, "Amizades");
        d.put(CHANNEL_TITLE, "#{channel} · {server}");
        d.put(NEW_MESSAGES_ONE, "1 mensagem nova");
        d.put(NEW_MESSAGES_MANY, "{n} mensagens novas");
        d.put(DM_TITLE, "Nova mensagem de {author}");
        d.put(FRIEND_REQUEST, "{author} mandou um pedido de amizade");
        d.put(FRIEND_ACCEPTED, "{author} aceitou seu pedido de amizade");
        d.put(ATTACHMENT, "Anexo: {name}");
        d.put(SOMEONE, "Alguém");
        d.put(YOU, "Você");
        DEFAULTS = Collections.unmodifiableMap(d);
    }

    private final Map<String, String> templates;

    PushTexts(Map<String, String> stored) {
        Map<String, String> t = new HashMap<>(DEFAULTS);
        if (stored != null) {
            for (String key : DEFAULTS.keySet()) {
                String value = stored.get(key);
                if (value != null && !value.trim().isEmpty()) t.put(key, value);
            }
        }
        this.templates = t;
    }

    String get(String key) {
        String value = templates.get(key);
        return value == null ? "" : value;
    }

    String author(String author) {
        return isBlank(author) ? get(SOMEONE) : author;
    }

    String channelTitle(String channelName, String serverName) {
        return get(CHANNEL_TITLE).replace("{channel}", channelName).replace("{server}", serverName).trim();
    }

    // O que vai na linha de uma mensagem de canal: o texto, ou o anexo quando
    // é só anexo, com o título da thread na frente nos fóruns.
    String messageLine(String text, String attachment, String threadTitle) {
        String line = text;
        if (isBlank(line)) {
            line = isBlank(attachment) ? "" : get(ATTACHMENT).replace("{name}", attachment);
        }
        if (!isBlank(threadTitle)) {
            line = line.isEmpty() ? threadTitle : threadTitle + " · " + line;
        }
        return line;
    }

    String newMessages(int count) {
        if (count == 1) return get(NEW_MESSAGES_ONE);
        return get(NEW_MESSAGES_MANY).replace("{n}", Integer.toString(count));
    }

    String dmTitle(String author) {
        return get(DM_TITLE).replace("{author}", author(author));
    }

    String friendText(String type, String author) {
        String key = PushPayload.TYPE_FRIEND_REQUEST.equals(type) ? FRIEND_REQUEST : FRIEND_ACCEPTED;
        return get(key).replace("{author}", author(author));
    }

    private static boolean isBlank(String s) {
        return s == null || s.trim().isEmpty();
    }
}
