# webhook-filter

A lightweight, generic webhook router and filter.

It authenticates incoming webhooks, evaluates configurable CEL expressions against request metadata and JSON body, and forwards only matching requests to downstream services.

## What it is

This service is intentionally *generic*.

It does **not** contain hardcoded GitHub logic. If you want GitHub issue filtering, you express it in config.

## Features

- single-container deployment
- YAML config at `/config/config.yml`
- request body size limits and timeouts
- header secret auth
- bearer token auth
- GitHub signature auth
- generic HMAC-SHA256 auth
- composite auth with `all` / `any`
- CEL condition evaluation
- configurable forwarding
- structured logs
- health endpoint
- Prometheus metrics endpoint
- distroless runtime image
- non-root container user

## Quick start

```bash
docker build -t webhook-filter:local .
docker run --rm -p 8080:8080           -v ./configs/github-label-filter.yml:/config/config.yml:ro           -e GITHUB_WEBHOOK_SECRET='github-secret'           -e TUNNEL_WEBHOOK_TOKEN='tunnel-secret'           webhook-filter:local
```

Check health:

```bash
curl http://localhost:8080/healthz
```

## Configuration

Default config path:

```
/config/config.yml
```

Override it:

```bash
webhook-filter --config /path/to/config.yml
```

See [`configs/github-label-filter.yml`](configs/github-label-filter.yml) for a complete example.

## Example GitHub issue filter

The example config forwards only when:

- the request is authenticated
- the event is a GitHub issues webhook
- the issue action is `labeled`
- the label matches `config.required_label`
- the actor is in `config.authorized_users`

That is all done in configuration. The code never needs to know what a GitHub issue is.

## Response behavior

- `healthz` -> `200`
- no route matched -> `404`
- auth failed -> `401`
- body too large -> `413`
- invalid JSON -> `400`
- condition false -> configured filtered response, default `202 ignored`
- forward failed -> configured error response, default `502 forward failed`

## Forwarding model

Forwarding is conservative:

- preserve only the configured headers
- never forward hop-by-hop headers
- never forward all headers by default
- preserve the original body unless configured otherwise

## Logging

Structured logs are emitted with `slog`.

Secrets, authorization headers, and webhook bodies are not logged by default.

## Metrics

Exposed at `GET /metrics`.

Recommended series:

- `webhook_requests_total`
- `webhook_auth_failed_total`
- `webhook_filtered_total`
- `webhook_forwarded_total`
- `webhook_forward_errors_total`
- `webhook_request_duration_seconds`

## GitHub Actions

- `ci.yml` runs tests and a build on push / PR
- `publish.yml` builds and pushes container images to GHCR on version tags

## Development

```bash
go test ./...
go build ./cmd/server
```

## License

MIT
