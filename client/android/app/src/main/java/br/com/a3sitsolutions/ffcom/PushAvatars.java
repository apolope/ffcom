package br.com.a3sitsolutions.ffcom;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.regex.Pattern;

// O avatar de quem mandou, na notificação de push: qual a chave no cache em
// disco, quando baixar e o que fazer quando algo falha. Classe pura, sem
// Android, para os testes JUnit (PushAvatarsTest); o download, a imagem e o
// disco ficam em PushAvatarImages.
//
// O server-central manda "authorAvatar" (link https sem login, que vence em
// até 48 h) e "authorKey" (hash da conta com a versão do avatar, sem nada
// secreto). A chave do cache é o authorKey: estável enquanto a pessoa não
// troca a foto, então o mesmo autor não é baixado de novo a cada mensagem,
// e a foto nova (outro authorKey) é baixada na primeira mensagem depois da
// troca. O link não serve de chave, porque muda pelo menos a cada 24 h.
// Ver docs/architecture.md, "Decisão: notificações push (fase 6)", "Avatar
// na notificação".
final class PushAvatars {

    // Tamanho máximo do download. O central aceita avatar de até 2 MB, mas o
    // client recorta em até 512 px antes de mandar; acima disso, desiste.
    static final int MAX_BYTES = 1024 * 1024;
    // Orçamento do download inteiro (conexão mais leitura). O FCM dá cerca
    // de 10 s ao onMessageReceived; a notificação sai com a letra se passar.
    static final int BUDGET_MS = 5000;
    static final int CONNECT_TIMEOUT_MS = 2500;
    // Lado do avatar decodificado, em px.
    static final int SIZE_PX = 160;

    private static final Pattern AUTHOR_KEY = Pattern.compile("[0-9a-f]{16,64}");

    // O disco (cache) e a rede, trocados por falsos nos testes.
    interface Cache {
        byte[] read(String key);

        void write(String key, byte[] image);
    }

    interface Fetcher {
        // Os bytes do link, até maxBytes, ou exceção.
        byte[] fetch(String url, int maxBytes) throws IOException;
    }

    // Transforma o que foi baixado na imagem guardada (recortada em
    // círculo, PNG). null: não é imagem.
    interface Processor {
        byte[] process(byte[] raw);
    }

    private PushAvatars() {}

    // Chave do cache do avatar da notificação, ou null sem avatar. Sem
    // authorKey (não deveria acontecer), o hash do link: ainda evita baixar
    // o mesmo link duas vezes.
    static String cacheKey(PushPayload payload) {
        if (payload == null || !acceptableUrl(payload.authorAvatar)) return null;
        if (AUTHOR_KEY.matcher(payload.authorKey).matches()) return payload.authorKey;
        return "u" + sha256Hex(payload.authorAvatar).substring(0, 32);
    }

    // Só https com host: o link sai da rede sem login, e o app não baixa
    // nada por texto claro.
    static boolean acceptableUrl(String url) {
        if (url == null || !url.startsWith("https://")) return false;
        try {
            String host = new URI(url).getHost();
            return host != null && !host.isEmpty();
        } catch (Exception e) {
            return false;
        }
    }

    // A imagem do avatar (já processada), do cache ou baixada agora. null
    // em qualquer falha: sem avatar no payload, link estranho, rede, tamanho,
    // imagem inválida. Nunca lança.
    static byte[] resolve(PushPayload payload, Cache cache, Fetcher fetcher, Processor processor) {
        String key = cacheKey(payload);
        if (key == null) return null;
        try {
            byte[] cached = cache.read(key);
            if (cached != null && cached.length > 0) return cached;
            byte[] raw = fetcher.fetch(payload.authorAvatar, MAX_BYTES);
            if (raw == null || raw.length == 0 || raw.length > MAX_BYTES) return null;
            byte[] image = processor.process(raw);
            if (image == null || image.length == 0) return null;
            cache.write(key, image);
            return image;
        } catch (Exception | OutOfMemoryError e) {
            return null;
        }
    }

    // Lê in até maxBytes, desistindo se passar disso ou se o prazo
    // (deadlineMs, em System.currentTimeMillis) vencer no meio.
    static byte[] readLimited(InputStream in, int maxBytes, long deadlineMs) throws IOException {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        byte[] buf = new byte[8192];
        int n;
        while ((n = in.read(buf)) != -1) {
            if (out.size() + n > maxBytes) throw new IOException("avatar maior que " + maxBytes + " bytes");
            out.write(buf, 0, n);
            if (System.currentTimeMillis() > deadlineMs) throw new IOException("prazo do avatar vencido");
        }
        return out.toByteArray();
    }

    // O inSampleSize do BitmapFactory (potência de 2) que deixa o menor
    // lado da imagem ainda maior ou igual a target.
    static int sampleSize(int width, int height, int target) {
        int sample = 1;
        int side = Math.min(width, height);
        while (side / (sample * 2) >= target) sample *= 2;
        return sample;
    }

    static String sha256Hex(String s) {
        try {
            byte[] sum = MessageDigest.getInstance("SHA-256").digest(s.getBytes(StandardCharsets.UTF_8));
            StringBuilder sb = new StringBuilder(sum.length * 2);
            for (byte b : sum) sb.append(String.format("%02x", b));
            return sb.toString();
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException(e);
        }
    }
}
