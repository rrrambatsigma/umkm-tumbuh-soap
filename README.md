# UMKM Tumbuh

**English** | [Bahasa Indonesia](README.id.md)

A platform for managing micro, small, and medium enterprises (UMKM), partner collaboration, documents, and business dashboards. The frontend uses React and TypeScript; four Go services share a PostgreSQL database. Garage provides S3-compatible storage, and Mailpit receives development email.

## Run locally

Requirements: Docker Desktop/Engine with Compose 2.24.4 or newer, Node.js 22, npm, and Git. Enable Docker Desktop integration for your distribution when using WSL.

Run these commands from the repository root. Existing environment files are preserved:

```bash
[ -f .env ] || cp .env.example .env
[ -f frontend/.env ] || cp frontend/.env.example frontend/.env
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait --wait-timeout 180
```

Start the frontend in a second terminal:

```bash
cd frontend
npm ci
npm run dev
```

Open [the application](http://localhost:5173). The default local admin is `admin@example.com` / `admin12345`. These values come from `ADMIN_EMAIL` and `ADMIN_PASSWORD` in the root `.env`; changing them does not reset an existing account.

The backend starts PostgreSQL, applies migrations, seeds the admin, and configures Garage before starting the dependent APIs. S3 credentials are generated and shared automatically. The frontend reads its own `frontend/.env`. The example enables `VITE_USE_DEV_PROXY=true`, routing browser API calls through Vite to the backends in WSL. Add this flag to an existing frontend environment file to enable the same behavior; see the [frontend guide](frontend/README.md#local-api-proxy).

See the [local development guide](docs/README_LOCAL_DEV.md) for environment variables, startup order, volumes, and troubleshooting.

## Services

| Component | Default address | Purpose |
| --- | --- | --- |
| Frontend | http://localhost:5173 | React application |
| Auth service | http://localhost:8080/api/v1 | Authentication and admin API |
| User service | http://localhost:8081/api/v1 | Profiles and dashboards |
| Partnerships service | http://localhost:8082/api/v1 | Partnership requests and collaboration |
| Document service | http://localhost:8083/api/v1 | Document uploads and downloads |
| PostgreSQL | localhost:5432 | Shared database |
| Garage | http://localhost:3900 / http://localhost:3903 | S3 API / admin API |
| Mailpit | http://localhost:8025 | Development email inbox; SMTP uses port 1025 |

Published backend ports bind to `127.0.0.1` by default. Change host ports in the root `.env` and restart Vite so its proxy reads the new values. With the proxy disabled, also update the matching frontend URLs. Container ports remain fixed.

## Repository structure

| Path | Contents |
| --- | --- |
| [frontend/](frontend/) | React 18, TypeScript, Vite, Tailwind CSS, and feature modules |
| [services/auth-service/](services/auth-service/) | Account authentication and admin operations |
| [services/user-service/](services/user-service/) | Profiles and dashboards |
| [services/partnerships-service/](services/partnerships-service/) | Partnership API |
| [services/document-service/](services/document-service/) | File API backed by Garage |
| [infra/db/migrations/](infra/db/migrations/) | Shared PostgreSQL schema and reference data |
| [infra/garage/](infra/garage/) | Garage configuration and bootstrap |
| [tests/stack/](tests/stack/) | Isolated Stage 1 integration checks and fixtures |
| [tests/postman/](tests/postman/) | Existing Postman/Newman collections |

Docker builds use Go 1.26.3. PostgreSQL uses version 16, and Garage is pinned to version 2.0.0.

## Test the local stack

```bash
bash tests/stack/run.sh
```

This builds a separate test project with six fixture accounts and one account registered by Newman, runs the API collection, checks all four services, uploads and downloads files through storage, reruns migrations and bootstrap, and recreates the containers to verify persistent data. It uses `.env.example` and the test overlay, publishes no host ports, and cleans up its test containers and volumes.

During the wrong-database check, this error is expected:

```text
ERROR:  Fixtures require the isolated umkm_tumbuh_test database
```

A successful run ends with:

```text
Stage 1 stack checks passed.
```

The test does not leave the development application running. Start it using the commands above. All test accounts are temporary; use the admin seed or register accounts in the normal application. Read the [fixture and test guide](docs/README_LOCAL_DEV.md#run-the-isolated-stage-1-check) for details.

The [Postman/Newman guide](tests/postman/README.md) documents the current contract suite, JUnit report, and archived collections. Passing Stage 1 confirms the local infrastructure and tested API flows; it does not validate every feature or authorization rule.

The stack command also runs the Stage 2 authorization regressions. See the [authorization guide](docs/README_AUTHORIZATION.md) for role/ownership rules, test coverage, and current limitations.

## Frontend and CI checks

```bash
npm --prefix frontend ci
npm --prefix frontend run check
```

The frontend check runs ESLint with zero warnings allowed, TypeScript and the production build, login and proxy regression tests, and page/upload regression tests. CI runs these same checks. The Go jobs retain formatting, module-tidiness, vet, tests, and build checks. See the [CI guide](docs/README_LOCAL_DEV.md#ci-and-checks-before-pushing) for local commands and workflow coverage.

## Manage the application

Run from the repository root:

```bash
# Inspect containers and logs
docker compose --env-file .env -f infra/docker-compose.yml ps -a
docker compose --env-file .env -f infra/docker-compose.yml logs --tail 80

# Stop the application and preserve its data
docker compose --env-file .env -f infra/docker-compose.yml down
```

Keep the same Compose project name to reuse existing volumes. Adding `--volumes` or `-v` to the application's `down` command deletes its local data.

The optional large CSV seed replaces application data. It is unnecessary for startup or Stage 1 checks. See the [database guide](infra/db/README.md) before using it; do not assume the dataset has a shared login password.

## Documentation

| Guide | English | Bahasa Indonesia |
| --- | --- | --- |
| Project overview | [Read](README.md) | [Baca](README.id.md) |
| Local development and Stage 1 | [Read](docs/README_LOCAL_DEV.md) | [Baca](docs/README_LOCAL_DEV.id.md) |
| Stage 2 authorization | [Read](docs/README_AUTHORIZATION.md) | [Baca](docs/README_AUTHORIZATION.id.md) |
| Frontend | [Read](frontend/README.md) | [Baca](frontend/README.id.md) |
| Database | [Read](infra/db/README.md) | [Baca](infra/db/README.id.md) |
| Postman/Newman | [Read](tests/postman/README.md) | [Baca](tests/postman/README.id.md) |
| CSV seed notes | [Read](infra/db/dummy/seed-csv/README.txt) | [Baca](infra/db/dummy/seed-csv/README.id.txt) |

Keep paired documentation consistent when changing commands, variables, or behavior. Command names, paths, API fields, and environment-variable names are identical in both languages. These language versions cover repository documentation.

## Contributing

Create a feature branch, make a focused change, run the relevant checks, and open a pull request describing the behavior and validation. Keep schema changes in `infra/db/migrations/`; do not add service-local migration folders without an architecture decision. Do not commit `.env` files or generated credentials.
