# Hasil Pengujian — SOAP Partnership Service

> Anggota 5: Testing, Dokumentasi & Demo.
> Test case: [test-case.md](test-case.md) · Panduan: [README.md](README.md) · Demo: [demo.md](demo.md)

## 1. Metadata eksekusi

| Item | Nilai |
|---|---|
| Tanggal | 28 September 2026 |
| Cabang / commit | `soap-wsdl` / `f5aaa9c` (*Implement SOAP WSDL service*) |
| Perintah | `bash tests/soap/run-soap-tests.sh` (dari root repo) |
| Stack | Docker Compose dev (`infra/docker-compose.yml`), 8 container healthy |
| Sistem | Windows + WSL (bash 5.2), Docker Desktop |
| Hasil akhir | **41 PASS · 0 FAIL · 0 SKIP** (exit code 0) |

## 2. Ringkasan hasil

Kategori dihitung dari `tests/soap/reports/results.tsv` hasil run terakhir:

| Kategori | Test case | Status |
|---|---|---|
| WSDL | TC00 | PASS |
| Create (valid & invalid) | TC01, TC02a, TC02b, TC02c | PASS |
| Get by ID / not-found / daftar | TC03, TC04, TC05 | PASS |
| Daftar masuk & ringkasan | TC12, TC13, TC14 | PASS |
| Transisi status (state machine) | TC15, TC15b, TC06, TC06b, TC07, TC07a, TC07b, TC09, TC09b, TC11, TC11a, TC16, TC10, TC10a | PASS |
| Tanda tangan kontrak | TC17, TC17b, TC18, TC19 | PASS |
| Filter daftar | TC20, TC21 | PASS |
| Autentikasi & identitas (Requirement A) | TC22, TC23, TC24, TC25, TC26, TC27 | PASS |
| Body rusak & validasi input | TC28, TC29 | PASS |
| Verifikasi database | V01 | PASS |
| **Total otomatis** | **41 pemeriksaan** | **41 PASS** |

### Rincian per test case

| Kode | Hasil | Kode | Hasil |
|---|---|---|---|
| TC00 | PASS | TC11a | PASS |
| TC01 | PASS | TC11 | PASS |
| TC02a | PASS | TC09 | PASS |
| TC02b | PASS | TC09b | PASS |
| TC02c | PASS | TC08 | PASS |
| TC03 | PASS | TC10a | PASS |
| TC04 | PASS | TC10 | PASS |
| TC05 | PASS | TC17 | PASS |
| TC12 | PASS | TC17b | PASS |
| TC13 | PASS | TC18 | PASS |
| TC14 | PASS | TC19 | PASS |
| TC15 | PASS | TC20 | PASS |
| TC15b | PASS | TC21 | PASS |
| TC06 | PASS | TC22 | PASS |
| TC06b | PASS | TC23 | PASS |
| TC07a | PASS | TC24 | PASS |
| TC07 | PASS | TC25 | PASS |
| TC07b | PASS | TC26 | PASS |
| TC16 | PASS | TC27 | PASS |
| TC28 | PASS | TC29 | PASS |
| V01 | PASS | | |

## 3. Jalur kegagalan preflight (TC-P1 & TC-P2)

Kedua skenario dijalankan manual dengan `PRECHECK_ATTEMPTS=3 PRECHECK_DELAY=1` agar cepat.

**TC-P1 — semua service mati** (`docker compose ... stop`):

```text
  [PRECHECK] Auth service   http://localhost:8080/api/v1/health ... menunggu .............
ERROR: Auth service is unreachable at http://localhost:8080/api/v1/health
       Jalankan stack Docker terlebih dahulu:
         docker compose --env-file .env -f infra/docker-compose.yml up -d --build
```

Keluaran: **exit code 2**, tanpa traceback.

**TC-P2 — hanya SOAP mati** (`docker stop infra-soap-partnerships-service-1`):

```text
  [PRECHECK] SOAP service   http://localhost:9090/partnership?wsdl ... menunggu .............
ERROR: SOAP service is unreachable at http://localhost:9090/partnership?wsdl
       Jalankan stack Docker terlebih dahulu:
         docker compose --env-file .env -f infra/docker-compose.yml up -d --build
```

Keluaran: **exit code 2**, pesan menyebut dependency yang tepat sesuai Requirement B.
Setelah keduanya, artefak run sebelumnya tetap utuh (pembersihan `reports/` hanya
dilakukan setelah preflight lulus). Run normal setelah stack dihidupkan kembali:
**41 PASS, 0 FAIL**.

## 4. Temuan (findings)

1. **Prasyarat FK dokumen kontrak.** `SignPartnership` menulis
   `dokumen_perjanjian_id` yang adalah FK ke `document.transaksi_dokumenterunggah`
   (migration `007_partnership_tables.up.sql:16`). Tabel ini hanya diisi oleh seed
   CSV/fixtures — tidak ada endpoint runtime yang membuat barisnya (upload
   document-service menulis `documents.master_dokumen`). Skrip test menyiapkan baris
   metadata pola `tests/stack/fixtures.sql` saat provisioning; bila docker/psql tidak
   terdeteksi, TC17/TC17b berstatus SKIP, bukan FAIL. Keterbatasan prototipe yang sama
   juga berlaku pada alur REST `POST /partnerships/{id}/sign`.
2. **SOAP tidak memvalidasi kepemilikan dokumen** (beda dengan REST yang memeriksa
   `OwnsDocument`). Ini disengaja dan didokumentasikan di kode
   (`service.go` SignPartnership: *"Prototype note: document ownership is NOT
   validated…"*) — dicatat sebagai catatan risiko, bukan defect test ini.
3. **Token SOAP HANYA via header HTTP.** `<soapenv:Header>` diparse tapi diabaikan
   untuk autentikasi — terbukti TC22/TC25 (401) dan TC23/TC26 (403) lulus sesuai
   Requirement A.
4. **Lingkungan WSL:** `docker` milik distro dapat menunjuk daemon kosong; skrip
   otomatis mendeteksi `docker.exe` / context `desktop-linux` yang benar-benar
   melihat stack Docker Desktop.
5. **Stage 1 (`tests/stack/run.sh`) sempat merah sejak commit `3662133`** —
   route products dihapus dari user-service tetapi `tests/stack/check.py` masih
   mengecek `POST /api/v1/products/` (404). Cek products yang sudah tidak ada
   fiturnya dihapus dari `check.py` (final: `Stage 1 stack checks passed.`
   dan `Stage 2 authorization checks passed.`, exit 0 — termasuk
   `soap-partnerships-service` di test stack).

## 5. Cara mereproduksi & artefak

```bash
# 1. hidupkan stack
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait
# 2. jalankan seluruh test
bash tests/soap/run-soap-tests.sh
echo $?   # 0 = semua lulus
```

| Artefak | Isi | Ikut repo? |
|---|---|---|
| `tests/soap/reports/results.tsv` | Tabel hasil per kasus (id, status, deskripsi, detail) | Tidak (gitignored) |
| `tests/soap/reports/*_req.xml` | SOAP request mentah per langkah | Tidak (gitignored) |
| `tests/soap/reports/*_res.xml` | SOAP response mentah per langkah | Tidak (gitignored) |
| `tests/soap/test-result.md` | Ringkasan hasil run (dokumen ini) | Ya |

Screenshot bukti: [screenshots.md](screenshots.md).
