package io.github.nebuloss.agendouille;

/**
 * What the screens may ask of the family API through the bridge ("api"):
 * managing calendars (create, edit, share, give, remove, hide), approving
 * a browser's sign-in, and listing and disconnecting devices and browsers,
 * nothing else. The app is a sync app: no
 * event is read through here (DECISIONS). Pure Java: tested on the JVM.
 */
public final class ApiPath {
    private ApiPath() {}

    /** Whether method and path are something the screens may do. */
    public static boolean allowed(String method, String path) {
        if (method == null || path == null) return false;
        if (!method.matches("GET|POST|PATCH|PUT|DELETE")) return false;
        if (path.contains("..") || path.contains("//") || path.contains("#")) return false;
        String bare = path;
        int q = bare.indexOf('?');
        if (q >= 0) {
            if (!bare.substring(q + 1).matches("[A-Za-z0-9_=&%.:-]*")) return false;
            bare = bare.substring(0, q);
        }
        if (!bare.matches("[A-Za-z0-9/_.%:~-]+")) return false;
        if (method.equals("GET") && (bare.equals("/api/me") || bare.equals("/api/people"))) return true;
        if (bare.startsWith("/api/approvals/")) {
            return (method.equals("GET") || method.equals("POST")) && bare.substring("/api/approvals/".length()).matches("[A-Za-z0-9-]+");
        }
        if (bare.equals("/api/devices") || bare.equals("/api/sessions")) return method.equals("GET");
        if (bare.matches("/api/(devices|sessions)/[0-9]+")) return method.equals("DELETE");
        if (bare.equals("/api/calendars")) return method.equals("GET") || method.equals("POST");
        if (bare.startsWith("/api/calendars/")) {
            String rest = bare.substring("/api/calendars/".length());
            String[] parts = rest.split("/", -1);
            if (parts[0].isEmpty()) return false;
            // Events and exports are not the app's to read.
            if (parts.length == 1) return method.equals("PATCH") || method.equals("DELETE");
            if (parts.length == 3 && parts[1].equals("shares") && !parts[2].isEmpty()) return method.equals("DELETE") || method.equals("PUT");
            if (parts.length == 2 && parts[1].equals("transfer")) return method.equals("POST");
            return false;
        }
        return false;
    }
}
