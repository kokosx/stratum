# StratumCMS

Modern self-hosted CMS in Go — WordPress familiarity, modern architecture, one binary.

## Status

Active development. Current vertical slice: `stratum serve → setup → login → create Page → add blocks → publish → public URL`.

## Build

```bash
go build -o stratum ./cmd/stratum
# or
make build
```

Requires Go 1.22+, no external build deps.

## Run

```bash
./stratum serve
# open http://localhost:8080/admin/setup
```

Options:

- `ADDR` / `LISTEN_ADDR` — listen address (default `:8080`), or pass via serve flags
- `STRATUM_DATA_DIR` — data directory (default `./data`)
- `STRATUM_ENV=production` — enable production mode

## Development

```bash
make fmt        # gofmt -w
make fmt-check  # verify formatting
make check      # fmt-check + vet + test + build
go test ./...
go vet ./...
```

CI runs `make check` on push/PR.

## Doctor

```bash
./stratum doctor
./stratum doctor --json
./stratum doctor --production
```

Admin also exposes Site Health at `/admin/tools/site-health`.

## Backup

```bash
./stratum backup create --output /tmp/stratum-backup.zip
./stratum backup verify /tmp/stratum-backup.zip
./stratum backup restore /tmp/stratum-backup.zip
```

Admin: `/admin/tools/backups`.

## Import

```bash
./stratum import wordpress dump.xml --dry-run
./stratum import wordpress dump.xml
```

Admin: `/admin/tools/import`.

## Configuration

Site settings are stored in the `site_settings` singleton row (site_title, language, timezone, site_url, indexing, etc.). No generic wp_options table for core settings.

## Project layout

- `internal/app` — composition root
- `internal/creator` — starter site generation
- `internal/content` — Entry / ContentTypeDefinition
- `internal/document` / `internal/blocks` / `internal/layouts` — SDT and block registry
- `internal/rendering` / `internal/themes` — Entry → Revision → SDT → Blocks → Theme → HTML pipeline
- `internal/routing` / `internal/publishing` / `internal/pagecache` — routing and publish
- `internal/storage` — SQLite / Turso via `database/sql` + `sqlc`

## License

MIT (or as configured for the repository).
