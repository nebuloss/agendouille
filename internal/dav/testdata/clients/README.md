# Client requests

One directory per client of PLAN.md 5.4, one request per step, replayed by
`cmd/agendouille/clients_test.go` against a seeded server.

**These were written by hand from each client's known behaviour** (the
properties DAVx5, iOS and Thunderbird ask for, as their sources and
public logs show), not captured from the clients themselves. The M2
by-hand checks replace them with real captures as they happen: capture
with the server's debug log or a proxy, strip credentials, keep the
request as sent (DECISIONS 2026-11-23, "M2 notes").

## Format

`NN-step.http` is the request:

```
METHOD /path
Header: value
...
(blank line)
body
```

`NN-step.want` is what the answer must hold, one check per line:

```
status 207
contains <text>
lacks <text>
```

The seed: user `lea` with a device, her calendar `lea~perso` holding one
event `judo.ics` (UID `judo-uid`, weekly on Wednesdays from 4 November
2026, 14:00 Paris).
