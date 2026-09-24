package partnerships

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// =============================================================
// Mock infrastructure
// =============================================================

// mockQuerier mengimplementasikan dbQuerier untuk keperluan test.
// Setiap method dapat dikustomisasi via field func.
type mockQuerier struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return &emptyRows{}, nil
}

func (m *mockQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &errRow{err: pgx.ErrNoRows}
}

// -------------------------------------------------------------------
// mockRow — pgx.Row yang mengembalikan nilai atau error yang ditentukan
// -------------------------------------------------------------------

type mockRow struct {
	values []any
	err    error
}

func (r *mockRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("mockRow: Scan expects %d destinations, got %d", len(r.values), len(dest))
	}
	for i, d := range dest {
		if err := assignValue(d, r.values[i]); err != nil {
			return fmt.Errorf("mockRow: field %d: %w", i, err)
		}
	}
	return nil
}

// errRow — pgx.Row yang selalu mengembalikan error tertentu saat Scan.
type errRow struct{ err error }

func (r *errRow) Scan(...any) error { return r.err }

// assignValue mengisi nilai mock ke pointer tujuan Scan.
// Mendukung tipe yang digunakan oleh repository (string, *string, int, *time.Time, time.Time, PartnershipStatus).
func assignValue(dest, src any) error {
	if src == nil {
		// Untuk pointer types, nil berarti tidak ada nilai.
		switch d := dest.(type) {
		case **string:
			*d = nil
		case **time.Time:
			*d = nil
		}
		return nil
	}
	switch d := dest.(type) {
	case *string:
		v, ok := src.(string)
		if !ok {
			return fmt.Errorf("cannot assign %T to *string", src)
		}
		*d = v
	case **string:
		v, ok := src.(string)
		if !ok {
			return fmt.Errorf("cannot assign %T to **string", src)
		}
		*d = &v
	case *int:
		switch v := src.(type) {
		case int:
			*d = v
		case int64:
			*d = int(v)
		default:
			return fmt.Errorf("cannot assign %T to *int", src)
		}
	case *PartnershipStatus:
		v, ok := src.(string)
		if !ok {
			return fmt.Errorf("cannot assign %T to *PartnershipStatus", src)
		}
		*d = PartnershipStatus(v)
	case *time.Time:
		v, ok := src.(time.Time)
		if !ok {
			return fmt.Errorf("cannot assign %T to *time.Time", src)
		}
		*d = v
	case **time.Time:
		v, ok := src.(time.Time)
		if !ok {
			return fmt.Errorf("cannot assign %T to **time.Time", src)
		}
		*d = &v
	default:
		return fmt.Errorf("unsupported destination type %T", dest)
	}
	return nil
}

// -------------------------------------------------------------------
// mockRows — pgx.Rows yang mengiterasi slice of []any
// -------------------------------------------------------------------

type mockRows struct {
	data    [][]any
	current int
	err     error
}

func newMockRows(data [][]any) *mockRows { return &mockRows{data: data, current: -1} }

func (r *mockRows) Next() bool {
	r.current++
	return r.current < len(r.data)
}

func (r *mockRows) Scan(dest ...any) error {
	if r.current < 0 || r.current >= len(r.data) {
		return errors.New("mockRows: Scan called out of range")
	}
	row := r.data[r.current]
	if len(dest) != len(row) {
		return fmt.Errorf("mockRows: Scan expects %d dests, got %d", len(row), len(dest))
	}
	for i, d := range dest {
		if err := assignValue(d, row[i]); err != nil {
			return fmt.Errorf("mockRows: field %d: %w", i, err)
		}
	}
	return nil
}

func (r *mockRows) Err() error                        { return r.err }
func (r *mockRows) Close()                            {}
func (r *mockRows) CommandTag() pgconn.CommandTag        { return pgconn.CommandTag{} }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockRows) Values() ([]any, error)            { return nil, nil }
func (r *mockRows) RawValues() [][]byte               { return nil }
func (r *mockRows) Conn() *pgx.Conn                   { return nil }
func (r *mockRows) TypeMap() *pgtype.Map               { return nil }

// emptyRows — pgx.Rows kosong (0 baris).
type emptyRows struct{}

func (r *emptyRows) Next() bool                                   { return false }
func (r *emptyRows) Scan(...any) error                            { return nil }
func (r *emptyRows) Err() error                                   { return nil }
func (r *emptyRows) Close()                                       {}
func (r *emptyRows) CommandTag() pgconn.CommandTag                   { return pgconn.CommandTag{} }
func (r *emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *emptyRows) Values() ([]any, error)                       { return nil, nil }
func (r *emptyRows) RawValues() [][]byte                          { return nil }
func (r *emptyRows) Conn() *pgx.Conn                              { return nil }
func (r *emptyRows) TypeMap() *pgtype.Map                          { return nil }

// commandTag membuat pgconn.CommandTag dengan RowsAffected tertentu.
func commandTag(rowsAffected int64) pgconn.CommandTag {
	return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", rowsAffected))
}

// =============================================================
// Helper: waktu dan pointer
// =============================================================

var testTime = time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)

func strPtr(s string) *string    { return &s }
func timePtr(t time.Time) *time.Time { return &t }

// =============================================================
// Tests: CountAll
// =============================================================

func TestCountAll_ReturnsTotalRows(t *testing.T) {
	mock := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{values: []any{42}}
		},
	}
	repo := newRepositoryWithQuerier(mock)

	count, err := repo.CountAll(context.Background())
	if err != nil {
		t.Fatalf("CountAll mengembalikan error tidak terduga: %v", err)
	}
	if count != 42 {
		t.Fatalf("CountAll = %d, ingin 42", count)
	}
}

func TestCountAll_PropagatesDBError(t *testing.T) {
	dbErr := errors.New("koneksi terputus")
	mock := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &errRow{err: dbErr}
		},
	}
	repo := newRepositoryWithQuerier(mock)

	_, err := repo.CountAll(context.Background())
	if err == nil {
		t.Fatal("CountAll seharusnya mengembalikan error dari database")
	}
}

// =============================================================
// Tests: CountByYear
// =============================================================

func TestCountByYear_ReturnsYearCount(t *testing.T) {
	mock := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{values: []any{7}}
		},
	}
	repo := newRepositoryWithQuerier(mock)

	count, err := repo.CountByYear(context.Background(), "2026")
	if err != nil {
		t.Fatalf("CountByYear mengembalikan error: %v", err)
	}
	if count != 7 {
		t.Fatalf("CountByYear = %d, ingin 7", count)
	}
}

// =============================================================
// Tests: FindReceiverAkunID
// =============================================================

func TestFindReceiverAkunID_MitraFound(t *testing.T) {
	mock := &mockQuerier{
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			// Pastikan query menggunakan tabel master_mitra
			if len(args) == 0 || args[0] != "MITRA-001" {
				return &errRow{err: errors.New("unexpected args")}
			}
			return &mockRow{values: []any{"AKUN-MITRA-001"}}
		},
	}
	repo := newRepositoryWithQuerier(mock)

	akunID, err := repo.FindReceiverAkunID(context.Background(), "MITRA-001", "MITRA")
	if err != nil {
		t.Fatalf("FindReceiverAkunID MITRA mengembalikan error: %v", err)
	}
	if akunID != "AKUN-MITRA-001" {
		t.Fatalf("akunID = %q, ingin AKUN-MITRA-001", akunID)
	}
}

func TestFindReceiverAkunID_UmkmFound(t *testing.T) {
	mock := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			if len(args) == 0 || args[0] != "UMKM-001" {
				return &errRow{err: errors.New("unexpected args")}
			}
			return &mockRow{values: []any{"AKUN-UMKM-001"}}
		},
	}
	repo := newRepositoryWithQuerier(mock)

	akunID, err := repo.FindReceiverAkunID(context.Background(), "UMKM-001", "UMKM")
	if err != nil {
		t.Fatalf("FindReceiverAkunID UMKM mengembalikan error: %v", err)
	}
	if akunID != "AKUN-UMKM-001" {
		t.Fatalf("akunID = %q, ingin AKUN-UMKM-001", akunID)
	}
}

func TestFindReceiverAkunID_NotFound(t *testing.T) {
	mock := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &errRow{err: pgx.ErrNoRows}
		},
	}
	repo := newRepositoryWithQuerier(mock)

	_, err := repo.FindReceiverAkunID(context.Background(), "MITRA-TIDAK-ADA", "MITRA")
	if err == nil {
		t.Fatal("FindReceiverAkunID seharusnya mengembalikan error jika penerima tidak ditemukan")
	}
	var appErr *AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error harus bertipe *AppError, dapat %T: %v", err, err)
	}
	if appErr.Code != http.StatusBadRequest {
		t.Fatalf("AppError.Code = %d, ingin %d", appErr.Code, http.StatusBadRequest)
	}
}

// =============================================================
// Tests: Create
// =============================================================

func TestCreate_Succeeds(t *testing.T) {
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return commandTag(1), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.Create(context.Background(), CreateInput{
		ID:            "PGJ000001",
		RequestCode:   "PKS-2026-000001",
		RequesterID:   "AKUN-UMKM-001",
		ReceiverID:    "AKUN-MITRA-001",
		RequesterRole: "UMKM",
		ProposalText:  "Judul\n\nDeskripsi proposal kemitraan.",
		SubmittedAt:   testTime,
	})
	if err != nil {
		t.Fatalf("Create mengembalikan error tidak terduga: %v", err)
	}
}

func TestCreate_PropagatesDBError(t *testing.T) {
	dbErr := errors.New("duplicate key violation")
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, dbErr
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.Create(context.Background(), CreateInput{
		ID:            "PGJ000001",
		RequestCode:   "PKS-2026-000001",
		RequesterID:   "AKUN-UMKM-001",
		ReceiverID:    "AKUN-MITRA-001",
		RequesterRole: "UMKM",
		ProposalText:  "Proposal",
		SubmittedAt:   testTime,
	})
	if err == nil {
		t.Fatal("Create seharusnya meneruskan error database")
	}
	if !errors.Is(err, dbErr) {
		t.Fatalf("error harus mewrap database error asli, dapat: %v", err)
	}
}

func TestCreate_SetsUmkmAndMitraIDsCorrectly_UmkmRequester(t *testing.T) {
	var capturedArgs []any
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
			capturedArgs = args
			return commandTag(1), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.Create(context.Background(), CreateInput{
		ID:                  "PGJ000002",
		RequestCode:         "PKS-2026-000002",
		RequesterID:         "AKUN-UMKM-001",
		ReceiverID:          "AKUN-MITRA-001",
		RequesterRole:       "UMKM",
		RequesterBusinessID: "UMKM-BIZ-001",
		ReceiverBusinessID:  "MITRA-BIZ-001",
		ProposalText:        "Proposal",
		SubmittedAt:         testTime,
	})
	if err != nil {
		t.Fatalf("Create mengembalikan error: %v", err)
	}
	// args urutan: ID, RequestCode, umkmID, mitraID, RequesterID, ReceiverID, status, proposal, date
	// Untuk requester UMKM: arg[2]=umkmID=RequesterBusinessID, arg[3]=mitraID=ReceiverBusinessID
	if len(capturedArgs) < 4 {
		t.Fatalf("tidak cukup argumen SQL: %v", capturedArgs)
	}
	gotUmkmID, _ := capturedArgs[2].(*string)
	gotMitraID, _ := capturedArgs[3].(*string)
	if gotUmkmID == nil || *gotUmkmID != "UMKM-BIZ-001" {
		t.Errorf("umkm_id = %v, ingin UMKM-BIZ-001", gotUmkmID)
	}
	if gotMitraID == nil || *gotMitraID != "MITRA-BIZ-001" {
		t.Errorf("mitra_id = %v, ingin MITRA-BIZ-001", gotMitraID)
	}
}

func TestCreate_SetsUmkmAndMitraIDsCorrectly_MitraRequester(t *testing.T) {
	var capturedArgs []any
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
			capturedArgs = args
			return commandTag(1), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.Create(context.Background(), CreateInput{
		ID:                  "PGJ000003",
		RequestCode:         "PKS-2026-000003",
		RequesterID:         "AKUN-MITRA-001",
		ReceiverID:          "AKUN-UMKM-001",
		RequesterRole:       "MITRA",
		RequesterBusinessID: "MITRA-BIZ-001",
		ReceiverBusinessID:  "UMKM-BIZ-001",
		ProposalText:        "Proposal dari Mitra",
		SubmittedAt:         testTime,
	})
	if err != nil {
		t.Fatalf("Create mengembalikan error: %v", err)
	}
	// Untuk requester MITRA: arg[2]=umkmID=ReceiverBusinessID, arg[3]=mitraID=RequesterBusinessID
	if len(capturedArgs) < 4 {
		t.Fatalf("tidak cukup argumen SQL: %v", capturedArgs)
	}
	gotUmkmID, _ := capturedArgs[2].(*string)
	gotMitraID, _ := capturedArgs[3].(*string)
	if gotUmkmID == nil || *gotUmkmID != "UMKM-BIZ-001" {
		t.Errorf("umkm_id = %v, ingin UMKM-BIZ-001", gotUmkmID)
	}
	if gotMitraID == nil || *gotMitraID != "MITRA-BIZ-001" {
		t.Errorf("mitra_id = %v, ingin MITRA-BIZ-001", gotMitraID)
	}
}

func TestCreate_AllowsNullBusinessIDs(t *testing.T) {
	var capturedArgs []any
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
			capturedArgs = args
			return commandTag(1), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	// RequesterBusinessID dan ReceiverBusinessID kosong → umkm_id dan mitra_id harus nil
	err := repo.Create(context.Background(), CreateInput{
		ID:            "PGJ000004",
		RequestCode:   "PKS-2026-000004",
		RequesterID:   "AKUN-UMKM-001",
		ReceiverID:    "AKUN-MITRA-001",
		RequesterRole: "UMKM",
		ProposalText:  "Proposal tanpa profil bisnis",
		SubmittedAt:   testTime,
	})
	if err != nil {
		t.Fatalf("Create mengembalikan error: %v", err)
	}
	if len(capturedArgs) < 4 {
		t.Fatalf("tidak cukup argumen SQL: %v", capturedArgs)
	}
	// umkmID dan mitraID harus (*string)(nil) — pointer nil, bukan nilai non-nil
	// Repository.Create menggunakan var umkmID, mitraID *string (nil by default),
	// dan hanya mengisinya jika business ID tidak kosong.
	// Saat nil pointer dikirim ke pgxpool.Exec sebagai any, nilainya tetap nil.
	// Kita verifikasi bahwa jika di-type-assert ke *string, hasilnya pointer nil.
	if ptr, ok := capturedArgs[2].(*string); ok && ptr != nil {
		t.Errorf("umkm_id harus nil pointer, dapat: %v", *ptr)
	} else if !ok && capturedArgs[2] != nil {
		t.Errorf("umkm_id harus nil atau *string nil, dapat tipe %T: %v", capturedArgs[2], capturedArgs[2])
	}
	if ptr, ok := capturedArgs[3].(*string); ok && ptr != nil {
		t.Errorf("mitra_id harus nil pointer, dapat: %v", *ptr)
	} else if !ok && capturedArgs[3] != nil {
		t.Errorf("mitra_id harus nil atau *string nil, dapat tipe %T: %v", capturedArgs[3], capturedArgs[3])
	}
}

// =============================================================
// Tests: FindByID
// =============================================================

func TestFindByID_Found(t *testing.T) {
	mock := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			// args[0] = pengajuan_id, args[1] = actorID
			if args[0] != "PGJ000001" || args[1] != "AKUN-UMKM-001" {
				return &errRow{err: pgx.ErrNoRows}
			}
			return &mockRow{values: []any{
				"PGJ000001",          // pengajuan_id
				"PKS-2026-000001",    // kode_pengajuan
				"AKUN-UMKM-001",      // pengaju_akun_id
				"AKUN-MITRA-001",     // penerima_akun_id
				string(StatusSubmitted), // status_pengajuan_id
				"Judul\n\nDeskripsi", // pesan_pengajuan
				nil,                  // catatan_keputusan (*string)
				nil,                  // dokumen_perjanjian_id (*string)
				"Test UMKM A",        // requester_name
				"Test Mitra A",       // receiver_name
				testTime,             // tanggal_pengajuan (*time.Time)
				nil,                  // tanggal_keputusan (*time.Time)
				testTime,             // created_at
				testTime,             // updated_at
			}}
		},
	}
	repo := newRepositoryWithQuerier(mock)

	row, err := repo.FindByID(context.Background(), "PGJ000001", "AKUN-UMKM-001")
	if err != nil {
		t.Fatalf("FindByID mengembalikan error: %v", err)
	}
	if row.ID != "PGJ000001" {
		t.Errorf("row.ID = %q, ingin PGJ000001", row.ID)
	}
	if row.Status != StatusSubmitted {
		t.Errorf("row.Status = %q, ingin DIAJUKAN", row.Status)
	}
	if row.RequesterName != "Test UMKM A" {
		t.Errorf("row.RequesterName = %q, ingin Test UMKM A", row.RequesterName)
	}
}

func TestFindByID_NotFound_ReturnsAppError404(t *testing.T) {
	mock := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &errRow{err: pgx.ErrNoRows}
		},
	}
	repo := newRepositoryWithQuerier(mock)

	_, err := repo.FindByID(context.Background(), "PGJ-TIDAK-ADA", "AKUN-001")
	if err == nil {
		t.Fatal("FindByID seharusnya mengembalikan error untuk ID yang tidak ada")
	}
	var appErr *AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error harus *AppError, dapat %T", err)
	}
	if appErr.Code != http.StatusNotFound {
		t.Fatalf("AppError.Code = %d, ingin 404", appErr.Code)
	}
}

func TestFindByID_DBError_PropagatedAsWrapped(t *testing.T) {
	dbErr := errors.New("connection reset")
	mock := &mockQuerier{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &errRow{err: dbErr}
		},
	}
	repo := newRepositoryWithQuerier(mock)

	_, err := repo.FindByID(context.Background(), "PGJ000001", "AKUN-001")
	if err == nil {
		t.Fatal("FindByID seharusnya meneruskan error database")
	}
	// Harus wrap, bukan AppError
	var appErr *AppError
	if errors.As(err, &appErr) {
		t.Fatalf("error DB biasa tidak seharusnya menjadi AppError, dapat: %v", err)
	}
}

// =============================================================
// Tests: FindByRequesterID
// =============================================================

func TestFindByRequesterID_ReturnsRows(t *testing.T) {
	rows := [][]any{
		{
			"PGJ000001", "PKS-2026-000001",
			"Test UMKM A", "Test Mitra A",
			"Usaha A", "Mitra A",
			"Judul proposal", string(StatusSubmitted),
			testTime, nil,
			5, // total_count
		},
	}
	mock := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return newMockRows(rows), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	list, total, err := repo.FindByRequesterID(context.Background(), "AKUN-UMKM-001", nil, 20, 0)
	if err != nil {
		t.Fatalf("FindByRequesterID mengembalikan error: %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, ingin 5", total)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, ingin 1", len(list))
	}
	if list[0].ID != "PGJ000001" {
		t.Errorf("list[0].ID = %q, ingin PGJ000001", list[0].ID)
	}
}

func TestFindByRequesterID_EmptyResult(t *testing.T) {
	mock := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &emptyRows{}, nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	list, total, err := repo.FindByRequesterID(context.Background(), "AKUN-UMKM-BARU", nil, 20, 0)
	if err != nil {
		t.Fatalf("FindByRequesterID mengembalikan error: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, ingin 0", total)
	}
	if len(list) != 0 {
		t.Errorf("len(list) = %d, ingin 0", len(list))
	}
}

func TestFindByRequesterID_PropagatesDBError(t *testing.T) {
	dbErr := errors.New("timeout")
	mock := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, dbErr
		},
	}
	repo := newRepositoryWithQuerier(mock)

	_, _, err := repo.FindByRequesterID(context.Background(), "AKUN-001", nil, 20, 0)
	if err == nil {
		t.Fatal("FindByRequesterID seharusnya meneruskan error database")
	}
}

func TestFindByRequesterID_WithStatusFilter(t *testing.T) {
	var capturedSQL string
	mock := &mockQuerier{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			capturedSQL = sql
			return &emptyRows{}, nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	status := StatusSubmitted
	_, _, err := repo.FindByRequesterID(context.Background(), "AKUN-001", &status, 20, 0)
	if err != nil {
		t.Fatalf("FindByRequesterID dengan status filter error: %v", err)
	}
	// Query harus mengandung filter status
	if capturedSQL == "" {
		t.Fatal("SQL tidak terekam")
	}
}

// =============================================================
// Tests: FindByReceiverID
// =============================================================

func TestFindByReceiverID_ReturnsRows(t *testing.T) {
	rows := [][]any{
		{
			"PGJ000002", "PKS-2026-000002",
			"Test UMKM B", "Test Mitra A",
			"Usaha B", "Mitra A",
			"Judul incoming", string(StatusSubmitted),
			testTime, nil,
			3, // total_count
		},
	}
	mock := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return newMockRows(rows), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	list, total, err := repo.FindByReceiverID(context.Background(), "AKUN-MITRA-001", nil, 20, 0)
	if err != nil {
		t.Fatalf("FindByReceiverID mengembalikan error: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, ingin 3", total)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, ingin 1", len(list))
	}
	if list[0].ID != "PGJ000002" {
		t.Errorf("list[0].ID = %q, ingin PGJ000002", list[0].ID)
	}
}

func TestFindByReceiverID_EmptyResult(t *testing.T) {
	mock := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &emptyRows{}, nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	list, total, err := repo.FindByReceiverID(context.Background(), "AKUN-MITRA-BARU", nil, 20, 0)
	if err != nil {
		t.Fatalf("FindByReceiverID mengembalikan error: %v", err)
	}
	if total != 0 || len(list) != 0 {
		t.Errorf("ingin empty result, dapat total=%d list=%d", total, len(list))
	}
}

// =============================================================
// Tests: UpdateStatus
// =============================================================

func TestUpdateStatus_Approve_Succeeds(t *testing.T) {
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return commandTag(1), nil // 1 row affected = sukses
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.UpdateStatus(context.Background(), "PGJ000001", "AKUN-MITRA-001", StatusActive, nil)
	if err != nil {
		t.Fatalf("UpdateStatus AKTIF mengembalikan error: %v", err)
	}
}

func TestUpdateStatus_Reject_RequiresRejectionReason(t *testing.T) {
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return commandTag(1), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	reason := "Tidak memenuhi syarat"
	err := repo.UpdateStatus(context.Background(), "PGJ000001", "AKUN-MITRA-001", StatusRejected, &reason)
	if err != nil {
		t.Fatalf("UpdateStatus DITOLAK mengembalikan error: %v", err)
	}
}

func TestUpdateStatus_Cancel_Succeeds(t *testing.T) {
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return commandTag(1), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.UpdateStatus(context.Background(), "PGJ000001", "AKUN-UMKM-001", StatusCancelled, nil)
	if err != nil {
		t.Fatalf("UpdateStatus DIBATALKAN mengembalikan error: %v", err)
	}
}

func TestUpdateStatus_WrongActor_ReturnsConflictError(t *testing.T) {
	// 0 rows affected berarti kondisi WHERE tidak terpenuhi (aktor salah / status salah)
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return commandTag(0), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.UpdateStatus(context.Background(), "PGJ000001", "AKUN-SALAH", StatusActive, nil)
	if err == nil {
		t.Fatal("UpdateStatus seharusnya mengembalikan error jika 0 rows affected")
	}
	var appErr *AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error harus *AppError, dapat %T: %v", err, err)
	}
	if appErr.Code != http.StatusConflict {
		t.Fatalf("AppError.Code = %d, ingin 409 Conflict", appErr.Code)
	}
}

func TestUpdateStatus_NotFound_ReturnsConflictError(t *testing.T) {
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return commandTag(0), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.UpdateStatus(context.Background(), "PGJ-TIDAK-ADA", "AKUN-001", StatusActive, nil)
	if err == nil {
		t.Fatal("UpdateStatus dengan ID tidak ada seharusnya error")
	}
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusConflict {
		t.Fatalf("ingin 409 Conflict, dapat: %v", err)
	}
}

func TestUpdateStatus_PropagatesDBError(t *testing.T) {
	dbErr := errors.New("deadlock detected")
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, dbErr
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.UpdateStatus(context.Background(), "PGJ000001", "AKUN-001", StatusActive, nil)
	if err == nil {
		t.Fatal("UpdateStatus seharusnya meneruskan error database")
	}
}

// =============================================================
// Tests: MarkAsRead
// =============================================================

func TestMarkAsRead_Succeeds(t *testing.T) {
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return commandTag(1), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.MarkAsRead(context.Background(), "PGJ000001", "AKUN-MITRA-001")
	if err != nil {
		t.Fatalf("MarkAsRead mengembalikan error: %v", err)
	}
}

func TestMarkAsRead_NotFoundOrAlreadyRead_ReturnsConflict(t *testing.T) {
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return commandTag(0), nil // 0 rows = tidak ditemukan atau sudah dibaca
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.MarkAsRead(context.Background(), "PGJ000001", "AKUN-MITRA-001")
	if err == nil {
		t.Fatal("MarkAsRead seharusnya error jika 0 rows affected")
	}
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusConflict {
		t.Fatalf("ingin 409 Conflict, dapat: %v", err)
	}
}

// =============================================================
// Tests: UpdateContract (SignPartnership)
// =============================================================

func TestUpdateContract_Succeeds(t *testing.T) {
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return commandTag(1), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.UpdateContract(context.Background(), "PGJ000001", "AKUN-UMKM-001", "DOK-001")
	if err != nil {
		t.Fatalf("UpdateContract mengembalikan error: %v", err)
	}
}

func TestUpdateContract_WrongActorOrStatus_ReturnsConflict(t *testing.T) {
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return commandTag(0), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.UpdateContract(context.Background(), "PGJ000001", "AKUN-SALAH", "DOK-001")
	if err == nil {
		t.Fatal("UpdateContract seharusnya error jika 0 rows affected")
	}
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusConflict {
		t.Fatalf("ingin 409 Conflict, dapat: %v", err)
	}
}

func TestUpdateContract_PropagatesDBError(t *testing.T) {
	dbErr := errors.New("connection timeout")
	mock := &mockQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, dbErr
		},
	}
	repo := newRepositoryWithQuerier(mock)

	err := repo.UpdateContract(context.Background(), "PGJ000001", "AKUN-UMKM-001", "DOK-001")
	if err == nil {
		t.Fatal("UpdateContract seharusnya meneruskan error database")
	}
}

// =============================================================
// Tests: GetSummary
// =============================================================

func TestGetSummary_ReturnsCounts(t *testing.T) {
	rows := [][]any{
		{string(StatusSubmitted), 3},
		{string(StatusActive), 1},
	}
	mock := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return newMockRows(rows), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	summary, err := repo.GetSummary(context.Background(), "AKUN-UMKM-001")
	if err != nil {
		t.Fatalf("GetSummary mengembalikan error: %v", err)
	}
	if summary[string(StatusSubmitted)] != 3 {
		t.Errorf("DIAJUKAN = %d, ingin 3", summary[string(StatusSubmitted)])
	}
	if summary[string(StatusActive)] != 1 {
		t.Errorf("AKTIF = %d, ingin 1", summary[string(StatusActive)])
	}
}

func TestGetSummary_EmptyReturnsEmptyMap(t *testing.T) {
	mock := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &emptyRows{}, nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	summary, err := repo.GetSummary(context.Background(), "AKUN-UMKM-BARU")
	if err != nil {
		t.Fatalf("GetSummary mengembalikan error: %v", err)
	}
	if len(summary) != 0 {
		t.Errorf("len(summary) = %d, ingin 0", len(summary))
	}
}

// =============================================================
// Tests: GetIncomingSummary
// =============================================================

func TestGetIncomingSummary_ReturnsCounts(t *testing.T) {
	rows := [][]any{
		{string(StatusSubmitted), 5},
		{string(StatusReviewed), 2},
	}
	mock := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return newMockRows(rows), nil
		},
	}
	repo := newRepositoryWithQuerier(mock)

	summary, err := repo.GetIncomingSummary(context.Background(), "AKUN-MITRA-001")
	if err != nil {
		t.Fatalf("GetIncomingSummary mengembalikan error: %v", err)
	}
	if summary[string(StatusSubmitted)] != 5 {
		t.Errorf("DIAJUKAN = %d, ingin 5", summary[string(StatusSubmitted)])
	}
	if summary[string(StatusReviewed)] != 2 {
		t.Errorf("DITINJAU = %d, ingin 2", summary[string(StatusReviewed)])
	}
}

func TestGetIncomingSummary_PropagatesDBError(t *testing.T) {
	dbErr := errors.New("query failed")
	mock := &mockQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, dbErr
		},
	}
	repo := newRepositoryWithQuerier(mock)

	_, err := repo.GetIncomingSummary(context.Background(), "AKUN-MITRA-001")
	if err == nil {
		t.Fatal("GetIncomingSummary seharusnya meneruskan error database")
	}
}

// =============================================================
// Tests: Model helpers (IsValidStatus, IsValidUpdateStatus)
// =============================================================

func TestIsValidStatus_AcceptsAllKnownStatuses(t *testing.T) {
	known := []string{
		"DRAFT", "DIAJUKAN", "DITINJAU", "AKTIF",
		"DITOLAK", "DIBATALKAN", "MENUNGGU_DOKUMEN_TTD", "SELESAI",
	}
	for _, s := range known {
		if !IsValidStatus(s) {
			t.Errorf("IsValidStatus(%q) = false, ingin true", s)
		}
	}
}

func TestIsValidStatus_RejectsUnknownStatus(t *testing.T) {
	unknown := []string{"", "APPROVED", "PENDING", "REJECTED", "invalid", "aktif"}
	for _, s := range unknown {
		if IsValidStatus(s) {
			t.Errorf("IsValidStatus(%q) = true, ingin false", s)
		}
	}
}

func TestIsValidUpdateStatus_AcceptsTransitionStatuses(t *testing.T) {
	allowed := []string{"AKTIF", "DITOLAK", "DIBATALKAN"}
	for _, s := range allowed {
		if !IsValidUpdateStatus(s) {
			t.Errorf("IsValidUpdateStatus(%q) = false, ingin true", s)
		}
	}
}

func TestIsValidUpdateStatus_RejectsNonTransitionStatuses(t *testing.T) {
	notAllowed := []string{"DRAFT", "DIAJUKAN", "DITINJAU", "MENUNGGU_DOKUMEN_TTD", "SELESAI", ""}
	for _, s := range notAllowed {
		if IsValidUpdateStatus(s) {
			t.Errorf("IsValidUpdateStatus(%q) = true, ingin false", s)
		}
	}
}
