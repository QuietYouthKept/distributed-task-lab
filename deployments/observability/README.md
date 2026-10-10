# Local observability overlay

This is a development-only Compose overlay. It starts Prometheus, Grafana, and one explicitly named Worker alongside the base PostgreSQL/API services so Prometheus can scrape both API and Worker metrics over the Compose network. The Worker exposes `/metrics` only inside that network; no host port is published.

From the repository root, set the API and admin tokens required by the base Compose file. Compose passes the API token to Prometheus as a runtime secret file; the token is not embedded in the Prometheus configuration or its command line.

```powershell
$env:API_TOKEN = "local-development-token"
$env:ADMIN_TOKEN = "another-local-development-token"
docker compose -f deployments/docker-compose.yml -f deployments/observability/docker-compose.observability.yml --profile observability up --build
```

Open `http://localhost:9090/targets` and confirm both `tasklab-api` and `tasklab-worker` are `UP`. API scrape uses the existing Bearer token authorization; the token is mounted only into Prometheus through a Compose secret. The Grafana datasource/dashboard are provisioned from the adjacent configuration.

The extra Worker is a single stable scrape target, not a model for production replica discovery. It consumes the same development queue as the base Worker service. Prometheus/Grafana image tags remain mutable in this local-only overlay; production pinning, authentication/TLS, alert ownership, and an Alertmanager/on-call loop are separate release work and are not implied by this setup.
