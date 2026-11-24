<p align="center">
  <img src="res/images/agendouille.svg" alt="agendouille" width="520">
</p>

*agendouille*, n. — an *agenda* with the *-ouille* of *andouille*. The
family's calendars, not serious ones.

A self-hosted calendar server for a family, at home, with nothing outside.

- **Users** own calendars and share them with the people and groups of
  their family, read-only or read-write.
- **Android**: the agendouille app syncs into the phone's own calendar,
  manages calendars and signs browsers in. DAVx5 and other CalDAV apps work
  too.
- **iPhone, iPad and Mac**, natively over CalDAV; **Thunderbird** too.
- **A web app**, good on a phone. No passwords and no email: a new device
  is approved from one already signed in.
- **An admin UI** on the LAN only: users, groups, sign-in QR codes,
  settings, backups. It never shows an event.

Sync is standard CalDAV for every client; sharing and sign-in go through an
additional API. One Go binary and one SQLite file, behind a reverse proxy
in the DMZ.

## Quick start

As root, on Debian, Ubuntu or Alpine (the same command installs and
upgrades):

```sh
curl -fsSL https://raw.githubusercontent.com/nebuloss/agendouille/main/deploy/install.sh | sh
```

It installs the latest release (checked against its `SHA256SUMS`) as a
service, starts it, and prints the admin password once. Open the admin UI
at **http://127.0.0.1:8792/**, make the family's users and groups, and
give each person a sign-in QR code. Then put the family's side behind your
proxy: [deployment](docs/deployment.md).

## Documentation

| | |
|---|---|
| [Install](docs/install.md) | Installing, upgrading, the first steps, a lost admin password. |
| [Deployment](docs/deployment.md) | The two listeners, the DMZ proxy, device certificates, backups, logs. |
| [Configuration](docs/configuration.md) | `/etc/agendouille.yaml`: every setting, which apply live. |
| [Clients](docs/clients.md) | Signing in without passwords; the Android app, DAVx5, iPhone, Thunderbird, read-only links; device keys. |
| [The Android app](docs/android-app.md) | Installing it, its tabs, its updates. |
| [Command line](docs/cli.md) | `agendouille serve` and the operator's commands. |
| [Development](docs/development.md) | The layout, building and testing, the scripts, releasing. |
| [Languages](docs/i18n.md) | French and English: how, and the style guide. |

The design is `PLAN.md`, the interfaces between the parts `CONTRACTS.md`,
and what was decided along the way, and why, `DECISIONS.md`.

## Status

Releases are on [GitHub](https://github.com/nebuloss/agendouille/releases):
the server for `linux-amd64`, `linux-arm64` and `linux-armv7`, and the
Android app. CalDAV works, read and write, with shared calendars; the
admin UI, the family web app and the Android app are complete. Still to
do: the by-hand checks with every client (the table in
[clients](docs/clients.md#checked-by-hand)).

## License

MIT: see `LICENSE`.
