# 3. Defer Syzygy endgame tablebases

Date: 2026-08-30

## Status

Accepted

## Context

Syzygy tablebases give perfect play for positions with few pieces: WDL
(win/draw/loss) tables probed inside the search and DTZ (distance-to-zero) tables
probed at the root to pick a move that makes progress under the fifty-move rule.
They are a meaningful strength gain in endgames and are on the design backlog.

Adding them now runs into two problems:

1. **No mature pure-Go prober.** The reference implementations (Fathom,
   `syzygy1/probetool`) are C. The only Go options are thin CGO wrappers around
   Fathom or unmaintained partial ports.
2. **The project is strictly `CGO_ENABLED=0`.** The Docker build, the release
   pipeline (`.goreleaser.yaml`) and the cross-compilation matrix all assume a
   static, cgo-free binary. Introducing CGO would fragment the build and the
   release artifacts.

A correct pure-Go WDL+DTZ prober is roughly two thousand lines of intricate code
(the RE-PAIR decompression and the Syzygy indexing scheme) that is hard to test
without tablebase files in CI. It deserves its own focused change, not a rider on
a large search/eval batch.

## Decision

Defer Syzygy tablebase support. Ship the rest of the backlog batch (PVS,
aspiration windows, richer Lazy SMP, tapered evaluation, pondering, Polyglot
book) without it.

When it is picked up, the preferred approach is a **pure-Go prober** (keeping
`CGO_ENABLED=0`), starting with WDL-only probing in the search and adding the
DTZ root probe afterwards. A CGO/Fathom binding gated behind a build tag is the
fallback if the pure-Go effort proves too large.

## Consequences

- Endgame play stays at search + tapered-eval strength; no perfect play with
  ≤ 7 pieces.
- `SearchParams` has no tablebase hook yet; adding one later is additive.
- The UCI layer will need `SyzygyPath` (and probably `SyzygyProbeDepth` /
  `SyzygyProbeLimit`) options when the feature lands.
