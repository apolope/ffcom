package br.com.a3sitsolutions.ffcom;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import org.junit.Test;

public class ApkUpdatesTest {

    // sha256("abc"), vetor do FIPS 180-2.
    private static final String ABC_SHA256 = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad";

    @Test
    public void newerOnlyWhenVersionCodeGrows() {
        assertTrue(ApkUpdates.isNewer(23002, 23001));
        assertTrue(ApkUpdates.isNewer(1_000_000, 999_999));
        assertFalse(ApkUpdates.isNewer(23001, 23001));
        assertFalse(ApkUpdates.isNewer(23000, 23001));
    }

    @Test
    public void sha256OfStream() throws IOException {
        String hex = ApkUpdates.sha256Hex(new ByteArrayInputStream("abc".getBytes(StandardCharsets.US_ASCII)));
        assertEquals(ABC_SHA256, hex);
    }

    @Test
    public void sha256Comparison() {
        assertTrue(ApkUpdates.sha256Matches(ABC_SHA256, ABC_SHA256));
        assertTrue(ApkUpdates.sha256Matches(ABC_SHA256, ABC_SHA256.toUpperCase() + "\n"));
        assertFalse(ApkUpdates.sha256Matches(ABC_SHA256, ABC_SHA256.replace('a', 'b')));
        assertFalse(ApkUpdates.sha256Matches(ABC_SHA256, ""));
        assertFalse(ApkUpdates.sha256Matches(ABC_SHA256, null));
        assertFalse(ApkUpdates.sha256Matches("", ""));
        assertFalse(ApkUpdates.sha256Matches(ABC_SHA256.substring(2), ABC_SHA256.substring(2)));
    }

    @Test
    public void apkUrlMustComeFromClientReleases() {
        assertTrue(ApkUpdates.isAllowedApkUrl(
            "https://github.com/apolope/ffcom/releases/download/client-v0.24.0/FFCom-0.24.0.apk"));
        assertFalse(ApkUpdates.isAllowedApkUrl(
            "http://github.com/apolope/ffcom/releases/download/client-v0.24.0/FFCom-0.24.0.apk"));
        assertFalse(ApkUpdates.isAllowedApkUrl(
            "https://github.com/outro/ffcom/releases/download/client-v0.24.0/FFCom-0.24.0.apk"));
        assertFalse(ApkUpdates.isAllowedApkUrl(
            "https://github.com/apolope/ffcom/releases/download/client-v0.24.0/../../x/FFCom.apk"));
        assertFalse(ApkUpdates.isAllowedApkUrl(
            "https://github.com/apolope/ffcom/releases/download/client-v0.24.0/FFCom.apk?x=.apk"));
        assertFalse(ApkUpdates.isAllowedApkUrl(
            "https://github.com/apolope/ffcom/releases/download/client-v0.24.0/latest.json"));
        assertFalse(ApkUpdates.isAllowedApkUrl(null));
    }

    // Conteúdo real do android-stable/latest.json da client-v0.24.2, que o
    // 0.24.0 (24000) devia ter oferecido.
    @Test
    public void realLatestJsonIsNewerThanInstalled() throws Exception {
        ApkUpdates.Index index = ApkUpdates.parseIndex("{\n"
            + "  \"version\": \"0.24.2\",\n"
            + "  \"versionCode\": 24002,\n"
            + "  \"url\": \"https://github.com/apolope/ffcom/releases/download/client-v0.24.2/FFCom-0.24.2.apk\",\n"
            + "  \"sha256\": \"411e330d12e1907abfc97e46dbdc4131fe5fcbac6006c4c955f6c716489ab9ed\"\n"
            + "}\n");
        assertEquals("0.24.2", index.version);
        assertEquals(24002, index.versionCode);
        assertTrue(ApkUpdates.isNewer(index.versionCode, 24000));
        assertTrue(ApkUpdates.isAllowedApkUrl(index.url));
        assertTrue(ApkUpdates.sha256Matches(index.sha256, index.sha256));
    }

    @Test(expected = org.json.JSONException.class)
    public void latestJsonWithoutVersionCodeIsRejected() throws Exception {
        ApkUpdates.parseIndex("{\"version\": \"0.24.2\", \"url\": \"x\", \"sha256\": \"y\"}");
    }

    // A consulta ao voltar ao app não esperava: o timer de 30 min do executor
    // não anda com o celular dormindo nem com o processo congelado.
    @Test
    public void checkIsDueByWallClock() {
        long min = 60_000;
        assertTrue(ApkUpdates.isCheckDue(5_000, -1, 30 * min));
        assertFalse(ApkUpdates.isCheckDue(10 * min, 6 * min, 5 * min));
        assertTrue(ApkUpdates.isCheckDue(11 * min, 6 * min, 5 * min));
        assertFalse(ApkUpdates.isCheckDue(35 * min, 6 * min, 30 * min));
        assertTrue(ApkUpdates.isCheckDue(36 * min, 6 * min, 30 * min));
    }

    @Test
    public void staleFilesAreEverythingButTheKeptApk() {
        assertFalse(ApkUpdates.isStale("FFCom-24000.apk", 24000));
        assertTrue(ApkUpdates.isStale("FFCom-23001.apk", 24000));
        assertTrue(ApkUpdates.isStale(ApkUpdates.partialFileName(24000), 24000));
        assertTrue(ApkUpdates.isStale("FFCom-24000.apk", 0));
    }
}
