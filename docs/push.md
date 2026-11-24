# Instant sync (push)

Without push, a phone sees a change made elsewhere at its next periodic
sync (every 15 minutes at best, Android's floor). With push, the server
tells the phone within seconds, and the phone syncs.

Nothing goes through Google. Three pieces:

- **The agendouille server** speaks
  [WebDAV-Push](https://github.com/bitfireAT/webdav-push) (the draft by
  DAVx5's authors): a phone subscribes to each calendar over CalDAV, and
  each change sends it an encrypted message (Web Push, RFC 8291, signed with
  VAPID, RFC 8292). The message holds the calendar's topic and sync token,
  never an event. Nothing to configure: it is always on.
- **A push server**: [ntfy](https://ntfy.sh), self-hosted. It holds each
  phone's connection and relays the messages. The agendouille server must
  reach it over HTTPS.
- **A distributor app** on each phone: ntfy's Android app (F-Droid or Play
  Store), pointed at that push server. It hands the endpoint to the
  agendouille app, or to DAVx5, through UnifiedPush.

## The push server

ntfy in a container, behind the proxy with HTTPS, for example
`ntfy.example.org`. Its `server.yml`:

```yaml
base-url: "https://ntfy.example.org"
listen-http: ":80"
behind-proxy: true
cache-file: "/var/cache/ntfy/cache.db"
# Private by default; UnifiedPush topics stay writable by anyone who knows
# them (each is a long random name the phone makes).
auth-file: "/var/lib/ntfy/user.db"
auth-default-access: "deny-all"
```

and, once: `ntfy access everyone 'up*' write-only`.

The phones reach it from wherever they are, so it is public, like the
agendouille server's public listener. The agendouille server only sends to
it (outbound HTTPS).

## The phones

1. Install ntfy, open it, Settings, « Default server »: your push server's
   address.
2. **The agendouille app**: Réglages, « Synchronisation instantanée » shows
   « Active, par ntfy » after its next sync. With several distributor apps
   installed, choose one there.
3. **DAVx5** (4.4 or later): its account settings, « Push »: choose ntfy.
4. Let ntfy run in the background (Android's battery settings), or its
   connection drops while the phone sleeps.

## How it works

- A calendar's PROPFIND offers `transports` (Web Push, with the server's
  VAPID key), `topic` and `supported-triggers` (members, depth 1; the
  calendar's own properties, depth 0). They are not part of `allprop`.
- `POST` a `push-register` to the calendar: `204`, the registration in
  `Location`, its end in `Expires` (7 days by default, 30 at most; a phone
  renews it). `DELETE` the registration to stop. One user has at most 200.
- A change is sent once its burst is over (300 ms), never to the device that
  made it, nor to the registrations its request names in
  `Push-Dont-Notify`. A push service that answers `404` or `410` loses the
  registration.
- The push resource must be `https`. A calendar no longer shared with the
  person is no longer pushed to them.
