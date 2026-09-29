# Stage 2: authorization

**English** | [Bahasa Indonesia](README_AUTHORIZATION.id.md)

This stage protects partnership operations and training/enrollment/certificate operations. It builds on the working Stage 1 stack and uses its isolated fixture accounts for regression tests.

## Authentication

The partnership and training services verify access tokens using the same `JWT_SECRET` as auth-service. They accept HS256 signatures, require a valid expiration time, and require a nonempty account ID (`sub`) and a recognized role (`ADMIN`, `UMKM`, or `MITRA`). Missing, expired, unsigned, incorrectly signed, or otherwise invalid tokens return HTTP 401. `X-User-Role` grants no access.

Health endpoints remain public. Training catalog routes remain public. All partnership API routes require a verified UMKM or Mitra token.

## Partnership rules

| Operation | Allowed caller | Allowed existing status |
| --- | --- | --- |
| Browse UMKM list/details | Mitra | — |
| Browse Mitra list/details | UMKM | — |
| Create a proposal | UMKM or Mitra, using their own account | — |
| Read proposal details | Requester or receiver | Any |
| Read outgoing/incoming lists and summaries | Current account, scoped to that account | Any |
| Acknowledge a proposal as read | Receiver | Any |
| Approve or reject | Receiver | `DIAJUKAN`, `DITINJAU` |
| Cancel | Requester | `DRAFT`, `DIAJUKAN`, `DITINJAU` |
| Submit a contract document | Requester, using their own document | `DIAJUKAN`, `DITINJAU`, `MENUNGGU_DOKUMEN_TTD` |

Current upload attachments must be active and owned by the requester. They are checked before creating the proposal. Status and contract updates also check the actor and existing status in the SQL statement, so a stale decision cannot overwrite a completed transition. Unauthorized actors receive HTTP 403; invalid or stale transitions receive HTTP 409.

Migration `031_seed_partnership_statuses` adds the required reference statuses, which previously existed only in the optional dummy dataset. Fresh databases can create proposals without loading dummy data. Existing labels are preserved; rollback retains these shared reference rows.

Contract submission still references the legacy `document.transaksi_dokumenterunggah` table. Regular attachments use `documents.master_dokumen`. The contract test uses two legacy metadata fixtures; connecting the current upload endpoint to contract submission still needs a separate schema migration. The read acknowledgment endpoint does not yet persist a read timestamp. Approval retains the existing behavior and does not require a signed contract.

## Training and certificate rules

| Operation | Allowed caller |
| --- | --- |
| Manage training programs, including admin lists/statistics | Admin |
| Create a training program | Admin; the stored creator is resolved from the authenticated account |
| Enroll, update progress, complete training | UMKM account that owns the business/enrollment |
| Read enrollment lists | Business owner or admin |
| Attach an evaluation document | Enrollment owner, using their own active document |
| Request a certificate | Enrollment owner |
| Read certificate lists, dashboards, details, or PDFs | Business/certificate owner or admin |
| Read global certificate lists/statistics; approve or reject certificates | Admin |

Ownership is resolved through the database: account → UMKM owner → business → enrollment → certificate. A submitted business, enrollment, or certificate ID does not establish ownership. Missing or inaccessible resources return HTTP 403 for ordinary users; database failures do not grant access. Admins can inspect learner records but cannot use learner mutation endpoints on their behalf.

Certificate PDFs use `sertifikat_<id>.pdf`, so two participants with the same name and training title cannot overwrite each other's files. Existing certificates can regenerate their PDF during an authorized download.

This change does not add cross-service logout/revocation checks, live account-status checks, or a new assessment/completion policy. Valid tokens are checked locally until expiration; registration approval and course-completion rules need separate coverage. The scope above is not a claim that every endpoint in all five services has been audited.

## Run the checks

From the repository root:

```bash
bash tests/stack/run.sh
```

The command builds all five services, runs the existing 46-request Newman suite, runs `tests/stack/authorization.py` against the real services, and repeats the Stage 1 storage/persistence checks. Its database and volumes are disposable, and no host ports are published. Go and Newman are not required on the host.

The authorization runner tests valid workflows as well as forged/expired tokens, role spoofing, non-admin management calls, cross-account access, foreign document references, and closed/replayed partnership decisions. It creates its own courses, enrollments, certificates, uploads, and proposals. The original six account fixtures remain the identity baseline.

Successful completion ends with:

```text
Stage 1 stack checks passed.
Stage 2 authorization checks passed.
```

For Go unit tests with Go 1.26.3 and a C compiler for race detection:

```bash
(cd services/partnerships-service && go test -race ./...)
(cd services/training-service && go test -race ./...)
```

CI also checks formatting, runs `go vet`, and builds these two services. The existing auth/user/frontend checks continue to run. Newman writes `tests/postman/reports/newman.xml`; the authorization runner writes `tests/stack/reports/authorization.txt`. The `Local stack integration` workflow uploads both as `api-contract-results`. Generated reports are ignored by Git.

After applying the patch, run the additive reference-data migration and rebuild the services in the normal development environment, from the repository root:

```bash
docker compose --env-file .env -f infra/docker-compose.yml run --build --rm db-migrate
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait --wait-timeout 180 partnerships-service training-service
```

Restart Vite if it is running. Keep your existing `.env` and database volumes; no new environment variables are required.
