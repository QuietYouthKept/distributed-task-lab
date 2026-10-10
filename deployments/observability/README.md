# Local observability overlay

This is a development-only Compose overlay. It starts Prometheus, Grafana, and one explicitly named Worker alongside the base PostgreSQL/API services so Prometheus can scrape both API and Worker metrics over the Compose network. The Worker exposes `/metrics` only inside that network; no host port is published.

From the repository root, set the API and admin tokens required by the base Compose file. Compose passes the API token to Prometheus as a runtime secret file; the token is not embedded in the Prometheus configuration or its command line.

```powershell
$env:API_TOKEN = "local-development-token"
$env:ADMIN_TOKEN = "another-local-development-token"
docker compose -f deployments/docker-compose.yml -f deployments/observability/docker-compose.observability.yml --profile observability up --build
```

Open `http://localhost:9090/targets` and confirm both `tasklab-api` and `tasklab-worker` are `UP`. API scrape uses the existing Bearer token authorization; the token is mounted only into Prometheus through a Compose secret. Development alert rules are loaded from `tasklab-alerts.yml` and can be inspected at `http://localhost:9090/alerts`. The Grafana datasource/dashboard are provisioned from the adjacent configuration.

The extra Worker is a single stable scrape target, not a model for production replica discovery. It consumes the same development queue as the base Worker service. Prometheus is pinned to the locally verified `prom/prometheus` registry digest; Grafana remains a mutable development image and anonymous Admin remains enabled. Production authentication/TLS, alert ownership, Alertmanager/on-call routing, SLO thresholds, and a complete immutable image policy are separate release work and are not implied by this setup. To exercise a real scrape-down alert lifecycle against an isolated API, run `scripts/e2e/run_observability_alert_e2e.ps1`; it creates a unique Compose project and removes only that project's containers, network, and volume.
