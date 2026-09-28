# Test Case — SOAP Partnership Service (Anggota 5)

Dokumen ini berisi katalog test case untuk **`soap-partnerships-service`** (port 9090,
endpoint `POST /partnership`). Test case ditulis **sebelum** eksekusi; hasil eksekusi
dicatat terpisah di [`test-result.md`](test-result.md).

- **Service yang diuji:** `services/soap-partnerships-service` (SOAP 1.1, document/literal)
- **Kontrak:** `services/soap-partnerships-service/wsdl/partnership.wsdl` (9 operasi)
- **Cara menjalankan:** `bash tests/soap/run-soap-tests.sh` (lihat [`README.md`](README.md))
- **Test otomatis:** TC00–TC29 dijalankan otomatis oleh script · TC-P1/P2 dijalankan manual

---

## 1. Lingkup dan Prasyarat

### 1.1 Prasyarat (dicek otomatis oleh script — preflight)

| # | Dependency | URL default | Kegunaan dalam tes |
|---|---|---|---|
| 1 | auth-service | `http://localhost:8080/api/v1` | Register/login akun → token JWT |
| 2 | user-service | `http://localhost:8081/api/v1` | `PUT/GET /profiles/me` → data profil & business id |
| 3 | partnerships-service | `http://localhost:8082/health` | Kesehatan stack (tidak dipakai langsung) |
| 4 | soap-partnerships-service | `http://localhost:9090/partnership?wsdl` | **Service yang diuji** |
| 5 | PostgreSQL | via semua service | Penyimpanan data pengajuan |

Jika ada yang mati, script **berhenti dengan pesan jelas** (contoh:
`ERROR: SOAP service is unreachable at http://localhost:9090/partnership?wsdl`)
bukan error teknis yang membingungkan.

### 1.2 Akun uji (dibuat otomatis tiap run, tidak perlu disiapkan manual)

| Akun | Peran | Dipakai sebagai |
|---|---|---|
| `umkm.<timestamp>@soap.test` | UMKM | **Pengaju** (requester) — semua aplikasi dibuat dari akun ini |
| `mitra.<timestamp>@soap.test` | MITRA | **Penerima** (receiver) — approve / reject / mark-as-read |
| `admin@example.com` | ADMIN | Hanya untuk tes penolakan role (TC23) |

Kedua akun uji diberi profil (`PUT /profiles/me`) sehingga memiliki
`umkm_id` / `mitra_id` (business id) yang dibutuhkan kolom `receiverId`.

### 1.3 Pemetaan istilah rencana → sistem nyata

Rencana asli (§18) memakai istilah generik. Sistem nyata memakai status Indonesia
(tabel `ref.ref_statuspengajuan`):

| Rencana §18 | Sistem nyata | Keterangan |
|---|---|---|
| `PENDING` (status awal) | `DIAJUKAN` | Status setelah Create |
| `APPROVED` | `AKTIF` | Hanya penerima yang boleh, dari `DIAJUKAN`/`DITINJAU` |
| `REJECTED` | `DITOLAK` | Penerima, **`rejectionReason` wajib diisi** |
| — | `DIBATALKAN` | Pengaju membatalkan |
| — | `DITINJAU` | Hasil `MarkPartnershipAsRead` (workaround prototipe) |

> **Tidak ada status `APPROVED` di database** — hal ini didokumentasikan di
> `internal/partnerships/model.go:14-16`. Karena itu TC06/TC07 memakai `AKTIF`/`DITOLAK`.

### 1.4 Mesin status yang diuji

```
DIAJUKAN ──MarkAsRead(penerima)──▶ DITINJAU ──AKTIF(penerima)──▶ AKTIF
   │                                  │
   │                                  ├──DITOLAK(penerima, +alasan)──▶ DITOLAK
   ├──DIBATALKAN(pengaju)◀────────────┘
   │
   └──SignPartnership(pengaju)──▶ dokumen_perjanjian_id terisi (status tidak berubah)
```

### 1.5 Cara autentikasi (dikonfirmasi dari kode backend)

- JWT dikirim **hanya lewat HTTP header** `Authorization: Bearer <token>`
  (`internal/soap/handler.go:97` → `internal/soap/auth.go:30`).
- Elemen `<soapenv:Header>` XML **diabaikan** untuk auth (`internal/soap/envelope.go:13`)
  → token di dalam SOAP Header **tanpa** HTTP Authorization = fault 401 (diuji TC25).
- Field `userId` + `userRole` tetap **wajib ada di body XML** (kontrak WSDL) tetapi
  harus identik dengan klaim token, jika tidak → fault 403 (diuji TC26).
- Setiap request SOAP juga membawa header `SOAPAction` dan `Content-Type: text/xml`.

---

## 2. Katalog Test Case

Kategori: **VALID** = request benar · **INVALID** = data/input salah · **NOTFOUND** = data tidak ada ·
**STATUS** = transisi status · **AUTH** = autentikasi/otorisasi · **WSDL** = kontrak

### 2.1 WSDL & operasi inti (TC01–TC08 sesuai §18)

| Kode | Kategori | Operasi | Input | Expected result |
|---|---|---|---|---|
| TC00 | WSDL | `GET /partnership?wsdl` | Tanpa auth | HTTP 200, body berisi `<definitions` + operasi `CreatePartnershipApplication` dst. |
| TC01 | VALID | `CreatePartnershipApplication` | UMKM token; `receiverId` = `mitra_id` mitra uji; `proposalTitle` 10–200 char; `proposalDescription` 30–1000 char | HTTP 200; `<success>true`; `applicationId` format `PGJ######`; `requestCode` format `PKS-YYYY-######`; `status` = `DIAJUKAN` |
| TC02a | INVALID | `CreatePartnershipApplication` | `proposalTitle` = `"Judul"` (<10 karakter) | HTTP 400, fault `soap:Client`, `faultstring` berisi `proposalTitle minimal 10 karakter` |
| TC02b | INVALID | `CreatePartnershipApplication` | `proposalDescription` <30 karakter | HTTP 400, fault `soap:Client`, `faultstring` berisi `proposalDescription minimal 30 karakter` |
| TC02c | INVALID | `CreatePartnershipApplication` | `receiverId` kosong | HTTP 400, fault `soap:Client`, `faultstring` berisi `receiverId wajib diisi` |
| TC03 | VALID | `GetPartnershipApplication` | `applicationId` hasil TC01, oleh pengaju (UMKM) | HTTP 200; berisi `applicationId` sama, `status` = `DIAJUKAN`, `requesterName`/`receiverName` terisi |
| TC04 | NOTFOUND | `GetPartnershipApplication` | `applicationId` = `PGJ999999` (tidak pernah ada) | HTTP 404, fault `soap:Client`, `faultstring` berisi `tidak ditemukan` |
| TC05 | VALID | `GetPartnershipApplications` | UMKM, `page=1`, `limit=10` | HTTP 200; `totalCount` ≥ 1; daftar berisi `applicationId` hasil TC01 |
| TC06 | STATUS | `UpdatePartnershipStatus` | Penerima (MITRA); `applicationId` TC01; `status` = `AKTIF` | HTTP 200; `<success>true`; `status` = `AKTIF`; konfirmasi via Get → `AKTIF` |
| TC07 | STATUS | `UpdatePartnershipStatus` | Penerima (MITRA); pengajuan baru (APP2); `status` = `DITOLAK`; `rejectionReason` = teks alasan | HTTP 200; `status` = `DITOLAK`; konfirmasi via Get → `DITOLAK` + `rejectionReason` terisi |
| TC08 | INVALID | `UpdatePartnershipStatus` | `status` = `APPROVED` (tidak valid di sistem) | HTTP 400, fault `soap:Client`, `faultstring` berisi `status harus salah satu dari` |

### 2.2 Perluasan transisi status (TC09–TC11)

| Kode | Kategori | Operasi | Input | Expected result |
|---|---|---|---|---|
| TC09 | STATUS | `UpdatePartnershipStatus` | Pengaju (UMKM) membatalkan APP3; `status` = `DIBATALKAN` | HTTP 200; konfirmasi Get → `DIBATALKAN` |
| TC10 | STATUS | `UpdatePartnershipStatus` | **Pengaju** (salah actor) mencoba `AKTIF` pada APP4 (dari pihak penerima) | HTTP 409, fault `soap:Client` (0 baris ter-update → konflik) |
| TC11 | INVALID | `UpdatePartnershipStatus` | Penerima menolak **tanpa** `rejectionReason` | HTTP 422, fault `soap:Client`, `faultstring` berisi `rejectionReason wajib diisi` |

### 2.3 Daftar masuk & ringkasan (TC12–TC14)

| Kode | Kategori | Operasi | Input | Expected result |
|---|---|---|---|---|
| TC12 | VALID | `GetIncomingPartnerships` | MITRA, `page=1`, `limit=10` | HTTP 200; `totalCount` ≥ 1; berisi pengajuan dari UMKM |
| TC13 | VALID | `GetPartnershipSummary` | UMKM | HTTP 200; `summary>item` berisi pasangan `status`+`count` (minimal `DIAJUKAN` ≥ 1) |
| TC14 | VALID | `GetIncomingPartnershipSummary` | MITRA | HTTP 200; `summary>item` berisi hitungan per status milik penerima |

### 2.4 Tandai dibaca & tanda tangan kontrak (TC15–TC19)

| Kode | Kategori | Operasi | Input | Expected result |
|---|---|---|---|---|
| TC15 | STATUS | `MarkPartnershipAsRead` | Penerima (MITRA) pada APP1 (status `DIAJUKAN`) | HTTP 200; `<success>true`; Get berikutnya → status `DITINJAU` |
| TC16 | STATUS | `MarkPartnershipAsRead` | **Pengaju** (salah actor) pada APP2 | HTTP 409, fault `soap:Client` |
| TC17 | VALID | `SignPartnership` | Pengaju (UMKM) pada APP4; `documentId` = `DOKTEST01` | HTTP 200; `<success>true`; Get → `contractDocumentId` = `DOKTEST01` (status tidak berubah) |
| TC18 | STATUS | `SignPartnership` | **Penerima** (salah actor) pada APP4 | HTTP 409, fault `soap:Client` |
| TC19 | INVALID | `SignPartnership` | `documentId` kosong | HTTP 400, fault `soap:Client`, `faultstring` berisi `documentId wajib diisi` |

### 2.5 Filter daftar (TC20–TC21)

| Kode | Kategori | Operasi | Input | Expected result |
|---|---|---|---|---|
| TC20 | VALID | `GetPartnershipApplications` | UMKM; `status` = `DIAJUKAN` | HTTP 200; semua item berstatus `DIAJUKAN` |
| TC21 | INVALID | `GetPartnershipApplications` | UMKM; `status` = `APPROVED` | HTTP 400, fault `soap:Client`, `faultstring` berisi `status tidak valid` |

### 2.6 Autentikasi & kontrak identitas (TC22–TC27)

| Kode | Kategori | Skenario | Input | Expected result |
|---|---|---|---|---|
| TC22 | AUTH | Tanpa token | POST SOAP **tanpa** header `Authorization` | HTTP 401, fault `soap:Client`, `faultstring` = `Token autentikasi tidak valid` |
| TC23 | AUTH | Role ADMIN ditolak | POST dengan token login `admin@example.com` | HTTP 403, fault `soap:Client`, `faultstring` berisi `Role tidak diizinkan` |
| TC24 | AUTH | Token rusak/tidak valid | Header `Authorization: Bearer abc.def.ghi` | HTTP 401, fault `soap:Client` |
| TC25 | AUTH | Token di SOAP Header saja (salah cara) | Token diletakkan di `<soapenv:Header>`, **tanpa** HTTP `Authorization` | HTTP 401, fault `soap:Client` — membuktikan JWT hanya di HTTP header |
| TC26 | AUTH | Identitas tidak cocok | Token UMKM tetapi `userId` = id MITRA + `userRole` = `MITRA` di body XML | HTTP 403, fault `soap:Client`, `faultstring` berisi `Identitas request tidak sesuai token` |
| TC27 | INVALID | Identitas tidak sesuai kontrak WSDL | `userId`/`userRole` kosong/dihilangkan dari body | HTTP 400, fault `soap:Client`, `faultstring` berisi `wajib diisi sesuai kontrak WSDL` |

### 2.7 Body rusak & validasi input lain (TC28–TC29)

| Kode | Kategori | Skenario | Input | Expected result |
|---|---|---|---|---|
| TC28 | INVALID | Body XML rusak | Body `<bukan-envelope/>` (bukan SOAP Envelope) dengan token valid | HTTP 400, fault `soap:Client`, `faultstring` = `SOAP Envelope tidak valid` |
| TC29 | INVALID | `GetPartnershipApplication` tanpa `applicationId` | Field `applicationId` kosong | HTTP 400, fault `soap:Client`, `faultstring` berisi `applicationId wajib diisi` |

### 2.8 Preflight dependency (TC-P1/P2 — manual)

| Kode | Skenario | Langkah | Expected result |
|---|---|---|---|
| TC-P1 | Semua service mati | `docker compose ... down`, lalu jalankan `run-soap-tests.sh` | Script **exit ≠ 0**; pesan `ERROR: Auth service is unreachable at http://localhost:8080/api/v1/health` + hint menjalankan Docker stack; **tanpa** traceback/error aneh |
| TC-P2 | Hanya SOAP mati | Matikan container `soap-partnerships-service` saja, jalankan script | Script exit ≠ 0; pesan spesifik `ERROR: SOAP service is unreachable at http://localhost:9090/partnership?wsdl` |

### 2.9 Verifikasi database (V01 — dilakukan script bila Docker tersedia)

| Kode | Skenario | Expected result |
|---|---|---|
| V01 | Query `partnership.transaksi_pengajuankerjasama` untuk APP1–APP4 | APP1=`AKTIF`, APP2=`DITOLAK` (+`catatan_keputusan`), APP3=`DIBATALKAN`, APP4=`DIAJUKAN` + `dokumen_perjanjian_id='DOKTEST01'` (sign tidak mengubah status; APP4 tidak di-MarkAsRead) |

---

## 3. Urutan Eksekusi (state machine dijaga agar tidak saling mengganggu)

```
TC00  WSDL tanpa auth
 ↓
TC01  Create APP1 (DIAJUKAN)
TC02  Create invalid ×3 (tidak menghasilkan data)
 ↓
TC03  Get APP1 ── TC04 not-found ── TC05 list outgoing
TC12  list incoming (MITRA) ── TC13/TC14 summary
 ↓
TC15  MarkAsRead APP1 (DIAJUKAN → DITINJAU)
TC06  Approve APP1 (→ AKTIF)  ← perubahan status pertama
 ↓
TC07  Create APP2 → Reject (→ DITOLAK) ── TC16 MarkAsRead salah actor (APP2, sudah DITOLAK → 409)
 ↓
TC11  Create APP3 → Reject tanpa alasan (422, tetap DIAJUKAN)
TC09  Cancel APP3 (→ DIBATALKAN) ── TC08 update status invalid
 ↓
TC10  Create APP4 → approve oleh pengaju (409)
TC17  Sign APP4 oleh pengaju ── TC18 sign salah actor ── TC19 sign tanpa documentId
 ↓
TC20/TC21  filter status valid/invalid
TC22–TC27  auth ── TC28/TC29 body & validasi
 ↓
V01   verifikasi database (psql)
```

> Urutan penting: tes transisi (TC06/TC07/TC09) berurutan agar setiap pengajuan
> berada pada status sumber yang benar; tes "salah actor" memakai pengajuan yang
> statusnya belum berubah (percobaan gagal tidak mengubah data).

---

## 4. Ringkasan Cakupan

| Area | Test case | Jumlah |
|---|---|---|
| WSDL | TC00 | 1 |
| 9 operasi SOAP | TC01, TC03, TC05, TC06, TC07, TC12, TC13, TC14, TC15, TC17 | 10 operasi tercakup |
| Request valid | TC01, TC03, TC05, TC06, TC07, TC12–TC15, TC17, TC20 | 12 |
| Request invalid | TC02a–c, TC08, TC11, TC19, TC21, TC27–TC29 | 11 |
| Data tidak ditemukan | TC04 | 1 |
| Perubahan status | TC06, TC07, TC09, TC10, TC15, TC16, TC18 | 7 |
| Autentikasi | TC22–TC26 | 5 |
| Preflight dependency | TC-P1, TC-P2 (manual) | 2 |
| Verifikasi database | V01 | 1 |
| **Total** | | **39 skenario (30 otomatis + 2 preflight manual + 1 DB + sub-kasus)** |

### Definisi selesai (§37 — bagian yang dicakup test ini)

- [x] SOAP service dapat dijalankan & endpoint dapat diakses (TC00 + preflight)
- [x] WSDL tersedia (TC00)
- [x] Minimal 4 operasi tersedia (9 operasi, semua dicakup — §2.4)
- [x] SOAP request dapat dikirim & response diterima (TC01, TC03, TC05, dll.)
- [x] Data Kemitraan dapat dibuat & dibaca (TC01, TC03, TC05, TC12)
- [x] Status dapat diubah (TC06, TC07, TC09)
- [x] Error dapat ditangani (TC02, TC04, TC08, TC10, TC22–TC29)
- [x] Test case berhasil dijalankan → bukti di `test-result.md`
