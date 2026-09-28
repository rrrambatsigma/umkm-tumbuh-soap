# Test SOAP Partnership Service

> Anggota 5: Testing, Dokumentasi & Demo.
> Test case: [test-case.md](test-case.md) · Hasil: [test-result.md](test-result.md) ·
> Demo: [demo.md](demo.md) · Screenshot: [screenshots.md](screenshots.md)

Dokumen ini menjelaskan cara menjalankan test otomatis fitur **SOAP Kemitraan**
(`services/soap-partnerships-service`) terhadap stack Docker lokal.

## Prasyarat

- Docker Desktop/Engine + Compose 2.24.4, dengan integrasi WSL aktif bila memakai WSL.
- `bash` (WSL/Git Bash) dan `python3`/`python`/`py` (untuk parsing JSON & JWT).
- Stack dev sudah dikonfigurasi: `[ -f .env ] || cp .env.example .env`.
- Port host default: auth `8080`, user `8081`, partnerships `8082`, SOAP `9090`.

## Menjalankan

```bash
# dari root repo — hidupkan stack bila belum
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait --wait-timeout 180

# jalankan seluruh test (TC00–TC29 + V01)
bash tests/soap/run-soap-tests.sh
```

Keluaran contoh (akhir run):

```text
============================================================
 Ringkasan hasil test SOAP
============================================================
  TC00    PASS  WSDL dapat diakses tanpa auth
  ...
  V01     PASS  Status di database sesuai (AKTIF/DITOLAK/DIBATALKAN + dokumen)
------------------------------------------------------------
  TOTAL: 41 PASS, 0 FAIL  (detail: tests/soap/reports/results.tsv)
============================================================
```

### Exit code

| Kode | Arti |
|---|---|
| `0` | Semua test lulus |
| `1` | Ada test gagal (detail di `reports/results.tsv`) |
| `2` | Preflight gagal — dependency service tidak terjangkau (pesan `ERROR: ... is unreachable at ...` + hint menjalankan stack) |

### Opsi environment

| Variabel | Default | Fungsi |
|---|---|---|
| `AUTH_URL` | `http://localhost:8080/api/v1` | Base URL auth-service |
| `USER_URL` | `http://localhost:8081/api/v1` | Base URL user-service |
| `PARTNERSHIP_URL` | `http://localhost:8082` | Base URL partnerships REST (health saja) |
| `SOAP_URL` | `http://localhost:9090/partnership` | Endpoint SOAP |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin@example.com` / `admin12345` | Akun admin (TC23) |
| `PRECHECK_ATTEMPTS` / `PRECHECK_DELAY` | `15` / `2` | Retry preflight (detik ≈ attempts × delay) |

Contoh: `PRECHECK_ATTEMPTS=3 PRECHECK_DELAY=1 bash tests/soap/run-soap-tests.sh`
untuk preflight cepat.

## Alur yang dilakukan skrip

1. **Preflight** — cek `/health` auth, user, partnerships, dan `GET ?wsdl` SOAP
   dengan retry; gagal → exit 2 dengan pesan spesifik (lihat Requirement B).
2. **Provisioning** — register akun UMKM & MITRA (unik per run), simpan profil
   (`PUT /profiles/me`), ambil `umkm_id`/`mitra_id` dari `GET /profiles/me`,
   login admin, siapkan baris metadata dokumen `DOKTEST01` (prasyarat FK sign,
   pola `tests/stack/fixtures.sql`) bila docker/psql terdeteksi.
3. **Eksekusi test** — 9 operasi SOAP dalam urutan state-machine
   (`DIAJUKAN → DITINJAU → AKTIF/DITOLAK/DIBATALKAN`), request invalid,
   autentikasi/identitas, body rusak. Setiap request/response disimpan sebagai
   XML di `reports/`.
4. **Verifikasi DB (V01)** — `psql` memastikan APP1=`AKTIF`, APP2=`DITOLAK`,
   APP3=`DIBATALKAN`, APP4=`DIAJUKAN` + `DOKTEST01` (SKIP bila tidak ada docker).

## Struktur

```text
tests/soap/
├── test-case.md        # katalog test case (39 skenario)
├── run-soap-tests.sh   # skrip test otomatis
├── test-result.md      # hasil eksekusi & temuan
├── demo.md             # skenario demo live
├── screenshots.md      # checklist screenshot bukti
├── README.md           # dokumen ini
├── reports/            # hasil runtime (results.tsv + XML) — gitignored
└── screenshots/        # bukti foto (diisi manual)
```

## Troubleshooting

| Gejala | Penyebab & solusi |
|---|---|
| `ERROR: Auth service is unreachable at ...` | Stack belum jalan → `docker compose --env-file .env -f infra/docker-compose.yml up -d --build` |
| `ERROR: SOAP service is unreachable at http://localhost:9090/partnership?wsdl` | Container SOAP mati → `docker compose ... up -d` atau `docker compose ... logs soap-partnerships-service` |
| `TC17/TC17b SKIP` + `container postgres tidak terlihat docker` | docker CLI tidak melihat stack (umum di WSL). Skrip sudah mencoba `docker` → `docker.exe` → context `desktop-linux`; pastikan Docker Desktop berjalan & integrasi WSL aktif |
| Preflight lama | Stack sedang booting; tunggu, atau turunkan `PRECHECK_ATTEMPTS` |
| `python ... tidak ditemukan` | Instal Python atau pastikan `python3`/`py` ada di PATH |
| Port bentrok saat start stack | `.env` lokal pernah mengubah `POSTGRES_PORT` (mis. `5432` dipakai PostgreSQL Windows); kembalikan/ubah port host lalu `up -d` |

## Catatan

- Token dikirim **hanya** via `Authorization: Bearer <token>`; token di
  `<soapenv:Header>` memang ditolak (401) — lihat TC25.
- `reports/` dibersihkan setiap run sukses; backup manual bila perlu.
- Test Stage 1 (`bash tests/stack/run.sh`) kini ikut membangun dan
  menghidupkan `soap-partnerships-service` (overlay `docker-compose.test.yml`).
