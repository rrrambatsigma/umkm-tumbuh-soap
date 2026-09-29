# Pengujian kontrak API dengan Postman dan Newman

[English](README.md) | **Bahasa Indonesia**

Koleksi otomatis yang digunakan adalah `umkm-tumbuh-current-progress.postman_collection.json`. Sebanyak 46 permintaan mencakup kondisi auth/user, registrasi, verifikasi email, login akun menunggu dan disetujui, tinjauan admin, profil, dasbor, beberapa kasus penolakan, serta pemeriksaan API REST kemitraan yang bersifat baca-saja (login, daftar pengajuan keluar/masuk, ringkasan, direktori UMKM dan mitra, serta pemeriksaan peran).

## Menjalankan suite terisolasi

Dari direktori utama repositori, dengan Docker aktif:

```bash
bash tests/stack/run.sh
```

Perintah ini menjalankan stack terisolasi, membuat akun admin uji, lalu menjalankan Newman 6.2.1 dalam container Node.js 22. Newman menggunakan `ci.postman_environment.json`, yang mengakses layanan melalui nama Compose dan hanya berisi kredensial pengujian sementara. Newman tidak perlu dipasang pada komputer Anda.

Runner berhenti jika permintaan atau assertion gagal, menulis `tests/postman/reports/newman.xml`, lalu menghapus container dan volume pengujiannya saat selesai. Git mengabaikan laporan yang dihasilkan. Workflow `Local stack integration` mengunggah laporan sebagai `api-contract-results` jika tersedia, termasuk saat gagal. Dua workflow Newman lama yang berdiri sendiri diganti oleh job terisolasi ini.

Perintah lengkap juga memeriksa unggahan, pengulangan migrasi/bootstrap, serta persistensi penyimpanan. Lihat [panduan pengembangan lokal](../../docs/README_LOCAL_DEV.id.md#menjalankan-pengujian-stage-1-terisolasi).

## Alur autentikasi yang diuji

1. Login sebagai admin uji dan daftarkan akun UMKM dengan identitas unik.
2. Pastikan login ditolak ketika email belum diverifikasi.
3. Minta kode verifikasi dan konfirmasikan. Layanan auth terisolasi memakai `APP_ENV=development`; pengujian membaca `dev_code` dari responsnya.
4. Pastikan akun terverifikasi dapat login untuk onboarding meskipun statusnya masih `MENUNGGU`.
5. Temukan akun dalam daftar registrasi menunggu, setujui sebagai admin, lalu pastikan login berikutnya mengembalikan `DISETUJUI`.
6. Pastikan token UMKM tidak dapat menjalankan operasi persetujuan admin.

Assertion daftar admin membaca `data.users`; respons persetujuan memakai `status: "success"`. Status registrasi diperiksa melalui respons login berikutnya.

Suite ini menguji kontrak API. Pengiriman SMTP, tampilan browser, seluruh alur pengisian profil/pengajuan dokumen, seluruh aturan otorisasi, dan semua fitur ketiga layanan lainnya belum diuji di sini. Persetujuan admin dijalankan sebelum skenario profil/produk/penjualan yang terpisah. Hasil berhasil hanya membuktikan permintaan yang diuji.

## Menjalankan secara opsional pada stack pengembangan

Permintaan ini membuat akun, menyetujui registrasi, serta menulis produk dan penjualan. Runner terisolasi di atas menghapus data ujinya secara otomatis; eksekusi manual pada stack pengembangan biasa meninggalkan data tersebut.

Dengan stack pengembangan sudah berjalan dan `APP_ENV=development`, jalankan:

```bash
mkdir -p tests/postman/reports
npx --yes newman@6.2.1 run \
  tests/postman/umkm-tumbuh-current-progress.postman_collection.json \
  --environment tests/postman/local.postman_environment.json \
  --bail --timeout-request 15000 --timeout-script 5000 \
  --reporters cli,junit \
  --reporter-junit-export tests/postman/reports/newman-local.xml
```

Lingkungan lokal mengasumsikan auth pada `http://localhost:8080` dan user pada `http://localhost:8081`. Base URL tidak menyertakan `/api/v1` karena bagian tersebut sudah ada dalam koleksi. Kredensial admin harus sesuai dengan basis data tujuan; gunakan salinan environment lokal pribadi jika nilainya berbeda. Koleksi saat ini membuat akun UMKM sendiri dan tidak memakai kredensial UMKM/Mitra dari seed lama pada environment lokal. Jalankan melalui WSL jika Docker terintegrasi dengan WSL.

Koleksi membuat email, nomor telepon, dan NIK baru ketika `run_id` belum diisi, serta menggunakan kata sandi uji bawaan `NewmanTest123!`. Dalam aplikasi Postman, kosongkan `run_id` sebelum menjalankan ulang seluruh koleksi. Jangan commit ekspor environment pribadi atau token.

## Koleksi arsip

`umkm-tumbuh-backend.postman_collection.json` merupakan koleksi lama berisi 35 permintaan. Koleksi ini belum menyertakan alur verifikasi terbaru dan memiliki assertion yang sudah tidak sesuai. Koleksi skenario lain tetap tersedia sebagai referensi dan untuk pemeliharaan manual. Koleksi tersebut bukan pemeriksaan CI saat ini; perbarui kontraknya sebelum menjadikannya dasar pengujian regresi.

## Pembuatan profil dan kategori usaha

Profil baru menggunakan jenis UMKM `UMKM` yang tersedia dari seed. Kategori usahanya disimpan terpisah, misalnya `MAKANAN` untuk nilai permintaan `Makanan`. ID kategori tidak boleh dimasukkan ke `jenis_umkm_id`: kedua field merujuk tabel yang berbeda.

Pengujian profil memeriksa HTTP 200 dan kategori usaha yang dikembalikan. Jika statusnya berbeda, assertion menyertakan body respons dalam `newman.xml` agar kesalahan backend terlihat tanpa tertutup kesalahan script tambahan. Build ulang user-service setelah memperbarui kode backend; `bash tests/stack/run.sh` membangun layanan pengujian terisolasi secara otomatis.

Stack terisolasi juga menjalankan [suite otorisasi Stage 2](../../docs/README_AUTHORIZATION.id.md) tersendiri untuk kemitraan dan pelatihan/sertifikat. Hasilnya tercatat di `tests/stack/reports/authorization.txt`, bukan di XML Newman.
