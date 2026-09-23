# API migration TODO

## Auth (`/api/auth`)

- [ ] `GET /email` (requireAuth) — return caller's email from JWT

## Boards (`/api/boards`)

Full request/response shapes are in `API.md`; this section tracks status
against the old API's feature set.

- [x] `POST /` — create board (`internal/board/board.go: create`) — writes
  the board's `META` item (`PK=<id>`, `SK="META"`) and the owner's
  `MEMBER#<email>` item together via `TransactWriteItems`, so a board
  can never exist without a membership record. Now auth-protected
  (`RequireAuth`)
- [x] `GET /` — list boards (`list`) — auth-protected via
      `TokenIssuer.RequireAuth`, member comes from the JWT. Queries the
      `Member` GSI (`Member` HASH, `PK` RANGE, `KEYS_ONLY`) for boards the
      caller belongs to, then `BatchGetItem`s the base table for full board
      data, then splits the result into `{boards, teamBoards}` — unlike the
      old API's two independently-overlapping Firestore queries (owner is
      trivially also a member there, so owned boards show up in both lists),
      this split is non-overlapping: `boards` is where `Owner == caller`,
      `teamBoards` is everything else the query returned. Confirmed both
      directions end-to-end (owner sees it only in `boards`, a manually
      added non-owner member sees it only in `teamBoards`). **Known limits:**
      `BatchGetItem`'s 100-key cap and `UnprocessedKeys` aren't handled
- [x] `PUT /order` — save the caller's own board order (`order`) as
      `Order` on their `MEMBER#` items, one `TransactWriteItems` of
      conditional updates (`attribute_exists(PK)`, so it can't create a
      membership). `list` now also `BatchGetItem`s the caller's `MEMBER#`
      items to sort by it, which halves its cap to 50 boards
- [~] `GET /:boardId` — fetch one board (`get`) — keys on `PK`/`SK` matching
  the real table (`SK="META"`). **Gap: no auth or membership check** —
  anyone can fetch any board by id
- [~] `DELETE /:boardId` — delete board (`delete`) — keys on `PK`/`SK`
  matching the real table now, but still **not wired into
  `RegisterRoutes`**; also doesn't clean up the board's `MEMBER#` item(s)
  or cascade-delete cards/tasks (n/a yet)
- [ ] `PUT /:boardId` — update name/description
- [ ] `POST /:boardId/invite` — generate invite token, store it, email it —
      this is what would actually add a second `MEMBER#<email>` item to a
      board; right now every board only ever has its owner as a member since
      nothing else creates membership items yet
- [ ] `POST /:boardId/invite/accept` — redeem token, add caller to `members`
- [x] `GET /:boardId/members` (`internal/board/board.go: members`) — 404 if
      the board's `META` item is missing, otherwise `Query(PK=<boardId>,
      SK begins_with "MEMBER#")` against the base table, returned as a plain
      array of emails (matches the old API's raw `members` field, not full
      objects). Auth-protected via `RequireAuth`, but not yet restricted to
      only actual board members (needs `requireBoardMembership` below).
      Confirmed end-to-end: single member, 401 with no token, 404 for a
      nonexistent board, and multiple members (via a manually inserted
      `MEMBER#` item, since there's still no invite flow to add one for real)
- [ ] `requireBoardMembership` check — 404 if board missing, 403 if caller
      isn't a member — needed on `GET /:boardId`, `PUT`, `DELETE`, and now
      every card/task route too; the `MEMBER#` items exist to check against,
      just nothing checks them yet outside of `list` boards
- [ ] `requireBoardOwnership` check — 403 if caller isn't `owner` (needed by
      update/delete/invite)

## Cards (`/api/boards/:boardId/cards`) — `internal/card/card.go`

Schema: `PK=<boardId>`, `SK="CARD#<cardId>"` — no GSI, cards fetched via a
single base-table `Query` and sorted by `Order` in Go (fine at per-board
scale). **Gap: no auth or membership check on any card route** — same class
of gap as `board.go`'s unrouted/unchecked endpoints, tracked together below.

- [x] `GET /` — list cards for a board, sorted by `Order`
- [x] `POST /` — create card, `Order` = last + `orderGap` (1000). Also
      persists `createdAt` from the request body (matching the old API,
      which stored it but never returned it — same here)
- [x] `GET /:id` — fetch one card
- [x] `PUT /:id` — update name/description
- [x] `PATCH /:id` — reorder via `{beforeId, afterId}` midpoint logic
- [x] `DELETE /:id` — cascade-deletes its tasks in the same
      `TransactWriteItems` call as the card itself (100-item transaction cap
      unhandled, same caveat as `list` boards' `BatchGetItem`)

Confirmed end-to-end against real AWS: create → list (ordering) → reorder
(midpoint) → update → cascade-delete (verified tasks actually gone via a
direct table query, not just via the API).

## Tasks (`/api/boards/:boardId/cards/:cardId/tasks`) — `internal/task/task.go`

Schema: `PK=<boardId>`, `SK="TASK#<cardId>#<taskId>"` (sibling prefix to
`CARD#`, not nested under it, so listing cards can never accidentally pull in
task items). Same no-auth gap as cards above.

- [x] `GET /` — list tasks for a card, sorted by `Order`
- [x] `POST /` — create task, `status` defaults `"pending"`, `Order` = last +
      `orderGap`
- [x] `GET /:id` — fetch one task
- [x] `PUT /:id` — update name/description/status
- [x] `PATCH /:id` (`move`) — move to a (possibly different) card and/or
      reorder. Since `cardId` is baked into the SK, moving to a different
      card is a `TransactWriteItems` delete+put (same task id, new SK);
      staying on the same card is a plain `UpdateItem` on `Order` instead —
      **has to** branch this way, since DynamoDB's `TransactWriteItems`
      rejects two operations against the same key in one call, which a
      same-card "move" would otherwise attempt. Matches the old API's lack
      of a no-op early return: a move with neither `beforeId` nor `afterId`
      still happens, landing at `Order = 0`
- [x] `GET /:id` and the response shape generally intentionally **don't**
      replicate the old API's `order` leak (`{...doc.data()}` spread) or the
      unstored `owner` field it put in `POST /`'s response only — both read
      as old-API oversights, not behavior worth preserving

Confirmed end-to-end: create → list (ordering) → move to a different card
(id preserved, fields carried over, old location empties out) → reorder
within the same card (via the `UpdateItem` branch specifically) → update →
delete.

## Cross-cutting / design decisions still open

- [ ] Decide DynamoDB schema for cards/tasks — Firestore subcollections
      (`boards/{id}/cards/{id}/tasks/{id}`) don't map 1:1; needs its own
      single-table or multi-table design
- [x] Decide how membership is modeled — see `SCHEMA.md` for the full
      writeup: one `MEMBER#<email>` item per (board, member) pair (the
      adjacency-list pattern), fanned out from `create` via
      `TransactWriteItems`, indexed by the `Member` GSI. Firestore's
      `members` array + `array-contains` query doesn't translate directly to
      DynamoDB (no array-contains on a GSI key), hence the separate items
- [x] Single-table migration for boards: `board.go`/`auth.go` each have their
      own local `const tableName = "Skipli"` (same table name, kept
      duplicated on purpose rather than sharing a package). `create`/`get`
      write/read `PK`/`SK` consistently with the real table, and `list`
      queries the `Member` GSI + `BatchGetItem`. `delete` has matching keys
      but still isn't routed (see above)
- [ ] Static file serving of `ui/dist` as SPA fallback — only if Go is meant to
      serve the built frontend
- [ ] Realtime sync (Socket.IO in the old API: `create/update/delete/move` for
      both cards and tasks, pure broadcast relay, no persistence) — needs a
      WebSocket/SSE approach on the Go side if kept
