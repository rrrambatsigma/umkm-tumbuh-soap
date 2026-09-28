#!/usr/bin/env bash
# ============================================================
# run-soap-tests.sh — Test otomatis SOAP Partnership Service
# (Anggota 5: Testing, Dokumentasi & Demo)
#
# Menjalankan TC00–TC29 + V01 dari tests/soap/test-case.md
# terhadap stack Docker yang sedang berjalan.
#
# Penggunaan (dari root repo):
#   bash tests/soap/run-soap-tests.sh
#
# Env opsional:
#   AUTH_URL, USER_URL, PARTNERSHIP_URL, SOAP_URL
#   ADMIN_EMAIL, ADMIN_PASSWORD
#   PRECHECK_ATTEMPTS, PRECHECK_DELAY
#
# Exit code: 0 = semua lulus · 1 = ada test gagal · 2 = preflight gagal
# ============================================================
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
AUTH_URL="${AUTH_URL:-http://localhost:8080/api/v1}"
USER_URL="${USER_URL:-http://localhost:8081/api/v1}"
PARTNERSHIP_URL="${PARTNERSHIP_URL:-http://localhost:8082}"
SOAP_URL="${SOAP_URL:-http://localhost:9090/partnership}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin12345}"
PRECHECK_ATTEMPTS="${PRECHECK_ATTEMPTS:-15}"
PRECHECK_DELAY="${PRECHECK_DELAY:-2}"
SOAP_ACTION_NS="http://umkm-tumbuh.example.com/partnerships"

REPORT_DIR="$ROOT_DIR/tests/soap/reports"
mkdir -p "$REPORT_DIR"

START_HINT="       Jalankan stack Docker terlebih dahulu:
         docker compose --env-file .env -f infra/docker-compose.yml up -d --build
       Cek status/logs:
         docker compose --env-file .env -f infra/docker-compose.yml ps
         docker compose --env-file .env -f infra/docker-compose.yml logs --tail 50 <service>"

# ------------------------------------------------------------
# Util dasar
# ------------------------------------------------------------
PY=""
for c in python3 python py; do
  if command -v "$c" >/dev/null 2>&1; then PY="$c"; break; fi
done
if [ -z "$PY" ]; then
  echo "ERROR: python (python3/python/py) tidak ditemukan — dibutuhkan untuk parsing JSON/JWT." >&2
  exit 2
fi

die() {
  echo "" >&2
  echo "ERROR: $1" >&2
  [ "${2:-}" = "hint" ] && printf '%s\n' "$START_HINT" >&2
  exit 1
}

rand_digits() {
  local n="$1" out="" i
  for ((i = 0; i < n; i++)); do out+="$((RANDOM % 10))"; done
  printf '%s' "$out"
}

# Deteksi docker yang benar-benar melihat stack. Di WSL, docker milik distro
# sering menunjuk daemon kosong — Docker Desktop engine diakses lewat docker.exe
# atau context desktop-linux; di Linux biasa cukup "docker".
DOCKER_CMD=()
for c in docker docker.exe; do
  command -v "$c" >/dev/null 2>&1 || continue
  if [ -n "$("$c" ps -q 2>/dev/null)" ]; then
    DOCKER_CMD=("$c")
    break
  fi
  if "$c" context ls 2>/dev/null | grep -q '^desktop-linux' &&
    [ -n "$("$c" --context desktop-linux ps -q 2>/dev/null)" ]; then
    DOCKER_CMD=("$c" --context desktop-linux)
    break
  fi
done

postgres_cid() {
  if [ ${#DOCKER_CMD[@]} -eq 0 ]; then
    return 1
  fi
  "${DOCKER_CMD[@]}" compose --env-file .env -f infra/docker-compose.yml ps -q postgres 2>/dev/null | head -n1
}

compose_psql() { # baca SQL dari stdin, jalankan di container postgres
  "${DOCKER_CMD[@]}" compose --env-file .env -f infra/docker-compose.yml exec -T postgres \
    sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tA -v ON_ERROR_STOP=1'
}

json_eval() { # baca JSON dari stdin, evaluasi ekspresi python (var: d)
  "$PY" -c "import sys,json; d=json.load(sys.stdin); print($1)"
}

extract_token() { # token akses dari response auth (top-level atau data)
  "$PY" -c 'import sys,json
d=json.load(sys.stdin)
t=d.get("access_token") or (d.get("data") or {}).get("access_token") or ""
print(t)'
}

jwt_sub() { # klaim sub (akun_id) dari token JWT
  "$PY" -c 'import base64,json,sys
p=sys.argv[1].split(".")[1]
p += "=" * (-len(p) % 4)
print(json.loads(base64.urlsafe_b64decode(p))["sub"])' "$1"
}

# ------------------------------------------------------------
# Pencatatan hasil
# ------------------------------------------------------------
PASS=0
FAIL=0
STEP=0

record() { # id status deskripsi detail
  local id="$1" status="$2" desc="$3" detail="${4:-}"
  printf '%s\t%s\t%s\t%s\n' "$id" "$status" "$desc" "$detail" >>"$REPORT_DIR/results.tsv"
  if [ "$status" = "PASS" ]; then
    PASS=$((PASS + 1))
    printf '  [PASS] %-6s %s\n' "$id" "$desc"
  elif [ "$status" = "SKIP" ]; then
    printf '  [SKIP] %-6s %s\n' "$id" "$desc"
  else
    FAIL=$((FAIL + 1))
    printf '  [FAIL] %-6s %s\n' "$id" "$desc"
    [ -n "$detail" ] && printf '         -> %s\n' "$detail"
  fi
}

# ------------------------------------------------------------
# HTTP helpers (REST & SOAP)
# ------------------------------------------------------------
HTTP_CODE=""
HTTP_BODY=""

http_call() { # method url [body_json] [token]
  local method="$1" url="$2" body="${3:-}" token="${4:-}"
  local args=(-sS -m 20 -X "$method" "$url" -w $'\n%{http_code}')
  [ -n "$token" ] && args+=(-H "Authorization: Bearer $token")
  [ -n "$body" ] && args+=(-H "Content-Type: application/json" --data "$body")
  local raw=""
  if ! raw="$(curl "${args[@]}")"; then
    HTTP_CODE=0
    HTTP_BODY="<connection error>"
    return 0
  fi
  HTTP_BODY="$(printf '%s' "$raw" | sed '$d')"
  HTTP_CODE="$(printf '%s' "$raw" | tail -n1)"
}

SOAP_CODE=""
SOAP_BODY=""

# soap_call slug operasi inner_xml [token] [mode]
#   mode: http (default) = Authorization HTTP header
#         none            = tanpa Authorization (untuk tes TC22)
#         soap-header     = token hanya di <soapenv:Header> (tes TC25)
#         raw             = inner_xml adalah body mentah, tanpa pembungkus Envelope
soap_call() {
  local slug="$1" op="$2" inner="$3" token="${4:-}" mode="${5:-http}"
  STEP=$((STEP + 1))
  local n reqf resf
  n="$(printf '%03d' "$STEP")"
  reqf="$REPORT_DIR/${n}_${slug}_req.xml"
  resf="$REPORT_DIR/${n}_${slug}_res.xml"

  if [ "$mode" = "raw" ]; then
    printf '%s' "$inner" >"$reqf"
  elif [ "$mode" = "soap-header" ]; then
    cat >"$reqf" <<XML
<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Header>
    <auth token="${token}"/>
  </soapenv:Header>
  <soapenv:Body>
${inner}
  </soapenv:Body>
</soapenv:Envelope>
XML
  else
    cat >"$reqf" <<XML
<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Header/>
  <soapenv:Body>
${inner}
  </soapenv:Body>
</soapenv:Envelope>
XML
  fi

  local args=(-sS -m 20 -X POST "$SOAP_URL"
    -H 'Content-Type: text/xml; charset=utf-8'
    -H "SOAPAction: \"${SOAP_ACTION_NS}/${op}\""
    --data-binary "@$reqf" -w $'\n%{http_code}')
  if [ "$mode" = "http" ] || [ "$mode" = "raw" ]; then
    [ -n "$token" ] && args+=(-H "Authorization: Bearer $token")
  fi

  local raw=""
  if ! raw="$(curl "${args[@]}")"; then
    SOAP_CODE=0
    SOAP_BODY="<connection error>"
  else
    SOAP_BODY="$(printf '%s' "$raw" | sed '$d')"
    SOAP_CODE="$(printf '%s' "$raw" | tail -n1)"
  fi
  printf '%s' "$SOAP_BODY" >"$resf"
}

expect_soap() { # id deskripsi http_diharapkan [pola...]  (pola: fixed-string, wajib ada)
  local id="$1" desc="$2" want="$3"
  shift 3
  local ok=1 detail="" pat
  if [ "$SOAP_CODE" != "$want" ]; then
    ok=0
    detail="HTTP $SOAP_CODE (diharapkan $want)"
  fi
  for pat in "$@"; do
    if ! printf '%s' "$SOAP_BODY" | grep -qF -- "$pat"; then
      ok=0
      detail="$detail | tidak ditemukan: \"$pat\""
    fi
  done
  if [ "$ok" = 1 ]; then
    record "$id" "PASS" "$desc"
  else
    detail="$detail | cuplikan: $(printf '%s' "$SOAP_BODY" | tr -d '\n' | cut -c1-220)"
    record "$id" "FAIL" "$desc" "$detail"
  fi
}

xml_field() { # ambil isi tag pertama dari SOAP_BODY: xml_field applicationId
  printf '%s' "$SOAP_BODY" | sed -n "s|.*<$1>\([^<]*\)</$1>.*|\1|p" | head -n1
}

# ------------------------------------------------------------
# Preflight — dependency service harus hidup & sehat
# ------------------------------------------------------------
preflight() { # nama url pola_wajib
  local name="$1" url="$2" pattern="$3"
  local attempt=1 code=0 body=""
  printf '  [PRECHECK] %-24s %s ... ' "$name" "$url"
  while [ "$attempt" -le "$PRECHECK_ATTEMPTS" ]; do
    local raw=""
    if raw="$(curl -sS -m 5 -w $'\n%{http_code}' "$url" 2>/dev/null)"; then
      code="$(printf '%s' "$raw" | tail -n1)"
      body="$(printf '%s' "$raw" | sed '$d')"
      if [ "$code" = "200" ] && printf '%s' "$body" | grep -qF -- "$pattern"; then
        echo "OK"
        return 0
      fi
    else
      code=0
    fi
    if [ "$attempt" -eq 3 ]; then printf 'menunggu '; fi
    if [ "$attempt" -ge 3 ]; then printf '.'; fi
    sleep "$PRECHECK_DELAY"
    attempt=$((attempt + 1))
  done
  echo ""
  if [ "$code" = "0" ]; then
    printf 'ERROR: %s is unreachable at %s\n' "$name" "$url" >&2
  else
    printf 'ERROR: %s is reachable at %s but unhealthy (HTTP %s, mengharapkan 200 berisi "%s").\n' \
      "$name" "$url" "$code" "$pattern" >&2
  fi
  printf '%s\n' "$START_HINT" >&2
  exit 2
}

echo "== UMKM Tumbuh — SOAP Partnership Service test =="
echo "AUTH_URL=$AUTH_URL"
echo "USER_URL=$USER_URL"
echo "SOAP_URL=$SOAP_URL"
echo
echo "== Preflight: dependency service =="
preflight "Auth service" "$AUTH_URL/health" '"status"'
preflight "User service" "$USER_URL/health" '"status"'
preflight "Partnerships REST" "$PARTNERSHIP_URL/health" '"status"'
preflight "SOAP service" "$SOAP_URL?wsdl" '<definitions'
echo

# Bersihkan artefak run sebelumnya hanya setelah preflight lulus,
# agar laporan run terakhir tetap tersedia bila preflight gagal.
find "$REPORT_DIR" -maxdepth 1 -type f \( -name '*.xml' -o -name 'results.tsv' \) -delete 2>/dev/null || true

# ------------------------------------------------------------
# Provision akun uji (register → profil → login admin)
# ------------------------------------------------------------
echo "== Persiapan akun uji =="
RUN_ID="$(date +%s)$RANDOM"
UMKM_EMAIL="umkm.${RUN_ID}@soap.test"
MITRA_EMAIL="mitra.${RUN_ID}@soap.test"
PASSWORD="password123"
UMKM_PHONE="62812$(rand_digits 8)"
MITRA_PHONE="62813$(rand_digits 8)"
NIK="$(rand_digits 16)"
NIB="$(rand_digits 13)"
NPWP="$(rand_digits 2).$(rand_digits 3).$(rand_digits 3).$(rand_digits 1)-$(rand_digits 3).$(rand_digits 3)"

http_call POST "$AUTH_URL/auth/register" "{\"full_name\":\"SOAP Test UMKM\",\"email\":\"$UMKM_EMAIL\",\"phone_number\":\"$UMKM_PHONE\",\"password\":\"$PASSWORD\",\"role\":\"UMKM\"}"
[ "$HTTP_CODE" = "201" ] || die "register UMKM gagal (HTTP $HTTP_CODE): $HTTP_BODY
       Pastikan auth-service hidup dan email/phone belum terdaftar."
UMKM_TOKEN="$(printf '%s' "$HTTP_BODY" | extract_token)"
[ -n "$UMKM_TOKEN" ] || die "response register UMKM tidak berisi access_token: $HTTP_BODY"

http_call POST "$AUTH_URL/auth/register" "{\"full_name\":\"SOAP Test Mitra\",\"email\":\"$MITRA_EMAIL\",\"phone_number\":\"$MITRA_PHONE\",\"password\":\"$PASSWORD\",\"role\":\"MITRA\"}"
[ "$HTTP_CODE" = "201" ] || die "register MITRA gagal (HTTP $HTTP_CODE): $HTTP_BODY"
MITRA_TOKEN="$(printf '%s' "$HTTP_BODY" | extract_token)"
[ -n "$MITRA_TOKEN" ] || die "response register MITRA tidak berisi access_token: $HTTP_BODY"

UMKM_ID="$(jwt_sub "$UMKM_TOKEN")"
MITRA_ID="$(jwt_sub "$MITRA_TOKEN")"

http_call PUT "$USER_URL/profiles/me" "{
  \"business_name\": \"Toko UMKM SOAP Test\",
  \"business_category\": \"FASHION\",
  \"jenis_umkm_id\": \"FASHION\",
  \"business_description\": \"Profil uji otomatis test case SOAP\",
  \"owner_name\": \"SOAP Test UMKM\",
  \"phone_number\": \"$UMKM_PHONE\",
  \"nik\": \"$NIK\",
  \"address\": \"Jl. SOAP Test No. 1\",
  \"city\": \"Sleman\",
  \"province\": \"DI Yogyakarta\",
  \"products\": \"Produk Uji SOAP\"
}" "$UMKM_TOKEN"
[ "$HTTP_CODE" = "200" ] || die "simpan profil UMKM gagal (HTTP $HTTP_CODE): $HTTP_BODY
       Pastikan user-service hidup."

http_call PUT "$USER_URL/profiles/me" "{
  \"organization_name\": \"Mitra SOAP Test Organization\",
  \"organization_type\": \"Inkubator Bisnis\",
  \"legal_name\": \"Mitra SOAP Test Organization\",
  \"nib\": \"$NIB\",
  \"npwp\": \"$NPWP\",
  \"description\": \"Profil mitra uji otomatis test case SOAP\",
  \"support_description\": \"Dukungan pendampingan untuk uji kemitraan SOAP\",
  \"address\": \"Jl. SOAP Test Mitra No. 2\",
  \"city\": \"Surakarta\",
  \"province\": \"Jawa Tengah\",
  \"contact_person\": \"PIC SOAP\",
  \"contact_person_title\": \"Manager\",
  \"phone_number\": \"$MITRA_PHONE\",
  \"operational_area\": \"Jawa Tengah\",
  \"cooperation_scale\": \"Provinsi\",
  \"partnership_field\": \"Pelatihan\",
  \"support_type\": \"Pendampingan\"
}" "$MITRA_TOKEN"
[ "$HTTP_CODE" = "200" ] || die "simpan profil MITRA gagal (HTTP $HTTP_CODE): $HTTP_BODY"

http_call GET "$USER_URL/profiles/me" "" "$UMKM_TOKEN"
[ "$HTTP_CODE" = "200" ] || die "GET profil UMKM gagal (HTTP $HTTP_CODE): $HTTP_BODY"
UMKM_BIZ="$(printf '%s' "$HTTP_BODY" | json_eval "d.get('profile',{}).get('id','')")"
[ -n "$UMKM_BIZ" ] || die "profil UMKM tidak memiliki id (business id): $HTTP_BODY"

http_call GET "$USER_URL/profiles/me" "" "$MITRA_TOKEN"
[ "$HTTP_CODE" = "200" ] || die "GET profil MITRA gagal (HTTP $HTTP_CODE): $HTTP_BODY"
MITRA_BIZ="$(printf '%s' "$HTTP_BODY" | json_eval "d.get('profile',{}).get('id','')")"
[ -n "$MITRA_BIZ" ] || die "profil MITRA tidak memiliki id (business id): $HTTP_BODY"

http_call POST "$AUTH_URL/auth/login" "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}"
[ "$HTTP_CODE" = "200" ] || die "login admin gagal (HTTP $HTTP_CODE): $HTTP_BODY
       Pastikan auth-seed-admin sudah berjalan: docker compose logs auth-seed-admin"
ADMIN_TOKEN="$(printf '%s' "$HTTP_BODY" | extract_token)"
[ -n "$ADMIN_TOKEN" ] || die "response login admin tidak berisi access_token: $HTTP_BODY"

echo "  akun UMKM  : $UMKM_EMAIL (akun_id=$UMKM_ID, umkm_id=$UMKM_BIZ)"
echo "  akun MITRA : $MITRA_EMAIL (akun_id=$MITRA_ID, mitra_id=$MITRA_BIZ)"
echo "  akun ADMIN : $ADMIN_EMAIL"

# SignPartnership butuh baris metadata dokumen di document.transaksi_dokumenterunggah
# (FK transaksi_pengajuankerjasama.dokumen_perjanjian_id). Prototype tidak punya
# endpoint runtime untuk ini — pola yang sama dipakai tests/stack/fixtures.sql.
DOC_READY=0
PG_CID="$(postgres_cid || true)"
if [ -n "$PG_CID" ]; then
  if printf '%s\n' \
    "INSERT INTO ref.ref_jenisdokumen (jenis_dokumen_id, nama_jenis_dokumen, allowed_extensions, max_size_mb)" \
    "VALUES ('TEST_CONTRACT', 'Kontrak sintetis pengujian', 'pdf', 1)" \
    "ON CONFLICT (jenis_dokumen_id) DO NOTHING;" \
    "INSERT INTO ref.ref_statusdokumen (status_dokumen_id, nama_status_dokumen)" \
    "VALUES ('TEST_UPLOADED', 'Dokumen sintetis pengujian')" \
    "ON CONFLICT (status_dokumen_id) DO NOTHING;" \
    "INSERT INTO document.transaksi_dokumenterunggah" \
    "  (dokumen_id, jenis_dokumen_id, status_dokumen_id, uploader_akun_id," \
    "   owner_type, owner_id, context_type, original_file_name, stored_file_name," \
    "   file_extension, mime_type, file_size_bytes, bucket_name, object_key, storage_path, checksum_sha256)" \
    "VALUES ('DOKTEST01', 'TEST_CONTRACT', 'TEST_UPLOADED', '$UMKM_ID'," \
    "   'UMKM', '$UMKM_BIZ', 'TEST_CONTRACT', 'contract.pdf', 'DOKTEST01.pdf'," \
    "   'pdf', 'application/pdf', 1, 'soap-test-metadata', 'DOKTEST01', 'DOKTEST01', repeat('0', 64))" \
    "ON CONFLICT (dokumen_id) DO UPDATE SET uploader_akun_id = EXCLUDED.uploader_akun_id, owner_id = EXCLUDED.owner_id;" |
    compose_psql >/dev/null 2>&1; then
    DOC_READY=1
    echo "  dokumen    : metadata DOKTEST01 siap (TC17/TC17b aktif)"
  else
    echo "  dokumen    : persiapan metadata DOKTEST01 gagal — TC17/TC17b akan di-SKIP"
  fi
else
  echo "  dokumen    : container postgres tidak terlihat docker — TC17/TC17b akan di-SKIP"
fi
echo

# ------------------------------------------------------------
# TC00 — WSDL dapat diakses tanpa autentikasi
# ------------------------------------------------------------
echo "== TC00 — WSDL =="
http_call GET "$SOAP_URL?wsdl"
SOAP_CODE="$HTTP_CODE"
SOAP_BODY="$HTTP_BODY"
expect_soap TC00 "WSDL dapat diakses tanpa auth" 200 \
  "<definitions" "CreatePartnershipApplication" "UpdatePartnershipStatus"
echo

# ------------------------------------------------------------
# Helpers inner XML
# ------------------------------------------------------------
create_inner() { # judul deskripsi receiver_id
  cat <<XML
    <CreatePartnershipApplication>
      <userId>$UMKM_ID</userId>
      <userRole>UMKM</userRole>
      <receiverId>$3</receiverId>
      <proposalTitle>$1</proposalTitle>
      <proposalDescription>$2</proposalDescription>
    </CreatePartnershipApplication>
XML
}

get_inner() { # aplikasi_id [user_id] [user_role]
  cat <<XML
    <GetPartnershipApplication>
      <userId>${2:-$UMKM_ID}</userId>
      <userRole>${3:-UMKM}</userRole>
      <applicationId>$1</applicationId>
    </GetPartnershipApplication>
XML
}

# ------------------------------------------------------------
# TC01 — Create berhasil
# ------------------------------------------------------------
echo "== TC01 — CreatePartnershipApplication (valid) =="
soap_call create_app1 CreatePartnershipApplication \
  "$(create_inner "Kemitraan Pengembangan Katalog Produk UMKM" \
    "Proposal pengajuan kemitraan untuk mengembangkan pemasaran produk UMKM bersama mitra sponsor regional." \
    "$MITRA_BIZ")" "$UMKM_TOKEN"
expect_soap TC01 "Create berhasil (DIAJUKAN)" 200 \
  "<success>true" "<status>DIAJUKAN" "<requestCode>PKS-"
APP1="$(xml_field applicationId)"
[ -n "$APP1" ] || APP1="PGJ000000"
echo "       applicationId=$APP1"
echo

# ------------------------------------------------------------
# TC02 — Create data tidak valid (a/b/c)
# ------------------------------------------------------------
echo "== TC02 — Create dengan data tidak valid =="
soap_call create_bad_title CreatePartnershipApplication \
  "$(create_inner "Judul" "Deskripsi panjang yang cukup panjang untuk lolos validasi minimal tiga puluh karakter." "$MITRA_BIZ")" \
  "$UMKM_TOKEN"
expect_soap TC02a "Create judul <10 karakter ditolak" 400 "proposalTitle minimal 10 karakter"

soap_call create_bad_desc CreatePartnershipApplication \
  "$(create_inner "Judul Yang Sudah Cukup Panjang" "Terlalu pendek." "$MITRA_BIZ")" "$UMKM_TOKEN"
expect_soap TC02b "Create deskripsi <30 karakter ditolak" 400 "proposalDescription minimal 30 karakter"

soap_call create_bad_receiver CreatePartnershipApplication \
  "$(create_inner "Kemitraan Pengembangan Katalog Produk UMKM" \
    "Proposal pengajuan kemitraan untuk mengembangkan pemasaran produk UMKM bersama mitra sponsor." "")" \
  "$UMKM_TOKEN"
expect_soap TC02c "Create receiverId kosong ditolak" 400 "receiverId wajib diisi"
echo

# ------------------------------------------------------------
# TC03 — Get by ID · TC04 — tidak ditemukan · TC05 — Get All
# ------------------------------------------------------------
echo "== TC03/TC04/TC05 — Get by ID, not-found, Get All =="
soap_call get_app1 GetPartnershipApplication "$(get_inner "$APP1")" "$UMKM_TOKEN"
expect_soap TC03 "Get by ID valid" 200 "<status>DIAJUKAN" "<requesterName>" "<receiverName>"

soap_call get_missing GetPartnershipApplication "$(get_inner "PGJ999999")" "$UMKM_TOKEN"
expect_soap TC04 "Get ID tidak ditemukan -> fault 404" 404 "tidak ditemukan"

soap_call list_outgoing GetPartnershipApplications '
    <GetPartnershipApplications>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
      <page>1</page>
      <limit>10</limit>
    </GetPartnershipApplications>' "$UMKM_TOKEN"
expect_soap TC05 "Get All (daftar keluar) berisi pengajuan" 200 \
  "<totalCount>" "$APP1" "GetPartnershipApplicationsResponse"
echo

# ------------------------------------------------------------
# TC12/TC13/TC14 — daftar masuk & ringkasan
# ------------------------------------------------------------
echo "== TC12/TC13/TC14 — Daftar masuk & ringkasan =="
soap_call list_incoming GetIncomingPartnerships '
    <GetIncomingPartnerships>
      <userId>'"$MITRA_ID"'</userId>
      <userRole>MITRA</userRole>
      <page>1</page>
      <limit>10</limit>
    </GetIncomingPartnerships>' "$MITRA_TOKEN"
expect_soap TC12 "GetIncomingPartnerships (kotak masuk penerima)" 200 \
  "<totalCount>" "$APP1" "GetIncomingPartnershipsResponse"

soap_call summary_out GetPartnershipSummary '
    <GetPartnershipSummary>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
    </GetPartnershipSummary>' "$UMKM_TOKEN"
expect_soap TC13 "GetPartnershipSummary (pengaju)" 200 \
  "GetPartnershipSummaryResponse" "<summary>"

soap_call summary_in GetIncomingPartnershipSummary '
    <GetIncomingPartnershipSummary>
      <userId>'"$MITRA_ID"'</userId>
      <userRole>MITRA</userRole>
    </GetIncomingPartnershipSummary>' "$MITRA_TOKEN"
expect_soap TC14 "GetIncomingPartnershipSummary (penerima)" 200 \
  "GetIncomingPartnershipSummaryResponse" "<summary>"
echo

# ------------------------------------------------------------
# TC15 — MarkAsRead (DIAJUKAN → DITINJAU) · TC06 — Approve (→ AKTIF)
# ------------------------------------------------------------
echo "== TC15/TC06 — Tandai dibaca lalu setujui APP1 =="
soap_call mark_read_app1 MarkPartnershipAsRead '
    <MarkPartnershipAsRead>
      <userId>'"$MITRA_ID"'</userId>
      <userRole>MITRA</userRole>
      <applicationId>'"$APP1"'</applicationId>
    </MarkPartnershipAsRead>' "$MITRA_TOKEN"
expect_soap TC15 "MarkPartnershipAsRead penerima" 200 "<success>true"

soap_call get_app1_after_read GetPartnershipApplication "$(get_inner "$APP1")" "$UMKM_TOKEN"
expect_soap "TC15b" "Status berubah menjadi DITINJAU" 200 "<status>DITINJAU"

soap_call approve_app1 UpdatePartnershipStatus '
    <UpdatePartnershipStatus>
      <userId>'"$MITRA_ID"'</userId>
      <userRole>MITRA</userRole>
      <applicationId>'"$APP1"'</applicationId>
      <status>AKTIF</status>
    </UpdatePartnershipStatus>' "$MITRA_TOKEN"
expect_soap TC06 "Update status -> AKTIF (approve)" 200 \
  "<success>true" "<status>AKTIF"

soap_call get_app1_approved GetPartnershipApplication "$(get_inner "$APP1")" "$UMKM_TOKEN"
expect_soap "TC06b" "Status terkonfirmasi AKTIF di DB-read" 200 "<status>AKTIF"
echo

# ------------------------------------------------------------
# APP2 — TC07 reject · TC16 MarkAsRead salah actor
# ------------------------------------------------------------
echo "== TC07/TC16 — Reject APP2 & salah actor =="
soap_call create_app2 CreatePartnershipApplication \
  "$(create_inner "Pengajuan Kemitraan Kedua Untuk Uji Reject" \
    "Pengajuan kedua khusus untuk menguji penolakan dengan alasan oleh penerima kemitraan." "$MITRA_BIZ")" \
  "$UMKM_TOKEN"
expect_soap "TC07a" "Create APP2 untuk tes reject" 200 "<success>true" "<status>DIAJUKAN"
APP2="$(xml_field applicationId)"

soap_call reject_app2 UpdatePartnershipStatus '
    <UpdatePartnershipStatus>
      <userId>'"$MITRA_ID"'</userId>
      <userRole>MITRA</userRole>
      <applicationId>'"$APP2"'</applicationId>
      <status>DITOLAK</status>
      <rejectionReason>Kuota kemitraan periode ini sudah terpenuhi.</rejectionReason>
    </UpdatePartnershipStatus>' "$MITRA_TOKEN"
expect_soap TC07 "Update status -> DITOLAK (reject + alasan)" 200 \
  "<success>true" "<status>DITOLAK"

soap_call get_app2_rejected GetPartnershipApplication "$(get_inner "$APP2")" "$UMKM_TOKEN"
expect_soap "TC07b" "Status DITOLAK + rejectionReason terbaca" 200 \
  "<status>DITOLAK" "Kuota kemitraan periode ini sudah terpenuhi"

soap_call mark_read_wrong_actor MarkPartnershipAsRead '
    <MarkPartnershipAsRead>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
      <applicationId>'"$APP2"'</applicationId>
    </MarkPartnershipAsRead>' "$UMKM_TOKEN"
expect_soap TC16 "MarkAsRead oleh pengaju (salah actor) -> 409" 409 "soap:Client"
echo

# ------------------------------------------------------------
# APP3 — TC11 reject tanpa alasan · TC09 cancel · TC08 status invalid
# ------------------------------------------------------------
echo "== TC11/TC09/TC08 — APP3 tanpa alasan, batal, status invalid =="
soap_call create_app3 CreatePartnershipApplication \
  "$(create_inner "Pengajuan Kemitraan Ketiga Untuk Uji Batal" \
    "Pengajuan ketiga khusus untuk menguji penolakan tanpa alasan lalu pembatalan oleh pengaju." "$MITRA_BIZ")" \
  "$UMKM_TOKEN"
expect_soap "TC11a" "Create APP3" 200 "<success>true" "<status>DIAJUKAN"
APP3="$(xml_field applicationId)"

soap_call reject_no_reason UpdatePartnershipStatus '
    <UpdatePartnershipStatus>
      <userId>'"$MITRA_ID"'</userId>
      <userRole>MITRA</userRole>
      <applicationId>'"$APP3"'</applicationId>
      <status>DITOLAK</status>
    </UpdatePartnershipStatus>' "$MITRA_TOKEN"
expect_soap TC11 "Reject tanpa rejectionReason -> 422" 422 "rejectionReason wajib diisi"

soap_call cancel_app3 UpdatePartnershipStatus '
    <UpdatePartnershipStatus>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
      <applicationId>'"$APP3"'</applicationId>
      <status>DIBATALKAN</status>
    </UpdatePartnershipStatus>' "$UMKM_TOKEN"
expect_soap TC09 "Update status -> DIBATALKAN (cancel pengaju)" 200 \
  "<success>true" "<status>DIBATALKAN"

soap_call get_app3_cancelled GetPartnershipApplication "$(get_inner "$APP3")" "$UMKM_TOKEN"
expect_soap "TC09b" "Status terkonfirmasi DIBATALKAN" 200 "<status>DIBATALKAN"

soap_call update_invalid_status UpdatePartnershipStatus '
    <UpdatePartnershipStatus>
      <userId>'"$MITRA_ID"'</userId>
      <userRole>MITRA</userRole>
      <applicationId>'"$APP3"'</applicationId>
      <status>APPROVED</status>
    </UpdatePartnershipStatus>' "$MITRA_TOKEN"
expect_soap TC08 "Update status invalid (APPROVED) -> 400" 400 "status harus salah satu dari"
echo

# ------------------------------------------------------------
# APP4 — TC10 salah actor · TC17/TC18/TC19 sign
# ------------------------------------------------------------
echo "== TC10/TC17/TC18/TC19 — APP4 salah actor & sign kontrak =="
soap_call create_app4 CreatePartnershipApplication \
  "$(create_inner "Pengajuan Kemitraan Keempat Untuk Uji Tanda Tangan" \
    "Pengajuan keempat khusus untuk menguji salah actor status dan unggah dokumen kontrak." "$MITRA_BIZ")" \
  "$UMKM_TOKEN"
expect_soap "TC10a" "Create APP4" 200 "<success>true" "<status>DIAJUKAN"
APP4="$(xml_field applicationId)"

soap_call approve_wrong_actor UpdatePartnershipStatus '
    <UpdatePartnershipStatus>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
      <applicationId>'"$APP4"'</applicationId>
      <status>AKTIF</status>
    </UpdatePartnershipStatus>' "$UMKM_TOKEN"
expect_soap TC10 "Approve oleh pengaju (salah actor) -> 409" 409 "soap:Client"

if [ "$DOC_READY" = "1" ]; then
  soap_call sign_app4 SignPartnership '
    <SignPartnership>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
      <applicationId>'"$APP4"'</applicationId>
      <documentId>DOKTEST01</documentId>
    </SignPartnership>' "$UMKM_TOKEN"
  expect_soap TC17 "SignPartnership oleh pengaju" 200 "<success>true"

  soap_call get_app4_signed GetPartnershipApplication "$(get_inner "$APP4")" "$UMKM_TOKEN"
  expect_soap "TC17b" "contractDocumentId = DOKTEST01 terbaca" 200 "<contractDocumentId>DOKTEST01"
else
  record TC17 "SKIP" "SignPartnership oleh pengaju" "metadata dokumen tidak tersedia (docker/psql tidak terdeteksi)"
  record TC17b "SKIP" "contractDocumentId = DOKTEST01 terbaca" "metadata dokumen tidak tersedia"
fi

soap_call sign_wrong_actor SignPartnership '
    <SignPartnership>
      <userId>'"$MITRA_ID"'</userId>
      <userRole>MITRA</userRole>
      <applicationId>'"$APP4"'</applicationId>
      <documentId>DOKTEST02</documentId>
    </SignPartnership>' "$MITRA_TOKEN"
expect_soap TC18 "Sign oleh penerima (salah actor) -> 409" 409 "soap:Client"

soap_call sign_no_doc SignPartnership '
    <SignPartnership>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
      <applicationId>'"$APP4"'</applicationId>
      <documentId></documentId>
    </SignPartnership>' "$UMKM_TOKEN"
expect_soap TC19 "Sign tanpa documentId -> 400" 400 "documentId wajib diisi"
echo

# ------------------------------------------------------------
# TC20/TC21 — filter daftar
# ------------------------------------------------------------
echo "== TC20/TC21 — Filter status daftar =="
soap_call list_filter_valid GetPartnershipApplications '
    <GetPartnershipApplications>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
      <status>DIAJUKAN</status>
      <page>1</page>
      <limit>10</limit>
    </GetPartnershipApplications>' "$UMKM_TOKEN"
expect_soap TC20 "Filter status valid (DIAJUKAN)" 200 \
  "<totalCount>" "$APP4" "GetPartnershipApplicationsResponse"

soap_call list_filter_invalid GetPartnershipApplications '
    <GetPartnershipApplications>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
      <status>APPROVED</status>
    </GetPartnershipApplications>' "$UMKM_TOKEN"
expect_soap TC21 "Filter status invalid (APPROVED) -> 400" 400 "status tidak valid"
echo

# ------------------------------------------------------------
# TC22–TC27 — autentikasi & identitas
# ------------------------------------------------------------
echo "== TC22–TC27 — Autentikasi =="
summary_inner='
    <GetPartnershipSummary>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
    </GetPartnershipSummary>'

soap_call auth_no_token GetPartnershipSummary "$summary_inner" "" none
expect_soap TC22 "Tanpa token -> 401" 401 "Token autentikasi tidak valid"

soap_call auth_admin GetPartnershipSummary "$summary_inner" "$ADMIN_TOKEN"
expect_soap TC23 "Token ADMIN ditolak -> 403" 403 "Role tidak diizinkan"

soap_call auth_bad_token GetPartnershipSummary "$summary_inner" "abc.def.ghi"
expect_soap TC24 "Token rusak -> 401" 401 "Token autentikasi tidak valid"

soap_call auth_in_soap_header GetPartnershipSummary "$summary_inner" "$UMKM_TOKEN" soap-header
expect_soap TC25 "Token hanya di <soapenv:Header> (tanpa HTTP Authorization) -> 401" 401 \
  "Token autentikasi tidak valid"

soap_call auth_identity_mismatch GetPartnershipSummary '
    <GetPartnershipSummary>
      <userId>'"$MITRA_ID"'</userId>
      <userRole>MITRA</userRole>
    </GetPartnershipSummary>' "$UMKM_TOKEN"
expect_soap TC26 "userId/userRole tidak cocok token -> 403" 403 \
  "Identitas request tidak sesuai token"

soap_call auth_missing_identity GetPartnershipSummary '
    <GetPartnershipSummary>
      <userId></userId>
      <userRole></userRole>
    </GetPartnershipSummary>' "$UMKM_TOKEN"
expect_soap TC27 "userId/userRole kosong (melanggar kontrak WSDL) -> 400" 400 \
  "wajib diisi sesuai kontrak WSDL"
echo

# ------------------------------------------------------------
# TC28/TC29 — body rusak & validasi input
# ------------------------------------------------------------
echo "== TC28/TC29 — Body rusak & validasi input =="
soap_call malformed_envelope CreatePartnershipApplication "<bukan-envelope/>" "$UMKM_TOKEN" raw
expect_soap TC28 "Body bukan SOAP Envelope -> 400" 400 "SOAP Envelope tidak valid"

soap_call get_no_id GetPartnershipApplication '
    <GetPartnershipApplication>
      <userId>'"$UMKM_ID"'</userId>
      <userRole>UMKM</userRole>
      <applicationId></applicationId>
    </GetPartnershipApplication>' "$UMKM_TOKEN"
expect_soap TC29 "Get tanpa applicationId -> 400" 400 "applicationId wajib diisi"
echo

# ------------------------------------------------------------
# V01 — verifikasi database (opsional jika docker compose tersedia)
# ------------------------------------------------------------
echo "== V01 — Verifikasi database =="
if [ -n "$PG_CID" ]; then
  DB_ROWS="$(printf "%s\n" \
    "SELECT pengajuan_id || '=' || status_pengajuan_id || COALESCE('|' || dokumen_perjanjian_id, '')" \
    "FROM partnership.transaksi_pengajuankerjasama" \
    "WHERE pengajuan_id IN ('$APP1','$APP2','$APP3','$APP4')" \
    "ORDER BY pengajuan_id;" |
    compose_psql 2>/dev/null || true)"
  db_ok=1
  db_detail=""
  for want in "$APP1=AKTIF" "$APP2=DITOLAK" "$APP3=DIBATALKAN" "$APP4=DIAJUKAN|DOKTEST01"; do
    if ! printf '%s' "$DB_ROWS" | grep -qF -- "$want"; then
      db_ok=0
      db_detail="$db_detail | hilang: $want"
    fi
  done
  if [ "$db_ok" = "1" ]; then
    record V01 "PASS" "Status di database sesuai (AKTIF/DITOLAK/DIBATALKAN + dokumen)"
  else
    record V01 "FAIL" "Status di database sesuai" "baris: $(printf '%s' "$DB_ROWS" | tr '\n' ' ')$db_detail"
  fi
else
  record V01 "SKIP" "docker compose postgres tidak terdeteksi (test dijalankan di luar stack repo)"
fi
echo

# ------------------------------------------------------------
# Ringkasan
# ------------------------------------------------------------
echo "============================================================"
echo " Ringkasan hasil test SOAP"
echo "============================================================"
if [ -f "$REPORT_DIR/results.tsv" ]; then
  while IFS=$'\t' read -r id status desc detail; do
    printf '  %-7s %-5s %s\n' "$id" "$status" "$desc"
  done <"$REPORT_DIR/results.tsv"
fi
echo "------------------------------------------------------------"
echo "  TOTAL: $PASS PASS, $FAIL FAIL  (detail: tests/soap/reports/results.tsv)"
echo "  Log request/response XML: tests/soap/reports/"
echo "============================================================"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
echo "Semua test SOAP lulus."
