package config

import (
	"testing"
	"time"
)

func TestLoadUsesSecureTransportDefaults(t *testing.T) {
	clearTransportEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")

	cfg := Load()

	if cfg.MaxRequestBytes != 1<<20 {
		t.Fatalf("MaxRequestBytes = %d, want %d", cfg.MaxRequestBytes, int64(1<<20))
	}
	assertDuration(t, "RequestTimeout", cfg.RequestTimeout, 10*time.Second)
	assertDuration(t, "DatabaseTimeout", cfg.DatabaseTimeout, 10*time.Second)
	assertDuration(t, "ReadHeaderTimeout", cfg.ReadHeaderTimeout, 5*time.Second)
	assertDuration(t, "ReadTimeout", cfg.ReadTimeout, 15*time.Second)
	assertDuration(t, "WriteTimeout", cfg.WriteTimeout, 30*time.Second)
	assertDuration(t, "IdleTimeout", cfg.IdleTimeout, 60*time.Second)
}

func TestLoadAcceptsPositiveTransportOverrides(t *testing.T) {
	clearTransportEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("JWT_SECRET", "configured-secret")
	t.Setenv("SOAP_MAX_REQUEST_BYTES", "2048")
	t.Setenv("SOAP_REQUEST_TIMEOUT", "3s")
	t.Setenv("SOAP_DATABASE_TIMEOUT", "4s")
	t.Setenv("SOAP_READ_HEADER_TIMEOUT", "2s")
	t.Setenv("SOAP_READ_TIMEOUT", "5s")
	t.Setenv("SOAP_WRITE_TIMEOUT", "8s")
	t.Setenv("SOAP_IDLE_TIMEOUT", "45s")

	cfg := Load()

	if cfg.JWTSecret != "configured-secret" {
		t.Fatalf("JWTSecret = %q, want configured value", cfg.JWTSecret)
	}
	if cfg.MaxRequestBytes != 2048 {
		t.Fatalf("MaxRequestBytes = %d, want 2048", cfg.MaxRequestBytes)
	}
	assertDuration(t, "RequestTimeout", cfg.RequestTimeout, 3*time.Second)
	assertDuration(t, "DatabaseTimeout", cfg.DatabaseTimeout, 4*time.Second)
	assertDuration(t, "ReadHeaderTimeout", cfg.ReadHeaderTimeout, 2*time.Second)
	assertDuration(t, "ReadTimeout", cfg.ReadTimeout, 5*time.Second)
	assertDuration(t, "WriteTimeout", cfg.WriteTimeout, 8*time.Second)
	assertDuration(t, "IdleTimeout", cfg.IdleTimeout, 45*time.Second)
}

func TestLoadRejectsUnsafeTransportOverrides(t *testing.T) {
	clearTransportEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("SOAP_MAX_REQUEST_BYTES", "-1")
	t.Setenv("SOAP_REQUEST_TIMEOUT", "0")
	t.Setenv("SOAP_DATABASE_TIMEOUT", "invalid")
	t.Setenv("SOAP_READ_HEADER_TIMEOUT", "-2s")
	t.Setenv("SOAP_READ_TIMEOUT", "0s")
	t.Setenv("SOAP_WRITE_TIMEOUT", "invalid")
	t.Setenv("SOAP_IDLE_TIMEOUT", "-1s")

	cfg := Load()

	if cfg.MaxRequestBytes != 1<<20 {
		t.Fatalf("MaxRequestBytes = %d, want safe default", cfg.MaxRequestBytes)
	}
	assertDuration(t, "RequestTimeout", cfg.RequestTimeout, 10*time.Second)
	assertDuration(t, "DatabaseTimeout", cfg.DatabaseTimeout, 10*time.Second)
	assertDuration(t, "ReadHeaderTimeout", cfg.ReadHeaderTimeout, 5*time.Second)
	assertDuration(t, "ReadTimeout", cfg.ReadTimeout, 15*time.Second)
	assertDuration(t, "WriteTimeout", cfg.WriteTimeout, 30*time.Second)
	assertDuration(t, "IdleTimeout", cfg.IdleTimeout, 60*time.Second)
}

func clearTransportEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"JWT_SECRET",
		"SOAP_MAX_REQUEST_BYTES",
		"SOAP_REQUEST_TIMEOUT",
		"SOAP_DATABASE_TIMEOUT",
		"SOAP_READ_HEADER_TIMEOUT",
		"SOAP_READ_TIMEOUT",
		"SOAP_WRITE_TIMEOUT",
		"SOAP_IDLE_TIMEOUT",
	} {
		t.Setenv(key, "")
	}
}

func assertDuration(t *testing.T, name string, got, want time.Duration) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %s, want %s", name, got, want)
	}
}
