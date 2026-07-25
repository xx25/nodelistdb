# pkg/emsi caller-role fix — instructions for the NodelistDB side

Reply to the `EMSI-INVESTIGATE.md` report ("the calling side never sends
`EMSI_INQ`", The Brake! 718.a15 wedge at 2:5025/2). The fix shipped in
`github.com/xx25/fidomail/pkg/emsi` — pin the module at the commit that
carries this file, or later. The investigation doc is retired; its
findings are summarised where they matter below.

The design combines the report's options (1) and (3): the role is now
explicit, and there is a bounded adaptive recovery. Option (2) — flipping
the library default — was deliberately NOT taken; see "Why the default
did not change".

## What you need to change: two call sites

The session now has explicit role entry points. Both testers should call

```go
err := sess.HandshakeCaller()   // instead of sess.Handshake()
```

(equivalently: `cfg.Role = emsi.RoleCaller` before building the session).
That is the whole migration:

- **IFCICO tester** — currently on the bare default, i.e. the broken
  answerer-shaped caller. `HandshakeCaller()` fixes it against
  Brake!-family answerers with no other configuration.
- **VModem tester** — currently sets `InitialStrategy: "send_inq"`
  (`emsiCallerStrategy`). Switch it to `HandshakeCaller()` too, and
  **keep** the `send_inq` strategy: it is now purely the *preamble*
  ("announce at connect") and is the recommended caller preamble — your
  §6 measurements (Xenia 2:371/52, FrontDoor 2:420/33 identities) came
  from announce-at-connect, and it saves a REQ round-trip against
  banner-first answerers. But it is no longer load-bearing: with the
  role set, even the default `"wait"` preamble completes against a
  strict answerer via the REQ→INQ response.

This satisfies §7.1 (a dialling session completes against a
spec-conforming answerer without per-deployment configuration) and §7.2
(one behaviour across both testers).

## What the caller role actually does

- **REQ → INQ** (FSC-0056 session setup step 2): the first `EMSI_REQ`
  is answered with a single `EMSI_INQ`, then `EMSI_DAT` goes out
  immediately — the exact hand-driven sequence your §2 confirmed
  against The Brake!. The response is **latched**: it fires only if no
  INQ has been sent yet this handshake, so a `send_inq` caller whose
  preamble already crossed the answerer's banner REQ stays
  byte-identical on the wire to the behaviour you field-validated in
  §6. (We reproduced the alternative during development: answering
  every REQ with INQ lands a redundant INQ mid-phase at the peer and
  provokes NAK/DAT-resend churn.)
- **Silence → INQ announce**: a caller that hears nothing (or only
  noise tokens such as a stray `EMSI_HBT`) escalates with an
  unsolicited INQ. It never solicits with `EMSI_REQ` — the pre-fix
  "wait" caller did, which claims the answerer role on the wire and
  deadlocks two answerers against each other.
- **Bounded lost-INQ recovery** (the report's option 3): a *second*
  `EMSI_REQ` received while awaiting ACK — your §1 wedge signature —
  triggers one INQ ahead of the DAT resend, once per handshake. This
  covers the INQ-eaten-by-line-noise case. It is a knowingly imprecise
  heuristic (a peer REQ-churning after a delivered INQ is wire-
  indistinguishable), so it is hard-bounded: at most one extra INQ per
  handshake, pinned by a regression test.
- qico's stricter forms were considered and **not** adopted:
  wait-for-second-REQ before DAT (the spec says "attempt to handshake
  immediately", and your §2 experiment proves immediate DAT works),
  the `INQ INQ\r` single-CR framing, and the single CR after DAT. The
  last two remain open, separable items — unchanged in this fix so any
  regression bisects cleanly.

## Why the default did not change

`Handshake()` + `DefaultConfig()` with no `Role` still derives the role
from `InitialStrategy` (`"wait"` → answerer, anything else → caller).
The library is consumed as a versioned dependency, and silently flipping
the bare default would change behaviour for any answerer-side consumer
that never set a strategy. The legacy surface is therefore
answerer-shaped and documented as such; callers opt in with one line.
That is the §7.1 trade-off resolved in favour of never breaking an
existing consumer — the cost is your two-line migration above.

## §7.3 — "handshake failed" vs "completed, no identity"

The signals already exist; use them together:

| Signal | Meaning |
|---|---|
| `Handshake*()` returns error | handshake **failed** (see `GetCompletionReason()`: `TIMEOUT` vs `ERROR`) |
| error `nil`, `GetRemoteInfo() != nil` | completed with identity |
| error `nil`, `GetRemoteInfo() == nil` | completed, peer supplied **nothing usable** (e.g. its DAT failed to parse after we ACKed) |
| `GetCompletionReason() == ReasonNCP` | completed, identity present, but no compatible transfer protocol |
| `GetRemoteInfo().SystemName == "[Extracted from banner]"` | identity is a **banner-text guess**, not an EMSI identity — report it as weaker evidence |

## §7.4 — regression posture

For a `send_inq` caller the wire is byte-identical to the previously
deployed behaviour except in two situations that previously wedged or
failed: (a) the peer solicits and no INQ has gone out yet, (b) the peer
re-solicits after our DAT. T-Mail / ifcico / Platinum Xpress /
FrontDoor flows are untouched. The one node your report flagged as
flaky either way — **2:423/81** (T-Mail 2608.OS2/C50) — remains the one
to watch after you upgrade; nothing in this change targets it.

Library-side coverage: ten role tests in `pkg/emsi/role_test.go`,
including a Brake!-shaped strict answerer (discards un-INQ'd DATs,
re-REQs on a timer), a lost-preamble recovery, REQ-doubling and
REQ-churn INQ-storm bounds, and a legacy pin that the old
`Handshake()`/`"wait"` path still never sends INQ.
