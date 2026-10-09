package br.com.a3sitsolutions.ffcom;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.BitmapShader;
import android.graphics.Canvas;
import android.graphics.Matrix;
import android.graphics.Paint;
import android.graphics.Shader;
import android.util.Log;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.file.Files;
import java.util.Arrays;
import java.util.Comparator;

// A parte Android do avatar na notificação (PushAvatars tem as regras): o
// download por HttpURLConnection, o recorte em círculo e o cache em disco,
// em cacheDir/push-avatars/<chave>.png. O sistema pode apagar o cacheDir
// quando falta espaço; no pior caso o avatar é baixado de novo.
final class PushAvatarImages {

    private static final String TAG = "FfcomPush";
    private static final String DIR = "push-avatars";
    // Avatares guardados; acima disso, os usados há mais tempo saem.
    private static final int MAX_FILES = 200;

    private PushAvatarImages() {}

    // O avatar de quem mandou a notificação, ou null (fica a letra). Roda na
    // thread do onMessageReceived: baixa na hora, com prazo curto, só quando
    // o autor ainda não está no cache.
    static Bitmap load(Context context, PushPayload payload) {
        if (PushAvatars.cacheKey(payload) == null) return null;
        final File dir = new File(context.getCacheDir(), DIR);
        PushAvatars.Cache cache = new PushAvatars.Cache() {
            @Override
            public byte[] read(String key) {
                return readFile(dir, key);
            }

            @Override
            public void write(String key, byte[] image) {
                writeFile(dir, key, image);
            }
        };
        byte[] image = PushAvatars.resolve(payload, cache, PushAvatarImages::fetch, PushAvatarImages::circle);
        return decode(image);
    }

    // O avatar já no cache (linhas anteriores de uma notificação de canal),
    // sem rede.
    static Bitmap cached(Context context, String key) {
        if (key == null || key.isEmpty()) return null;
        return decode(readFile(new File(context.getCacheDir(), DIR), key));
    }

    private static Bitmap decode(byte[] image) {
        if (image == null) return null;
        try {
            return BitmapFactory.decodeByteArray(image, 0, image.length);
        } catch (RuntimeException | OutOfMemoryError e) {
            return null;
        }
    }

    private static byte[] fetch(String url, int maxBytes) throws IOException {
        long deadline = System.currentTimeMillis() + PushAvatars.BUDGET_MS;
        HttpURLConnection conn = (HttpURLConnection) new URL(url).openConnection();
        try {
            conn.setConnectTimeout(PushAvatars.CONNECT_TIMEOUT_MS);
            conn.setReadTimeout((int) Math.max(500, deadline - System.currentTimeMillis()));
            conn.setInstanceFollowRedirects(false);
            conn.setUseCaches(false);
            int status = conn.getResponseCode();
            if (status != HttpURLConnection.HTTP_OK) throw new IOException("avatar respondeu " + status);
            if (conn.getContentLengthLong() > maxBytes) throw new IOException("avatar grande demais");
            try (InputStream in = conn.getInputStream()) {
                return PushAvatars.readLimited(in, maxBytes, deadline);
            }
        } catch (IOException e) {
            Log.i(TAG, "avatar não baixou: " + e.getMessage());
            throw e;
        } finally {
            conn.disconnect();
        }
    }

    // Decodifica já reduzido (inSampleSize), corta o quadrado do meio em
    // SIZE_PX e recorta em círculo, como os avatares do app.
    private static byte[] circle(byte[] raw) {
        BitmapFactory.Options bounds = new BitmapFactory.Options();
        bounds.inJustDecodeBounds = true;
        BitmapFactory.decodeByteArray(raw, 0, raw.length, bounds);
        if (bounds.outWidth <= 0 || bounds.outHeight <= 0) return null;
        BitmapFactory.Options opts = new BitmapFactory.Options();
        opts.inSampleSize = PushAvatars.sampleSize(bounds.outWidth, bounds.outHeight, PushAvatars.SIZE_PX);
        Bitmap src = BitmapFactory.decodeByteArray(raw, 0, raw.length, opts);
        if (src == null) return null;

        int size = PushAvatars.SIZE_PX;
        int side = Math.min(src.getWidth(), src.getHeight());
        float scale = (float) size / side;
        Matrix matrix = new Matrix();
        matrix.setTranslate(-(src.getWidth() - side) / 2f, -(src.getHeight() - side) / 2f);
        matrix.postScale(scale, scale);
        BitmapShader shader = new BitmapShader(src, Shader.TileMode.CLAMP, Shader.TileMode.CLAMP);
        shader.setLocalMatrix(matrix);
        Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG | Paint.FILTER_BITMAP_FLAG);
        paint.setShader(shader);

        Bitmap out = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888);
        new Canvas(out).drawCircle(size / 2f, size / 2f, size / 2f, paint);
        src.recycle();
        ByteArrayOutputStream png = new ByteArrayOutputStream();
        out.compress(Bitmap.CompressFormat.PNG, 100, png);
        out.recycle();
        return png.toByteArray();
    }

    private static byte[] readFile(File dir, String key) {
        File file = new File(dir, key + ".png");
        if (!file.isFile()) return null;
        try {
            byte[] bytes = Files.readAllBytes(file.toPath());
            file.setLastModified(System.currentTimeMillis());
            return bytes;
        } catch (IOException e) {
            return null;
        }
    }

    // Grava num temporário e troca, para nunca deixar um arquivo pela
    // metade com o nome final.
    private static void writeFile(File dir, String key, byte[] image) {
        if (!dir.isDirectory() && !dir.mkdirs()) return;
        File tmp = new File(dir, key + ".tmp");
        try (FileOutputStream out = new FileOutputStream(tmp)) {
            out.write(image);
        } catch (IOException e) {
            tmp.delete();
            return;
        }
        if (!tmp.renameTo(new File(dir, key + ".png"))) tmp.delete();
        prune(dir);
    }

    private static void prune(File dir) {
        File[] files = dir.listFiles();
        if (files == null || files.length <= MAX_FILES) return;
        Arrays.sort(files, Comparator.comparingLong(File::lastModified));
        for (int i = 0; i < files.length - MAX_FILES; i++) files[i].delete();
    }
}
