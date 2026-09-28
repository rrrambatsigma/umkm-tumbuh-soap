# Checklist Screenshot — Demo & Bukti Test SOAP

> Anggota 5: Testing, Dokumentasi & Demo.
> Skenario: [demo.md](demo.md) · Hasil: [test-result.md](test-result.md)

Screenshot diambil **manual** oleh pelaku saat/ setelah menjalankan skenario demo.
Simpan di folder ini (`tests/soap/screenshots/`) memakai nama file pada kolom
**File**. Format PNG, resolusi minimal 1280×720, termasuk baris terminal/URL yang
membuktikan konteks.

## A. Test otomatis (wajib)

| ID | Bukti | Aksi | File |
|---|---|---|---|
| SS01 | Preflight 4 service OK | Bagian awal run script | `01-preflight-ok.png` |
| SS02 | Provisioning akun + metadata dokumen | Bagian `Persiapan akun uji` | `02-provisioning.png` |
| SS03 | Sesi test berjalan (TC00–TC07) | Tengah run | `03-run-crud-status.png` |
| SS04 | Transisi status approve/reject/cancel | Bagian `TC15/TC06`, `TC07`, `TC09` | `04-transisi-status.png` |
| SS05 | Autentikasi TC22–TC27 | Bagian `TC22–TC27` | `05-autentikasi.png` |
| SS06 | Ringkasan akhir `41 PASS, 0 FAIL` + exit 0 | Akhir run + `echo $?` | `06-ringkasan-41-pass.png` |
| SS07 | Isi `results.tsv` | `column -t -s$'\t' tests/soap/reports/results.tsv \| less` | `07-results-tsv.png` |
| SS08 | Verifikasi DB V01 PASS | Bagian `V01` | `08-v01-database.png` |

## B. Inspeksi manual (minimal 4)

| ID | Bukti | Aksi | File |
|---|---|---|---|
| SS09 | WSDL publik di browser | Buka `http://localhost:9090/partnership?wsdl` | `09-wsdl-browser.png` |
| SS10 | Operasi & tipe di WSDL | Gulir ke `CreatePartnershipApplication` / `UpdatePartnershipStatus` | `10-wSDL-operasi.png` |
| SS11 | Request sukses + response | curl/SOAP UI: Create → `<status>DIAJUKAN</status>` | `11-create-sukses.png` |
| SS12 | SOAP Fault 401 (tanpa token) | Hapus header `Authorization` | `12-fault-401.png` |
| SS13 | SOAP Fault 403 (token ADMIN) | Kirim dengan token admin | `13-fault-403.png` |
| SS14 | SOAP Fault 404 / 422 / 400 | Salah satu dari TC04/TC11/TC28 | `14-fault-lain.png` |

## C. Jalur error preflight (wajib — Requirement B)

| ID | Bukti | Aksi | File |
|---|---|---|---|
| SS15 | `ERROR: SOAP service is unreachable at http://localhost:9090/partnership?wsdl` + hint + exit 2 | `docker stop infra-soap-partnerships-service-1` lalu jalankan script | `15-preflight-soap-mati.png` |
| SS16 | `ERROR: Auth service is unreachable at http://localhost:8080/api/v1/health` + exit 2 | `docker compose ... stop` (semua mati) lalu jalankan script | `16-preflight-semua-mati.png` |

## D. Opsional

| ID | Bukti | Aksi | File |
|---|---|---|---|
| SS17 | Container stack healthy | `docker compose --env-file .env -f infra/docker-compose.yml ps` | `17-docker-ps.png` |
| SS18 | Isi `reports/` (XML req/res) | `ls -la tests/soap/reports/` | `18-reports-xml.png` |
| SS19 | Log SOAP service (operasi + event) | `docker compose ... logs --tail 40 soap-partnerships-service` | `19-soap-logs.png` |
| SS20 | Verifikasi psql manual (Skenario 4) | Query `transaksi_pengajuankerjasama` | `20-psql-verifikasi.png` |

## Status pengisian

| Kelompok | Jumlah | Terisi | Keterangan |
|---|---|---|---|
| A — Test otomatis | 8 | | |
| B — Inspeksi manual | 6 (min. 4) | | |
| C — Preflight error | 2 | | |
| D — Opsional | 4 | | |

- [ ] Semua screenshot wajib (A + C) dan minimal 4 dari B sudah diisi di `screenshots/`
- [ ] Nama file sesuai tabel agar mudah dipasang ke laporan presentasi
