# Deploying agendouille

One static binary, one SQLite file, at home. Nothing calls out: no mail,
no outside service.

## Layout

```
internet ──443──> DMZ proxy (nginx, Caddy, Nginx Proxy Manager)
                      │  forwards ONLY the public listener
                      ▼
          agendouille :8791  public: CalDAV (/dav/), the family web app,
                             /.well-known/caldav, /ics/ subscription links
          agendouille :8792  admin UI: users, groups, sign-in QR codes,
                             settings, backups. LAN only, never forwarded.
```

The two listeners can be on different addresses: the admin one comes from
`/etc/agendouille.yaml` (so a mistake in the UI never locks the admin out),
the public one from the `public_listen` setting, changed in the UI and
applied at once.

At home today: container 408 `dmz-agendouille` (Alpine, 10.0.60.8) on the
Proxmox host, behind HAProxy (10.0.60.1) and Nginx Proxy Manager
(10.0.60.2, proxy host 7), at https://agendouille.patapouf.club; the admin
UI at https://agendouille.dmz.patapouf.club/admin, through the LAN's Nginx
Proxy Manager (`srv-npm`, 10.0.10.1, proxy host 26), which is also in
`trusted_proxies`.

## Install

On Debian (systemd) or Alpine (OpenRC), as root:

```sh
curl -fsSL https://raw.githubusercontent.com/nebuloss/agendouille/main/deploy/install.sh |
  AGENDOUILLE_INSTALL_ADMIN_LISTEN=0.0.0.0:8792 \
  AGENDOUILLE_INSTALL_PUBLIC_URL=https://agenda.example.org sh
```

It downloads the release for this machine and checks its SHA256SUMS,
creates the `agendouille` user, writes `/etc/agendouille.yaml`, sets a
random admin password and **prints it once** (only its hash is kept), and
starts the service. Both variables are optional and read only on a first
install:

- `AGENDOUILLE_INSTALL_ADMIN_LISTEN`: the admin UI stays on 127.0.0.1:8792
  without it (reach it with `ssh -L 8792:127.0.0.1:8792`). With
  `0.0.0.0:8792` it is on the LAN, in plain http, which a LAN-only admin UI
  accepts.
- `AGENDOUILLE_INSTALL_PUBLIC_URL`: turns the public listener on at :8791
  for that address, in the file. Without it, add `public_listen` and
  `public_url` to the file later.

Every setting lives in `/etc/agendouille.yaml`, as cafard's and
claudication's do; `deploy/agendouille.yaml.example` lists them with their
defaults. A running server applies an edit of a setting within seconds (an
error is logged and the settings in force are kept); `data_dir`,
`admin_listen`, `admin_public_url` and `log_level` need a restart.
Each key can also come from the environment (`AGENDOUILLE_PUBLIC_URL`),
which wins over the file.

From a checkout, `sudo deploy/install.sh` builds from source instead.
From the editing machine, `scripts/devdeploy.sh` builds on dev-build and
installs that build:

```sh
PVE=root@10.0.0.2 CT=408 scripts/devdeploy.sh
```

Run either again to **upgrade**: the configuration, the data and the admin
password are left alone; the service is stopped, replaced and started.

Lost the admin password: `agendouille admin passwd --random` on the
server, as the `agendouille` user.

## The DMZ proxy

`nginx.conf.example` and `Caddyfile.example` are complete. What matters:

- Forward to the public listener only.
- Pass every method (`PROPFIND`, `REPORT`, `PROPPATCH`, `MKCALENDAR`,
  `MKCOL`, `OPTIONS`) and the `Depth`, `If-Match`, `If-None-Match`,
  `Authorization` headers. nginx and Caddy do by default.
- Do not buffer `/api/live` (Server-Sent Events).
- `/.well-known/caldav` is answered by agendouille itself: forward it.

Then, in `/etc/agendouille.yaml` (the server applies an edit within seconds;
the admin UI's Réglages page shows the values and where they came from):

- `public_url`: the proxy's `https://` address. Sign-in QR codes, DAVx5
  links and every phone's account point there, so pick it once: changing
  it means setting every phone up again.
- `trusted_proxies`: the proxy's address (here 10.0.60.2), so the activity
  log shows the family's addresses.

**Nginx Proxy Manager**: a proxy host for the domain to
`http://10.0.60.8:8791`, Let's Encrypt, "Force SSL", "Block common
exploits" is fine, and in the "Advanced" tab:

```nginx
client_max_body_size 20m;
location /api/live {
    proxy_pass http://10.0.60.8:8791;
    proxy_http_version 1.1;
    proxy_buffering off;
    proxy_read_timeout 1h;
    include /etc/nginx/conf.d/include/proxy.conf;
}
```

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

Copy the backups directory off the machine (they hold the whole family's
calendars): a nightly `rsync` from the NAS is enough. With the server
stopped, `agendouille restore <file>` does the same as the UI.

## Logs and service

- Alpine: `rc-service agendouille status`, `/var/log/agendouille.log`.
- Debian: `systemctl status agendouille`, `journalctl -u agendouille -f`.

`agendouille config -config /etc/agendouille.yaml` prints the bootstrap
file and every setting.

## Clients, checked by hand

After an upgrade that touches CalDAV, on a test calendar:

| Step | Web app | DAVx5 (Android) | iPhone | Thunderbird |
|---|---|---|---|---|
| Set up from Appareils | QR or link | QR, one tap | profile | by hand |
| The calendars appear, with their colours | | | | |
| An event made here shows on the others | | | | |
| A weekly event, one Wednesday moved | | | | |
| An alarm made on the phone survives a web edit | | | | |
| A calendar shared read-only cannot be written | | | | |
| A hidden calendar leaves the phone | | | | |
| A revoked device stops syncing | | | | |
