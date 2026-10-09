package br.com.a3sitsolutions.ffcom;

import android.content.Intent;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.SystemClock;
import android.provider.Settings;
import android.util.Log;
import androidx.activity.result.ActivityResult;
import androidx.core.content.FileProvider;
import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.ActivityCallback;
import com.getcapacitor.annotation.CapacitorPlugin;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.security.DigestInputStream;
import java.security.MessageDigest;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;

// Atualização do próprio APK (fase 2 do docs/android-runbook.md; ver
// docs/architecture.md, "Decisão: atualização do APK (fase 2)"). Faz para o
// app Android o que o electron-updater faz para o desktop: consulta o
// latest.json do índice android-stable ao abrir o app, ao voltar a ele e a
// cada 30 min de relógio (contando o celular dormindo), baixa
// o APK novo em segundo plano para a pasta privada do app, confere o sha256
// e só então avisa a parte web (evento "updateReady"), que acende o mesmo
// botão verde (client/src/lib/nativeUpdater.ts). Tocar no botão chama
// install(), que abre o instalador do Android.
//
// Falha de rede, GitHub fora ou índice estranho: só registra no logcat e
// tenta de novo no próximo ciclo. APK com sha256 diferente é apagado e nunca
// oferecido. Cada passo deixa uma linha Log.i com a tag FfcomUpdate, para o
// build de release ser diagnosticado pelo logcat.
@CapacitorPlugin(name = "FfcomUpdate")
public class UpdatePlugin extends Plugin {

    private static final String TAG = "FfcomUpdate";
    // Intervalo entre consultas com o app aberto, e o mínimo ao voltar a ele.
    // O executor só acorda a cada TICK_MINUTES e confere pelo relógio
    // (ApkUpdates.isCheckDue): um timer de 30 min do ScheduledExecutorService
    // conta pelo System.nanoTime, que para com o celular dormindo, e não roda
    // com o processo congelado em segundo plano.
    private static final long CHECK_INTERVAL_MS = TimeUnit.MINUTES.toMillis(30);
    private static final long RESUME_CHECK_INTERVAL_MS = TimeUnit.MINUTES.toMillis(5);
    private static final long TICK_MINUTES = 5;
    private static final int CONNECT_TIMEOUT_MS = 15_000;
    private static final int READ_TIMEOUT_MS = 60_000;
    private static final int MAX_INDEX_BYTES = 64 * 1024;
    // Pasta dentro de getFilesDir(), exposta só pelo FileProvider
    // (res/xml/file_paths.xml) para o instalador ler.
    private static final String UPDATES_DIR = "updates";
    private static final String APK_MIME = "application/vnd.android.package-archive";

    private ScheduledExecutorService executor;
    private long installedVersionCode;
    // elapsedRealtime da última consulta; -1 antes da primeira.
    private volatile long lastCheckMs = -1;

    // APK pronto (baixado e conferido) e a versão dele; null sem atualização.
    private volatile File readyApk;
    private volatile String readyVersion;
    private volatile long readyVersionCode;

    // Último APK que veio com sha256 errado, para não baixar de novo o mesmo
    // arquivo a cada 30 min. Um latest.json novo (outro sha256) tenta outra vez.
    private String rejectedSha256;

    @Override
    public void load() {
        PackageInfo info;
        try {
            info = getContext().getPackageManager().getPackageInfo(getContext().getPackageName(), 0);
        } catch (PackageManager.NameNotFoundException e) {
            Log.w(TAG, "sem PackageInfo do próprio app", e);
            return;
        }
        installedVersionCode = info.getLongVersionCode();
        // Build local (sem -PffcomVersionName): assinado com outra chave, o APK
        // oficial não instala por cima. Como o desktop fora do app empacotado,
        // não procura atualização.
        if ("0.0.0".equals(info.versionName)) {
            Log.i(TAG, "build local (0.0.0): atualização do APK desligada");
            return;
        }
        Log.i(TAG, "atualização do APK ligada; instalado " + info.versionName + " (" + installedVersionCode + ")");
        executor = Executors.newSingleThreadScheduledExecutor();
        executor.scheduleWithFixedDelay(() -> checkIfDue(CHECK_INTERVAL_MS), 0, TICK_MINUTES, TimeUnit.MINUTES);
    }

    // Voltar ao app também consulta. Sem isso, com o processo vivo em segundo
    // plano (o caso comum: o Android não recria a Activity ao reabrir pelos
    // recentes ou por uma notificação), a única consulta era a da criação da
    // Activity e o timer não andava: uma versão nova não aparecia por horas.
    @Override
    protected void handleOnResume() {
        ScheduledExecutorService ex = executor;
        if (ex == null || ex.isShutdown()) return;
        try {
            ex.execute(() -> checkIfDue(RESUME_CHECK_INTERVAL_MS));
        } catch (RuntimeException e) {
            Log.w(TAG, "consulta ao voltar não agendada", e);
        }
    }

    // Roda no executor (uma consulta por vez). Pega Throwable: uma exceção
    // que escapasse cancelaria em silêncio o scheduleWithFixedDelay.
    private void checkIfDue(long minIntervalMs) {
        try {
            long now = SystemClock.elapsedRealtime();
            if (!ApkUpdates.isCheckDue(now, lastCheckMs, minIntervalMs)) return;
            lastCheckMs = now;
            check();
        } catch (Throwable t) {
            Log.w(TAG, "consulta falhou", t);
        }
    }

    @Override
    protected void handleOnDestroy() {
        if (executor != null) executor.shutdownNow();
    }

    // Se já há APK pronto. A parte web pergunta ao carregar, porque o evento
    // pode ter saído antes do listener existir (ele também fica retido).
    @PluginMethod
    public void getUpdateReady(PluginCall call) {
        call.resolve(readyInfo());
    }

    // true quando o Android já deixa este app instalar APKs ("Permitir desta
    // fonte"). Falso, o client explica antes de levar às configurações.
    @PluginMethod
    public void canInstall(PluginCall call) {
        JSObject ret = new JSObject();
        ret.put("granted", canRequestPackageInstalls());
        call.resolve(ret);
    }

    // Abre "Instalar apps desconhecidos" deste app e resolve com a situação
    // ao voltar.
    @PluginMethod
    public void openInstallSettings(PluginCall call) {
        Intent intent = new Intent(
            Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
            Uri.parse("package:" + getContext().getPackageName())
        );
        startActivityForResult(call, intent, "installSettingsResult");
    }

    @ActivityCallback
    private void installSettingsResult(PluginCall call, ActivityResult result) {
        if (call == null) return;
        JSObject ret = new JSObject();
        ret.put("granted", canRequestPackageInstalls());
        call.resolve(ret);
    }

    // Abre o instalador do Android com o APK pronto. Rejeita com
    // "needs-permission" se a pessoa ainda não liberou esta fonte e com
    // "not-ready" se não há APK conferido.
    @PluginMethod
    public void install(PluginCall call) {
        File apk = readyApk;
        if (apk == null || !apk.isFile()) {
            call.reject("Nenhuma atualização pronta", "not-ready");
            return;
        }
        if (!canRequestPackageInstalls()) {
            call.reject("Falta liberar a instalação desta fonte", "needs-permission");
            return;
        }
        Uri uri = FileProvider.getUriForFile(getContext(), getContext().getPackageName() + ".fileprovider", apk);
        Intent intent = new Intent(Intent.ACTION_VIEW);
        intent.setDataAndType(uri, APK_MIME);
        intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            getContext().startActivity(intent);
            call.resolve();
        } catch (RuntimeException e) {
            Log.w(TAG, "instalador não abriu", e);
            call.reject("O instalador não abriu", "install-failed", e);
        }
    }

    private boolean canRequestPackageInstalls() {
        return getContext().getPackageManager().canRequestPackageInstalls();
    }

    private JSObject readyInfo() {
        JSObject ret = new JSObject();
        File apk = readyApk;
        boolean ready = apk != null && apk.isFile();
        ret.put("ready", ready);
        if (ready) {
            ret.put("version", readyVersion);
            ret.put("versionCode", readyVersionCode);
        }
        return ret;
    }

    // Um ciclo: lê o índice, baixa e confere se houver versão nova. Roda no
    // executor, fora da thread principal. Qualquer falha só espera o próximo.
    private void check() {
        try {
            Log.i(TAG, "consultando latest.json (instalado " + installedVersionCode + ")");
            File dir = new File(getContext().getFilesDir(), UPDATES_DIR);
            ApkUpdates.Index index = ApkUpdates.parseIndex(fetchIndex());
            long versionCode = index.versionCode;
            String version = index.version;
            String url = index.url;
            String sha256 = index.sha256;
            Log.i(TAG, "latest.json: " + version + " (" + versionCode + ")");

            if (!ApkUpdates.isNewer(versionCode, installedVersionCode)) {
                Log.i(TAG, "sem versão nova");
                cleanup(dir, 0);
                return;
            }
            if (!ApkUpdates.isAllowedApkUrl(url)) {
                Log.w(TAG, "url fora das releases do FFCom: " + url);
                return;
            }
            if (readyApk != null && readyVersionCode == versionCode && readyApk.isFile()) {
                Log.i(TAG, "APK " + version + " já pronto");
                return;
            }
            if (sha256.equalsIgnoreCase(rejectedSha256)) {
                Log.i(TAG, "APK " + version + " já recusado pelo sha256");
                return;
            }
            Log.i(TAG, "versão nova " + version + "; preparando o APK");

            if (!dir.isDirectory() && !dir.mkdirs()) throw new IOException("sem pasta " + dir);
            cleanup(dir, versionCode);
            File apk = new File(dir, ApkUpdates.apkFileName(versionCode));
            // Já baixado num ciclo ou abertura anterior: confere de novo antes de oferecer.
            if (apk.isFile()) {
                boolean ok = ApkUpdates.sha256Matches(sha256Of(apk), sha256);
                Log.i(TAG, "APK já baixado; sha256 " + (ok ? "confere" : "não confere, baixando de novo"));
                if (!ok && !apk.delete()) throw new IOException("não apagou " + apk);
            }
            if (!apk.isFile()) {
                File part = new File(dir, ApkUpdates.partialFileName(versionCode));
                String actual = download(url, part);
                Log.i(TAG, "download de " + version + " terminou (" + part.length() + " bytes)");
                boolean ok = ApkUpdates.sha256Matches(actual, sha256);
                Log.i(TAG, "sha256 " + (ok ? "confere" : "não confere"));
                if (!ok) {
                    Log.w(TAG, "sha256 diferente do latest.json; APK " + version + " descartado");
                    rejectedSha256 = sha256;
                    //noinspection ResultOfMethodCallIgnored
                    part.delete();
                    return;
                }
                if (!part.renameTo(apk)) throw new IOException("não renomeou " + part);
            }

            readyVersion = version;
            readyVersionCode = versionCode;
            readyApk = apk;
            // retainUntilConsumed: se a página ainda não pôs o listener, recebe ao pôr.
            notifyListeners("updateReady", readyInfo(), true);
            Log.i(TAG, "APK " + version + " pronto; evento updateReady enviado");
        } catch (Exception e) {
            Log.i(TAG, "sem atualização agora (" + e + "); tenta de novo no próximo ciclo");
        }
    }

    // Apaga da pasta tudo que não é o APK de keepVersionCode: downloads
    // interrompidos, versões já instaladas ou substituídas.
    private void cleanup(File dir, long keepVersionCode) {
        File[] files = dir.listFiles();
        if (files == null) return;
        for (File f : files) {
            if (ApkUpdates.isStale(f.getName(), keepVersionCode) && !f.delete()) {
                Log.w(TAG, "não apagou " + f);
            }
        }
        File ready = readyApk;
        if (ready != null && !ready.isFile()) readyApk = null;
    }

    private String fetchIndex() throws IOException {
        HttpURLConnection conn = open(ApkUpdates.INDEX_URL);
        try (InputStream in = conn.getInputStream(); ByteArrayOutputStream out = new ByteArrayOutputStream()) {
            byte[] buffer = new byte[8 * 1024];
            int read;
            while ((read = in.read(buffer)) != -1) {
                out.write(buffer, 0, read);
                if (out.size() > MAX_INDEX_BYTES) throw new IOException("latest.json grande demais");
            }
            return new String(out.toByteArray(), StandardCharsets.UTF_8);
        } finally {
            conn.disconnect();
        }
    }

    // Baixa para `part` calculando o sha256 no caminho; devolve o hex.
    private String download(String url, File part) throws IOException {
        HttpURLConnection conn = open(url);
        MessageDigest digest = ApkUpdates.newSha256();
        try (
            InputStream in = new DigestInputStream(conn.getInputStream(), digest);
            OutputStream out = new FileOutputStream(part)
        ) {
            byte[] buffer = new byte[64 * 1024];
            int read;
            while ((read = in.read(buffer)) != -1) out.write(buffer, 0, read);
        } catch (IOException e) {
            //noinspection ResultOfMethodCallIgnored
            part.delete();
            throw e;
        } finally {
            conn.disconnect();
        }
        return ApkUpdates.toHex(digest.digest());
    }

    private static String sha256Of(File file) throws IOException {
        try (InputStream in = new FileInputStream(file)) {
            return ApkUpdates.sha256Hex(in);
        }
    }

    // GET com timeouts. O GitHub redireciona os downloads de release para
    // outro host https; o HttpURLConnection segue (mesmo protocolo).
    private static HttpURLConnection open(String url) throws IOException {
        HttpURLConnection conn = (HttpURLConnection) new URL(url).openConnection();
        conn.setConnectTimeout(CONNECT_TIMEOUT_MS);
        conn.setReadTimeout(READ_TIMEOUT_MS);
        conn.setInstanceFollowRedirects(true);
        conn.setRequestProperty("Accept", "application/octet-stream, application/json");
        int status = conn.getResponseCode();
        if (status != HttpURLConnection.HTTP_OK) {
            conn.disconnect();
            throw new IOException("HTTP " + status + " em " + url);
        }
        return conn;
    }
}
