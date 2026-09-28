# Demo SOAP Partnership Service

> Anggota 5: Testing, Dokumentasi & Demo.
> Panduan: [README.md](README.md) · Test case: [test-case.md](test-case.md) · Hasil: [test-result.md](test-result.md)

Skenario demo live untuk memperkenalkan fitur **SOAP Kemitraan** dan kualitas
pengujiannya. Total durasi ± 10–15 menit. Siapkan satu terminal di root repo.

## Persiapan (sekali, sebelum demo)

```bash
# 1. konfigurasi env (bila belum ada)
[ -f .env ] || cp .env.example .env

# 2. hidupkan seluruh stack (build bisa memakan waktu saat pertama)
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait --wait-timeout 180

# 3. pastikan semua sehat
docker compose --env-file .env -f infra/docker-compose.yml ps
```

Semua container berstatus `healthy` (kecuali one-shot `db-migrate`/
`auth-seed-admin`/`garage-bootstrap` yang memang `Exited (0)`).

---

## Skenario 1 — Test otomatis penuh (± 3 menit)

**Poin yang ditekankan:** 41 pemeriksaan otomatis, urutan state machine,
pembersihan artefak, exit code.

```bash
bash tests/soap/run-soap-tests.sh
echo "exit=$?"        # harus 0
```

Tunjukkan kepada penonton:

1. **Preflight** — empat dependency diperiksa dengan pesan jelas bila mati.
2. **Provisioning** — akun UMKM/MITRA/ADMIN dibuat otomatis + metadata dokumen `DOKTEST01`.
3. **Alur test** — mulai `TC00` (WSDL) → create/validasi → get/list/summary →
   transisi `DIAJUKAN → DITINJAU → AKTIF` (approve), `→ DITOLAK` (reject + alasan),
   `→ DIBATALKAN` (cancel) → sign kontrak → filter → autentikasi → body rusak.
4. **V01** — verifikasi status final langsung di PostgreSQL via `psql`.
5. **Ringkasan akhir** — `TOTAL: 41 PASS, 0 FAIL`.

Kaitkan singkat dengan requirement: token hanya via HTTP header (TC22–TC25),
fault code 400/401/403/404/409/422, pesan fault bahasa Indonesia.

```bash
# bukti artefak request/response per langkah
ls tests/soap/reports/
less tests/soap/reports/results.tsv
```

---

## Skenario 2 — Inspeksi manual via WSDL & curl (± 5 menit)

**Poin yang ditekankan:** WSDL publik, format Envelope, autentikasi header,
bentuk SOAP Fault.

### 2a. WSDL di browser

Buka <http://localhost:9090/partnership?wsdl> — tunjukkan 9 operasi
(`CreatePartnershipApplication`, `UpdatePartnershipStatus`,
`SignPartnership`, dst.) tanpa perlu login.

### 2b. Siapkan token & identitas (satu kali)

```bash
BASE=http://localhost:8080/api/v1
RAND=$RANDOM$RANDOM
curl -sS -X POST "$BASE/auth/register" -H "Content-Type: application/json" -d '{
  "full_name":"Demo UMKM","email":"demo.umkm.'"$RAND"'@soap.test",
  "phone_number":"62812'"$RAND"'","password":"password123","role":"UMKM"}'

curl -sS -X POST "$BASE/auth/register" -H "Content-Type: application/json" -d '{
  "full_name":"Demo Mitra","email":"demo.mitra.'"$RAND"'@soap.test",
  "phone_number":"62813'"$RAND"'","password":"password123","role":"MITRA"}'

# login masing-masing → simpan access_token
TOKEN_UMKM=$(curl -sS -X POST "$BASE/auth/login" -H "Content-Type: application/json" \
  -d '{"email":"demo.umkm.'"$RAND"'@soap.test","password":"password123"}' \
  | python -c "import sys,json;print(json.load(sys.stdin)['access_token'])")

# buka https://jwt.io, tempel token → catat "sub" = akun_id (userId)
# lengkapi profil dulu agar punya umkm_id/mitra_id (receiverId)
curl -sS -X PUT http://localhost:8081/api/v1/profiles/me \
  -H "Authorization: Bearer $TOKEN_UMKM" -H "Content-Type: application/json" -d '{
  "business_name":"Demo Toko","business_category":"FASHION",
  "business_description":"Akun demo SOAP","owner_name":"Demo UMKM",
  "phone_number":"62812'"$RAND"'","nik":"3273015509900001",
  "address":"Jl. Demo 1","city":"Sleman","province":"DI Yogyakarta"}'
```

Ulangi token/profil untuk MITRA (field `organization_*`, `nib`, `npwp`, dst. —
lihat payload di `run-soap-tests.sh` bagian *Persiapan akun uji*).
`GET /profiles/me` mengembalikan `profile.id` (inilah `umkm_id`/`mitra_id`).

### 2c. Kirim SOAP request (curl)

```bash
SOAP=http://localhost:9090/partnership
cat > /tmp/create.xml <<'XML'
<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Header/>
  <soapenv:Body>
    <CreatePartnershipApplication>
      <userId>GANTI_DENGAN_SUB_DARI_JWT</userId>
      <userRole>UMKM</userRole>
      <receiverId>GANTI_DENGAN_MITRA_ID</receiverId>
      <proposalTitle>Kemitraan Demo Presentasi</proposalTitle>
      <proposalDescription>Proposal demo kemitraan SOAP untuk presentasi pengujian aplikasi besar.</proposalDescription>
    </CreatePartnershipApplication>
  </soapenv:Body>
</soapenv:Envelope>
XML

curl -sS -X POST "$SOAP" \
  -H "Content-Type: text/xml; charset=utf-8" \
  -H 'SOAPAction: "http://umkm-tumbuh.example.com/partnerships/CreatePartnershipApplication"' \
  -H "Authorization: Bearer $TOKEN_UMKM" \
  --data-binary @/tmp/create.xml
```

Respons sukses: `<success>true</success>`, `<applicationId>PGJ######</applicationId>`,
`<status>DIAJUKAN</status>`, `<requestCode>PKS-…</requestCode>`.

### 2d. Demo SOAP Fault (pilih 2–3)

| Skenario | Ubahan | HTTP | Isi `faultstring` |
|---|---|---|---|
| Tanpa token | hapus header `Authorization` | 401 | `Token autentikasi tidak valid` |
| Token rusak | `Authorization: Bearer abc.def.ghi` | 401 | `Token autentikasi tidak valid` |
| Token hanya di `<soapenv:Header>` | pindahkan token ke Header XML, hapus HTTP header | 401 | `Token autentikasi tidak valid` |
| Token ADMIN (role salah) | token admin di header | 403 | `Role tidak diizinkan untuk operasi kemitraan` |
| Get ID tidak ada | `applicationId` = `PGJ999999` | 404 | `...tidak ditemukan...` |
| Reject tanpa alasan | `status=DITOLAK` tanpa `rejectionReason` | 422 | `rejectionReason wajib diisi saat menolak pengajuan` |
| Body bukan Envelope | kirim `<bukan-envelope/>` | 400 | `SOAP Envelope tidak valid` |

Contoh (tanpa token):

```bash
curl -sS -o /tmp/fault.xml -w "HTTP %{http_code}\n" -X POST "$SOAP" \
  -H "Content-Type: text/xml; charset=utf-8" \
  -H 'SOAPAction: "http://umkm-tumbuh.example.com/partnerships/GetPartnershipSummary"' \
  --data-binary @/tmp/summary.xml
cat /tmp/fault.xml
```

---

## Skenario 3 — Jalur error preflight (± 2 menit)

**Poin yang ditekankan:** Requirement B — pesan error jelas & kontekstual.

```bash
docker stop infra-soap-partnerships-service-1
PRECHECK_ATTEMPTS=3 PRECHECK_DELAY=1 bash tests/soap/run-soap-tests.sh
echo "exit=$?"    # 2
```

Keluaran yang muncul:

```text
  [PRECHECK] SOAP service  http://localhost:9090/partnership?wsdl ... menunggu .............
ERROR: SOAP service is unreachable at http://localhost:9090/partnership?wsdl
       Jalankan stack Docker terlebih dahulu:
         docker compose --env-file .env -f infra/docker-compose.yml up -d --build
```

Pulihkan lalu buktikan laporan run sebelumnya masih ada:

```bash
docker start infra-soap-partnerships-service-1
ls tests/soap/reports/results.tsv   # masih utuh — pembersihan hanya setelah preflight lulus
```

Skenario serupa untuk semua service mati: `docker compose --env-file .env -f
infra/docker-compose.yml stop` → pesan pertama yang muncul
`ERROR: Auth service is unreachable at http://localhost:8080/api/v1/health`.

---

## Skenario 4 (opsional) — Verifikasi database manual (± 2 menit)

```bash
docker compose --env-file .env -f infra/docker-compose.yml exec -T postgres \
  sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<SQL
SELECT pengajuan_id, status_pengajuan_id, dokumen_perjanjian_id
FROM partnership.transaksi_pengajuankerjasama
ORDER BY created_at DESC LIMIT 4;
SQL
```

Harus terlihat hasil V01: `AKTIF`, `DITOLAK` (+ alasan), `DIBATALKAN`, dan
`DIAJUKAN` dengan `DOKTEST01` (dari run test terakhir).

---

## Checklist penutup demo

- [ ] `bash tests/soap/run-soap-tests.sh` → `TOTAL: 41 PASS, 0 FAIL`, exit 0
- [ ] WSDL terbuka di browser tanpa token
- [ ] Minimal 1 request sukses + 2 SOAP Fault didemokan
- [ ] Preflight error menampilkan pesan `ERROR: ... is unreachable` + exit 2
- [ ] Artefak `reports/` dan `test-result.md` ditunjukkan sebagai bukti
- [ ] Stack dikembalikan sehat (`docker compose ps`) sebelum sesi berikutnya

Bila perlu bukti visual, isi screenshot sesuai [screenshots.md](screenshots.md).
