# Finenumbers SMS Service

Production SMS portal (Admin + Client LK + Client API) on Runexis DIDAPI.

Образ: `ghcr.io/finenumbers/sms:latest` (linux/amd64, последний [GitHub Release](https://github.com/finenumbers/sms/releases)).

**Прод (Portainer + NPM):** [`docs/deploy/PORTAINER.md`](docs/deploy/PORTAINER.md).

## Quick start

```bash
cp deploy/compose/.env.example deploy/compose/.env
# set APP_MASTER_KEY (32+ random bytes, hex) and SEED_ADMIN_PASSWORD
make up
curl -s -H 'Host: api.sms.localhost' http://127.0.0.1:8080/healthz
```

Docs: [`docs/README.md`](docs/README.md). Architecture: [`docs/architecture/README.md`](docs/architecture/README.md).

## Layout

- `cmd/sms` — api / worker / all / migrate
- `internal/` — domain packages
- `db/migrations/` — PostgreSQL
- `api/openapi.yaml` — HTTP contract
- `deploy/compose/` — Postgres + Redis + api + worker + backup; прод: [`docker-compose.portainer.yml`](deploy/compose/docker-compose.portainer.yml) за NPM
- `web/` — Admin / Client SPAs (`web/admin`, `web/client`, общий `web/ui`)
