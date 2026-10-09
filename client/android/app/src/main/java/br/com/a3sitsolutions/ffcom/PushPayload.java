package br.com.a3sitsolutions.ffcom;

import java.util.Map;

// Os dados de uma mensagem FCM do FFCom, lidos do formato "v1" documentado em
// docs/protocol.md, "Notificações push" (montado em
// server-central/internal/push/payload.go). Todos os valores chegam como
// texto. Campo desconhecido é ignorado; outro "v" ou outro "type" não é
// mostrado, porque o sentido dos campos pode ter mudado. Classe pura, sem
// Android, para os testes JUnit (PushPayloadTest).
final class PushPayload {

    static final String SCHEMA_VERSION = "1";

    static final String TYPE_CHANNEL_MESSAGE = "channel_message";
    static final String TYPE_DM = "dm";
    static final String TYPE_FRIEND_REQUEST = "friend_request";
    static final String TYPE_FRIEND_ACCEPTED = "friend_accepted";

    final String type;
    // channel_message
    final String serverAddress;
    final String serverName;
    final String channelId;
    final String channelName;
    final String text;
    final String messageId;
    final String threadId;
    final String threadTitle;
    final String attachment;
    // dm e friend_*: a conta de quem mandou, pediu ou aceitou.
    final String accountId;
    final String requestId;
    // Todos; pode vir vazio em dm e friend_* (conta sem nome de exibição).
    final String author;
    final String sentAt;

    private PushPayload(Map<String, String> data, String type) {
        this.type = type;
        this.serverAddress = value(data, "serverAddress");
        this.serverName = value(data, "serverName");
        this.channelId = value(data, "channelId");
        this.channelName = value(data, "channelName");
        this.text = value(data, "text");
        this.messageId = value(data, "messageId");
        this.threadId = value(data, "threadId");
        this.threadTitle = value(data, "threadTitle");
        this.attachment = value(data, "attachment");
        this.accountId = value(data, "accountId");
        this.requestId = value(data, "requestId");
        this.author = value(data, "author");
        this.sentAt = value(data, "sentAt");
    }

    // null quando a mensagem não é deste formato ou falta o que identifica
    // o destino do toque (servidor e canal, ou a conta).
    static PushPayload parse(Map<String, String> data) {
        if (data == null || !SCHEMA_VERSION.equals(data.get("v"))) return null;
        String type = value(data, "type");
        PushPayload payload = new PushPayload(data, type);
        switch (type) {
            case TYPE_CHANNEL_MESSAGE:
                return payload.serverAddress.isEmpty() || payload.channelId.isEmpty() ? null : payload;
            case TYPE_DM:
            case TYPE_FRIEND_REQUEST:
            case TYPE_FRIEND_ACCEPTED:
                return payload.accountId.isEmpty() ? null : payload;
            default:
                return null;
        }
    }

    boolean isChannelMessage() {
        return TYPE_CHANNEL_MESSAGE.equals(type);
    }

    boolean isDm() {
        return TYPE_DM.equals(type);
    }

    boolean isFriend() {
        return TYPE_FRIEND_REQUEST.equals(type) || TYPE_FRIEND_ACCEPTED.equals(type);
    }

    // Chave do agrupamento: uma notificação por canal (servidor + canal), uma
    // por conversa de DM e uma por pessoa nos avisos de amizade.
    String groupKey() {
        if (isChannelMessage()) return channelKey(serverAddress, channelId);
        if (isDm()) return dmKey(accountId);
        return "friend\n" + accountId;
    }

    static String channelKey(String serverAddress, String channelId) {
        return "channel\n" + serverAddress + "\n" + channelId;
    }

    static String dmKey(String accountId) {
        return "dm\n" + accountId;
    }

    // Id da notificação no Android, estável por chave. Sempre positivo e
    // acima de 0xFFFF, longe do 1001 da notificação "Em chamada"
    // (CallService).
    static int notificationId(String groupKey) {
        return (groupKey.hashCode() & 0x3fffffff) | 0x10000;
    }

    private static String value(Map<String, String> data, String key) {
        String v = data.get(key);
        return v == null ? "" : v.trim();
    }
}
