package io.github.nebuloss.agendouille;

import android.content.Context;
import android.os.Build;

import java.io.File;
import java.io.FileOutputStream;
import java.io.PrintWriter;
import java.io.StringWriter;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.Date;

/**
 * What made the app stop, kept for its next start: an exception nobody
 * caught, or the page's renderer gone. The screens show it, to copy or to
 * send to the server's log (/api/app/crash): a phone's own log is out of
 * reach of a family's administrator.
 */
final class CrashLog {
    private static final String FILE = "last-crash.txt";

    private CrashLog() {}

    /** Records every uncaught exception before the process dies. */
    static void install(Context ctx) {
        Context app = ctx.getApplicationContext();
        Thread.UncaughtExceptionHandler before = Thread.getDefaultUncaughtExceptionHandler();
        if (before instanceof Handler) return;
        Thread.setDefaultUncaughtExceptionHandler(new Handler(app, before));
    }

    private static final class Handler implements Thread.UncaughtExceptionHandler {
        private final Context ctx;
        private final Thread.UncaughtExceptionHandler next;

        Handler(Context ctx, Thread.UncaughtExceptionHandler next) {
            this.ctx = ctx;
            this.next = next;
        }

        @Override
        public void uncaughtException(Thread t, Throwable e) {
            StringWriter w = new StringWriter();
            e.printStackTrace(new PrintWriter(w));
            write(ctx, "thread " + t.getName() + "\n" + w);
            if (next != null) next.uncaughtException(t, e);
        }
    }

    /** Keeps one report, with what identifies the phone and the app. */
    static void write(Context ctx, String what) {
        String version;
        try {
            version = ctx.getPackageManager().getPackageInfo(ctx.getPackageName(), 0).versionName;
        } catch (Exception e) {
            version = "?";
        }
        String report = "agendouille " + version + " on " + Build.MANUFACTURER + " " + Build.MODEL
                + ", Android " + Build.VERSION.RELEASE + " (" + Build.VERSION.SDK_INT + "), " + new Date() + "\n" + what;
        if (report.length() > 12000) report = report.substring(0, 12000);
        try (FileOutputStream out = new FileOutputStream(new File(ctx.getFilesDir(), FILE))) {
            out.write(report.getBytes(StandardCharsets.UTF_8));
        } catch (Exception ignored) {
            // nowhere to write: nothing more to do while dying
        }
    }

    /** The last report, or null. */
    static String read(Context ctx) {
        File f = new File(ctx.getFilesDir(), FILE);
        if (!f.exists()) return null;
        try {
            return new String(Files.readAllBytes(f.toPath()), StandardCharsets.UTF_8);
        } catch (Exception e) {
            return null;
        }
    }

    static void clear(Context ctx) {
        //noinspection ResultOfMethodCallIgnored
        new File(ctx.getFilesDir(), FILE).delete();
    }
}
