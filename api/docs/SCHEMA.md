# Database schema

Single DynamoDB table, name `"Skipli"` (defined as a duplicated local
`const tableName` in both `internal/board/board.go` and
`internal/auth/auth.go` — deliberately not shared, see the comment history in
those files if curious why). Region `ap-southeast-7`, billing mode
`PAY_PER_REQUEST` (on-demand).

Base table key: `PK` (partition, string) + `SK` (sort, string). Item types
are distinguished by their `SK` prefix, standard single-table design.

## Item types

### Board (`PK=<boardId>`, `SK="META"`)

The board record itself. One per board.

| Attribute     | Type | Notes                                                                                                                                             |
| ------------- | ---- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PK`          | S    | The board's id (a UUID)                                                                                                                           |
| `SK`          | S    | Always `"META"`                                                                                                                                   |
| `Owner`       | S    | Creator's email                                                                                                                                   |
| `ID`          | S    | Same value as `PK` — kept as a plain attribute so `attributevalue.UnmarshalMap` can populate `Board.ID` directly without deriving it from the key |
| `Name`        | S    |                                                                                                                                                   |
| `Description` | S    |                                                                                                                                                   |

### Board membership (`PK=<boardId>`, `SK="MEMBER#<email>"`)

One item per (board, member) pair — the fan-out that makes "boards this
person belongs to" queryable. Written alongside the board's `META` item in
the same `TransactWriteItems` call on create, so a board can never exist
without at least one membership record. Right now every board has exactly
one of these (the owner) — there's no invite/join flow yet, so `Member` is
always a copy of the board's `Owner`.

| Attribute | Type | Notes                            |
| --------- | ---- | -------------------------------- |
| `PK`      | S    | The board's id                   |
| `SK`      | S    | `"MEMBER#"` + the member's email |
| `Member`  | S    | The member's email               |
| `Order`   | N    | This member's position for the board on their workspace page. Set to the creation time (Unix ms) on create, rewritten to `0..n-1` by `PUT /boards/order`. Missing on memberships from before it existed, which read as `0` |

### Verification code (`PK="USER#<email>"`, `SK="VERIFICATION"`)

Holds the current email-verification code for a user. Overwritten (not
appended) on each `POST /auth/send` — only the latest code per email is kept.

| Attribute          | Type | Notes                     |
| ------------------ | ---- | ------------------------- |
| `PK`               | S    | `"USER#"` + the email     |
| `SK`               | S    | Always `"VERIFICATION"`   |
| `VerificationCode` | S    | 6-digit code, zero-padded |

**Known gap:** no TTL attribute — codes never expire. DynamoDB TTL would be
cheap to add here (a Unix-timestamp attribute + enabling TTL on the table),
this just hasn't been done yet.

## Global Secondary Indexes

### `Member` — supports "boards this person belongs to"

| Key   | Attribute | Type |
| ----- | --------- | ---- |
| HASH  | `Member`  | S    |
| RANGE | `PK`      | S    |

Projection: `KEYS_ONLY`. Only board **membership** items carry a `Member`
attribute, so only those items appear in this index — board `META` items and
verification items are naturally excluded (a GSI only indexes items that
have all of its key attributes present, so this is a "sparse index" without
needing anything special to make it so).

Because the projection is `KEYS_ONLY`, querying this index only returns
`PK`/`SK`/`Member` — not `Name`/`Description`. `board.go`'s `list` handler
does this as two steps: `Query` the index for `Member = <email from JWT>` to
get the matching board ids, then `BatchGetItem` against the base table
(`PK=<id>, SK="META"`) for the full board data. See the comment on `list` in
`board.go` for the reasoning (this was previously a single richer-projected
`Query`, but that duplicates `Name`/`Description` into every membership item
and goes stale on rename once boards have more than one member — reverted
back to the two-step form for that reason).

**Design history worth knowing if you're touching this:** this index used to
be keyed the other way (`Owner` HASH, `Member` RANGE), which only supported
"boards a given _owner_ has" — not the actually-needed "boards a given
_member_ belongs to" (an owner is just one specific kind of member; you
can't `Query` on a value that isn't the partition key). It was inverted for
that reason. If you're about to add a second access pattern, check whether
it can reuse this index's `Member` partition key before reaching for a third
GSI.

## Known gaps / not yet modeled

- No cards or tasks yet — the old Firestore API nested these as
  subcollections under a board (`boards/{id}/cards/{id}/tasks/{id}`), which
  doesn't translate directly to DynamoDB. Needs its own item-shape decision
  when that work starts (likely `PK=<boardId>`, `SK="CARD#<cardId>"` /
  `SK="CARD#<cardId>#TASK#<taskId>"`, following the same pattern as boards).
- No invite/accept flow, so membership items are only ever created for a
  board's owner at creation time — nothing adds a second member yet.
- `DELETE` on a board doesn't clean up its `MEMBER#` item(s) (and isn't even
  routed yet — see `API.md`).
