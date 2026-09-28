//go:build integration

package main

// Integration test untuk SOAP Partnerships Service — Anggota 4 (Database & Integration)
//
// Test ini memverifikasi koneksi end-to-end:
//   database → Repository → Service → (response tidak error fatal)
//
// Cara menjalankan (memerlukan database yang sudah ter-migrate dan ter-seed):
//
//   DATABASE_URL="postgres://umkm_user:umkm_password@localhost:5432/umkm_tumbuh?sslmode=disable" \
//   go test -tags integration ./cmd/... -v
//
// Semua test di file ini di-skip secara otomatis jika DATABASE_URL tidak tersedia,
// sehingga CI tanpa database tetap aman.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/config"
	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/database"
	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/partnerships"
)

// requireDatabaseURL mengembalikan DATABASE_URL dari environment atau skip test.
func requireDatabaseURL(t *testing.T) string {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL tidak tersedia — skip integration test")
	}
	return dbURL
}

// setupIntegration membuat koneksi database dan mengembalikan Service yang siap dipakai.
func setupIntegration(t *testing.T) *partnerships.Service {
	t.Helper()
	dbURL := requireDatabaseURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Koneksi database gagal: %v\nPastikan database berjalan dan DATABASE_URL benar.", err)
	}
	t.Cleanup(func() { pool.Close() })

	repo := partnerships.NewRepository(pool)
	return partnerships.NewService(repo)
}

// ---------------------------------------------------------------
// TC-INT-01: Koneksi database berhasil
// ---------------------------------------------------------------

func TestIntegration_DatabaseConnects(t *testing.T) {
	dbURL := requireDatabaseURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Connect gagal: %v", err)
	}
	defer pool.Close()

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := pool.Ping(pingCtx); err != nil {
		t.Fatalf("Ping gagal setelah Connect: %v", err)
	}
}

// ---------------------------------------------------------------
// TC-INT-02: Config load berhasil dari environment
// ---------------------------------------------------------------

func TestIntegration_ConfigLoadsFromEnv(t *testing.T) {
	requireDatabaseURL(t)

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		t.Fatal("Config.DatabaseURL kosong — seharusnya terisi dari DATABASE_URL")
	}
	if cfg.Port == "" {
		t.Fatal("Config.Port kosong — seharusnya ada default")
	}
}

// ---------------------------------------------------------------
// TC-INT-03: Repository — CountAll tidak error jika tabel ada
// ---------------------------------------------------------------

func TestIntegration_Repository_CountAll(t *testing.T) {
	dbURL := requireDatabaseURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Koneksi gagal: %v", err)
	}
	defer pool.Close()

	repo := partnerships.NewRepository(pool)

	count, err := repo.CountAll(ctx)
	if err != nil {
		t.Fatalf("CountAll gagal: %v\nPastikan tabel partnership.transaksi_pengajuankerjasama sudah ada (jalankan migrasi).", err)
	}
	t.Logf("Total pengajuan di database: %d", count)
}

// ---------------------------------------------------------------
// TC-INT-04: Repository — CountByYear tidak error
// ---------------------------------------------------------------

func TestIntegration_Repository_CountByYear(t *testing.T) {
	dbURL := requireDatabaseURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Koneksi gagal: %v", err)
	}
	defer pool.Close()

	repo := partnerships.NewRepository(pool)
	year := time.Now().Format("2006")

	count, err := repo.CountByYear(ctx, year)
	if err != nil {
		t.Fatalf("CountByYear gagal: %v", err)
	}
	t.Logf("Total pengajuan tahun %s: %d", year, count)
}

// ---------------------------------------------------------------
// TC-INT-05: Service — GetPartnershipApplications tidak error untuk user yang ada
//
// Menggunakan TEST_UMKM_A dari fixtures.sql (tests/stack/fixtures.sql).
// Jika fixtures belum di-load, test tetap pass dengan list kosong.
// ---------------------------------------------------------------

func TestIntegration_Service_GetPartnershipApplications_KnownUser(t *testing.T) {
	svc := setupIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := svc.GetPartnershipApplications(ctx, &partnerships.GetPartnershipApplicationsRequest{
		UserID:   "TEST_UMKM_A",
		UserRole: "UMKM",
		Page:     1,
		Limit:    20,
	})
	if err != nil {
		t.Fatalf("GetPartnershipApplications gagal: %v", err)
	}
	t.Logf("Pengajuan keluar TEST_UMKM_A: %d total, %d dalam halaman ini",
		result.TotalCount, len(result.Applications))
}

// ---------------------------------------------------------------
// TC-INT-06: Service — GetIncomingPartnerships tidak error untuk user yang ada
// ---------------------------------------------------------------

func TestIntegration_Service_GetIncomingPartnerships_KnownUser(t *testing.T) {
	svc := setupIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := svc.GetIncomingPartnerships(ctx, &partnerships.GetIncomingPartnershipsRequest{
		UserID:   "TEST_MITRA_A",
		UserRole: "MITRA",
		Page:     1,
		Limit:    20,
	})
	if err != nil {
		t.Fatalf("GetIncomingPartnerships gagal: %v", err)
	}
	t.Logf("Pengajuan masuk TEST_MITRA_A: %d total, %d dalam halaman ini",
		result.TotalCount, len(result.Applications))
}

// ---------------------------------------------------------------
// TC-INT-07: Service — GetPartnershipSummary tidak error
// ---------------------------------------------------------------

func TestIntegration_Service_GetPartnershipSummary(t *testing.T) {
	svc := setupIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	summary, err := svc.GetPartnershipSummary(ctx, &partnerships.GetPartnershipSummaryRequest{
		UserID:   "TEST_UMKM_A",
		UserRole: "UMKM",
	})
	if err != nil {
		t.Fatalf("GetPartnershipSummary gagal: %v", err)
	}
	t.Logf("Summary TEST_UMKM_A: %v", summary)
}

// ---------------------------------------------------------------
// TC-INT-08: Service — GetIncomingPartnershipSummary tidak error
// ---------------------------------------------------------------

func TestIntegration_Service_GetIncomingPartnershipSummary(t *testing.T) {
	svc := setupIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	summary, err := svc.GetIncomingPartnershipSummary(ctx, &partnerships.GetIncomingPartnershipSummaryRequest{
		UserID:   "TEST_MITRA_A",
		UserRole: "MITRA",
	})
	if err != nil {
		t.Fatalf("GetIncomingPartnershipSummary gagal: %v", err)
	}
	t.Logf("Incoming summary TEST_MITRA_A: %v", summary)
}

// ---------------------------------------------------------------
// TC-INT-09: Service — GetPartnershipApplication untuk ID yang tidak ada → 404
// ---------------------------------------------------------------

func TestIntegration_Service_GetPartnershipApplication_NotFound(t *testing.T) {
	svc := setupIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := svc.GetPartnershipApplication(ctx, &partnerships.GetPartnershipApplicationRequest{
		UserID:        "TEST_UMKM_A",
		UserRole:      "UMKM",
		ApplicationID: "PGJ-TIDAK-ADA-SAMA-SEKALI",
	})
	if err == nil {
		t.Fatal("GetPartnershipApplication untuk ID tidak ada seharusnya mengembalikan error")
	}

	var appErr *partnerships.AppError
	isAppErr := false
	// Cek apakah error adalah AppError dengan cara manual (errors.As tidak tersedia langsung)
	if ae, ok := err.(*partnerships.AppError); ok {
		appErr = ae
		isAppErr = true
	}

	if !isAppErr {
		t.Fatalf("error harus *AppError, dapat: %T: %v", err, err)
	}
	if appErr.Code != 404 {
		t.Fatalf("AppError.Code = %d, ingin 404", appErr.Code)
	}
	t.Logf("404 error sebagaimana diharapkan: %v", err)
}

// ---------------------------------------------------------------
// TC-INT-10: Service — FindReceiverAkunID untuk penerima yang tidak ada → error 400
// ---------------------------------------------------------------

func TestIntegration_Repository_FindReceiverAkunID_NotFound(t *testing.T) {
	dbURL := requireDatabaseURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Koneksi gagal: %v", err)
	}
	defer pool.Close()

	repo := partnerships.NewRepository(pool)

	_, err = repo.FindReceiverAkunID(ctx, "MITRA-TIDAK-ADA-SAMA-SEKALI", "MITRA")
	if err == nil {
		t.Fatal("FindReceiverAkunID untuk ID tidak ada seharusnya mengembalikan error")
	}

	var appErr *partnerships.AppError
	if ae, ok := err.(*partnerships.AppError); ok {
		appErr = ae
	}
	if appErr == nil || appErr.Code != 400 {
		t.Fatalf("ingin 400 AppError, dapat: %T: %v", err, err)
	}
	t.Logf("400 error sebagaimana diharapkan: %v", err)
}
