# Local development and Stage 1 checks

**English** | [Bahasa Indonesia](README_LOCAL_DEV.id.md)

Run commands from the repository root in WSL or a Linux/macOS terminal. Docker Desktop must have WSL integration enabled when using WSL. The backend runs in Docker; the frontend runs separately.

## Start the application

Requirements: Docker Engine/Desktop, Docker Compose 2.24.4 or newer, Node.js 22, and npm. Go is only needed if you run a backend outside Docker.

Run from the repository root; these commands copy only missing files:

```bash
[ -f .env ] || cp .env.example .env
[ -f frontend/.env ] || cp frontend/.env.example frontend/.env
```

Keep existing `.env` files when upgrading. Compare them with the examples and add any missing variables. The sample passwords and tokens are for local development.

```bash
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait --wait-timeout 180
docker compose --env-file .env -f infra/docker-compose.yml ps -a
```

PostgreSQL starts first, migrations run, and the admin account is seeded. Garage bootstrap configures its single node, creates the configured buckets, and publishes the generated credentials before the user and document services start. Migration, seed, and bootstrap containers should exit with code 0. The five API containers should be healthy.

In another terminal:

```bash
cd frontend
npm ci
npm run dev
```

Open [the application](http://localhost:5173). The default admin is `admin@example.com` / `admin12345`, controlled by `ADMIN_EMAIL` and `ADMIN_PASSWORD` in the root `.env`. Seeding preserves an existing admin; changing these values does not reset an existing password.

## Service addresses

| Service | Default host address | Host port variable |
| --- | --- | --- |
| Auth/admin API | http://localhost:8080/api/v1 | `AUTH_SERVICE_PORT` |
| User API | http://localhost:8081/api/v1 | `USER_SERVICE_PORT` |
| Partnerships API | http://localhost:8082/api/v1 | `PARTNERSHIP_SERVICE_PORT` |
| Document API | http://localhost:8083/api/v1 | `DOCUMENT_SERVICE_PORT` |
| Training/certificates API | http://localhost:8084/api/v1 | `TRAINING_SERVICE_PORT` |
| PostgreSQL | localhost:5432 | `POSTGRES_PORT` |
| Garage S3 | http://localhost:3900 | `GARAGE_S3_PORT` |
| Garage admin | http://localhost:3903 | `GARAGE_ADMIN_PORT` |
| Mailpit inbox | http://localhost:8025 | `MAILPIT_HTTP_PORT` |
| Mailpit SMTP | localhost:1025 | `MAILPIT_SMTP_PORT` |

All published ports bind to `127.0.0.1` by default. Container ports stay fixed, so changing a host port does not break service-to-service calls. With `VITE_USE_DEV_PROXY=true` in `frontend/.env`, Vite reads the five API host ports from the root `.env` and forwards requests to `127.0.0.1` inside WSL. Restart Vite after changing a port. With the proxy disabled, also update the corresponding frontend API URL. See the [frontend proxy guide](../frontend/README.md#local-api-proxy) if Windows can load the frontend but cannot reach a backend port. If you change `GARAGE_S3_PORT`, also update `OBJECT_STORAGE_PUBLIC_ENDPOINT`. Keep `OBJECT_STORAGE_ENDPOINT=http://garage:3900` for this Compose stack.

## Garage credentials and persistent data

`garage-bootstrap` reuses the key named `UMKM App Key`. It never deletes other keys. Bucket names come from the same environment variables used by the services. Setup failures stop bootstrap and prevent dependent services from starting.

The `garage_credentials` volume contains `/run/garage/garage.env`, owned by UID 10001 with mode 0600. The user and document containers mount it read-only and load it before starting. Do not copy placeholder S3 keys into `.env`; the Compose services use the generated credentials. Garage and bootstrap both receive `GARAGE_ADMIN_TOKEN` from Compose.

The database, Garage objects, shared credentials, legacy uploads, and generated training certificates have separate named volumes. A normal stop/recreate keeps them:

```bash
docker compose --env-file .env -f infra/docker-compose.yml down
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait --wait-timeout 180
```

Do not add `--volumes` or `-v` to the application's `down` command unless you intend to erase its local data. Keep the same Compose project name when upgrading so Docker continues using the existing volumes. Container names are now managed by Compose; use service names in commands such as `docker compose ... exec postgres`.

To rerun Garage setup explicitly:

```bash
docker compose --env-file .env -f infra/docker-compose.yml run --rm --no-deps garage-bootstrap
```

If the credentials file is lost while Garage data remains, this recovers the existing named key. Duplicate application key names cause a clear failure; inspect them in Garage and resolve the naming ambiguity before retrying. Do not delete the Garage data volume to fix credentials.

## Run the isolated Stage 1 check

```bash
bash tests/stack/run.sh
```

The script reads `.env.example` and `infra/docker-compose.test.yml`. Normal development uses the root `.env`, and Vite uses `frontend/.env`; the isolated test does not exercise those private settings or the browser UI.

The script creates a unique Compose project, publishes no host ports, and uses a separate `umkm_tumbuh_test` database and volumes. It builds the backend, runs migrations, seeds six synthetic accounts, and checks:

- Health of all five APIs and database connectivity where available.
- Fixture seeding twice and login with the expected roles and registration states.
- The 46-request Newman collection: email verification, pending/approved login, admin review, profiles, dashboards, selected rejected requests, and a read-only pass over the partnerships REST API.
- Product image upload/download through user-service.
- Document upload/download through document-service, including custom bucket names.
- Migration and bootstrap reruns without rotating the application key or deleting an unrelated key.
- Container recreation with the same credentials and byte-for-byte intact uploads.

The script prints service logs on failure and removes only its own test project and volumes on exit, including the state volume used by the `check` profile. It does not load the large CSV dataset. The same command runs in `.github/workflows/local-stack.yml`. Newman creates one additional UMKM account within this disposable database. It uses the development-only verification code in the API response; this does not test email delivery. The test overlay fixes `APP_ENV=development` for auth.

Newman writes `tests/postman/reports/newman.xml`, which Git ignores. GitHub Actions uploads it as `api-contract-results`, including on test failure when the report exists. The earlier duplicate Newman workflows have been replaced by this one isolated job. See the [API test guide](../tests/postman/README.md) for the collection and its limitations.

### Read the test output

The wrong-database test deliberately attempts to seed the `postgres` database. Its expected rejection is:

```text
ERROR:  Fixtures require the isolated umkm_tumbuh_test database
```

`INSERT 0 0` during reseeding means the existing fixture rows were preserved. Migration messages saying `Skipping already applied migration` are also expected. A successful complete run ends with:

```text
Stage 1 stack checks passed.
Stage 2 authorization checks passed.
```

`Exited` is normal for migrations, seeding, and bootstrap after they finish successfully. The final container and volume removals are test cleanup. The development app is not running afterward unless you started it separately. Use the startup commands at the beginning of this guide, then run the frontend in another terminal.

### Test accounts

All fixture passwords are `Stage1Test123!`:

| Email | Account ID | State |
| --- | --- | --- |
| admin@stage1.test | TEST_ADMIN | Admin |
| umkm.a@stage1.test | TEST_UMKM_A | Approved UMKM, separate business |
| umkm.b@stage1.test | TEST_UMKM_B | Approved UMKM, separate business |
| mitra.a@stage1.test | TEST_MITRA_A | Approved Mitra, separate profile |
| mitra.b@stage1.test | TEST_MITRA_B | Approved Mitra, separate profile |
| onboarding@stage1.test | TEST_ONBOARDING | Email verified, no profile or submitted application |

These accounts exist only inside the test project during the run. `tests/stack/fixtures.sql` refuses to run against any database whose name is not `umkm_tumbuh_test`. Rerunning it preserves existing fixture rows rather than truncating tables.

## Troubleshooting

```bash
docker compose --env-file .env -f infra/docker-compose.yml logs --tail 80 db-migrate garage garage-bootstrap
docker compose --env-file .env -f infra/docker-compose.yml logs --tail 80 auth-service user-service partnerships-service document-service training-service
```

If a port is occupied, change its host port variable and rerun `up`. If an API stays unhealthy, inspect that service's logs before changing data or credentials. An `unknown tag !reset` error means Compose must be updated to at least 2.24.4 for the isolated test overlay.

The optional `seed` profile imports the old large CSV dataset and truncates application tables. It is not required to start the application or run Stage 1 checks. Use it only with a disposable development database. See the [database guide](../infra/db/README.md) for commands and the [Postman/Newman guide](../tests/postman/README.md) for the current API contract suite and archived collections.

## CI and checks before pushing

From the repository root:

```bash
npm --prefix frontend ci
npm --prefix frontend run check
bash tests/stack/run.sh
```

The frontend checks need Node.js 22 and npm. The stack check needs Docker and Compose and builds all five Go services; no host Go installation or host Newman installation is required.

The separate Go CI jobs cover auth-service and user-service. With Go 1.26.3 installed, run their checks locally:

```bash
(
  set -e
  for service in auth-service user-service; do
    (
      cd "services/$service"
      test -z "$(gofmt -l .)"
      go mod tidy
      git diff --exit-code -- go.mod go.sum
      go vet ./...
      go test ./...
      go build ./...
    )
  done
)
```

Commit intentional module changes before running the tidy check, because it compares the files with Git. If the formatting check fails, `gofmt -l .` inside that service lists the files to format. Do not disable a gate to make CI pass.

`ci.yml` checks Go, frontend, and Compose configuration on pull requests to `main`, supported branch pushes, and manual runs. `local-stack.yml` runs the actual stack for changes to services, infrastructure, API tests, workflow files, or `.env.example`, and on manual runs. A green Compose configuration check alone does not confirm the stack starts.

The same command now includes [Stage 2 authorization tests](README_AUTHORIZATION.md). It writes the detailed authorization log to `tests/stack/reports/authorization.txt`; CI uploads it alongside the Newman report. Additional Go jobs cover partnership and training formatting, vet, race tests, and builds.

## Development rules

Do not commit:

```text
.env
infra/db/dummy/generated/
infra/db/dummy/metadata/
generated_credentials.csv
```

Commit:

```text
.env.example
infra/db/migrations/
infra/db/loaders/
infra/db/dummy/seed-csv/
```

The source of truth for schema is:

```text
infra/db/migrations/
```

Do not add new service-local migration folders unless the architecture decision changes.
