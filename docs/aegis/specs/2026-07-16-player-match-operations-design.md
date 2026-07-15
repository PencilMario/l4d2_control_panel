# Player Match And Operations Design

## Intent

Expand the game instance player page into a live match and player operations view. The page must show a concise match summary and an operations-oriented list of human players with the fields exposed by L4D2 `status`, while preserving A2S scores and existing kick/ban actions.

## User Experience

The confirmed layout has two layers:

1. A match summary card showing hostname, map, human/max player count, host operating system, game version and secure state, plus private and public addresses.
2. A human-player operations table showing UserID, name, UniqueID, connected time, ping, loss, score and actions.

On desktop, player information uses a full-width operations table. On mobile, each row folds into a two-line detail card so no horizontal scrolling is required. Kick and permanent-ban controls remain visible only for a mapped positive UserID.

BOT entries such as Rochelle, Ellis, Coach and Nick are never displayed. `state` and `rate` remain out of scope.

## Canonical Data Owner

`internal/players/status.go` becomes the sole parser for the current SRCDS `status` response. A structured parser returns both match metadata and human player records from one response. It must support the observed L4D2 human format with an optional numeric entity column:

```text
#  2 1 "Sir.P" STEAM_1:0:526095818 00:48 29 0 active 100000
```

The parser also accepts the existing single-number format and filters lines whose UniqueID is `BOT`. Missing optional summary fields produce zero/empty values rather than failing the complete parse.

`players.Service` joins parsed human players with A2S players by unique name. SRCDS `status` owns UserID, name, UniqueID, connected text, ping and loss. A2S owns score and legacy duration. The merged list begins with status humans so an operational record can still appear when no unique A2S score match exists; unmatched A2S humans are appended as compatibility fallback rows with an unmapped UserID.

## API Contract

`GET /api/instances/{id}/players` retains the existing top-level `map`, `max_players` and `players` fields and adds a `match` object:

```json
{
  "map": "c2m1_highway",
  "max_players": 12,
  "match": {
    "hostname": "6",
    "version": "2.2.4.3 10097",
    "secure": true,
    "os": "Linux Dedicated",
    "map": "c2m1_highway",
    "private_address": "127.0.1.1:27991",
    "public_address": "221.215.78.153:27991",
    "humans": 1,
    "max_players": 12
  },
  "players": [{
    "user_id": 2,
    "name": "Sir.P",
    "unique_id": "STEAM_1:0:526095818",
    "connected": "00:48",
    "ping": 29,
    "loss": 0,
    "score": 12,
    "duration": 48000000000
  }]
}
```

`score` is nullable for a status human without a unique A2S match. Existing matched players continue to receive a JSON number. Legacy `duration` remains present for A2S-backed players to avoid an unnecessary contract removal, although the new UI displays `connected`.

Unknown match strings are empty, numeric values are zero, and secure state is nullable when the status version line does not identify it. The UI renders unknown values as `--`.

## Parsing Rules

- `hostname`, `version`, `udp/ip`, `os`, `map` and `players` summary lines are parsed by anchored line prefixes.
- The version display excludes the trailing parenthesized build annotation but retains the game version and build number.
- Secure state is `true` for `secure`, `false` for `insecure`, and unknown otherwise.
- The UDP/IP line separates the first address as `private_address` and the bracketed `public` address as `public_address`.
- The players line extracts current human count and maximum players.
- Human rows extract the first numeric value as UserID, allow one optional numeric entity column, then parse quoted name, UniqueID, connected text, ping and loss.
- BOT rows are excluded before service merging.
- Duplicate status names retain their parsed operational identities, but their scores remain null when a unique A2S join cannot be proven; the service must not guess which player owns an A2S score. Only A2S-only compatibility rows use an unmapped UserID.

## Error And Refresh Behavior

The player modal keeps its existing one-shot load behavior and error surface. Repository, A2S transport or console transport errors continue to show the current player-query error rather than stale data. A malformed optional line does not fail the entire snapshot; successfully parsed sections remain visible.

The API does not expose the raw console response. Console response framing and command restrictions remain unchanged.

## Compatibility Boundary

- Preserve authenticated player routes and existing kick/permanent-ban request payloads.
- Preserve positive numeric UserID as the sole action identifier.
- Preserve top-level `map`, `max_players`, player `name`, `user_id`, `score` and `duration` fields.
- Preserve A2S as score owner and SRCDS status as operational identity owner.
- Do not display BOTs, `state`, `rate`, raw console text or network addresses beyond the match summary.
- Do not add persistent match history or automatic polling in this change.

## Testing

Backend tests cover the supplied real L4D2 response, optional entity column, legacy row format, BOT filtering, all match fields, secure/insecure/unknown parsing, private/public address extraction, missing optional fields, status-first/A2S score merging, duplicate-name ambiguity and compatibility fallback rows.

Frontend tests cover the summary card, desktop operations columns, mobile detail labels, unknown-value rendering, nullable score, positive-UserID actions, unmapped action suppression and existing confirmation behavior. Playwright verifies the real fixture player modal at desktop and mobile viewport sizes.

## Working Drafts

### TaskIntentDraft

Show live match metadata and human-player operational fields on the instance player page without exposing raw console output or changing player action semantics.

### BaselineReadSetHint

The design is constrained by `CONTEXT.md`, `internal/players/status.go`, `internal/players/service.go`, `internal/docker/client.go`, `internal/httpapi/server.go`, `web/src/app/App.tsx`, and their focused tests. The observed `sirphomesv` status output is the real-format parser fixture.

### ImpactStatementDraft

Affected owners are the status parser, player aggregation service, player API JSON and player modal. The key invariants are human-only display, unique identity matching, additive API compatibility and unchanged UserID-based moderation. Persistent history, polling and BOT operations are non-goals.
