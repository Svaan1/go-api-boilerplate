# Go API Boilerplate

Production-oriented Go HTTP API boilerplate tailored to my specific needs.

## Quick start

Requirements: Go 1.25.13 or newer Go 1.25 patch release, Docker Compose v2, and GNU Make. `.env.example` is reference only; shell variables must be exported explicitly for native runs.

### Compose

Compose publishes PostgreSQL, API, Prometheus, and Grafana only on loopback. PostgreSQL credentials and fallback HMAC secret in `compose.yaml` are local-development examples; never reuse them outside this stack.

```sh
# Optional: supply a non-production local signing secret (at least 32 bytes).
export APP_AUTH_HMAC_SECRET='local-development-secret-change-this-value'
docker compose up --build -d postgres api
curl -i http://127.0.0.1:8080/healthz
curl -i http://127.0.0.1:8080/readyz
# Optional dashboards and Prometheus UI.
docker compose --profile observability up --build -d
```

Stop services with `docker compose down`. Named PostgreSQL volume persists; `docker compose down -v` deletes local database data. Compose API connects to service host `postgres`, not host loopback. API and DB published ports bind to `127.0.0.1`; preserve that restriction unless deployment network policy is deliberately changed. Observability profile exposes Prometheus at `http://127.0.0.1:9090` and Grafana at `http://127.0.0.1:3000` (local default password `local-grafana-only`).

### Native

Start only PostgreSQL with Compose, then configure application connection for host loopback. `config/development.yaml` selects loopback API bind and debug logging.

```sh
docker compose up -d postgres
export APP_ENV=development
export APP_DATABASE_URL='postgres://local:local@127.0.0.1:5432/api?sslmode=disable'
export APP_AUTH_HMAC_SECRET='local-development-secret-change-this-value'
make run
```

`make run` runs API in foreground; Ctrl-C sends termination through signal-aware shutdown. Native Go execution requires PostgreSQL already running and healthy; app startup verifies DB connectivity.

## Configuration

`config.Load` accepts exactly `development`, `test`, `staging`, or `production`; `APP_ENV` selects stage and defaults to `development`. Resolution order: built-in defaults < `config/<stage>.yaml` < `APP_` environment variables. Environment key names uppercase path components separated by underscores, e.g. `APP_HTTP_READ_TIMEOUT`. Values in environment override stage files. Stage files contain operational, non-secret values only. `.env` is not a configuration-file source for the application; export variables or provide them through deployment environment.

Every supported setting and default:

| Environment variable                 |                                   Default | Meaning                                                                                        |
| ------------------------------------ | ----------------------------------------: | ---------------------------------------------------------------------------------------------- |
| `APP_ENV`                            |                             `development` | Select stage YAML.                                                                             |
| `APP_APP_PLUGINS`                    |                                  `system` | Enabled built-in plugins; unknown/duplicate names fail startup. Empty list disables plugins.   |
| `APP_HTTP_ADDRESS`                   |                          `127.0.0.1:8080` | Listen host and port. Production/staging YAML bind `0.0.0.0:8080`; test uses `127.0.0.1:8081`. |
| `APP_HTTP_READ_HEADER_TIMEOUT`       |                                      `5s` | Maximum time to read request headers.                                                          |
| `APP_HTTP_READ_TIMEOUT`              |                                     `15s` | HTTP server read timeout.                                                                      |
| `APP_HTTP_WRITE_TIMEOUT`             |                                     `30s` | HTTP server write timeout.                                                                     |
| `APP_HTTP_IDLE_TIMEOUT`              |                                     `60s` | Keep-alive idle timeout.                                                                       |
| `APP_HTTP_MAX_HEADER_BYTES`          |                                 `1048576` | Header size cap in bytes.                                                                      |
| `APP_HTTP_MAX_BODY_BYTES`            |                                 `1048576` | Request body cap in bytes.                                                                     |
| `APP_HTTP_TRUSTED_PROXIES`           |                                     empty | Trusted proxy CIDRs; empty means ignore forwarded client-IP headers.                           |
| `APP_HTTP_HSTS`                      |                                   `false` | Emit HSTS; stage production and staging YAML set true for TLS-terminated deployments.          |
| `APP_DATABASE_URL`                   |                              **required** | PostgreSQL URL. Not logged.                                                                    |
| `APP_DATABASE_MAX_CONNECTIONS`       |                                      `10` | Pool maximum.                                                                                  |
| `APP_DATABASE_MIN_CONNECTIONS`       |                                       `0` | Pool minimum; cannot exceed maximum.                                                           |
| `APP_DATABASE_MAX_LIFETIME`          |                                     `30m` | Maximum pooled connection lifetime.                                                            |
| `APP_DATABASE_MAX_IDLE_TIME`         |                                      `5m` | Maximum idle connection duration.                                                              |
| `APP_DATABASE_HEALTH_CHECK_PERIOD`   |                                      `1m` | Pool health-check interval.                                                                    |
| `APP_DATABASE_PING_TIMEOUT`          |                                      `5s` | Startup connectivity ping timeout.                                                             |
| `APP_AUTH_HMAC_SECRET`               |                              **required** | HS256 secret, at least 32 bytes; keep in secret manager/environment, not repository.           |
| `APP_AUTH_ISSUER`                    |                      `go-api-boilerplate` | Required JWT issuer claim.                                                                     |
| `APP_AUTH_AUDIENCE`                  |                                  `go-api` | Required JWT audience claim.                                                                   |
| `APP_AUTH_ALGORITHM`                 |                                   `HS256` | Only accepted algorithm.                                                                       |
| `APP_CORS_ORIGINS`                   |                                     empty | Allowed origins. Wildcard cannot be combined with credentials.                                 |
| `APP_CORS_METHODS`                   |               `GET,POST,PUT,PATCH,DELETE` | Allowed CORS methods.                                                                          |
| `APP_CORS_HEADERS`                   | `Authorization,Content-Type,X-Request-ID` | Allowed request headers.                                                                       |
| `APP_CORS_CREDENTIALS`               |                                   `false` | Allow credentialed cross-origin requests.                                                      |
| `APP_RATE_LIMIT_REQUESTS_PER_SECOND` |                                      `10` | Per-client token refill rate.                                                                  |
| `APP_RATE_LIMIT_BURST`               |                                      `20` | Per-client burst capacity.                                                                     |
| `APP_RATE_LIMIT_MAX_CLIENTS`         |                                   `10000` | Maximum in-memory client buckets.                                                              |
| `APP_RATE_LIMIT_IDLE_TTL`            |                                     `10m` | Client bucket expiration after inactivity.                                                     |
| `APP_RATE_LIMIT_CLEANUP_INTERVAL`    |                                      `1m` | Managed worker cleanup cadence.                                                                |
| `APP_OBSERVABILITY_DEBUG`            |                                   `false` | Debug logging; development YAML enables it; production rejects it.                             |
| `APP_OBSERVABILITY_METRICS`          |                                    `true` | Register Prometheus endpoint.                                                                  |
| `APP_OBSERVABILITY_METRICS_PATH`     |                                `/metrics` | Metrics endpoint path; cannot overlap reserved routes.                                         |
| `APP_SHUTDOWN_TIMEOUT`               |                                     `10s` | Bounded HTTP/worker shutdown.                                                                  |
| `APP_SHUTDOWN_READINESS_TIMEOUT`     |                                      `2s` | Readiness database ping timeout.                                                               |

Required secrets: `APP_DATABASE_URL` and `APP_AUTH_HMAC_SECRET`. Minimum secret length does not establish entropy; production secret must be randomly generated and managed outside source control. `.env.example` lists common names but is not loaded automatically. Never commit `.env` or real credentials.

## HTTP contract

- `GET /healthz`: liveness only; small JSON status/timestamp. Does not prove database health.
- `GET /readyz`: bounded PostgreSQL ping; returns `503` until dependency is available, without exposing dependency details.
- `GET /metrics` (configurable or disabled): Prometheus exposition.
- `GET /v1/me`: requires valid bearer JWT; returns authenticated subject, roles, and permissions.
- Unknown `/v1/` route: Problem Details response; no domain endpoints are implied.

Errors use `application/problem+json` with RFC 7807-style `type`, `title`, `status`, `detail`, `instance`, and optional validation errors. Internal failures return generic public detail; causes stay server-side. JSON request helpers require `application/json`, reject unknown fields and trailing JSON values, and distinguish malformed, oversized, and unsupported-media requests. Initialize slice/map response fields when API must emit `[]`/`{}` rather than `null`. JSON helper serializes times in RFC 3339 UTC.

Handlers may use `apiquery.ParsePagination`: one-based `page` default 1, `page_size` default 20 and max 100, with computed zero-based offset; repeated conflicting values, invalid numbers, and overflow are validation errors. `ParseSort` accepts comma-separated `sort=field,-field` against explicit handler allowlist and rejects repeated/unknown fields. `ParseFilters` accepts handler-allowlisted filter names; identical repeated values are okay, conflicting repeats and unknown fields fail. Allowlisting does not make SQL identifiers safe: bind values and safely quote/compose only allowlisted identifiers. Resource identifiers should use UUIDv7 when resource schemas are introduced; no resource schema exists now.

## Authentication example

Bearer verifier accepts only HS256, validates signature, expiry, not-before, issuer, audience, non-empty subject, and string-array roles/permissions. Missing credentials remain anonymous on public routes; invalid supplied credentials fail authentication. Route guards distinguish unauthenticated `401` from authenticated but unauthorized `403`. Current `/v1/me` needs authentication and has no role/permission policy.

Create test JWT with Python standard library; environment secret must match API secret. This local example prints bearer token to terminal, so do not use production credentials or paste token into logs/tickets.

```sh
export APP_AUTH_HMAC_SECRET='local-development-secret-change-this-value'
python3 - <<'PY'
import base64, hashlib, hmac, json, os, time

def b64(data):
    return base64.urlsafe_b64encode(data).rstrip(b'=').decode()
now = int(time.time())
header = b64(json.dumps({'alg':'HS256','typ':'JWT'}, separators=(',',':')).encode())
claims = b64(json.dumps({
    'iss': os.getenv('APP_AUTH_ISSUER', 'go-api-boilerplate'),
    'aud': os.getenv('APP_AUTH_AUDIENCE', 'go-api'),
    'sub': 'smoke-user', 'iat': now, 'nbf': now, 'exp': now + 300,
    'roles': ['admin'], 'permissions': ['profile:read'],
}, separators=(',',':')).encode())
message = f'{header}.{claims}'.encode()
signature = hmac.new(os.environ['APP_AUTH_HMAC_SECRET'].encode(), message, hashlib.sha256).digest()
print(f'{header}.{claims}.{b64(signature)}')
PY
```

Pass printed token as `Authorization: Bearer <token>` to `GET /v1/me`. Never enable production token logging.

## PostgreSQL, migrations, and sqlc

Compose PostgreSQL 17 uses local-only credentials and named data volume. There is deliberately no initial schema or placeholder migration. First real application migration establishes the initial schema; its timestamp is the migration version. Add paired timestamped `.up.sql`/`.down.sql` migrations under `migrations/`, and parameterized queries under `internal/database/query/`; review and test migrations against disposable PostgreSQL before shared environments. `sqlc.yaml` generates pgx/v5 code into `internal/database/sqlc/` from migration up files and query directory. Until actual SQL exists, generation and migrations must be safely skipped; do not invent empty tables or no-op schema.

```sh
# Set target URL explicitly; verify target before every migration operation.
export APP_DATABASE_URL='postgres://local:local@127.0.0.1:5432/api?sslmode=disable'
# Creates paired migration files; author and review real SQL before applying.
make migrate-create name=add_real_feature
# Only after reviewed, non-empty up/down SQL exists:
make migrate-up
# Roll back one migration version after reviewing data-loss implications:
make migrate-down
make generate
make generate-check
```

`migrate-down` rolls back one version; review data-loss implications first. `APP_DATABASE_URL` controls target and must never appear in committed files or shared command output.

## Plugins and workers

`plugin.Plugin` exposes `Name() string` and `RegisterRoutes(*http.ServeMux, plugin.Dependencies) error`. Plugin dependencies expose logger, pool/transaction boundary, and authentication/role/permission route guards. Enable names with `APP_APP_PLUGINS`; built-in `system` owns `/v1/me`. Duplicate or unknown names fail startup. Add real built-in plugins deliberately; do not treat unused provider interfaces as implemented integrations.

`worker.Worker` exposes `Name() string` and `Run(context.Context) error`. Supervisor starts each worker once, recovers panic as error, cancels siblings on unexpected failure, and joins workers during bounded shutdown. Rate-limit bucket cleanup is managed worker; no dummy jobs are included.

## Metrics and network exposure

Dedicated Prometheus registry exports HTTP request count, duration, in-flight requests, response size, and rate-limit rejections. Labels use method, normalized ServeMux route pattern, and status only; never raw path, query, client IP, token, or identity. `/metrics` is enabled by default and is an HTTP route: restrict reachability with bind address, reverse-proxy/network policy, firewall, or disable via `APP_OBSERVABILITY_METRICS=false`. Metrics path is configurable. Compose observability profile provides Prometheus scraping and provisioned Grafana datasource/dashboard. Do not expose metrics publicly without deliberate access controls.

Rate limiting uses bounded in-memory token buckets per client. It is not shared between process replicas; each instance has independent quota and restart clears buckets. Forwarded IP headers are trusted only from explicitly configured peer CIDRs. Deploy appropriate trusted CIDRs and network boundaries; empty list ignores forwarded headers.

## Development, tests, and CI

```sh
make fmt
make fmt-check
make lint
make vet
make test
make test-race
make test-integration
make build
make generate
make generate-check
```

Integration tests are opt-in/tagged and need PostgreSQL plus `APP_TEST_DATABASE_URL`. End-to-end test additionally requires `APP_RUN_E2E=1` and environment-only `APP_AUTH_HMAC_SECRET`. Never put secrets in CI source or artifacts. Repository CI runs formatting/generated checks, lint/static analysis, tests/race, PostgreSQL integration, build, vulnerability scan, and Docker image build.

Use `make compose-up` and `make compose-down` for repository Compose workflow, or standard `docker compose` commands shown above. `make run` runs native service. Migration target set: `migrate-create`, `migrate-up`, `migrate-down`.

## Docker deployment and shutdown

`Dockerfile` builds static Linux binary with Go 1.25.13 or newer Go 1.25 patch release and runs it as non-root in `gcr.io/distroless/static-debian12:nonroot`; config stage YAML is included, secrets are not. Build with `docker build -t go-api-boilerplate:local .`. Provide required `APP_DATABASE_URL` and strong random `APP_AUTH_HMAC_SECRET` through deployment secret management. TLS terminates externally; configure production bind/network policy, CORS, trusted proxies, metrics reachability, and HSTS only when TLS termination is guaranteed.

Configure orchestrator liveness probe `GET /healthz` and readiness probe `GET /readyz`. Set termination grace period longer than `APP_SHUTDOWN_TIMEOUT`; shutdown stops accepting requests, drains server, cancels and joins managed workers, and closes pool. Compose is local development, not a production credential/network template.

## License

MIT; see [LICENSE](LICENSE).
