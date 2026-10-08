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

    @Test
    public void staleFilesAreEverythingButTheKeptApk() {
        assertFalse(ApkUpdates.isStale("FFCom-24000.apk", 24000));
        assertTrue(ApkUpdates.isStale("FFCom-23001.apk", 24000));
        assertTrue(ApkUpdates.isStale(ApkUpdates.partialFileName(24000), 24000));
        assertTrue(ApkUpdates.isStale("FFCom-24000.apk", 0));
    }
}
