# The command line

One binary, `agendouille`, for the server and for its operator. Every
command reads the same configuration as `serve` (`-config`, or
`/etc/agendouille.yaml`); run them as the `agendouille` user, so the
database keeps its owner. `agendouille <command> -h` prints a command's
flags.

## The server

```sh
agendouille serve                      # refuses to start without an admin password
agendouille version                    # agendouille 0.1.0
agendouille config                     # every setting, and where it came from
agendouille config migrate             # settings an older version kept in the database, into the file
agendouille admin passwd               # set the admin password (read from stdin)
agendouille admin passwd --random      # a random one, printed once
agendouille admin passwd --if-unset    # only when none is set (the installer's)
```

## People

```sh
agendouille group add famille --display-name Famille [--color C]
agendouille group list
agendouille group member add|remove famille lea
agendouille group remove famille

agendouille user add lea --display-name Léa --group famille [--color #rrggbb]
agendouille user enroll lea [--ttl 72h]    # a one-time sign-in link and QR code
agendouille user list
agendouille user disable|enable|remove lea
```

## Calendars

A calendar is named `<owner>~<slug>` (`lea~perso`); a share target is
`u:<user>` or `g:<group>`.

```sh
agendouille calendar add lea perso --name "Perso" [--color C] [--timezone Z]
agendouille calendar list [--user lea]
agendouille calendar share lea~perso g:famille read|readwrite
agendouille calendar unshare lea~perso g:famille
agendouille calendar transfer lea~perso maman
agendouille calendar delete lea~perso

agendouille import lea~perso google-export.ics [--replace]   # or - for stdin
agendouille export lea~perso > perso.ics
```

## Devices

A sync credential for a CalDAV client, as the web app's Appareils page
makes it (with DAVx5's link):

```sh
agendouille device add lea --label "Pixel de Léa" [--kind davx5|ios|thunderbird|other] [--read]
agendouille device list lea
agendouille device revoke lea 12
```

## Backups

```sh
agendouille backup                      # a backup now, into data_dir/backups
agendouille restore backups/agendouille-<time>.db   # the server stopped
```

The admin UI does both too; see [deployment](deployment.md#backups).
