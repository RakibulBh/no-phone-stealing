# safe-london

Real-time crowdsourced safety reporting backend for London. Users submit photos of incidents along with location data. The server cross-references the report against historical Metropolitan Police crime data, sends both the image and nearby crime history to GPT-4o for threat verification and trend analysis, then pushes enriched alerts to all connected clients over WebSockets.

Built at the April 2026 Cursor Hackathon.

## How it works

```
User submits report (image + lat/lng + theft_type)
        │
        ▼
  ┌─────────────┐     ┌──────────────────┐
  │  Validate    │────▶│  SQLite: fetch    │
  │  input       │     │  nearby crimes    │
  └─────────────┘     └────────┬─────────┘
                               │
                               ▼
                    ┌──────────────────────┐
                    │  Format crimes as    │
                    │  plain text for LLM  │
                    └────────┬─────────────┘
                             │
                             ▼
                  ┌────────────────────────┐
                  │  GPT-4o Vision API     │
                  │  image + crime history │
                  │  → threat assessment   │
                  │  → trend analysis      │
                  └────────┬───────────────┘
                           │
               ┌───────────┴───────────┐
               ▼                       ▼
        threat_level < 3         threat_level ≥ 3
        & is_threat=false        or is_threat=true
               │                       │
               ▼                       ▼
           discard              ┌──────────────┐
                                │ Save to DB   │
                                │ Broadcast WS │
                                └──────────────┘
```

The LLM does all spatial reasoning. No clustering math lives in the Go codebase — the model receives formatted text like `"On 2024-01 at Oxford Street, robbery occurred."` and produces natural-language trend analysis such as `"Based on 5 recent moped thefts within 500m, suspects typically flee North towards Camden High Street"`.

Historical crime data comes from the [Police UK API](https://data.police.uk/docs/). A background worker seeds a local SQLite cache on startup by sweeping a coarse grid across Greater London. This avoids hitting rate limits during live report processing.

## Project structure

```
cmd/server/              Server entrypoint and dependency wiring
internal/
  domain/                Entities, interfaces, validation — zero external deps
  usecase/               Orchestrates report processing pipeline
  infrastructure/
    openai/              GPT-4o Vision client, prompt construction, response parsing
    policeuk/            Police UK API client, background grid seeder
    sqlite/              SQLite repository for crimes and reports
  interface/
    http/                Echo REST handlers (POST /reports, GET /trends)
    ws/                  WebSocket hub with ping/pong keepalive
```

Clean Architecture — dependencies point inward. Domain knows nothing about HTTP, databases, or OpenAI.

## API

All responses use a standard envelope:

```json
{ "success": true, "data": { }, "error": "" }
```

### `POST /api/v1/reports`

Submit a safety report. Returns `202 Accepted` immediately; processing happens async.

**Content-Type:** `multipart/form-data`

| Field      | Type        | Description                                              |
|------------|-------------|----------------------------------------------------------|
| `image`    | file        | JPEG/PNG, max 5MB                                        |
| `metadata` | JSON string | `{"lat": 51.5074, "lng": -0.1278, "theft_type": "..."}` |

Coordinates must fall within Greater London bounds (lat 51.28–51.70, lng -0.51–0.33).

```bash
curl -X POST http://localhost:8080/api/v1/reports \
  -F "image=@photo.jpg" \
  -F 'metadata={"lat":51.5074,"lng":-0.1278,"theft_type":"phone_snatch"}'
```

### `GET /api/v1/trends?lat={lat}&lng={lng}`

Returns cached historical crimes near the given coordinates. Useful for rendering map overlays independent of active reports.

```bash
curl "http://localhost:8080/api/v1/trends?lat=51.5074&lng=-0.1278"
```

### `GET /api/v1/ws`

WebSocket endpoint. Connected clients receive enriched alerts as JSON whenever a verified threat is processed. The server sends pings every 30s and drops dead connections.

### `GET /health`

Returns `{"status": "ok"}`.

## Setup

**Requirements:** Go 1.21+, Node.js (for git hooks), CGO enabled (SQLite).

```bash
git clone https://github.com/RakibulBh/no-phone-stealing.git
cd no-phone-stealing

cp .env.example .env
# Edit .env and add your OpenAI API key

go mod download
npm install          # installs husky + commitlint

# Run with hot-reload
air

# Or build and run directly
go build -o server ./cmd/server && ./server
```

The server starts on port 8080 by default (configure via `PORT` env var). The Police UK data seeder runs in the background on startup — it takes a few minutes to populate the local cache across London's grid.

## Environment variables

| Variable         | Required | Default | Description          |
|------------------|----------|---------|----------------------|
| `OPENAI_API_KEY` | Yes      | —       | OpenAI API key       |
| `PORT`           | No       | `8080`  | Server listen port   |

## Testing

All tests use the standard library. No external mocking frameworks. Infrastructure tests run against in-memory SQLite. HTTP client tests use `httptest` servers.

```bash
CGO_ENABLED=1 go test ./...
```

35 tests across 6 packages:

- **domain** — coordinate validation, report validation, crime text formatting, threat actionability
- **infrastructure/sqlite** — table migrations, spatial queries, idempotent inserts, alert persistence
- **infrastructure/policeuk** — API response parsing, error handling, grid generation
- **infrastructure/openai** — payload construction, base64 encoding, response parsing, markdown fence stripping, error cases
- **usecase** — full pipeline flow, low-threat filtering, error propagation, save-failure resilience
- **interface/http** — multipart parsing, input validation, 202/400 responses, trends endpoint

## Development

Commits are enforced via [Conventional Commits](https://www.conventionalcommits.org/) using husky + commitlint. The pre-commit hook runs the full test suite.

```
feat: implement openai multimodal client with history context
test: add domain entity validation tests
fix: handle markdown-fenced llm responses
chore: update dependencies
```

Hot-reload via [air](https://github.com/air-verse/air) — edit any `.go` file and the server restarts automatically.

## License

MIT
