# Frontend UMKM Tumbuh

[English](README.md) | **Bahasa Indonesia**

Aplikasi React 18 dan TypeScript yang dibangun dengan Vite. Modul fitur berada di `src/features/`; utilitas HTTP dan autentikasi bersama berada di `src/shared/`.

## Menjalankan pengembangan

Gunakan Node.js 22 dan npm. Jalankan backend melalui [panduan pengembangan lokal](../docs/README_LOCAL_DEV.id.md), lalu jalankan dari direktori utama repositori:

```bash
[ -f frontend/.env ] || cp frontend/.env.example frontend/.env
cd frontend
npm ci
npm run dev
```

Buka [aplikasi](http://localhost:5173). Pertahankan `.env` yang sudah ada dan bandingkan dengan [.env.example](.env.example) saat memperbarui proyek. `.env` utama mengatur backend dan tidak menggantikan `frontend/.env`.

## Proxy API lokal

Berkas contoh mengaktifkan `VITE_USE_DEV_PROXY=true`. Saat pengembangan, browser mengirim permintaan API ke Vite pada port 5173. Vite meneruskan setiap layanan ke alamat IPv4 di dalam WSL dan membaca kelima port API yang dipublikasikan dari `.env` utama.

Pada `frontend/.env` yang sudah ada, tambahkan atau perbarui:

```dotenv
VITE_USE_DEV_PROXY=true
```

Jalankan ulang `npm run dev` setelah mengubah flag ini atau port backend. Untuk mencoba satu kali tanpa mengubah berkas lingkungan, jalankan `VITE_USE_DEV_PROXY=true npm run dev` di dalam `frontend/`.

Dengan port bawaan, login diteruskan dari `localhost:5173/backend/auth/api/v1/auth/login` ke `127.0.0.1:8080/api/v1/auth/login` di dalam WSL. Prefix layanan memisahkan rute user, document, partnership, dan training, termasuk lokasi seperti `/admin/training` yang harus menuju training, bukan auth. Pemanggilan relatif `/api/v1/documents/...` yang sudah ada tetap didukung.

Periksa [kondisi basis data auth melalui Vite](http://localhost:5173/backend/auth/api/v1/health/db). Halaman seharusnya menampilkan JSON dari auth-service. Rute ini membantu ketika endpoint backend berhasil diakses dari WSL tetapi URL langsungnya terus loading di Windows.

Penggantian URL oleh proxy hanya berlaku pada `npm run dev` saat flag aktif. Nilai `false` menggunakan URL API eksplisit di bawah. Build produksi juga menggunakan URL eksplisit yang harus disesuaikan dengan deployment; flag pengembangan tidak memasang proxy produksi. Lihat [referensi proxy Vite](https://vite.dev/config/server-options.html#server-proxy).

## Alamat backend

Klien HTTP pada [src/shared/api/http.ts](src/shared/api/http.ts) memilih API untuk setiap permintaan. Nilai di bawah digunakan untuk koneksi langsung dan build produksi. Flag proxy mengganti kedelapan nilai tersebut saat pengembangan. Atur satu base URL untuk setiap layanan, termasuk bagian `/api/v1`:

| Variabel | Nilai bawaan |
| --- | --- |
| `VITE_AUTH_API_BASE_URL` | `http://localhost:8080/api/v1` |
| `VITE_ADMIN_API_BASE_URL` | `http://localhost:8080/api/v1` |
| `VITE_USER_API_BASE_URL` | `http://localhost:8081/api/v1` |
| `VITE_PARTNERSHIP_API_BASE_URL` | `http://localhost:8082/api/v1` |
| `VITE_DOCUMENT_API_BASE_URL` | `http://localhost:8083/api/v1` |
| `VITE_TRAINING_API_BASE_URL` | `http://localhost:8084/api/v1` |
| `VITE_CERTIFICATE_API_BASE_URL` | `http://localhost:8084/api/v1` |
| `VITE_API_BASE_URL` | `http://localhost:8082/api/v1` |

`VITE_API_BASE_URL` merupakan alamat cadangan untuk permintaan dengan layanan default, bukan gateway bersama bagi kelima layanan. Fitur sertifikat menggunakan training service.

Jika port backend yang dipublikasikan diubah, jalankan ulang Vite untuk membacanya dari `.env` utama. Jika proxy dinonaktifkan, sesuaikan juga URL frontend. Jika Vite menggunakan origin lain, misalnya port 5174, ubah `FRONTEND_URL` pada `.env` utama dan buat ulang container backend menggunakan `docker compose ... up -d` dengan perintah lengkap pada panduan lokal.

Nilai `VITE_*` merupakan konfigurasi yang tersedia di browser. Jangan menyimpan kata sandi basis data, `JWT_SECRET`, atau secret key S3 di dalamnya.

## Perintah yang tersedia

Jalankan di dalam `frontend/`:

| Perintah | Fungsi |
| --- | --- |
| `npm ci` | Memasang dependensi sesuai lockfile |
| `npm run dev` | Menjalankan server pengembangan Vite |
| `npm run lint` | Menjalankan ESLint; error maupun warning menggagalkan pemeriksaan |
| `npm run test:login` | Memeriksa permintaan login, pesan kesalahan, pembatalan, dan batas waktu dengan server uji lokal |
| `npm run test:proxy` | Memeriksa rute melalui Vite ke lima layanan uji lokal |
| `npm run build` | Memeriksa TypeScript dan membuat hasil build di `dist/` |
| `npm run test:pages` | Menguji pencarian, pagination, draf pelatihan, filter produk, dan respons unggahan registrasi |
| `npm run check` | Menjalankan lint, build, serta seluruh pengujian di atas |
| `npm run preview` | Meninjau hasil build yang sudah ada secara lokal |

Jalankan `npm run build` sebelum preview. Preview biasanya menggunakan port 4173; permintaan ke backend tetap memerlukan `FRONTEND_URL` yang sesuai. Preview tidak menjalankan backend.

`npm run check` menjalankan pemeriksaan frontend yang sama dengan job frontend di CI. Pengujian memakai server HTTP lokal dan data sintetis. Suite `test:pages` memuat modul frontend melalui loader SSR Vite dan memeriksa penanganan request/response serta jalur galat tanpa browser atau Docker; suite ini tidak me-render komponen React. Berkas uji skema lama pada `src/**/__tests__` belum termasuk perintah ini.

## Akun dan pengujian

Stack biasa membuat akun admin sesuai konfigurasi `.env` utama. Daftarkan akun UMKM dan Mitra melalui aplikasi; email lokal dapat dibaca melalui [Mailpit](http://localhost:8025).

Perintah `bash tests/stack/run.sh`, yang dijalankan dari direktori utama repositori, memeriksa integrasi backend, menjalankan koleksi API Newman, lalu menghapus seluruh akun pengujiannya setelah selesai. Akun tersebut tidak tersedia untuk sesi browser berikutnya. Pengujian Stage 1 tidak memeriksa tampilan browser atau seluruh fitur aplikasi.

Jika aplikasi gagal berjalan, periksa console dan permintaan jaringan pada browser, lalu baca log backend yang terkait. Lihat [panduan lokal](../docs/README_LOCAL_DEV.id.md) untuk alamat layanan dan perintahnya.

## Login berhenti pada “Memproses...”

Permintaan login dibatasi 15 detik dan menampilkan pesan kesalahan agar formulir dapat dicoba kembali. Batas ini mencakup pembacaan body respons. Permintaan status registrasi tambahan setelah login non-admin juga memiliki batas 15 detik; jika gagal, halaman menggunakan rute yang ditentukan dari respons login. Permintaan tidak diulang secara otomatis.

Timeout berarti respons belum selesai diterima; penyebab masalah koneksinya masih perlu diperiksa. Dari direktori utama repositori, periksa stack pengembangan biasa:

```bash
docker compose --env-file .env -f infra/docker-compose.yml ps auth-service postgres
curl -i --max-time 10 http://127.0.0.1:8080/api/v1/health/db
```

Jika perintah ini berhasil di WSL sementara [alamat auth langsung](http://localhost:8080/api/v1/health/db) terus loading di Windows, aktifkan [proxy API lokal](#proxy-api-lokal) dan gunakan tautan pemeriksaan melalui port 5173. Jika berhasil tetapi login masih gagal, lihat URL, status, dan waktu permintaan login pada tab Network browser. Dalam mode proxy, URL permintaan seharusnya diawali `http://localhost:5173/backend/auth/`. Dalam mode langsung, sesuaikan `VITE_AUTH_API_BASE_URL` dengan port auth yang dipublikasikan; kesalahan origin/CORS memerlukan pemeriksaan `FRONTEND_URL` terhadap alamat frontend.

`npm run test:login` memerlukan sekitar 30 detik tanpa basis data Docker atau akun asli. Pengujian memeriksa kode permintaan menggunakan respons HTTP yang dikendalikan; pengujian ini belum memvalidasi koneksi browser pengguna atau seluruh alur dasbor.
