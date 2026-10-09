package br.com.a3sitsolutions.ffcom;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotEquals;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;
import static org.junit.Assert.fail;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import org.junit.Test;

public class PushAvatarsTest {

    private static final String LINK = "https://central.exemplo.com/api/push/avatars/AbC-_123";
    private static final String KEY = "0123456789abcdef0123456789abcdef";

    private static Map<String, String> dm(String avatar, String key) {
        Map<String, String> data = new HashMap<>();
        data.put("v", "1");
        data.put("type", "dm");
        data.put("accountId", "a1");
        data.put("author", "Marina");
        if (avatar != null) data.put("authorAvatar", avatar);
        if (key != null) data.put("authorKey", key);
        return data;
    }

    private static PushPayload payload(String avatar, String key) {
        PushPayload p = PushPayload.parse(dm(avatar, key));
        assertNotNull(p);
        return p;
    }

    // Cache em memória que conta leituras e gravações.
    private static final class FakeCache implements PushAvatars.Cache {
        final Map<String, byte[]> files = new HashMap<>();
        int writes;

        @Override
        public byte[] read(String key) {
            return files.get(key);
        }

        @Override
        public void write(String key, byte[] image) {
            writes++;
            files.put(key, image);
        }
    }

    private static final class FakeFetcher implements PushAvatars.Fetcher {
        final List<String> urls = new ArrayList<>();
        byte[] response = {1, 2, 3};
        IOException error;

        @Override
        public byte[] fetch(String url, int maxBytes) throws IOException {
            urls.add(url);
            if (error != null) throw error;
            return response;
        }
    }

    private static final PushAvatars.Processor IDENTITY = raw -> raw;

    @Test
    public void payloadLeOsCamposDoAvatar() {
        PushPayload p = payload(LINK, KEY);
        assertEquals(LINK, p.authorAvatar);
        assertEquals(KEY, p.authorKey);
        PushPayload without = payload(null, null);
        assertEquals("", without.authorAvatar);
        assertEquals("", without.authorKey);
    }

    @Test
    public void chaveDoCacheEOAuthorKey() {
        assertEquals(KEY, PushAvatars.cacheKey(payload(LINK, KEY)));
        // Mesmo autor com outro link (renovado): mesma chave.
        assertEquals(KEY, PushAvatars.cacheKey(payload(LINK + "outro", KEY)));
        assertNull(PushAvatars.cacheKey(payload(null, KEY)));
        assertNull(PushAvatars.cacheKey(null));
    }

    @Test
    public void semAuthorKeyValidoUsaOHashDoLink() {
        String k1 = PushAvatars.cacheKey(payload(LINK, null));
        String k2 = PushAvatars.cacheKey(payload(LINK, "../../etc/passwd"));
        assertNotNull(k1);
        assertEquals(k1, k2);
        assertTrue(k1.matches("u[0-9a-f]{32}"));
        assertNotEquals(k1, PushAvatars.cacheKey(payload(LINK + "x", null)));
    }

    @Test
    public void soLinkHttpsComHost() {
        assertTrue(PushAvatars.acceptableUrl(LINK));
        assertFalse(PushAvatars.acceptableUrl("http://central.exemplo.com/api/push/avatars/x"));
        assertFalse(PushAvatars.acceptableUrl("https:///sem-host"));
        assertFalse(PushAvatars.acceptableUrl("file:///sdcard/x.png"));
        assertFalse(PushAvatars.acceptableUrl(""));
        assertNull(PushAvatars.cacheKey(payload("http://central.exemplo.com/x", KEY)));
    }

    @Test
    public void baixaUmaVezEDepoisUsaOCache() {
        FakeCache cache = new FakeCache();
        FakeFetcher fetcher = new FakeFetcher();
        assertArrayEquals(new byte[] {1, 2, 3}, PushAvatars.resolve(payload(LINK, KEY), cache, fetcher, IDENTITY));
        assertArrayEquals(new byte[] {1, 2, 3}, PushAvatars.resolve(payload(LINK + "novo", KEY), cache, fetcher, IDENTITY));
        assertEquals(1, fetcher.urls.size());
        assertEquals(LINK, fetcher.urls.get(0));
        assertEquals(1, cache.writes);
    }

    @Test
    public void falhaViraNullSemGravar() {
        FakeCache cache = new FakeCache();
        FakeFetcher fetcher = new FakeFetcher();

        fetcher.error = new IOException("timeout");
        assertNull(PushAvatars.resolve(payload(LINK, KEY), cache, fetcher, IDENTITY));

        fetcher.error = null;
        assertNull("imagem inválida", PushAvatars.resolve(payload(LINK, KEY), cache, fetcher, raw -> null));
        assertNull("processador quebrado", PushAvatars.resolve(payload(LINK, KEY), cache, fetcher, raw -> {
            throw new IllegalStateException("bitmap");
        }));
        fetcher.response = new byte[PushAvatars.MAX_BYTES + 1];
        assertNull("grande demais", PushAvatars.resolve(payload(LINK, KEY), cache, fetcher, IDENTITY));
        assertEquals(0, cache.writes);

        // Sem avatar ou com link recusado, nem tenta a rede.
        int calls = fetcher.urls.size();
        assertNull(PushAvatars.resolve(payload(null, null), cache, fetcher, IDENTITY));
        assertNull(PushAvatars.resolve(payload("http://x/y", KEY), cache, fetcher, IDENTITY));
        assertEquals(calls, fetcher.urls.size());
    }

    @Test
    public void leituraLimitada() throws IOException {
        long later = System.currentTimeMillis() + 60_000;
        byte[] ok = new byte[1000];
        assertEquals(1000, PushAvatars.readLimited(new ByteArrayInputStream(ok), 1000, later).length);
        try {
            PushAvatars.readLimited(new ByteArrayInputStream(new byte[1001]), 1000, later);
            fail("passou do limite");
        } catch (IOException expected) {
            // ok
        }
        try {
            PushAvatars.readLimited(new ByteArrayInputStream(ok), 1000, System.currentTimeMillis() - 1);
            fail("passou do prazo");
        } catch (IOException expected) {
            // ok
        }
    }

    @Test
    public void reducaoNaDecodificacao() {
        assertEquals(1, PushAvatars.sampleSize(160, 160, 160));
        assertEquals(1, PushAvatars.sampleSize(300, 300, 160));
        assertEquals(2, PushAvatars.sampleSize(512, 512, 160));
        assertEquals(4, PushAvatars.sampleSize(1280, 700, 160));
        assertEquals(1, PushAvatars.sampleSize(100, 100, 160));
    }

    @Test
    public void grupoGuardaAChaveDoAvatarPorLinha() {
        PushGroup group = new PushGroup();
        group.add("Marina", "oi", 1L, KEY);
        group.add("Bia", "olá", 2L);
        PushGroup back = PushGroup.decode(group.encode());
        assertEquals(KEY, back.lines().get(0).avatarKey);
        assertEquals("", back.lines().get(1).avatarKey);
        // Formato guardado antes do avatar (3 campos) continua lendo.
        PushGroup old = PushGroup.decode("1\u001E5\u001FMarina\u001Foi");
        assertEquals(1, old.lines().size());
        assertEquals("", old.lines().get(0).avatarKey);
    }
}
