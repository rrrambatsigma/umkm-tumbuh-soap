# API contract tests with Postman and Newman

**English** | [Bahasa Indonesia](README.id.md)

The automated collection is `umkm-tumbuh-current-progress.postman_collection.json`. Its 46 requests cover auth/user health, registration, email verification, pending and approved login, admin review, profiles, dashboards, selected rejection cases, and a read-only pass over the partnerships REST API (login, sent and incoming listings, summaries, UMKM and mitra directories, and role checks).

## Run the isolated suite

From the repository root, with Docker running:

```bash
bash tests/stack/run.sh
```

This starts an isolated stack, creates the fixture admin, and runs Newman 6.2.1 in a Node.js 22 container. Newman uses `ci.postman_environment.json`, which addresses services by their Compose names and contains only disposable test credentials. You do not need Newman installed on your computer.

The runner stops on a failed request or assertion, writes `tests/postman/reports/newman.xml`, then removes its test containers and volumes on exit. Git ignores generated reports. The `Local stack integration` workflow uploads the report as `api-contract-results` whenever it exists, including on failure. The two older standalone Newman workflows have been replaced by this isolated job.

The full command also checks uploads, migration/bootstrap reruns, and storage persistence. See the [local development guide](../../docs/README_LOCAL_DEV.md#run-the-isolated-stage-1-check).

## Authentication flow under test

1. Log in as the fixture admin and register a unique UMKM account.
2. Confirm that login fails while the email is unverified.
3. Request a verification code and confirm it. The isolated auth service uses `APP_ENV=development`; the test reads `dev_code` from its response.
4. Confirm the verified account can log in for onboarding while its status is `MENUNGGU`.
5. Find it in the pending-registration list, approve it as admin, and confirm its next login returns `DISETUJUI`.
6. Confirm a UMKM token cannot perform the admin approval operation.

Admin list assertions use `data.users`; the approval response uses `status: "success"`. Registration status is checked on the subsequent login response.

This is an API contract suite. It does not test SMTP delivery, browser rendering, the full profile/document submission journey, every authorization rule, or every feature of the other three services. Admin approval is exercised before the separate profile/product/sales scenarios. A passing run is evidence for these requests only.

## Optional run against your development stack

These requests create accounts, approve registration, and write products and sales. The isolated runner above removes its test data automatically; a manual run against the normal development stack leaves those records there.

With the development stack already running and `APP_ENV=development`, run:

```bash
mkdir -p tests/postman/reports
npx --yes newman@6.2.1 run \
  tests/postman/umkm-tumbuh-current-progress.postman_collection.json \
  --environment tests/postman/local.postman_environment.json \
  --bail --timeout-request 15000 --timeout-script 5000 \
  --reporters cli,junit \
  --reporter-junit-export tests/postman/reports/newman-local.xml
```

The local environment assumes auth at `http://localhost:8080` and user at `http://localhost:8081`. Base URLs omit `/api/v1`, which is already in the collection. Its admin credentials must match the target database; use a private local environment copy if yours differ. The current collection creates its own test UMKM account and does not use the old seeded UMKM/Mitra credentials in the local environment. Run from WSL when Docker is integrated with WSL.

The collection generates a fresh email, phone, and NIK when `run_id` is unset, and defaults the test password to `NewmanTest123!`. In the Postman app, clear `run_id` before a new complete run. Do not commit private environment exports or tokens.

## Archived collections

`umkm-tumbuh-backend.postman_collection.json` is an older 35-request collection. It lacks the current verification flow and has outdated assertions. Other scenario collections remain available for reference and manual maintenance. They are not the current CI gate; update their contracts before treating their results as regressions.

## Profile creation and business categories

A new profile uses the seeded UMKM type `UMKM`. Its business category is stored separately, for example `MAKANAN` for the request value `Makanan`. A category ID must not be inserted into `jenis_umkm_id`: these fields reference different tables.

The profile test checks HTTP 200 and the returned business category. If it receives another status, the assertion includes the response body in `newman.xml` so the backend error is visible without a second script error hiding it. Rebuild user-service after updating backend code; `bash tests/stack/run.sh` builds the isolated test services automatically.

The isolated stack additionally runs a separate [Stage 2 authorization suite](../../docs/README_AUTHORIZATION.md) for partnerships and training/certificates. Those checks are recorded in `tests/stack/reports/authorization.txt`, not in the Newman XML.
