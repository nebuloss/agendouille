package io.github.nebuloss.agendouille;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

/**
 * The iCalendar the app reads (RFC 5545, PLAN.md 7.3): lines unfolded,
 * components nested, properties with their parameters. Only the parsing;
 * what the phone keeps of it is EventMapper's. The text is kept as the
 * server sent it, for uploads that patch rather than rewrite (PLAN 4.6).
 * Pure Java: tested on the JVM against testdata/ical.
 */
final class ICal {
    private ICal() {}

    static final class Property {
        final String name;                  // upper case
        final Map<String, String> params;   // upper-case names, values unquoted
        final String value;                 // raw: escapes still in

        Property(String name, Map<String, String> params, String value) {
            this.name = name;
            this.params = params;
            this.value = value;
        }

        String param(String p) {
            return params.get(p);
        }

        /** The value as TEXT (RFC 5545 3.3.11): \\n, \\, \\; \\, unescaped. */
        String text() {
            StringBuilder b = new StringBuilder();
            for (int i = 0; i < value.length(); i++) {
                char c = value.charAt(i);
                if (c == '\\' && i + 1 < value.length()) {
                    char n = value.charAt(++i);
                    b.append(n == 'n' || n == 'N' ? '\n' : n);
                } else {
                    b.append(c);
                }
            }
            return b.toString();
        }
    }

    static final class Component {
        final String name;
        final List<Property> props = new ArrayList<>();
        final List<Component> children = new ArrayList<>();

        Component(String name) {
            this.name = name;
        }

        Property get(String name) {
            for (Property p : props) if (p.name.equals(name)) return p;
            return null;
        }

        List<Property> all(String name) {
            List<Property> out = new ArrayList<>();
            for (Property p : props) if (p.name.equals(name)) out.add(p);
            return out;
        }

        List<Component> children(String name) {
            List<Component> out = new ArrayList<>();
            for (Component c : children) if (c.name.equals(name)) out.add(c);
            return out;
        }

        String text(String name) {
            Property p = get(name);
            return p == null ? null : p.text();
        }
    }

    /** Parses a VCALENDAR; throws on text that is not iCalendar. */
    static Component parse(String text) {
        List<String> lines = unfold(text);
        Component root = null;
        List<Component> stack = new ArrayList<>();
        for (String line : lines) {
            if (line.isEmpty()) continue;
            Property p = property(line);
            if (p.name.equals("BEGIN")) {
                Component c = new Component(p.value.trim().toUpperCase(Locale.ROOT));
                if (stack.isEmpty()) {
                    if (root != null) throw new IllegalArgumentException("more than one top component");
                    root = c;
                } else {
                    stack.get(stack.size() - 1).children.add(c);
                }
                stack.add(c);
            } else if (p.name.equals("END")) {
                if (stack.isEmpty()) throw new IllegalArgumentException("END without BEGIN");
                Component c = stack.remove(stack.size() - 1);
                if (!c.name.equals(p.value.trim().toUpperCase(Locale.ROOT))) throw new IllegalArgumentException("END:" + p.value + " closes " + c.name);
            } else if (!stack.isEmpty()) {
                stack.get(stack.size() - 1).props.add(p);
            }
        }
        if (root == null || !stack.isEmpty() || !root.name.equals("VCALENDAR")) throw new IllegalArgumentException("not a VCALENDAR");
        return root;
    }

    static List<String> unfold(String text) {
        List<String> out = new ArrayList<>();
        StringBuilder cur = null;
        for (String raw : text.split("\r\n|\n|\r", -1)) {
            if (!raw.isEmpty() && (raw.charAt(0) == ' ' || raw.charAt(0) == '\t') && cur != null) {
                cur.append(raw, 1, raw.length());
            } else {
                if (cur != null) out.add(cur.toString());
                cur = new StringBuilder(raw);
            }
        }
        if (cur != null) out.add(cur.toString());
        return out;
    }

    /** NAME;P1=v;P2="v:w":value. */
    static Property property(String line) {
        int i = 0;
        int n = line.length();
        while (i < n && line.charAt(i) != ';' && line.charAt(i) != ':') i++;
        String name = line.substring(0, i).toUpperCase(Locale.ROOT);
        Map<String, String> params = new LinkedHashMap<>();
        while (i < n && line.charAt(i) == ';') {
            int start = ++i;
            while (i < n && line.charAt(i) != '=' && line.charAt(i) != ';' && line.charAt(i) != ':') i++;
            String pname = line.substring(start, i).toUpperCase(Locale.ROOT);
            StringBuilder v = new StringBuilder();
            if (i < n && line.charAt(i) == '=') {
                i++;
                boolean quoted = false;
                while (i < n) {
                    char c = line.charAt(i);
                    if (c == '"') quoted = !quoted;
                    else if (!quoted && (c == ';' || c == ':')) break;
                    else v.append(c);
                    i++;
                }
            }
            params.put(pname, v.toString());
        }
        if (i >= n || line.charAt(i) != ':') throw new IllegalArgumentException("no value: " + line);
        return new Property(name, params, line.substring(i + 1));
    }
}
