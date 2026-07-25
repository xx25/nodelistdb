# The Virtual Modem Protocol (VMP)

Reverse-engineered specification of the protocol behind the FidoNet `IVM` nodelist flag.

> **Revision 2026-07-26 — answerer pass.** Everything here was previously written from the
> caller's side. This revision adds what an *answerer* implementation needs, from a cross-read of
> the vendor manual against the disassembly. Changed or new:
>
> - **§7 item 4 — corrected.** The ≥24-byte `CONNECT-REQ` path is **not** the manual's "Shared
>   Secret login". Shared Secret is an application-layer exchange that never touches a control
>   frame; the dword compared there is an SIO serial number (a self-connect guard).
> - **§15 — new.** What the vendor manual settles that the disassembly alone does not: `ATDV`,
>   S7, the `CONNECT …` result code, `BUSY`, and where Shared Secret really lives.
> - **§16 — new.** Implementing an answerer: eight rules, plus §16.1 on what actually answers on
>   IVM ports in the wild and how to multiplex them.
> - **§14 — extended.** Two open questions added (no file transfer has ever crossed a VMP data
>   channel; the caller's state machine is documented structurally, not behaviourally), and the
>   field-results table is now dated against the EMSI role fix.
> - **§1, §6, §8** — pointers to the above; no findings changed.
>
> Section numbers 1–14 are deliberately unchanged so existing references still resolve; the new
> material is appended rather than inserted.

## 1. Scope and provenance

`IVM` announces Ray Gwinn's **VMODEM**, part of the SIO package for OS/2 (1997). VMODEM presents
a TCP service that behaves like a modem attached to a virtual COM port: a caller "dials" it, the
local mailer sees `RING`, answers, and a session runs over TCP.

VMODEM speaks **two** protocols on two different ports:

| | Port | Wire |
|---|---|---|
| **VMP** (Virtual Modem Protocol) | 3141 (IANA `vmodem`) | binary, framed control + raw data |
| **Telnet server** (`VMOTelnet`) | 23 by default | ordinary telnet |

This document specifies **VMP**. Everything here was recovered from `VMODEM.EXE` (54 272 bytes,
OS/2 LX, `V1.20`) and confirmed against live nodes. There is no published specification; the
vendor manual (`VMODEM.TXT`) documents only user-visible behaviour, and is cited where it
corroborates the disassembly. Four of its user-visible statements turn out to be load-bearing for
an implementer — and one of them contradicts an inference made here; both are collected in §15.

**How to reproduce the analysis.** `VMODEM.EXE` is an LX image whose objects must be unpacked
(iterated/EXEPACK pages) and relocated before anything makes sense. The critical detail: **object 2
(`0x70000`, 26 624 bytes) is 16-bit code** — its object flags lack the LX "BIG/DEFAULT" bit, so
32-bit disassembly and 32-bit cross-reference searches both produce garbage, which is why string
references appear to be missing. Disassemble it as 16-bit with 0x66/0x67 prefixes. All addresses
below are offsets **within that code object**.

---

## 2. Architecture: VMP uses two TCP connections

This is the single most important property of the protocol, and the one that makes VMP unlike
every other FidoNet-over-IP transport.

```
                  CALLER                                  ANSWERER
                    │                                        │
   listen(P) ───────┤                                        │
                    │  1. TCP connect ─────────────────────► │  accept()
                    │                                        │  getpeername() → caller IP
                    │  2. HELLO      (cmd 6) ──────────────► │  assign a virtual COM port
                    │  3. CONNECT-REQ(cmd 0, port=P) ──────► │
                    │                                        │
                    │ ◄──────────── 4. TCP connect to caller-IP:P
   accept() ────────┤                    (the DATA channel)  │
                    │                                        │
                    │ ◄───────────── 5. RINGING (cmd 2) ──── │  assert RI, print "RING" to COM
                    │ ◄───────────── 5. RINGING (cmd 2) ──── │  … every 4.5 s, indefinitely
                    │                                        │
                    │ ◄───────────── 6. CONNECT (cmd 3) ──── │  the mailer answered
                    │                                        │
                    │ ◄════════ raw binary session ════════► │  (on the DATA channel)
                    │                                        │
                    │  7. DISCONNECT (cmd 1, reason) ──────► │
```

- The **control channel** is the connection the caller opened. It carries *nothing but frames*;
  VMODEM's parser silently discards any byte outside a frame.
- The **data channel** is opened **by the answerer, back to the caller**, at the address
  `getpeername()` reported for the control channel and the port the caller named in the connect
  request. It carries the mailer session verbatim — no framing, no escaping. (`VMODEM.TXT`: *"VMP
  is Vmodem to Vmodem only, but is true binary while Telnet it not."*)
- **The data channel is established before the mailer is rung.** If the reverse connection fails,
  the answerer sends `DISCONNECT` with reason 4 and nothing is ever rung.

**Consequence:** a VMP caller must accept an inbound TCP connection. A caller that can only make
outbound connections cannot complete a VMP call, and neither can one whose advertised port is not
reachable from the node it is calling.

---

## 3. Frame format

Frames appear only on the control channel.

```
 10 02   <len:16 BE>   <payload: len bytes>
 └─┬─┘   └────┬────┘   └────────┬────────┘
   │          │                 │
   │          │                 └── DLE-stuffed
   │          └──────────────────── DLE-stuffed
   └─────────────────────────────── literal marker, never stuffed
```

* **Marker** `0x10 0x02` (DLE STX), emitted literally.
* **Length** is the payload byte count, big-endian, *before* stuffing.
* **Stuffing**: every `0x10` byte in the length or payload is doubled (`10` → `10 10`).
  Because of this, the sequence `10 02` can never occur inside a frame body, so the marker is
  unambiguous.
* **Maximum payload is 0x100 (256) bytes.** VMODEM's parser rejects any frame declaring more.
* The payload begins with a **16-bit big-endian command word**.

Sender: `0x2d60`. Receiver: byte FSM at `0x2100`–`0x2245`, length bound at `0x21ca`.

Example — the disconnect frame observed on the wire:

```
10 02 00 04 00 01 00 08
└─┬─┘ └─┬─┘ └─┬─┘ └─┬─┘
  │     │     │     └── reason 8
  │     │     └──────── command 1 (DISCONNECT)
  │     └────────────── payload length 4
  └──────────────────── marker
```

---

## 4. Command vocabulary

From the dispatch table at `0x2ce0` — 4 states × 8 commands. The dispatcher (`0x33cd`) clamps the
command word to 7, so **any command ≥ 8 is answered with a NAK**, not ignored.

| # | Name | Direction | Payload after the command word |
|---|------|-----------|-------------------------------|
| 0 | `CONNECT-REQ` | caller → answerer | data-channel port (2, BE), version stamp (4) |
| 1 | `DISCONNECT` | either | reason (2, BE) |
| 2 | `RINGING` | answerer → caller | — |
| 3 | `CONNECT` | answerer → caller | — |
| 4 | `NAK` | either | the offending command word (2) |
| 5 | `BUSY` | answerer → caller | — |
| 6 | `HELLO` | caller → answerer | protocol level (2, **LE**), requested port (2, BE) |

Commands 0 and 6 travel caller→answerer only; 2, 3 and 5 answerer→caller only. A reply frame
carrying command 0 or 6 did not come from a VMODEM.

### Dispatch table (state × command → handler)

| State | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 |
|-------|---|---|---|---|---|---|---|---|
| `0x2ce0` (never selected) | err | err | err | err | NAK-log | err | err | send NAK |
| `0x2d00` answerer, awaiting CONNECT-REQ | **accept** | disc | err | err | NAK-log | err | err | send NAK |
| `0x2d20` caller, call in progress | err | disc | RINGING | CONNECT | NAK-log | BUSY | err | send NAK |
| `0x2d40` connected | err | disc | err | err | NAK-log | err | err | send NAK |

`err` = `0x30e4`, which sends `DISCONNECT` reason 3. `NAK-log` records that the *peer* rejected
one of our commands. The active row is held in `[0x28c3]`; only three rows are ever selected —
`0x2d00` (`0x346f`), `0x2d20` (`0x3537` and the dialer), `0x2d40` (`0x3104`).

---

## 5. Disconnect reasons

Recovered from the call sites of the disconnect senders (`0x239c`, `0x30a6`).

| Reason | Meaning | Emitted at |
|--------|---------|-----------|
| 1 | The ring loop gave up — the virtual COM port went away | `0x3aaa` |
| 2 | Normal local hangup (DTR dropped) | `0x0e47` |
| 3 | Command not valid in the current state | `0x30e4` |
| 4 | **The reverse data connection to the caller failed** | `0x32b5` |
| 5 | Local abort | `0x413f` |
| 6 | `CONNECT-REQ` payload shorter than 4 bytes | `0x3265` |
| 7 | Caller is this same machine (loopback refused) | `0x3299` |
| 8 | **Opening frame was not a well-formed VMP hello** | `0x2433` |

Reasons 4 and 8 are the two a caller will meet in practice, and they discriminate cleanly:
**8 means "your hello was wrong"; 4 means "your hello was right, and I could not call you back."**

---

## 6. The HELLO frame (caller's first frame)

The answerer reads the opening frame with a **blocking `recv(sock, buf, 10, 0)` that demands
exactly 10 bytes** (`0x240b`). It does *not* DLE-unstuff this read, so the hello must contain no
byte that requires stuffing.

```
 10 02 | 00 06 | 00 06 | 01 00 | ff ff
 └─┬─┘   └─┬─┘   └─┬─┘   └─┬─┘   └─┬─┘
   │       │       │       │       └── requested virtual COM port, BE. 0xFFFF = "any free port"
   │       │       │       └────────── protocol level, LITTLE-endian, must be >= 1
   │       │       └────────────────── command 6 (HELLO), BE
   │       └────────────────────────── payload length 6
   └────────────────────────────────── marker
```

Validation, in order (`0x2445`–`0x2467`):

1. `recv` returned exactly 10 → else reason 8.
2. `buf[0..1] == 10 02` → else reason 8.
3. LE word at payload+2 ≥ 1 → else reason 8.
4. BE word at payload+0 == 6 → else reason 8.
5. BE word at payload+4 is passed to the port assigner: `0xFFFF` scans for any free virtual COM
   port; any other value selects that port index, and a value ≥ the configured port count yields
   `BUSY`.

> **The protocol-level word is the one field VMODEM does not byte-swap.** Every other multi-byte
> field on the wire is big-endian; this one is written and read as a native little-endian word
> (`0x34af` writes it, `0x244e` reads it). Sending `00 01` here fails validation.

On failure the answerer logs *"The caller is not using a current VMODEM"*, sends reason 8, and
closes. **No COM port has been assigned and nothing has been rung at this point.**

The fixed-length read is an implementation quirk, not a wire rule — VMODEM cannot accept a hello
whose port index needs stuffing. Do not copy it; see §16 item 1.

---

## 7. The CONNECT-REQ frame

```
 10 02 | 00 08 | 00 00 | PP PP | VV VV VV VV
                 └─┬─┘   └─┬─┘   └─────┬────┘
                   │       │           └── version stamp (see below)
                   │       └────────────── caller's data-channel TCP port, BE (network order)
                   └────────────────────── command 0, BE
```

Handler `0x3252`:

1. Reject a repeat (`[0x28c5]` latch) — one connect request per call.
2. Payload length ≥ 4 → else reason 6.
3. **If payload length < 0x18 (24), skip straight to the connect.** VMODEM 1.20's own client always
   sends an 8-byte payload (`0x3327`), so this is the normal path and the version stamp is never
   examined.
4. Otherwise (payload ≥ 24) take the dword at payload offset 0x14 and compare it against the
   answerer's own version stamp (`gs:[0x29b]`). If it is zero, or differs, or its top byte is
   ≥ 0x15, the checks are skipped and the call proceeds. If it *matches*, the caller is claiming to
   be the same VMODEM build, and the call is then accepted **only from the answerer's own IP** —
   any other address gets reason 7 (`0x328d`–`0x3299`). VMODEM 1.20's own client never sends a
   payload this long, so the path is dormant in practice.

   > **Corrected 2026-07-26.** This paragraph previously called that dword "the hook behind the
   > manual's Shared Secret login". It is not. The dword is an SIO **serial number**, and matching
   > it identifies the *same installation* — a self-connect guard, which is exactly why the only
   > consequence is refusing every address but the answerer's own. The manual's Shared Secret is a
   > different mechanism at a different layer entirely (§15). Nothing else in this section changes:
   > the path stays dormant either way.
5. Connect: `socket(AF_INET, SOCK_STREAM, 0)`, then a **blocking `connect()`** to
   `(sin_addr = the control channel's peer address, sin_port = the port word from this frame)`
   (`0x398e`). Failure of either call → `DISCONNECT` reason 4.
6. Success → the ring loop.

The callback address is taken from the sockaddr filled by `accept()` (`0x26f6` → copied at
`0x231c` → `sin_addr` at `[0x28a8]`). **The caller cannot influence it**; only the port is
negotiable.

### 7.1 Which port to advertise

The answerer imposes **no constraint** on the port word — it is copied into `sin_port` unvalidated.
But VMODEM's own caller never uses an ephemeral port: it binds from a fixed candidate list
(`0x3790`), advancing to the next entry on `EADDRINUSE` (`0x388c`, errno 10048):

| Order | Port |
|---|---|
| 1st | **14592** |
| 2nd | 5376 |
| 3rd | 17920 |
| 4th | 8448 |
| then | 10240, 9984, 9728, … 1280 (256 × 40 counting down to 256 × 5) |
| exhausted | reason 4 |

The table at `0x35f0` holds `0x0039, 0x0015, 0x0046, 0x0021`, written straight into `sin_port`
(network order), so the wire ports are those values × 256. The index at `[0x4912]` resets on every
dial, so **a real VMODEM caller always tries 14592 first**.

This matters in practice: a node whose firewall was ever configured "for VMODEM" is configured for
*these* ports. 2:371/52 refuses a callback to port 50090 with reason 4 but completes the call when
the caller advertises 14592. **Advertise 14592 unless something else forces a different choice.**

### 7.2 The callback may come from a different address

The answerer's data connection is not guaranteed to originate from the address that answered the
control channel. 2:5025/2 answers on `109.106.139.152` and dials back from `95.32.211.149`.
A caller that insists the two match will refuse the connection *after the node has already rung its
mailer*, and the failure is indistinguishable from an unreachable data channel. Accept the
connection and rely on the EMSI address exchange for identity.

---

## 8. Ring cadence

Once the data channel is up, the answerer runs this loop (`0x3af5`–`0x3b4e`) **indefinitely**
until the mailer answers or the caller drops:

```
  answered? ──yes──► enter data mode (0x3100), send CONNECT (cmd 3), done
      │no
      ▼
  assert RI on the virtual COM port
  write "\r\nRING\r\n" to the COM port
  send RINGING (cmd 2) to the caller
  wait 1500 ms  (or until answered)
  answered? ──yes──► connect
      │no
      ▼
  drop RI, wait 3000 ms, repeat
```

So a caller sees one `RINGING` frame every **4.5 s**. A ring budget should be a multiple of that;
45 s is ten rings.

`0x3100` (entering data mode) sets the dispatch row to `0x2d40`, raises carrier, and prints
`CONNECT 57600/ARQ/VMP` to the COM port — the bit rate is fiction, present only to satisfy the
mailer. **To the COM port, never to the wire**: the manual lists that string among the modem
result codes returned to the application (§15). An answerer that writes it to the data channel
corrupts the first bytes the caller's mailer reads.

Nor is this cadence a protocol constant. It describes VMODEM ringing a COM port whose mailer may
take a while to notice; how long the *caller* will tolerate it is the caller's S7 setting (§15),
which can be far under 45 s. An answerer with a mailer already listening should ring once and
connect — see §16 item 5.

---

## 9. Receiver state machines

### 9.1 Byte-level frame parser

Table pointer in `[0x2660]`, byte value clamped to 0x11 and used as the index; payload accumulates
at `[0x2664]` with the count in `[0x2764]`.

| State | Table | `0x10` (DLE) | any other byte |
|-------|-------|--------------|----------------|
| hunt | `0x1f50` | → after-DLE | **discard** |
| after-DLE | `0x1f98` | → hunt (reset) | `0x02` → in-frame; else → hunt (reset) |
| length-1 | `0x1fe0` | → escape | store, → length-2 |
| escape | `0x2028` | store literal, → saved state | → hunt (reset) |
| length-2 | `0x2070` | → escape | assemble length, bound-check ≤ 0x100, reset count, → payload |
| payload | `0x20b8` | → escape | store at `[0x2664 + count++]` |

The **hunt state discards non-frame bytes**, which is why sending arbitrary junk to the control
channel produces no reaction — until the opening-frame validator, which is a plain `recv` of 10
bytes and therefore *does* reject junk with reason 8.

### 9.2 Command dispatch

`0x2245` reads one complete frame, then `0x33cd` dispatches `table[[0x28c3] + min(cmd,7)*4]`.

---

## 10. The data channel

After `CONNECT`, the two sides exchange the mailer session over the reverse connection as **raw
bytes with no framing and no escaping**. The only byte-transformation table anywhere in the binary
is telnet's, indexed over `0xF0`–`0xFF` for IAC handling (`0x5958`, used by the telnet pump at
`0x59b2`); the VMP path has no counterpart, matching the manual's "true binary".

The control channel stays open alongside it and continues to carry frames — in practice only
`DISCONNECT`.

---

## 11. Diagnostics

What an observation means, for a caller:

| Observation | Meaning |
|---|---|
| Silence on connect, forever | Normal. VMODEM is blocked in `recv()` waiting for your 10-byte hello. It never greets. |
| `10 02 00 04 00 01 00 08` then close | Your opening frame was malformed (or was not a VMP frame at all). Nothing was rung. |
| `10 02 00 04 00 01 00 04` then close | **Your hello was accepted.** The answerer could not open the data channel back to you. |
| `10 02 00 04 00 01 00 07` | The answerer thinks you are itself. |
| Repeating `10 02 00 02 00 02` every 4.5 s | It is ringing its mailer; keep waiting. |
| `10 02 00 02 00 05` | No free virtual COM port. |
| `10 02 00 02 00 03` | The mailer answered; the session is live on the data channel. |
| Anything sent before you speak | Not VMODEM — some other service on the IVM port. |

**Timing tells you where reason 4 came from.** The answerer's `connect()` is blocking and
unguarded, so:

- reason 4 arriving in *less than one round-trip time* ⇒ `socket()`/`connect()` failed **locally**
  on the answerer, before any packet was sent. Typical causes: no route to the caller's address
  family, a host-only/NAT'd VM with no default route, or a firewall rejecting the outbound call.
- reason 4 arriving after several seconds ⇒ the SYN went out and was refused.
- No reply at all for tens of seconds ⇒ the SYN went out and is being dropped (the caller is
  firewalled), and the answerer is stuck in `connect()`.

---

## 12. Provenance

| Offset | What lives there |
|--------|------------------|
| `0x2100` | buffered `recv` on the control socket, one byte at a time |
| `0x214d`–`0x21e0` | byte-FSM state transitions, length assembly, 0x100 bound |
| `0x2245` | read one complete frame |
| `0x239c` | send `DISCONNECT(reason)` on the current socket |
| `0x240b` | **opening-frame validator** (the 10-byte `recv`) |
| `0x24cc` | assign a virtual COM port / `BUSY` |
| `0x26e4` | `accept()`; peer sockaddr → `gs:0x738` |
| `0x2ce0` | state × command dispatch table |
| `0x2d60` | **frame sender** (marker, BE length, DLE stuffing) |
| `0x2e05` | send `NAK`, echoing the offending command |
| `0x2e50` | send `RINGING` |
| `0x2ecd` | send `CONNECT` |
| `0x3100` | enter data mode; `CONNECT 57600/ARQ/VMP` to the COM port |
| `0x3252` | **`CONNECT-REQ` handler** |
| `0x3327` | send `CONNECT-REQ` (caller side) |
| `0x33cd` | command dispatch loop |
| `0x3913` | caller's data-channel listener setup |
| `0x37c7` | `getsockname()` → the port advertised in `CONNECT-REQ` |
| `0x398e` | **`socket()` + `connect()` back to the caller** |
| `0x3af5` | ring loop |
| `0x34a9` | send `HELLO` (caller side) |
| `0x3509` | caller's VMP dial sequence |
| `0x59b2` | telnet data pump (for contrast — not used by VMP) |

---

## 13. Confirmed on the wire

Against live nodes, July 2026.

**The hello format is right** — a malformed opener and a well-formed one take visibly different
paths (`fido.bajer.cz:3141`):

```
>>> 2a 2a 45 4d 53 49 5f 49 4e 51 0d          "**EMSI_INQ\r"  (deliberately malformed)
<<< 10 02 00 04 00 01 00 08                   DISCONNECT reason 8, close

>>> 10 02 00 06 00 06 01 00 ff ff             HELLO
    (silence — accepted)
>>> 10 02 00 08 00 00 c3 aa 00 00 00 00       CONNECT-REQ, port 50090
```

**A complete call**, 2:423/81, caller `132.145.41.221` listening on 50090:

```
 t+0.00  ──►  TCP connect to 78.44.170.214:3141
 t+2.03  ──►  10 bytes   HELLO
 t+2.73  ──►  12 bytes   CONNECT-REQ, port 50090
 t+2.76  ◄──  SYN 78.44.170.214:59838 → 132.145.41.221:50090     the DATA channel
 t+2.80  ◄──   6 bytes   RINGING          (control channel)
 t+5.02  ◄──   6 bytes   RINGING
 t+6.82  ◄══ 166 bytes   mailer banner + EMSI_REQ   (data channel)
 t+6.82  ══►  EMSI_DAT
 t+7.82       identified, DISCONNECT sent, both channels closed
```

yielding `T-Mail 2608.OS2/C50`, *Guru (fido.bajer.cz) #2*, sysop Milos Bajer, and seven AKAs
across five networks.

---

## 14. Field results and open questions

Five VMP responders, called from a host advertising port 14592 (verified reachable from an
unrelated internet host):

| Node | Callback | Mailer answered | Identity |
|---|---|---|---|
| 2:423/81 | yes | yes | **full** — T-Mail 2608.OS2/C50, system, sysop, 7 AKAs |
| 2:5025/2 | yes, **from a different IP** (§7.2) | yes | none — EMSI stalls |
| 2:371/52 | yes, **only on port 14592** (§7.1) | yes | none — EMSI stalls |
| 2:334/403 | yes | yes | none — EMSI stalls |
| 2:221/360 | **never attempted** | — | — |

The identity column is as measured *before* the EMSI role fix in item 2 below; with the fix
2:371/52 also yields a full identity (Xenia/2 Mailer 1.99.00). The VMP result — four of five
responders opened the data channel and rang their mailer — is unaffected by it.

So the protocol and this implementation work. What remains is two unrelated problems:

1. **2:221/360 never attempts the callback.** Reason 4 arrives with **no SYN ever leaving their
   side**, on a port proven reachable, and — decisively — with *the same latency as reason 6*
   (119 ms vs 122 ms against a 40 ms round trip). Reason 6 is a pure-CPU path that touches no
   network at all, so an identical timing means the callback costs nothing: `socket()`/`connect()`
   is failing locally. Causes consistent with that: no route to the caller, a `getpeername()`
   returning an address the answerer cannot reach (a proxy or NAT in front of it), or a
   reimplementation that never tries. Its TCP fingerprint on 3141 (window scale 9, SACK,
   timestamps) is a modern stack, not 1997 OS/2 TCP/IP. VMODEM's own log would settle it —
   `Making outgoing call to address <ip>`, `to port <n>`, `The outgoing connect returned error
   <n>` (`0x393e`, `0x395f`, `0x3984`).
2. **The EMSI stall was a caller-side bug, and it is not VMP-specific.** Several nodes reached
   their mailer but produced no identity: the remote sent `**EMSI_REQ`, ignored the `**EMSI_DAT`
   that followed, and re-sent `**EMSI_REQ` on its own timer until it hung up.

   The cause is that we never announced ourselves. FTS-0056 has the *calling* system transmit
   `EMSI_INQ`; `fidomail/pkg/emsi` defaults to `InitialStrategy: "wait"` (answering-side
   behaviour), which only sends INQ if the peer stays silent. A mailer that greets immediately and
   then waits for the caller's INQ therefore never receives one. Confirmed by driving the exchange
   by hand against 2:5025/2 ("The Brake! Mailer 718.a15"): sending `**EMSI_INQC816\r` before the
   DAT made it answer `EMSI_ACK EMSI_ACK` plus its own `EMSI_DAT` immediately.

   Two things were ruled out first, cheaply: the packet length and CRC are correct (`A4C7` over
   `EMSI_DAT`+LLLL+data — the same span qico uses at `emsi.c:265`), and the double CR our library
   appends (`send.go:239`) is *not* the trigger — a single-CR DAT failed identically.

   The tester first worked around this with `InitialStrategy: "send_inq"`, which gained full
   identity from 2:371/52 (Xenia/2 Mailer 1.99.00) and 2:420/33 (FrontDoor, POPPER BBS).

   **Fixed properly in the library.** `fidomail/pkg/emsi` now carries an explicit handshake role
   (`Config.Role`, `Session.HandshakeCaller`): a caller answers the answerer's `EMSI_REQ` with
   `EMSI_INQ` (FSC-0056 session setup step 2) before its `EMSI_DAT`, escalates silence with an
   unsolicited INQ, and never solicits with `EMSI_REQ`. Every dialling site here calls
   `HandshakeCaller()` — VModem, IFCICO, and both modem-test paths. `send_inq` is kept in the
   VModem tester as the *preamble* only: it announces at connect, which is what the results above
   were measured with and saves a REQ round-trip against banner-first answerers. The library's
   bare `Handshake()` default is deliberately unchanged (still answerer-shaped when no role is
   set), so the role has to be requested explicitly. Response is latched to one INQ per handshake,
   with one bounded extra INQ if a second `EMSI_REQ` arrives while we await ACK — which covers an
   announce that crossed the mailer's own greeting and was never acted on. (The announce cannot be
   lost by ringing: EMSI only runs after the `CONNECT` frame, i.e. after the mailer picked up.)
3. The VMP socket→COM pump was not located; only the telnet one (`0x59b2`) was. This does not
   affect a caller — §10's "raw binary" rests on the absence of any non-telnet escaping table plus
   the manual's explicit statement, and is now corroborated by a live session in which the EMSI
   stream crossed the data channel byte-for-byte. What stays unknown is its *teardown*: what the
   pump does when carrier drops mid-stream, and in which order the two sockets go down.
4. **No file transfer has ever crossed a VMP data channel.** *(recorded 2026-07-26)* Every call in
   §13/§14 stopped after the EMSI handshake and hung up. Byte transparency is therefore
   established over a few hundred bytes of EMSI packets, not over a Hydra/ZedZap batch — minutes
   of sustained bidirectional traffic followed by a DTR-drop teardown mid-flow. Anyone turning
   this into a mailer rather than a prober is running that path first, not last.
5. **The caller's state machine is documented structurally, not behaviourally.**
   *(recorded 2026-07-26)* §4's row `0x2d20` says which commands the caller routes where; it does
   not say what VMODEM's own caller does with them — whether it needs a `RINGING` before
   `CONNECT`, how it reacts to `BUSY` or `NAK`, whether it emits anything after `CONNECT`, or its
   teardown order. That matters only in one direction: a *caller* can be written without it (this
   one was), an *answerer* cannot, because an answerer has to tolerate whatever real callers do.
   The code is located — `0x3509` (dial sequence) with `0x34a9`, `0x3327`, `0x3913`, `0x37c7` —
   so this and item 3 are the two disassembly targets left that are worth the effort. Everything
   else an answerer needs is in §15 and §16.

> **A caller-side trap worth recording.** These calls first appeared to fail *universally*. The
> cause was the caller's own cloud firewall: an ingress rule permitted only a narrow port range,
> and packets to any other port were dropped **at the cloud edge, before reaching the instance**.
> That makes the answerer's callback SYN invisible to `tcpdump` running on the instance, which
> looks exactly like "the remote never tried". Before concluding anything about a remote node,
> confirm the advertised port is reachable *from a third-party host*, not merely bound locally.

---

## 15. What the vendor manual settles *(new, 2026-07-26)*

`VMODEM.TXT` is a user manual — AT commands, S registers, result codes — and §1 dismisses it as
"user-visible behaviour". Five of its statements are nevertheless load-bearing, and one of them
corrects this document.

- **`ATDV n host` dials a specific virtual port.** *"ATDV 3 vmbbs.gwinn.com" will dial the third
  VMODEM port at the given internet address.* This is the caller-side origin of a non-`0xFFFF`
  port index in the hello (§6 item 5): a real caller can and does name one, so an answerer needs a
  policy for an index it cannot honour. VMODEM's own is `BUSY` for any index ≥ its configured port
  count.
- **S7 is "number of seconds to wait for carrier (connection) when dialing".** The caller's
  patience is an operator setting on the *caller's* machine, not a protocol constant, and
  expiring it yields `NO CARRIER` to the application. §8's "45 s is ten rings" is advice for
  writing a caller, and is not something an answerer may assume it has.
- **`CONNECT 57600/ARQ/VMP` is a result code**, listed alongside `NO CARRIER`, `NO DIALTONE`,
  `BUSY`, `RING` and `RINGING` — all of them things VMODEM reports *to the application* over the
  COM port. The manual says outright that the rate *"is given only to satisfy the application
  program"*. §8 reaches the same conclusion from `0x3100`; between them there is no room left to
  read that string as wire traffic.
- **`BUSY` means what the frame says it means:** *"a connection to the vmodem port was established
  at the remote site. However, no available communications ports (COM1, COM2 etc) were available
  to assign the connection to."* Confirms §4 command 5 — the call reached a VMODEM, and the
  VMODEM had nowhere to put it.
- **Shared Secret is not a VMP mechanism.** Under SECURITY the manual describes an APOP-style
  challenge/response (it cites RFC 1321 and RFC 1725): the *BBS* sends a unique string each logon,
  VMODEM appends the user's secret, runs MD5 over the pair, and returns the digest — configured by
  quoting the secret in the dial string, `ATDT vmbbs.gwinn.com "Hi There"`. That entire exchange
  lives in the **terminal data stream**, between a BBS and a human's session; it never reaches a
  control frame, and it has nothing to do with the version-stamp comparison in §7 item 4. The wire
  format is documented only in an `MD5.ZIP` the author distributed by email
  (`ray@gwinn.com`, 1997) — unobtainable now, and not needed: an FTN mailer never sees this layer.

---

## 16. Implementing an answerer *(new, 2026-07-26)*

Sections 2–14 describe how to *call* a VMODEM. Answering one is a different exercise, because
compatibility now runs the other way: a caller only has to satisfy VMODEM, an answerer has to
tolerate whatever real callers send. These are the rules that fall out of the sections above.

1. **Read the hello as a frame, not as ten bytes.** VMODEM's validator is a blocking
   `recv(sock, buf, 10, 0)` with no DLE-unstuffing (§6) — which means it cannot accept its own
   protocol's hello if the requested port index happens to contain `0x10`. That is a defect, not a
   wire rule. Parse with the ordinary frame reader (§9.1) and accept anything well-formed.
2. **Emit nothing on the data channel that the peer's mailer did not ask for** — in particular not
   `CONNECT 57600/ARQ/VMP`, which is a COM-port result code (§15), and not a banner of your own.
   The caller hands those bytes straight to its mailer.
3. **Do not refuse loopback.** Reason 7 is reachable only down the ≥24-byte `CONNECT-REQ` path
   (§7 item 4), which VMODEM 1.20's own client never takes, so on the wire the check is dormant.
   Replicating it buys nothing and costs the ability to test a caller against your own answerer on
   one host.
4. **Order: data channel, then `CONNECT`, then bytes.** Open the reverse connection before ringing
   anything (§2), and send `CONNECT` before the mailer writes its first byte. A caller is entitled
   to ignore data-channel traffic until it holds *both* the accepted connection and the `CONNECT`
   frame — this repository's caller does exactly that (`vmp_client.go`, the `vmpCmdConnected`
   arm), and an answerer that starts the session early simply stalls until the caller's timeout.
5. **Answer promptly, but ring at least once.** §8's 4.5 s cadence exists because VMODEM is
   ringing a COM port that a human-configured mailer may take several rings to notice. An answerer
   whose mailer is already listening should send one `RINGING` and then `CONNECT`. The caller's
   budget is its S7 (§15), which may be well under the ten rings §8 suggests.
6. **Bound the reverse dial, and gate admission before it.** VMODEM's `connect()` is blocking and
   unguarded — that is precisely why reason 4 can arrive in 100 ms, in several seconds, or never
   (§11). Use a deadline. And apply whatever per-IP admission and session caps you have to the
   *control* connection **before** dialing back, or every unauthenticated connect buys an outbound
   connection and a goroutine on your side.
7. **Take the callback address from `accept()`, never from the frame — and never from a PROXY
   header.** §7: the caller controls the port and nothing else. That one property is what keeps an
   answerer from being a connection-laundering service for arbitrary destinations. A listener that
   honours PROXY-protocol source rewriting on the VMP port gives it away, since the "peer address"
   then becomes something the relay — or whoever can reach the relay — supplies.
8. **Keep reading the control channel for the whole session, and tear down through it.**
   `DISCONNECT` can arrive at any moment (§10), including mid-transfer. On normal local completion
   send `DISCONNECT` reason 2 — the code VMODEM itself sends when DTR drops (§5) — and close both
   sockets.

### 16.1 An IVM port is not necessarily a VMP port

Most IVM-flagged nodes do not speak VMP at all. A scan of all 19 IVM nodes in the FidoNet nodelist
(day 198, 2026-07-17) found **4** genuine VMP responders, about **5** running an ordinary EMSI
mailer session over telnet-binary or raw TCP, about **9** with the port down or filtered, and one
flagged `IVM` with no host to connect to. `internal/testing/protocols/vmodem_tester.go`'s
`classify()` carries a `binkp` arm as well, because some IVM ports answer with binkd.

An answerer that wants to serve what callers actually send should therefore dispatch on the
opening bytes rather than assume VMP:

| First bytes | Serve |
|---|---|
| `10 02` | VMP (this document) |
| `FF` (telnet IAC) | telnet-binary shim, then EMSI |
| `**EMSI_` | EMSI directly |
| binkp `M_NUL` | binkp |
| silence | ambiguous — see below |

Silence is the interesting case, and it resolves in the answerer's favour. A VMP caller sends its
hello immediately, so after a few seconds of nothing an EMSI greeting is the better guess — and it
is safe for a VMP caller that turns up late, because §9.1's hunt state discards every byte outside
a frame. It is *not* safe early: a caller that probes for a greeting before committing (this
repository's tester reads for 700 ms after its hello, and treats any non-VMP bytes as proof the
peer is not a VMODEM) will walk away if you speak first. Wait out the silence window before
greeting.
