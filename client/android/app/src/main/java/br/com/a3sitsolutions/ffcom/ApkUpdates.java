package br.com.a3sitsolutions.ffcom;

import java.io.IOException;
import java.io.InputStream;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.Locale;
import org.json.JSONException;
import org.json.JSONObject;

// Regras puras da atualização do APK (sem Android), separadas do
// UpdatePlugin para serem testadas por JUnit (app/src/test). Ver
// docs/architecture.md, "Decisão: atualização do APK (fase 2)".
final class ApkUpdates {

    // Índice publicado pelo job android do deploy-ffcom-client.yml:
    // {version, versionCode, url, sha256}.
    static final String INDEX_URL = "https://github.com/apolope/ffcom/releases/download/android-stable/latest.json";

    // O APK só é baixado das releases client-v* do próprio repositório. Um
    // latest.json adulterado não leva o app a baixar de outro lugar (e,
    // mesmo assim, o Android só instala por cima um APK com a nossa chave).
    static final String APK_URL_PREFIX = "https://github.com/apolope/ffcom/releases/download/client-v";

    private static final String APK_PREFIX = "FFCom-";
    private static final String APK_SUFFIX = ".apk";

    private ApkUpdates() {}

    // Conteúdo do latest.json.
    static final class Index {
        final String version;
        final long versionCode;
        final String url;
        final String sha256;

        Index(String version, long versionCode, String url, String sha256) {
            this.version = version;
            this.versionCode = versionCode;
            this.url = url;
            this.sha256 = sha256;
        }
    }

    // Lê o latest.json; JSONException (campo faltando ou de outro tipo)
    // vira "sem atualização agora" no UpdatePlugin.
    static Index parseIndex(String json) throws JSONException {
        JSONObject index = new JSONObject(json);
        return new Index(
            index.getString("version"),
            index.getLong("versionCode"),
            index.getString("url"),
            index.getString("sha256")
        );
    }

    // Se já é hora de consultar o latest.json de novo. Os tempos são do
    // SystemClock.elapsedRealtime(), que conta também o celular dormindo
    // (o System.nanoTime do ScheduledExecutorService não conta, e um
    // processo congelado em segundo plano não roda timer nenhum).
    // lastCheckMs < 0: ainda não consultou neste processo.
    static boolean isCheckDue(long nowMs, long lastCheckMs, long minIntervalMs) {
        return lastCheckMs < 0 || nowMs - lastCheckMs >= minIntervalMs;
    }

    // O Android só instala por cima um versionCode maior que o instalado.
    static boolean isNewer(long remoteVersionCode, long installedVersionCode) {
        return remoteVersionCode > installedVersionCode;
    }

    static boolean isAllowedApkUrl(String url) {
        if (url == null || !url.startsWith(APK_URL_PREFIX) || !url.endsWith(APK_SUFFIX)) return false;
        String rest = url.substring("https://".length());
        return !rest.contains("..") && !rest.contains("?") && !rest.contains("#") && !rest.contains("\\");
    }

    // Nome do APK baixado (e já conferido) para um versionCode.
    static String apkFileName(long versionCode) {
        return APK_PREFIX + versionCode + APK_SUFFIX;
    }

    // Nome do arquivo enquanto baixa; só vira apkFileName depois do sha256.
    static String partialFileName(long versionCode) {
        return apkFileName(versionCode) + ".part";
    }

    // Tudo na pasta de atualizações que não é o APK pronto que se quer
    // manter é sobra: downloads interrompidos, versões antigas ou já
    // instaladas. keepVersionCode <= 0 apaga tudo.
    static boolean isStale(String fileName, long keepVersionCode) {
        return keepVersionCode <= 0 || !apkFileName(keepVersionCode).equals(fileName);
    }

    static String sha256Hex(InputStream in) throws IOException {
        MessageDigest digest = newSha256();
        byte[] buffer = new byte[64 * 1024];
        int read;
        while ((read = in.read(buffer)) != -1) digest.update(buffer, 0, read);
        return toHex(digest.digest());
    }

    static MessageDigest newSha256() {
        try {
            return MessageDigest.getInstance("SHA-256");
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException(e);
        }
    }

    static String toHex(byte[] bytes) {
        StringBuilder sb = new StringBuilder(bytes.length * 2);
        for (byte b : bytes) sb.append(String.format(Locale.ROOT, "%02x", b & 0xff));
        return sb.toString();
    }

    // O sha256 do latest.json sai do sha256sum (hex minúsculo); aceita
    // maiúsculas, mas nunca um valor vazio ou de outro tamanho.
    static boolean sha256Matches(String actualHex, String expectedHex) {
        if (actualHex == null || expectedHex == null) return false;
        String expected = expectedHex.trim().toLowerCase(Locale.ROOT);
        if (!expected.matches("[0-9a-f]{64}")) return false;
        return expected.equals(actualHex.toLowerCase(Locale.ROOT));
    }
}
