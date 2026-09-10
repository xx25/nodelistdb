# FidoNet test zone

An isolated zone for testing FidoNet software against real mailer
implementations, rather than against our own code.

Everything in the nodelistdb test suite is Go talking to Go: the protocol
testers are exercised against fidomail's EMSI answerer and fidomail's ZMODEM
on both ends. That proves self-consistency, not interoperability, and the bugs
that reach a sysop's mailbox are interoperability bugs. This zone runs the C
mailer those reports come from.

## What is in it

| Node | Software | Address | Protocols |
|---|---|---|---|
| `172.28.0.10` | MBSE `mbcico` 1.1.7.2 | `999:1/1@testzone` | IBN 24554, IFC 60179, ITN 23 |

Zone 999 is not a real FidoNet zone, so a misconfigured caller can never
mistake a test node for a live one. The zone has its own bridge network and
every node answers on the port its nodelist entry would advertise, so software
under test needs no port remapping. Nothing is published to the host: the zone
is reachable over the bridge, and reaching it does not mean exposing a mailer
on a host port.

## Use

```bash
make up                                  # start (builds on first run)
make test CONFIG=/path/to/testdaemon.yaml
make mailer-log                          # what the remote sysop would read
make down
```

The config needs `protocols.telnet.enabled: true` to exercise ITN.

## The verdict is the peer's log, not ours

`run-tests.sh` runs each protocol and then reads mbcico's own log, failing on
any `!` or `?` line the session produced. This matters: a test that reports
success on our side can still have left the peer logging an error, and that is
the entire class of defect this zone exists to catch. Two lines are filtered as
noise any unknown caller provokes on a node with no nodelist -- `Unexpected
remote password` and the missing `node.files`.

A clean run looks like this on the mailer's side:

```
+ Start WaZOO session
+ Zmodem: start ZedZap receive
+ Zmodem: start ZedZap send
+ WaZOO session completed
+ Incoming call successful (rc=0)
```

## What it has already caught

Built on 2026-09-10 to check the fixes made after the sysop of 1:320/219
reported that our testdaemon left errors in his log. It confirmed the BinkP
and telnet fixes and then found a further defect that only appears between
implementations: our ZMODEM sender abandoned the ZFIN handshake when mbcico
emitted a ZRINIT that was already in flight, so mbcico finished its receive,
turned around to send, and found the link gone. Fixed in `go-zmodem`
(`Keep reading for the ZFIN the receiver owes us`).

## How the container is put together

MBSE is a full BBS, so the image builds it from source, creates the `mbse` and
`bbs` accounts it insists on, and lets `mbtask` write its own defaults on first
boot. Three details are not obvious:

- **`mbsetup init`** builds the databases without curses. `mbtask` launches it
  the first time it finds no `config.data`.
- **`patch-config.c`** stamps in the AKA and identity afterwards. `config.data`
  is a raw dump of `struct sysconfig`, so the patcher is compiled against
  MBSE's own header and the layout cannot drift from the binaries reading it.
- **`mbstat open`** runs on every boot. A fresh MBSE is *closed*, and mbcico
  answers a closed system with `M_BSY` before anything else
  (`binkp.c` `WaitConn` asks mbtask `SBBS:0;`). The flag lives in mbtask's
  status rather than in `config.data`, so it does not survive a restart.

`socat` accepts each connection and hands the socket to `mbcico` as fd 0/1
with `nofork`, which is the inetd contract mbcico is written against: it reads
the session from stdin and calls `getpeername(0)` to log who called. Without
`nofork` socat would relay through a pipe and mbcico would see no peer at all.

## Adding another mailer

Add a service to `docker-compose.yml` with its own address on the `testzone`
network and give it an address in zone 999. `run-tests.sh` takes `ZONE_IP` and
`CONTAINER`, so pointing it at a second node needs no new code.
