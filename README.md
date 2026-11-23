# agendouille

> *agendouille*: an *agenda* with the *-ouille* of *andouille*. The
> family's calendars, not serious ones.

A self-hosted calendar server for a family, at home, with nothing outside.

- **Users** own calendars and share them with the people of their groups,
  read-only or read-write.
- **Android**: the agendouille app syncs into the phone's calendar (any
  calendar app shows the events) and signs browsers in with a QR code.
  DAVx5 and other CalDAV apps work too.
- **iPhone and macOS**, natively over CalDAV; **Thunderbird** too.
- **A web app**, good on a phone. No passwords and no email: sign in by
  approving from a device already signed in.
- **An admin UI** on the LAN only: users, groups, sign-in QR codes, ports,
  storage, backups, limits. It never shows events (a promise of the UI:
  whoever runs the server can still read its database).

Sync is standard CalDAV for everyone; sharing and sign-in go through an
additional API. One Go binary, SQLite, behind a reverse proxy in the DMZ.

**Status: planning.** See `PLAN.md` for the design, `CONTRACTS.md` for the
interfaces and `DECISIONS.md` for what was decided and why.
