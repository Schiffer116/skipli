# API

Documents the Go server as it actually behaves today, not the target design —
see `TODO.md` for what's still missing. Base path for every route is `/api`
(e.g. `POST /auth/send` below is really `POST /api/auth/send`), added by a
prefix-stripping wrapper in `internal/server/server.go` so the resource
packages (`internal/board`, `internal/auth`) never see it.

All responses are `application/json` unless noted. CORS is wide open
(`Access-Control-Allow-Origin: *`, see `internal/server/cors.go`) since auth
is Bearer-token based, not cookies.

## Auth (`internal/auth`)

### `POST /auth/send`

Generates a 6-digit verification code, stores it, and emails it via Gmail
SMTP.

Request:

```json
{ "email": "you@example.com" }
```

Response: `200` empty body. `500` if storing the code or sending the email
fails (e.g. missing `APP_EMAIL`/`APP_EMAIL_PASSWORD` credentials).

### `POST /auth/verify`

Exchanges an email + code for a JWT.

Request:

```json
{ "email": "you@example.com", "code": "123456" }
```

Response:

```json
{ "accessToken": "<JWT, HS256, 1-day expiry, {email, exp} claims>" }
```

**Known gap:** `code` is currently accepted unconditionally, not checked
against the stored value — see the comment on `verify` in `auth.go`. This is
a deliberate temporary bypass, not a bug to work around.

### `GET /auth/email`

Returns the caller's own email, decoded from their Bearer token.

Request: `Authorization: Bearer <token>` header, no body.

Response:

```json
{ "email": "you@example.com" }
```

`401` if the header is missing/malformed or the token fails verification.

## Boards (`internal/board`)

### `POST /boards`

Creates a board and a membership record for its owner in one transaction.

Request:

```json
{ "name": "...", "owner": "you@example.com", "description": "..." }
```

Response: `201`, body is the request echoed back (does **not** currently
include the generated board `id` — callers can't learn it from this
response; use `GET /boards` afterward).

**Known gap:** this route is **not** behind `RequireAuth`. `owner` is
whatever the caller puts in the request body — anyone can create a board
"as" any email right now. Every other board route that should be
auth-protected (`GET /boards/{id}`) has the same gap; only `GET /boards` is
currently wired to the JWT middleware.

### `GET /boards`

Lists every board the caller is a member of (owned or otherwise — a single
query covers both, unlike the old API's separate `boards`/`teamBoards`
lists).

Request: `Authorization: Bearer <token>` header, no body. Owner/member for
the query comes from the token, not a request parameter.

Response:

```json
[
  { "id": "...", "owner": "...", "name": "...", "description": "..." }
]
```

`[]` (not `null`) when the caller has no boards. `401` if the token is
missing/invalid.

Boards come back sorted by the caller's own `Order` (see
`PUT /boards/order`), with name breaking ties.

### `PUT /boards/order`

Saves the caller's own order for one section of the workspace page (owned or
team boards). Each id's position in the list becomes the `Order` on the
caller's `MEMBER#` item for that board, so members don't affect each
other's order.

Request: `Authorization: Bearer <token>` header, body:

```json
{ "boardIds": ["...", "..."] }
```

Response: `204`. `400` for duplicate ids or more than 100 of them, `403` if
the caller isn't a member of every listed board (nothing is written; the
update is conditional so it can't create a membership), `401` if the token
is missing/invalid.

### `GET /boards/{id}`

Fetches one board by id.

Response: the board object (same shape as above), or `404` if it doesn't
exist. **No auth or membership check** — any caller can fetch any board by
id if they know it.

### `DELETE /boards/{id}` — implemented but not exposed

A `delete` handler exists in `board.go` but is **not registered** in
`RegisterRoutes`, so this endpoint doesn't actually exist yet. Also doesn't
clean up the board's `MEMBER#` item(s) if/when it is wired up.

## Not yet implemented

Cards, tasks, board membership management (invite/accept/list members),
board update. See `TODO.md` for the full breakdown against the old API.
