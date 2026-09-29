# UMKM Tumbuh frontend

**English** | [Bahasa Indonesia](README.id.md)

React 18 and TypeScript application built with Vite. Feature modules live in `src/features/`; shared HTTP and authentication utilities live in `src/shared/`.

## Start development

Use Node.js 22 and npm. Start the backend using the [local development guide](../docs/README_LOCAL_DEV.md), then run from the repository root:

```bash
[ -f frontend/.env ] || cp frontend/.env.example frontend/.env
cd frontend
npm ci
npm run dev
```

Open [the application](http://localhost:5173). Keep an existing `.env` and compare it with [.env.example](.env.example) when updating. The root `.env` configures the backend; it does not replace `frontend/.env`.

## Local API proxy

The example environment enables `VITE_USE_DEV_PROXY=true`. In development, the browser sends API requests to Vite on port 5173. Vite forwards each service to its IPv4 address inside WSL, reading the five published API ports from the root `.env`.

For an existing `frontend/.env`, add or update:

```dotenv
VITE_USE_DEV_PROXY=true
```

Restart `npm run dev` after changing this flag or a backend port. To try it for one run without editing the environment file, run `VITE_USE_DEV_PROXY=true npm run dev` inside `frontend/`.

With default ports, login follows `localhost:5173/backend/auth/api/v1/auth/login` to `127.0.0.1:8080/api/v1/auth/login` within WSL. Service prefixes keep user, document, partnership, and training routes separate, including paths such as `/admin/training` that must reach training instead of auth. Existing relative `/api/v1/documents/...` calls remain supported.

Check [auth database health through Vite](http://localhost:5173/backend/auth/api/v1/health/db). It should return JSON from auth-service. This route is useful when the same backend endpoint works in WSL but its direct Windows URL keeps loading.

Proxy URL overrides apply only to `npm run dev` with the flag enabled. Setting the flag to `false` uses the explicit API URLs below. Production builds also use the explicit URLs, which must match the deployment; the development flag does not install a production proxy. See [Vite's proxy reference](https://vite.dev/config/server-options.html#server-proxy).

## Backend addresses

The HTTP client in [src/shared/api/http.ts](src/shared/api/http.ts) selects the API for each request. The values below are used for direct connections and production builds. The development proxy flag overrides these eight values during development. Set one base URL per service, including `/api/v1`:

| Variable | Default value |
| --- | --- |
| `VITE_AUTH_API_BASE_URL` | `http://localhost:8080/api/v1` |
| `VITE_ADMIN_API_BASE_URL` | `http://localhost:8080/api/v1` |
| `VITE_USER_API_BASE_URL` | `http://localhost:8081/api/v1` |
| `VITE_PARTNERSHIP_API_BASE_URL` | `http://localhost:8082/api/v1` |
| `VITE_DOCUMENT_API_BASE_URL` | `http://localhost:8083/api/v1` |
| `VITE_TRAINING_API_BASE_URL` | `http://localhost:8084/api/v1` |
| `VITE_CERTIFICATE_API_BASE_URL` | `http://localhost:8084/api/v1` |
| `VITE_API_BASE_URL` | `http://localhost:8082/api/v1` |

`VITE_API_BASE_URL` is the fallback for default-service requests. It is not a shared gateway for all five services. Certificates use the training service.

If you change a published backend port, restart Vite to reload it from the root `.env`. With the proxy disabled, update the matching frontend URL as well. If Vite uses another origin, such as port 5174, update `FRONTEND_URL` in the root `.env` and recreate the backend containers with `docker compose ... up -d` using the full command in the local guide.

`VITE_*` values are browser configuration. Do not store database passwords, `JWT_SECRET`, or S3 secret keys in them.

## Available commands

Run inside `frontend/`:

| Command | Purpose |
| --- | --- |
| `npm ci` | Install dependencies from the lockfile |
| `npm run dev` | Start the Vite development server |
| `npm run lint` | Run ESLint; errors and warnings fail the check |
| `npm run test:login` | Check login requests, errors, cancellation, and timeouts against a local test server |
| `npm run test:proxy` | Check routing through Vite to five local test services |
| `npm run build` | Check TypeScript and build into `dist/` |
| `npm run test:pages` | Test search, pagination, training drafts, product filters, and registration upload responses |
| `npm run check` | Run lint, build, and all the tests above |
| `npm run preview` | Preview an existing build locally |

Run `npm run build` before previewing. Preview commonly uses port 4173; backend requests still require a matching `FRONTEND_URL`. Preview does not start the backend.

`npm run check` runs the same frontend checks as the frontend CI job. Tests use local HTTP servers and synthetic data. The `test:pages` suite loads the frontend modules through Vite's SSR loader and checks request/response handling and error paths without a browser or Docker; it does not render React components. Older schema test files under `src/**/__tests__` are not included in this command.

## Accounts and testing

The normal stack seeds the admin configured in the root `.env`. Register UMKM and Mitra accounts through the application; local email is available in [Mailpit](http://localhost:8025).

`bash tests/stack/run.sh`, run from the repository root, checks backend integration, runs the Newman API collection, and removes all its test accounts afterward. Those accounts are not available for a later browser session. The Stage 1 check does not test browser rendering or every application feature.

For startup failures, inspect the browser console and network requests, then check the corresponding backend logs. See the [local guide](../docs/README_LOCAL_DEV.md) for service addresses and commands.

## Login stays on “Memproses...”

Login requests time out after 15 seconds and show an error so the form can be retried. The deadline includes reading the response body. The optional registration-status request after a non-admin login also has a 15-second limit; if it fails, the page uses the route derived from the login response. Requests are not retried automatically.

A timeout means the response did not finish; it does not identify the connection problem. From the repository root, check the normal development stack:

```bash
docker compose --env-file .env -f infra/docker-compose.yml ps auth-service postgres
curl -i --max-time 10 http://127.0.0.1:8080/api/v1/health/db
```

If this command works in WSL while [direct auth health](http://localhost:8080/api/v1/health/db) keeps loading in Windows, enable the [local API proxy](#local-api-proxy) and use the health link through port 5173. If that succeeds but login still fails, inspect the login request's URL, status, and timing in the browser's Network tab. In proxy mode, the request URL should start with `http://localhost:5173/backend/auth/`. In direct mode, check `VITE_AUTH_API_BASE_URL` against the published auth port; an origin/CORS error requires checking `FRONTEND_URL` against the frontend address.

`npm run test:login` takes about 30 seconds and requires no Docker database or real account. It checks the request code against controlled HTTP responses; it does not validate the user's browser connection or the full dashboard flow.
