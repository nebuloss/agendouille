# Deployment

How agendouille sits on a home network: two listeners, a proxy in the DMZ
for the family's side, the admin UI on the LAN only. Installing the server
itself: [install](install.md).

## Layout

```
internet ──443──> DMZ proxy (nginx, Caddy, Nginx Proxy Manager)
                      │  forwards ONLY the public listener
                      ▼
          agendouille :8791  public: CalDAV (/dav/), the family web app,
                             /.well-known/caldav, /ics/ subscription links,
                             the Android app (/app/agendouille.apk)
          agendouille :8792  admin UI: users, groups, sign-in QR codes,
                             settings, backups. LAN only, never forwarded.
```

The two listeners can be on different addresses: the admin one needs a
restart to move (so a mistake never locks the admin out), the public one
moves live when `public_listen` changes. See
[configuration](configuration.md).

## The DMZ proxy

`deploy/nginx.conf.example` and `deploy/Caddyfile.example` are complete.
What matters:

- Forward to the public listener only.
- Pass every method (`PROPFIND`, `REPORT`, `PROPPATCH`, `MKCALENDAR`,
  `MKCOL`, `OPTIONS`) and the `Depth`, `If-Match`, `If-None-Match`,
  `Authorization` headers. nginx and Caddy do by default.
- Do not buffer `/api/live` (Server-Sent Events).
- `/.well-known/caldav` is answered by agendouille itself: forward it.

Then, in `/etc/agendouille.yaml` (applied within seconds):

- `public_url`: the proxy's `https://` address. Sign-in QR codes, DAVx5
  links and every phone's account point there, so pick it once: changing
  it means setting every phone up again.
- `trusted_proxies`: the proxy's address, so the activity log shows the
  family's addresses and device certificates can be passed on.

### Nginx Proxy Manager

A proxy host for the domain to `http://<server>:8791`, Let's Encrypt,
"Force SSL" ("Block common exploits" is fine), and in the "Advanced" tab:

```nginx
client_max_body_size 20m;
location /api/live {
    proxy_pass http://<server>:8791;
    proxy_http_version 1.1;
    proxy_buffering off;
    proxy_read_timeout 1h;
    include /etc/nginx/conf.d/include/proxy.conf;
}
```

### Device certificates

When the admin chooses « Mot de passe et certificat » or « Certificat seul »
for CalDAV (see [clients](clients.md#device-keys)), agendouille checks the
certificate itself; the proxy only asks the client for one and passes it
on. With `ca.pem` copied from the server's `data_dir` to the proxy:

```nginx
ssl_verify_client optional_no_ca;
ssl_client_certificate /data/custom_ssl/agendouille-ca.pem; # names the CA to clients
location /dav/ {
    proxy_pass http://<server>:8791;
    proxy_set_header X-Client-Cert $ssl_client_escaped_cert;
    include /etc/nginx/conf.d/include/proxy.conf;
}
```

A client cannot forge it: the server reads the header only from
`trusted_proxies` and verifies the certificate against its own CA.

## The admin UI on the LAN

Either listen on the LAN in plain http (`admin_listen: 0.0.0.0:8792`,
`admin_public_url`, `admin_allow_insecure: true`), or keep it on loopback
behind a LAN-only proxy with its own name. Never forward it from the
internet.

## DNS and certificates

One name (`agenda.example.org`) pointing at the proxy; the proxy holds the
certificate. agendouille itself speaks plain http behind it.

## Backups

Every day at `backup_schedule` (03:30) the server writes
`/var/lib/agendouille/backups/agendouille-<UTC time>.db` and keeps
`backup_keep` (14) of them. The admin UI lists, makes and downloads them,
and restores one: the server checks it, stages it, exits with 75, and the
service manager starts it again (systemd `Restart=on-failure`, OpenRC
`supervise-daemon`); the start applies it and keeps the database it
replaced as `backups/before-restore-<time>.db`.

Copy the backups directory off the machine (it holds the whole family's
calendars): a nightly `rsync` from a NAS is enough. With the server
stopped, `agendouille restore <file>` does the same as the UI.

## Logs and service

- Alpine: `rc-service agendouille status`, `/var/log/agendouille.log`.
- Debian: `systemctl status agendouille`, `journalctl -u agendouille -f`.

`agendouille config -config /etc/agendouille.yaml` prints every setting and
where it came from. The Android app's crash reports land in the same log
(`android app crash`).

## Example: our home

Container 408 `dmz-agendouille` (Alpine) on the Proxmox host, behind HAProxy
and Nginx Proxy Manager (proxy host 7), at https://agendouille.patapouf.club;
the admin UI through the LAN's Nginx Proxy Manager (`srv-npm`, proxy host
26), which is also in `trusted_proxies`. `scripts/conf/targets/home.conf`
names it for the deploy scripts.
