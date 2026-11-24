# Configuration

All of agendouille's configuration is one file, `/etc/agendouille.yaml`
(or `-config`, or `AGENDOUILLE_CONFIG`), as cafard's and claudication's.
`deploy/agendouille.yaml.example` lists every key with its default; the
installer writes the file once and never overwrites it.

- **Every key** is also a flag (`-public-url`) and an environment variable
  (`AGENDOUILLE_PUBLIC_URL`). The order is file, then environment, then
  flag: the last one set wins.
- **An unknown key** is an error, on purpose: a typo never goes unnoticed.
- **Live**: a running server re-reads the file within seconds and applies
  the settings; an invalid edit is logged and the settings in force are
  kept. The keys of the first table below need a restart.
- **The admin UI** shows every value and where it came from (Réglages); it
  does not edit the file.
- `agendouille config` prints the effective configuration, the same way.

## Needs a restart

| Key | Default | |
|---|---|---|
| `data_dir` | `/var/lib/agendouille` | The database and its backups. The systemd unit lets the service write only here. |
| `admin_listen` | `127.0.0.1:8792` | The admin UI. LAN only: never forwarded by the DMZ proxy. |
| `admin_public_url` | `http://<admin_listen>` | Where the admin UI is reached; required off loopback. |
| `admin_allow_insecure` | `false` | Plain http for the admin UI off loopback, in so many words. |
| `log_level` | `info` | `debug`, `info`, `warn` or `error`. |

## Applied live

| Key | Default | |
|---|---|---|
| `public_listen` | off | CalDAV and the family web app, behind the DMZ proxy (`0.0.0.0:8791`). Off until both this and `public_url` are set. |
| `public_url` | | The family's `https://` address. In every QR code and every phone: pick it once. |
| `public_allow_insecure` | `false` | Accept an `http://` public URL (a test on the LAN). |
| `trusted_proxies` | `[]` | Whose `X-Forwarded-For` is believed. |
| `backup_schedule` | `"03:30"` | A backup a day, server time. |
| `backup_keep` | `14` | Backups kept. |
| `sync_token_retention_days` | `180` | How long a CalDAV client can be away and still sync by changes only. |
| `max_object_bytes` | `262144` | One event, alarms and all. |
| `max_calendars_per_user` | `50` | |
| `max_events_per_calendar` | `50000` | |
| `max_devices_per_user` | `20` | |
| `family_timezone` | `Europe/Paris` | The web app shows and edits times in this zone, whatever the browser's. |
| `palette` | twelve colours | The colours offered for calendars and people. |
| `enrollment_link_ttl_hours` | `72` | How long the admin's sign-in QR codes work. |
| `session_ttl_days` | `90` | How long a browser stays signed in. |
| `app_sync_interval_minutes` | `30` | How often the Android app syncs on its own. |

## Not in the file

The admin password, set with `agendouille admin passwd`.

## Example

```yaml
admin_listen: 0.0.0.0:8792
admin_public_url: http://10.0.60.8:8792
admin_allow_insecure: true

public_listen: 0.0.0.0:8791
public_url: https://agenda.example.org
trusted_proxies: [10.0.60.2]
family_timezone: Europe/Paris
```
