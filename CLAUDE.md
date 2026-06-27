# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Architecture

This is a Google Calendar sync pipeline with three components:

1. **`crud-api/`** — Go HTTP server (port 8080, deployed to Vercel). Exposes `POST /events` to batch-upsert Google Calendar events into MongoDB, deleting any events not in the submitted batch (full-sync semantics). Also serves `GET /docs` with a JSON Schema of the event payload.

2. **`mcp-server/`** — Go HTTP server (port 8081, deployed to Vercel). Implements the [MCP protocol](https://spec.modelcontextprotocol.io/) over HTTP POST. Exposes a single tool `search_events(start_date, end_date)` that queries MongoDB and returns events in the given range. Intended to be connected to Claude or another LLM as an MCP server.

3. **`google-script/sync_calendar.gs`** — Google Apps Script. Fetches all calendar events ±30 days from today and POSTs them to the `crud-api`. Run on a time-based trigger in Google Apps Script.

Both Go services share the same MongoDB collection (`calendar.events`), keyed by Google Calendar `id`. The `Event` struct is duplicated in both services.

## Local Development

Start MongoDB:
```bash
docker compose up -d
```

Run crud-api locally:
```bash
cd crud-api && go run ./cmd/main.go
```

Run mcp-server locally:
```bash
cd mcp-server && go run ./cmd/main.go
```

## Environment Variables

Both services read from a `.env` file at the module root:

| Variable      | Description                          | Default                    |
|---------------|--------------------------------------|----------------------------|
| `MONGODB_URI` | MongoDB connection string            | `mongodb://localhost:27017` |
| `MONGODB_DB`  | Database name                        | `calendar`                 |
| `API_TOKEN`   | Bearer token for auth (optional)     | (none — auth disabled)     |

## Deployment (Vercel)

Each service is an independent Vercel project. The `vercel.json` in each module rewrites all routes to `api/index.go`, which is the Vercel serverless entry point. The `cmd/main.go` is only used for local development.

## MongoDB Connection Pattern

Both services use a `sync.Once`-guarded singleton (`getCollection()`) so the MongoDB connection is reused across Vercel invocations within the same container lifetime. The MongoDB collection is always `events`.
