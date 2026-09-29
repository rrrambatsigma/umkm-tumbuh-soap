# Stage 2: otorisasi

[English](README_AUTHORIZATION.md) | **Bahasa Indonesia**

Tahap ini melindungi operasi kemitraan serta operasi pelatihan, pendaftaran, dan sertifikat. Perubahan melanjutkan stack Stage 1 yang sudah berjalan dan memakai akun uji terisolasi untuk pengujian regresi.

## Autentikasi

Layanan kemitraan dan pelatihan memverifikasi access token menggunakan `JWT_SECRET` yang sama dengan auth-service. Keduanya menerima tanda tangan HS256, mewajibkan waktu kedaluwarsa yang valid, serta mewajibkan ID akun (`sub`) yang tidak kosong dan peran yang dikenal (`ADMIN`, `UMKM`, atau `MITRA`). Token yang tidak ada, kedaluwarsa, tanpa tanda tangan, ditandatangani dengan kunci yang salah, atau tidak valid akan menghasilkan HTTP 401. Header `X-User-Role` tidak memberikan hak akses.

Endpoint health tetap publik. Rute katalog pelatihan tetap publik. Semua rute API kemitraan memerlukan token UMKM atau Mitra yang sudah diverifikasi.

## Aturan kemitraan

| Operasi | Pihak yang diizinkan | Status awal yang diizinkan |
| --- | --- | --- |
| Melihat daftar/detail UMKM | Mitra | — |
| Melihat daftar/detail Mitra | UMKM | — |
| Membuat pengajuan | UMKM atau Mitra, memakai akun sendiri | — |
| Membaca detail pengajuan | Pengaju atau penerima | Semua |
| Membaca daftar dan ringkasan pengajuan keluar/masuk | Akun saat ini, terbatas pada pengajuannya sendiri | Semua |
| Menandai pengajuan dibaca | Penerima | Semua |
| Menyetujui atau menolak | Penerima | `DIAJUKAN`, `DITINJAU` |
| Membatalkan | Pengaju | `DRAFT`, `DIAJUKAN`, `DITINJAU` |
| Mengirim dokumen kontrak | Pengaju, menggunakan dokumennya sendiri | `DIAJUKAN`, `DITINJAU`, `MENUNGGU_DOKUMEN_TTD` |

Lampiran dari endpoint upload saat ini harus aktif dan dimiliki pengaju. Pemeriksaan dilakukan sebelum pengajuan dibuat. Perubahan status dan kontrak juga memeriksa pelaku serta status awal di pernyataan SQL, sehingga keputusan berdasarkan data lama tidak dapat menimpa perubahan yang sudah selesai. Pelaku yang tidak berhak menerima HTTP 403; transisi yang tidak diizinkan atau memakai status lama menerima HTTP 409.

Migrasi `031_seed_partnership_statuses` menambahkan referensi status yang diperlukan, yang sebelumnya hanya tersedia dalam dataset dummy opsional. Database baru dapat membuat pengajuan tanpa memuat data dummy. Label yang sudah ada dipertahankan; rollback tetap menyimpan data referensi bersama ini.

Pengiriman kontrak masih merujuk tabel lama `document.transaksi_dokumenterunggah`. Lampiran biasa memakai `documents.master_dokumen`. Pengujian kontrak menggunakan dua data uji metadata lama; menghubungkan endpoint upload saat ini ke pengiriman kontrak masih memerlukan migrasi skema tersendiri. Endpoint penanda dibaca belum menyimpan waktu baca. Persetujuan mempertahankan perilaku sebelumnya dan belum mewajibkan kontrak bertanda tangan.

## Aturan pelatihan dan sertifikat

| Operasi | Pihak yang diizinkan |
| --- | --- |
| Mengelola program pelatihan, termasuk daftar/statistik admin | Admin |
| Membuat program pelatihan | Admin; pembuat yang disimpan ditentukan dari akun terautentikasi |
| Mendaftar, memperbarui progres, menyelesaikan pelatihan | Akun UMKM pemilik usaha/pendaftaran tersebut |
| Membaca daftar pendaftaran pelatihan | Pemilik usaha atau admin |
| Melampirkan dokumen evaluasi | Pemilik pendaftaran, menggunakan dokumen aktif miliknya sendiri |
| Mengajukan sertifikat | Pemilik pendaftaran |
| Membaca daftar, dashboard, detail, atau PDF sertifikat | Pemilik usaha/sertifikat atau admin |
| Membaca daftar/statistik sertifikat global; menyetujui atau menolak sertifikat | Admin |

Kepemilikan ditentukan melalui basis data: akun → pelaku UMKM → usaha → pendaftaran → sertifikat. ID usaha, pendaftaran, atau sertifikat yang dikirim pengguna bukan bukti kepemilikan. Data yang tidak ada atau tidak dapat diakses menghasilkan HTTP 403 bagi pengguna biasa; kegagalan basis data tidak memberikan akses. Admin dapat memeriksa data peserta, tetapi tidak dapat memakai endpoint perubahan data peserta atas nama mereka.

PDF sertifikat memakai nama `sertifikat_<id>.pdf`, sehingga dua peserta dengan nama dan judul pelatihan yang sama tidak saling menimpa berkas. PDF sertifikat yang sudah ada dapat dibuat ulang saat diunduh oleh pihak yang berhak.

Perubahan ini belum menambahkan pemeriksaan logout/pencabutan token lintas layanan, pemeriksaan status akun terkini, atau aturan penilaian/kelulusan baru. Token valid diperiksa secara lokal hingga kedaluwarsa; aturan persetujuan registrasi dan kelulusan perlu cakupan pengujian tersendiri. Cakupan di atas bukan pernyataan bahwa seluruh endpoint pada kelima layanan sudah diaudit.

## Menjalankan pengujian

Dari direktori utama repositori:

```bash
bash tests/stack/run.sh
```

Perintah ini membangun kelima layanan, menjalankan suite Newman yang berisi 46 request, menjalankan `tests/stack/authorization.py` terhadap layanan sebenarnya, lalu mengulangi pemeriksaan penyimpanan/persistensi Stage 1. Basis data dan volume pengujian bersifat sementara, tanpa port yang dipublikasikan ke host. Go dan Newman tidak perlu dipasang pada host.

Pengujian otorisasi memeriksa alur yang diizinkan serta token palsu/kedaluwarsa, pemalsuan peran, pengelolaan oleh non-admin, akses lintas akun, referensi dokumen milik pihak lain, serta keputusan kemitraan pada status tertutup atau yang diulang. Pengujian membuat pelatihan, pendaftaran, sertifikat, upload, dan pengajuannya sendiri. Enam akun uji semula tetap menjadi dasar identitas.

Pengujian yang berhasil diakhiri dengan:

```text
Stage 1 stack checks passed.
Stage 2 authorization checks passed.
```

Untuk unit test Go, gunakan Go 1.26.3 dan compiler C untuk deteksi race:

```bash
(cd services/partnerships-service && go test -race ./...)
(cd services/training-service && go test -race ./...)
```

CI juga memeriksa format, menjalankan `go vet`, dan membangun kedua layanan tersebut. Pemeriksaan auth/user/frontend yang sudah ada tetap berjalan. Newman menulis `tests/postman/reports/newman.xml`; pengujian otorisasi menulis `tests/stack/reports/authorization.txt`. Workflow `Local stack integration` mengunggah keduanya sebagai `api-contract-results`. Laporan hasil pengujian diabaikan oleh Git.

Setelah menerapkan patch, jalankan migrasi penambahan data referensi dan bangun ulang layanan di lingkungan pengembangan biasa, dari direktori utama repositori:

```bash
docker compose --env-file .env -f infra/docker-compose.yml run --build --rm db-migrate
docker compose --env-file .env -f infra/docker-compose.yml up -d --build --wait --wait-timeout 180 partnerships-service training-service
```

Jalankan ulang Vite jika sedang aktif. Pertahankan `.env` dan volume database yang sudah ada; tidak ada variabel lingkungan baru yang diperlukan.
