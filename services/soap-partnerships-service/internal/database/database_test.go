package database

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestConnectRejectsEmptyURL memastikan Connect gagal dengan cepat jika URL kosong.
func TestConnectRejectsEmptyURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := Connect(ctx, "")
	if err == nil {
		pool.Close()
		t.Fatal("Connect dengan URL kosong seharusnya mengembalikan error")
	}
}

// TestConnectRejectsInvalidURL memastikan Connect gagal jika URL tidak valid
// (host tidak ada / tidak bisa dijangkau).
func TestConnectRejectsInvalidURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := Connect(ctx, "postgres://invalid_user:invalid_pass@127.0.0.1:1/nonexistent_db?sslmode=disable&connect_timeout=1")
	if err == nil {
		pool.Close()
		t.Fatal("Connect ke host yang tidak tersedia seharusnya mengembalikan error")
	}
}

// TestConnectRejectsMalformedURL memastikan Connect gagal jika format URL salah.
func TestConnectRejectsMalformedURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := Connect(ctx, "bukan-url-database-sama-sekali")
	if err == nil {
		pool.Close()
		t.Fatal("Connect dengan URL malformed seharusnya mengembalikan error")
	}
}

// TestConnectSucceedsWithRealDatabase menguji koneksi nyata ke database.
// Test ini di-skip secara otomatis jika DATABASE_URL tidak tersedia di environment,
// sehingga aman dijalankan di lingkungan CI yang tidak memiliki database.
func TestConnectSucceedsWithRealDatabase(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL tidak tersedia, skip integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Connect gagal dengan DATABASE_URL yang valid: %v", err)
	}
	defer pool.Close()

	// Pastikan pool benar-benar dapat digunakan dengan ping sekali lagi.
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := pool.Ping(pingCtx); err != nil {
		t.Fatalf("Ping setelah Connect gagal: %v", err)
	}
}

// TestConnectReturnsWorkingPool memastikan pool yang dikembalikan dapat langsung digunakan
// (tidak nil) ketika koneksi berhasil.
// Sama seperti TestConnectSucceedsWithRealDatabase, di-skip jika tidak ada database.
func TestConnectReturnsWorkingPool(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL tidak tersedia, skip integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("Connect gagal: %v", err)
	}
	defer pool.Close()

	if pool == nil {
		t.Fatal("Connect mengembalikan pool nil tanpa error")
	}

	// Jalankan query trivial untuk membuktikan pool benar-benar berfungsi.
	var result int
	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()
	if err := pool.QueryRow(queryCtx, "SELECT 1").Scan(&result); err != nil {
		t.Fatalf("Query sederhana gagal: %v", err)
	}
	if result != 1 {
		t.Fatalf("Query SELECT 1 mengembalikan %d, ingin 1", result)
	}
}
