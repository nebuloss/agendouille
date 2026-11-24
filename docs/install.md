# Installing agendouille

One static binary and one SQLite file, at home. Nothing calls out: no mail,
no outside service.

## Install

On Debian or Ubuntu (systemd) or Alpine (OpenRC), as root:

```sh
curl -fsSL https://raw.githubusercontent.com/nebuloss/agendouille/main/deploy/install.sh | sh
```

The script:

1. downloads the latest release for this machine (`linux-amd64`, `linux-arm64`
   or `linux-armv7`) and checks it against the release's `SHA256SUMS`,
   refusing anything it cannot verify;
2. creates the `agendouille` user and `/var/lib/agendouille`;
3. writes `/etc/agendouille.yaml` (never overwritten afterwards);
4. installs the service (a hardened systemd unit, or OpenRC under
   `supervise-daemon`) and starts it;
5. sets a random admin password and **prints it once**: only its hash is
   kept.

Two variables are read on a first install only (afterwards the file wins):

| Variable | Effect |
|---|---|
| `AGENDOUILLE_INSTALL_ADMIN_LISTEN` | Where the admin UI listens. Default `127.0.0.1:8792`, reached with `ssh -L 8792:127.0.0.1:8792`. `0.0.0.0:8792` puts it on the LAN, in plain http. |
| `AGENDOUILLE_INSTALL_PUBLIC_URL` | The family's address (`https://agenda.example.org`): turns the public listener on at `:8791`. Without it, set `public_listen` and `public_url` in the file later. |
| `AGENDOUILLE_INSTALL_VERSION` | A release (`v0.1.0`) instead of the latest. |

For example:

```sh
curl -fsSL https://raw.githubusercontent.com/nebuloss/agendouille/main/deploy/install.sh |
  AGENDOUILLE_INSTALL_ADMIN_LISTEN=0.0.0.0:8792 \
  AGENDOUILLE_INSTALL_PUBLIC_URL=https://agenda.example.org sh
```

From a checkout, `sudo deploy/install.sh` builds from source instead (Go
and Node needed).

## Upgrade

Run the same command again. The configuration, the data and the admin
password are left alone; the service is stopped, replaced and started, and
the script waits until it answers.

From a checkout, `scripts/deploy.sh <target> [vX.Y.Z]` installs a release
on a server over ssh (see [development](development.md#deploying)).

## First steps

1. Open the admin UI (`http://127.0.0.1:8792/`, or the LAN address) and
   sign in with the printed password.
2. Make the groups (« famille ») and the users, each in their groups.
3. Give each user a sign-in QR code (the user's Appareils page, « Créer
   un QR code »): it signs in their first phone, in the agendouille app
   or the browser.
4. Put the public side behind the DMZ proxy: [deployment](deployment.md).

Then each person sets up their own devices from the web app's Appareils
page: [clients](clients.md).

## Lost the admin password

On the server, as the `agendouille` user:

```sh
agendouille admin passwd --random -config /etc/agendouille.yaml
```

## Remove

Stop and disable the service, then delete `/usr/local/bin/agendouille`, the
unit (`/etc/systemd/system/agendouille.service` or
`/etc/init.d/agendouille`), `/etc/agendouille.yaml` and, once its backups
are copied elsewhere, `/var/lib/agendouille`.
