# Pengembangan lokal dan pengujian Stage 1

[English](README_LOCAL_DEV.md) | **Bahasa Indonesia**

Jalankan perintah dari direktori utama repositori melalui WSL atau terminal Linux/macOS. Jika menggunakan WSL, aktifkan integrasi Docker Desktop untuk distribusi yang digunakan. Backend berjalan dalam Docker; frontend dijalankan secara terpisah.

## Menjalankan aplikasi

Kebutuhan: Docker Engine/Desktop, Docker Compose versi 2.24.4 atau lebih baru, Node.js 22, dan npm. Go hanya diperlukan jika backend dijalankan di luar Docker.

Jalankan dari direktori utama repositori; perintah ini hanya menyalin berkas yang belum ada:

```bash
[ -f .env ] || cp .env.example .env
[ -f frontend/.env ] || cp frontend/.env.example frontend/.env
```

Pertahankan berkas `.env` yang sudah digunakan saat memperbarui proyek. Bandingkan dengan berkas contoh, lalu tambahkan variabel yang belum ada. Kata sandi dan token contoh ditujukan untuk pengembangan lokal.

```bash
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait --wait-timeout 180
docker compose --env-file .env -f infra/docker-compose.yml ps -a
```

PostgreSQL dijalankan terlebih dahulu, migrasi diterapkan, lalu akun admin dibuat. Bootstrap Garage menyiapkan satu node, membuat bucket sesuai konfigurasi, dan menyimpan kredensial sebelum user-service serta document-service dijalankan. Container migrasi, seed, dan bootstrap seharusnya selesai dengan exit code 0. Kelima container API seharusnya berstatus healthy.

Di terminal lain:

```bash
cd frontend
npm ci
npm run dev
```

Buka [aplikasi](http://localhost:5173). Akun admin bawaan adalah `admin@example.com` / `admin12345`, yang diatur melalui `ADMIN_EMAIL` dan `ADMIN_PASSWORD` pada `.env` utama. Proses seed mempertahankan admin yang sudah ada; perubahan variabel tersebut tidak mereset kata sandi akun lama.

## Alamat layanan

| Layanan | Alamat host bawaan | Variabel port host |
| --- | --- | --- |
| API auth/admin | http://localhost:8080/api/v1 | `AUTH_SERVICE_PORT` |
| API user | http://localhost:8081/api/v1 | `USER_SERVICE_PORT` |
| API partnerships | http://localhost:8082/api/v1 | `PARTNERSHIP_SERVICE_PORT` |
| API document | http://localhost:8083/api/v1 | `DOCUMENT_SERVICE_PORT` |
| API training/certificates | http://localhost:8084/api/v1 | `TRAINING_SERVICE_PORT` |
| PostgreSQL | localhost:5432 | `POSTGRES_PORT` |
| Garage S3 | http://localhost:3900 | `GARAGE_S3_PORT` |
| Admin Garage | http://localhost:3903 | `GARAGE_ADMIN_PORT` |
| Kotak masuk Mailpit | http://localhost:8025 | `MAILPIT_HTTP_PORT` |
| SMTP Mailpit | localhost:1025 | `MAILPIT_SMTP_PORT` |

Seluruh port yang dipublikasikan menggunakan `127.0.0.1` secara bawaan. Port container tetap sama sehingga perubahan port host tidak mengganggu komunikasi antarlayanan. Dengan `VITE_USE_DEV_PROXY=true` pada `frontend/.env`, Vite membaca kelima port host API dari `.env` utama dan meneruskan permintaan ke `127.0.0.1` di dalam WSL. Jalankan ulang Vite setelah mengubah port. Jika proxy dinonaktifkan, sesuaikan juga URL API frontend. Lihat [panduan proxy frontend](../frontend/README.id.md#proxy-api-lokal) jika Windows dapat membuka frontend tetapi tidak dapat mengakses port backend. Jika `GARAGE_S3_PORT` diubah, sesuaikan juga `OBJECT_STORAGE_PUBLIC_ENDPOINT`. Pertahankan `OBJECT_STORAGE_ENDPOINT=http://garage:3900` untuk stack Compose ini.

## Kredensial Garage dan penyimpanan data

`garage-bootstrap` menggunakan kembali key bernama `UMKM App Key` dan tidak menghapus key lain. Nama bucket berasal dari variabel lingkungan yang juga digunakan oleh layanan. Kegagalan penyiapan menghentikan bootstrap sehingga layanan yang bergantung padanya tidak dijalankan.

Volume `garage_credentials` menyimpan `/run/garage/garage.env`, dimiliki UID 10001 dengan izin 0600. Container user dan document memasangnya dalam mode hanya-baca dan memuatnya sebelum layanan berjalan. Jangan menyalin key S3 contoh yang tidak valid ke `.env`; layanan Compose memakai kredensial yang dibuat otomatis. Garage dan bootstrap sama-sama menerima `GARAGE_ADMIN_TOKEN` dari Compose.

Basis data, objek Garage, kredensial bersama, unggahan lama, dan sertifikat pelatihan memiliki volume tersendiri. Penghentian dan pembuatan ulang container secara normal mempertahankan data tersebut:

```bash
docker compose --env-file .env -f infra/docker-compose.yml down
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait --wait-timeout 180
```

Jangan menambahkan `--volumes` atau `-v` pada perintah `down` aplikasi kecuali memang ingin menghapus data lokalnya. Pertahankan nama proyek Compose saat memperbarui aplikasi agar volume lama tetap digunakan. Nama container dikelola oleh Compose; gunakan nama layanan pada perintah, misalnya `docker compose ... exec postgres`.

Untuk menjalankan ulang penyiapan Garage:

```bash
docker compose --env-file .env -f infra/docker-compose.yml run --rm --no-deps garage-bootstrap
```

Jika berkas kredensial hilang sementara data Garage masih ada, perintah ini memulihkan key bernama sama yang sudah tersedia. Nama key aplikasi yang duplikat menyebabkan kegagalan; periksa key di Garage dan selesaikan duplikasi namanya sebelum mencoba lagi. Jangan menghapus volume data Garage untuk memperbaiki kredensial.

## Menjalankan pengujian Stage 1 terisolasi

```bash
bash tests/stack/run.sh
```

Skrip membaca `.env.example` dan `infra/docker-compose.test.yml`. Pengembangan biasa menggunakan `.env` utama, sedangkan Vite menggunakan `frontend/.env`; pengujian terisolasi tidak menguji konfigurasi pribadi tersebut ataupun antarmuka browser.

Skrip membuat proyek Compose dengan nama unik, tidak memublikasikan port host, dan menggunakan basis data `umkm_tumbuh_test` serta volume terpisah. Skrip membangun backend, menerapkan migrasi, membuat enam akun sintetis, lalu memeriksa:

- Kondisi kelima API dan koneksi basis data pada layanan yang menyediakan pemeriksaan tersebut.
- Pengisian data uji dua kali serta login dengan peran dan status registrasi yang sesuai.
- Koleksi Newman berisi 37 permintaan: verifikasi email, login akun menunggu/disetujui, tinjauan admin, profil, produk, penjualan, dasbor, dan beberapa permintaan yang harus ditolak.
- Unggah dan unduh gambar produk melalui user-service.
- Unggah dan unduh dokumen melalui document-service, termasuk penggunaan nama bucket khusus.
- Pengulangan migrasi dan bootstrap tanpa mengganti key aplikasi atau menghapus key lain.
- Pembuatan ulang container dengan kredensial yang sama serta isi unggahan yang identik hingga setiap byte.

Skrip menampilkan log layanan jika gagal dan hanya menghapus proyek serta volume pengujiannya saat selesai, termasuk volume penyimpan status dari profil `check`. Dataset CSV besar tidak dimuat. Perintah yang sama digunakan pada `.github/workflows/local-stack.yml`. Newman membuat satu akun UMKM tambahan dalam basis data sementara ini. Verifikasi menggunakan kode khusus pengembangan dari respons API; pengiriman email belum diuji. Konfigurasi pengujian menetapkan `APP_ENV=development` untuk auth.

Newman menulis `tests/postman/reports/newman.xml`, yang diabaikan Git. GitHub Actions mengunggahnya sebagai `api-contract-results`, termasuk saat pengujian gagal jika laporannya tersedia. Dua workflow Newman lama diganti oleh satu job terisolasi ini. Lihat [panduan pengujian API](../tests/postman/README.id.md) untuk koleksi dan keterbatasannya.

### Membaca hasil pengujian

Pengujian basis data yang salah sengaja mencoba mengisi data uji ke basis data `postgres`. Penolakan yang diharapkan adalah:

```text
ERROR:  Fixtures require the isolated umkm_tumbuh_test database
```

Pesan `INSERT 0 0` saat pengisian ulang berarti baris data uji yang sudah ada dipertahankan. Pesan migrasi `Skipping already applied migration` juga normal. Pengujian lengkap yang berhasil diakhiri dengan:

```text
Stage 1 stack checks passed.
```

Status `Exited` normal untuk migrasi, seed, dan bootstrap yang sudah selesai dengan sukses. Penghapusan container dan volume pada akhir log merupakan pembersihan pengujian. Setelah itu, aplikasi pengembangan tidak berjalan kecuali sebelumnya dijalankan secara terpisah. Gunakan perintah startup pada awal panduan ini, lalu jalankan frontend di terminal lain.

### Akun uji

Seluruh kata sandi akun uji adalah `Stage1Test123!`:

| Email | ID akun | Kondisi |
| --- | --- | --- |
| admin@stage1.test | TEST_ADMIN | Admin |
| umkm.a@stage1.test | TEST_UMKM_A | UMKM disetujui, memiliki usaha tersendiri |
| umkm.b@stage1.test | TEST_UMKM_B | UMKM disetujui, memiliki usaha tersendiri |
| mitra.a@stage1.test | TEST_MITRA_A | Mitra disetujui, memiliki profil tersendiri |
| mitra.b@stage1.test | TEST_MITRA_B | Mitra disetujui, memiliki profil tersendiri |
| onboarding@stage1.test | TEST_ONBOARDING | Email terverifikasi, belum memiliki profil atau mengajukan registrasi |

Akun tersebut hanya ada dalam proyek pengujian selama skrip berjalan. `tests/stack/fixtures.sql` menolak basis data yang namanya bukan `umkm_tumbuh_test`. Pengulangan skrip mempertahankan baris data uji yang sudah ada, tanpa mengosongkan tabel.

## Penanganan masalah

```bash
docker compose --env-file .env -f infra/docker-compose.yml logs --tail 80 db-migrate garage garage-bootstrap
docker compose --env-file .env -f infra/docker-compose.yml logs --tail 80 auth-service user-service partnerships-service document-service
```

Jika port sudah digunakan, ubah variabel port host lalu jalankan ulang `up`. Jika API tetap unhealthy, baca log layanan tersebut sebelum mengubah data atau kredensial. Pesan `unknown tag !reset` berarti Compose perlu diperbarui ke versi minimal 2.24.4 agar dapat membaca konfigurasi tambahan untuk pengujian terisolasi.

Profil `seed` yang opsional mengimpor dataset CSV lama dan mengosongkan tabel aplikasi. Profil ini tidak diperlukan untuk menjalankan aplikasi maupun pengujian Stage 1. Gunakan hanya pada basis data pengembangan yang boleh diganti seluruh isinya. Lihat [panduan basis data](../infra/db/README.id.md) untuk perintahnya dan [panduan Postman/Newman](../tests/postman/README.id.md) untuk suite kontrak API saat ini dan koleksi arsip.

## CI dan pemeriksaan sebelum push

Dari direktori utama repositori:

```bash
npm --prefix frontend ci
npm --prefix frontend run check
bash tests/stack/run.sh
```

Pemeriksaan frontend memerlukan Node.js 22 dan npm. Pengujian stack memerlukan Docker dan Compose serta membangun kelima layanan Go; Go dan Newman tidak perlu dipasang pada host.

Job Go tersendiri di CI mencakup auth-service dan user-service. Jika Go 1.26.3 sudah terpasang, jalankan pemeriksaannya secara lokal:

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

Commit perubahan modul yang memang diperlukan sebelum pemeriksaan tidy, karena berkas dibandingkan dengan Git. Jika pemeriksaan format gagal, `gofmt -l .` dari direktori layanan tersebut menampilkan berkas yang perlu dirapikan. Jangan menonaktifkan pemeriksaan agar CI lolos.

`ci.yml` memeriksa Go, frontend, dan konfigurasi Compose pada pull request ke `main`, push ke pola branch yang didukung, serta eksekusi manual. `local-stack.yml` menjalankan stack sebenarnya saat layanan, infrastruktur, pengujian API, workflow, atau `.env.example` berubah, dan saat dijalankan manual. Keberhasilan pemeriksaan konfigurasi Compose saja belum membuktikan stack dapat berjalan.

Perintah yang sama kini mencakup [pengujian otorisasi Stage 2](README_AUTHORIZATION.id.md). Log otorisasi terperinci disimpan di `tests/stack/reports/authorization.txt`; CI mengunggahnya bersama laporan Newman. Job Go tambahan mencakup format, vet, pengujian race, dan build layanan kemitraan serta pelatihan.

## Aturan pengembangan

Jangan commit:

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

Sumber utama skema berada di:

```text
infra/db/migrations/
```

Jangan menambahkan direktori migrasi per layanan kecuali keputusan arsitekturnya berubah.
