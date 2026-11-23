package io.github.nebuloss.agendouille;

import org.json.JSONException;
import org.json.JSONObject;

/**
 * The bridge's messages (CONTRACTS.md 6), version 2: requests from the
 * page, answers and events from the app, as JSON strings. Unknown fields
 * are ignored; a malformed request gets no answer (it has no id to answer).
 * Uses only org.json, so it is tested on the JVM.
 */
public final class BridgeCodec {
    public static final int VERSION = 2;

    private BridgeCodec() {}

    /** A call from the page. */
    public static final class Request {
        public final long id;
        public final String call;
        public final JSONObject args;

        Request(long id, String call, JSONObject args) {
            this.id = id;
            this.call = call;
            this.args = args;
        }
    }

    /** Reads a page message, or null when it is not a call. */
    public static Request parse(String text) {
        try {
            JSONObject o = new JSONObject(text);
            if (!o.has("id") || !o.has("call")) return null;
            long id = o.getLong("id");
            String call = o.getString("call");
            JSONObject args = o.optJSONObject("args");
            return new Request(id, call, args != null ? args : new JSONObject());
        } catch (JSONException | NullPointerException e) {
            return null;
        }
    }

    /** {"id": id, "ok": true, "result": result}. */
    public static String ok(long id, JSONObject result) {
        try {
            JSONObject o = new JSONObject().put("id", id).put("ok", true);
            if (result != null) o.put("result", result);
            return o.toString();
        } catch (JSONException e) {
            throw new IllegalStateException(e);
        }
    }

    /** {"id": id, "ok": false, "error": code, "message": message}: the code for the page, the message for a person. */
    public static String error(long id, String code, String message) {
        try {
            JSONObject o = new JSONObject().put("id", id).put("ok", false).put("error", code);
            if (message != null) o.put("message", message);
            return o.toString();
        } catch (JSONException e) {
            throw new IllegalStateException(e);
        }
    }

    /** {"event": name, "data": data}, from the app on its own. */
    public static String event(String name, JSONObject data) {
        try {
            JSONObject o = new JSONObject().put("event", name);
            if (data != null) o.put("data", data);
            return o.toString();
        } catch (JSONException e) {
            throw new IllegalStateException(e);
        }
    }

    /** The answer to hello. */
    public static JSONObject hello(String appVersion, int android) {
        try {
            return new JSONObject().put("app_bridge", VERSION).put("app_version", appVersion).put("android", android);
        } catch (JSONException e) {
            throw new IllegalStateException(e);
        }
    }
}
