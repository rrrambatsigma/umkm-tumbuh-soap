# UMKM Tumbuh

[English](README.md) | **Bahasa Indonesia**

Platform untuk mengelola usaha mikro, kecil, dan menengah (UMKM), kerja sama mitra, dokumen, serta dasbor usaha. Frontend menggunakan React dan TypeScript; empat layanan Go berbagi basis data PostgreSQL. Garage menyediakan penyimpanan yang kompatibel dengan S3, sedangkan Mailpit menerima email selama pengembangan.

## Menjalankan secara lokal

Kebutuhan: Docker Desktop/Engine dengan Compose versi 2.24.4 atau lebih baru, Node.js 22, npm, dan Git. Jika menggunakan WSL, aktifkan integrasi Docker Desktop untuk distribusi yang digunakan.

Jalankan perintah berikut dari direktori utama repositori. Berkas konfigurasi yang sudah ada tetap dipertahankan:

```bash
[ -f .env ] || cp .env.example .env
[ -f frontend/.env ] || cp frontend/.env.example frontend/.env
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait --wait-timeout 180
```

Jalankan frontend di terminal kedua:

```bash
cd frontend
npm ci
npm run dev
```

Buka [aplikasi](http://localhost:5173). Akun admin lokal bawaan adalah `admin@example.com` / `admin12345`. Nilainya berasal dari `ADMIN_EMAIL` dan `ADMIN_PASSWORD` dalam `.env` utama; mengubah nilainya tidak mereset akun yang sudah ada.

Backend menjalankan PostgreSQL, menerapkan migrasi, membuat akun admin, dan menyiapkan Garage sebelum menjalankan API yang bergantung padanya. Kredensial S3 dibuat dan dibagikan secara otomatis. Frontend membaca konfigurasi tersendiri dari `frontend/.env`. Berkas contoh mengaktifkan `VITE_USE_DEV_PROXY=true`, sehingga permintaan API browser diteruskan melalui Vite ke backend di WSL. Tambahkan flag ini pada berkas lingkungan frontend yang sudah ada untuk mengaktifkannya; lihat [panduan frontend](frontend/README.id.md#proxy-api-lokal).

Lihat [panduan pengembangan lokal](docs/README_LOCAL_DEV.id.md) untuk variabel konfigurasi, urutan startup, volume, dan penanganan masalah.

## Layanan

| Komponen | Alamat bawaan | Fungsi |
| --- | --- | --- |
| Frontend | http://localhost:5173 | Aplikasi React |
| Auth service | http://localhost:8080/api/v1 | Autentikasi dan API admin |
| User service | http://localhost:8081/api/v1 | Profil dan dasbor |
| Partnerships service | http://localhost:8082/api/v1 | Pengajuan dan pengelolaan kerja sama |
| Document service | http://localhost:8083/api/v1 | Unggah dan unduh dokumen |
| PostgreSQL | localhost:5432 | Basis data bersama |
| Garage | http://localhost:3900 / http://localhost:3903 | API S3 / API admin |
| Mailpit | http://localhost:8025 | Kotak masuk email pengembangan; SMTP menggunakan port 1025 |

Port backend yang dipublikasikan hanya menerima koneksi melalui `127.0.0.1` secara bawaan. Ubah port host pada `.env` utama dan jalankan ulang Vite agar proxy membaca nilai baru. Jika proxy dinonaktifkan, sesuaikan juga URL frontend. Port di dalam container tetap sama.

## Struktur repositori

| Lokasi | Isi |
| --- | --- |
| [frontend/](frontend/) | React 18, TypeScript, Vite, Tailwind CSS, dan modul fitur |
| [services/auth-service/](services/auth-service/) | Autentikasi akun dan operasi admin |
| [services/user-service/](services/user-service/) | Profil dan dasbor |
| [services/partnerships-service/](services/partnerships-service/) | API kerja sama |
| [services/document-service/](services/document-service/) | API berkas dengan penyimpanan Garage |
| [infra/db/migrations/](infra/db/migrations/) | Skema PostgreSQL bersama dan data referensi |
| [infra/garage/](infra/garage/) | Konfigurasi dan penyiapan awal Garage |
| [tests/stack/](tests/stack/) | Pengujian integrasi Stage 1 dan data uji terisolasi |
| [tests/postman/](tests/postman/) | Koleksi Postman/Newman yang sudah ada |

Proses build Docker menggunakan Go 1.26.3. PostgreSQL menggunakan versi 16, sedangkan Garage ditetapkan pada versi 2.0.0.

## Menguji stack lokal

```bash
bash tests/stack/run.sh
```

Skrip ini membangun proyek pengujian terpisah dengan enam akun fixture dan satu akun yang didaftarkan oleh Newman, menjalankan koleksi permintaan API, memeriksa keempat layanan, mengunggah dan mengunduh berkas melalui layanan penyimpanan, menjalankan ulang migrasi dan bootstrap, lalu membuat ulang container untuk memeriksa ketahanan data. Skrip menggunakan `.env.example` dan konfigurasi pengujian tambahan, tidak memublikasikan port host, serta membersihkan container dan volume pengujiannya.

Pada pemeriksaan basis data yang salah, pesan berikut memang diharapkan:

```text
ERROR:  Fixtures require the isolated umkm_tumbuh_test database
```

Pengujian yang berhasil diakhiri dengan:

```text
Stage 1 stack checks passed.
```

Pengujian tidak membiarkan aplikasi pengembangan tetap berjalan. Jalankan aplikasi dengan perintah startup di atas. Seluruh akun uji bersifat sementara; gunakan akun admin bawaan atau lakukan registrasi pada aplikasi biasa. Penjelasan lengkap tersedia pada [panduan data uji dan pengujian](docs/README_LOCAL_DEV.id.md#menjalankan-pengujian-stage-1-terisolasi).

[Panduan Postman/Newman](tests/postman/README.id.md) menjelaskan suite kontrak API saat ini, laporan JUnit, dan koleksi arsip. Keberhasilan Stage 1 memastikan infrastruktur lokal dan alur API yang diuji berfungsi; hasil ini belum memvalidasi seluruh fitur atau aturan otorisasi.

Perintah stack juga menjalankan regresi otorisasi Stage 2. Lihat [panduan otorisasi](docs/README_AUTHORIZATION.id.md) untuk aturan peran/kepemilikan, cakupan pengujian, dan batasan saat ini.

## Pemeriksaan frontend dan CI

```bash
npm --prefix frontend ci
npm --prefix frontend run check
```

Pemeriksaan frontend menjalankan ESLint tanpa toleransi warning, TypeScript dan build produksi, pengujian regresi login dan proxy, serta pengujian halaman/unggahan. CI menjalankan pemeriksaan yang sama. Job Go tetap memeriksa format, kerapian modul, vet, pengujian, dan build. Lihat [panduan CI](docs/README_LOCAL_DEV.id.md#ci-dan-pemeriksaan-sebelum-push) untuk perintah lokal dan cakupan workflow.

## Mengelola aplikasi

Jalankan dari direktori utama repositori:

```bash
# Memeriksa container dan log
docker compose --env-file .env -f infra/docker-compose.yml ps -a
docker compose --env-file .env -f infra/docker-compose.yml logs --tail 80

# Menghentikan aplikasi dengan tetap menyimpan datanya
docker compose --env-file .env -f infra/docker-compose.yml down
```

Gunakan nama proyek Compose yang sama agar volume lama digunakan kembali. Menambahkan `--volumes` atau `-v` pada perintah `down` aplikasi akan menghapus data lokalnya.

Seed CSV berukuran besar bersifat opsional dan mengganti data aplikasi. Seed ini tidak diperlukan untuk startup atau pengujian Stage 1. Baca [panduan basis data](infra/db/README.id.md) sebelum menggunakannya; jangan menganggap seluruh akun dalam dataset memiliki kata sandi yang sama.

## Dokumentasi

| Panduan | English | Bahasa Indonesia |
| --- | --- | --- |
| Gambaran proyek | [Read](README.md) | [Baca](README.id.md) |
| Pengembangan lokal dan Stage 1 | [Read](docs/README_LOCAL_DEV.md) | [Baca](docs/README_LOCAL_DEV.id.md) |
| Otorisasi Stage 2 | [Read](docs/README_AUTHORIZATION.md) | [Baca](docs/README_AUTHORIZATION.id.md) |
| Frontend | [Read](frontend/README.md) | [Baca](frontend/README.id.md) |
| Basis data | [Read](infra/db/README.md) | [Baca](infra/db/README.id.md) |
| Postman/Newman | [Read](tests/postman/README.md) | [Baca](tests/postman/README.id.md) |
| Catatan seed CSV | [Read](infra/db/dummy/seed-csv/README.txt) | [Baca](infra/db/dummy/seed-csv/README.id.txt) |

Perbarui kedua versi bahasa ketika mengubah perintah, variabel, atau perilaku aplikasi. Nama perintah, lokasi berkas, field API, dan nama variabel lingkungan tetap sama pada kedua bahasa. Pilihan bahasa ini berlaku untuk dokumentasi repositori.

## Berkontribusi

Buat branch fitur, lakukan perubahan yang terfokus, jalankan pemeriksaan yang relevan, lalu ajukan pull request yang menjelaskan perilaku dan hasil validasi. Simpan perubahan skema di `infra/db/migrations/`; jangan menambahkan direktori migrasi per layanan tanpa keputusan arsitektur. Jangan commit berkas `.env` atau kredensial hasil generator.
