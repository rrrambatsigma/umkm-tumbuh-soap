package partnerships

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AppError is a simple application error carrying an HTTP status code.
type AppError struct {
	Code    int
	Message string
}

func (e *AppError) Error() string { return e.Message }

func newError(code int, msg string) error {
	return &AppError{Code: code, Message: msg}
}

// dbQuerier adalah interface yang merangkum method database yang digunakan oleh Repository.
// Dengan interface ini, Repository bisa diuji menggunakan mock tanpa koneksi database nyata.
// *pgxpool.Pool mengimplementasikan interface ini secara alami.
type dbQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository handles all SQL operations for the SOAP partnerships service.
type Repository struct {
	db dbQuerier
}

// NewRepository creates a new Repository backed by a real pgxpool.Pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// newRepositoryWithQuerier creates a Repository backed by any dbQuerier.
// Digunakan hanya untuk keperluan testing.
func newRepositoryWithQuerier(db dbQuerier) *Repository {
	return &Repository{db: db}
}

// -------------------------------------------------------
// Create
// -------------------------------------------------------

// CreateInput holds the fields needed to insert a new partnership.
type CreateInput struct {
	ID                  string
	RequestCode         string
	RequesterID         string
	ReceiverID          string
	RequesterBusinessID string // may be empty (nullable FK)
	ReceiverBusinessID  string // may be empty (nullable FK)
	RequesterRole       string // "UMKM" or "MITRA"
	ProposalText        string // stored in pesan_pengajuan
	SubmittedAt         time.Time
}

// Create inserts a new partnership request.
// umkm_id and mitra_id are nullable (migration 025 dropped NOT NULL).
func (r *Repository) Create(ctx context.Context, in CreateInput) error {
	var umkmID, mitraID *string
	if in.RequesterRole == "UMKM" {
		if in.RequesterBusinessID != "" {
			umkmID = &in.RequesterBusinessID
		}
		if in.ReceiverBusinessID != "" {
			mitraID = &in.ReceiverBusinessID
		}
	} else { // MITRA
		if in.RequesterBusinessID != "" {
			mitraID = &in.RequesterBusinessID
		}
		if in.ReceiverBusinessID != "" {
			umkmID = &in.ReceiverBusinessID
		}
	}

	const query = `
		INSERT INTO partnership.transaksi_pengajuankerjasama (
			pengajuan_id, kode_pengajuan, umkm_id, mitra_id,
			pengaju_akun_id, penerima_akun_id,
			status_pengajuan_id, pesan_pengajuan,
			tanggal_pengajuan, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW()
		)
	`
	_, err := r.db.Exec(ctx, query,
		in.ID, in.RequestCode, umkmID, mitraID,
		in.RequesterID, in.ReceiverID,
		string(StatusSubmitted), in.ProposalText,
		in.SubmittedAt,
	)
	if err != nil {
		return fmt.Errorf("create partnership: %w", err)
	}
	return nil
}

// -------------------------------------------------------
// Count helpers (for ID/code generation)
// -------------------------------------------------------

// CountAll returns the total number of partnership requests.
func (r *Repository) CountAll(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM partnership.transaksi_pengajuankerjasama`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count all: %w", err)
	}
	return count, nil
}

// CountByYear returns the number of PKS codes for the given year (for code generation).
func (r *Repository) CountByYear(ctx context.Context, year string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM partnership.transaksi_pengajuankerjasama WHERE kode_pengajuan LIKE $1`,
		"PKS-"+year+"-%",
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count by year: %w", err)
	}
	return count, nil
}

// -------------------------------------------------------
// FindByID — with actor-based authorization (Issue #4 fix)
// -------------------------------------------------------

// FindByID returns the full detail of a single partnership.
// The query enforces that the caller (actorID) is either the requester OR receiver.
// This prevents any authenticated user from reading another party's applications.
// Derived from: existing service.go GetPartnershipByID checks RequesterID/ReceiverID after fetch.
// We push this check into SQL for defence-in-depth.
func (r *Repository) FindByID(ctx context.Context, id, actorID string) (*PartnershipRow, error) {
	const query = `
		SELECT
			pr.pengajuan_id,
			pr.kode_pengajuan,
			pr.pengaju_akun_id,
			pr.penerima_akun_id,
			pr.status_pengajuan_id,
			pr.pesan_pengajuan,
			pr.catatan_keputusan,
			pr.dokumen_perjanjian_id,
			COALESCE(u1.nama_lengkap, '') AS requester_name,
			COALESCE(u2.nama_lengkap, '') AS receiver_name,
			pr.tanggal_pengajuan,
			pr.tanggal_keputusan,
			pr.created_at,
			pr.updated_at
		FROM partnership.transaksi_pengajuankerjasama pr
		LEFT JOIN auth.master_akunpengguna u1 ON pr.pengaju_akun_id = u1.akun_id
		LEFT JOIN auth.master_akunpengguna u2 ON pr.penerima_akun_id = u2.akun_id
		WHERE pr.pengajuan_id = $1
		  AND (pr.pengaju_akun_id = $2 OR pr.penerima_akun_id = $2)
	`
	var row PartnershipRow
	err := r.db.QueryRow(ctx, query, id, actorID).Scan(
		&row.ID, &row.RequestCode,
		&row.RequesterID, &row.ReceiverID,
		&row.Status,
		&row.ProposalText,
		&row.RejectionReason,
		&row.ContractDocumentID,
		&row.RequesterName, &row.ReceiverName,
		&row.SubmittedAt, &row.DecidedAt,
		&row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			// ErrNoRows here could mean: not found, OR found but actor not authorized.
			// We return a generic not-found to avoid leaking existence of the record.
			return nil, newError(http.StatusNotFound, "Pengajuan kemitraan tidak ditemukan atau Anda tidak memiliki akses")
		}
		return nil, fmt.Errorf("find by id: %w", err)
	}
	return &row, nil
}

// findByIDNoAuth is an internal helper for operations that already hold the row
// (e.g., UpdateStatus which needs to first verify actor roles).
// Only used internally — never exposed to SOAP callers directly.
func (r *Repository) findByIDNoAuth(ctx context.Context, id string) (*PartnershipRow, error) {
	const query = `
		SELECT
			pr.pengajuan_id, pr.kode_pengajuan,
			pr.pengaju_akun_id, pr.penerima_akun_id,
			pr.status_pengajuan_id,
			pr.pesan_pengajuan, pr.catatan_keputusan, pr.dokumen_perjanjian_id,
			COALESCE(u1.nama_lengkap, '') AS requester_name,
			COALESCE(u2.nama_lengkap, '') AS receiver_name,
			pr.tanggal_pengajuan, pr.tanggal_keputusan,
			pr.created_at, pr.updated_at
		FROM partnership.transaksi_pengajuankerjasama pr
		LEFT JOIN auth.master_akunpengguna u1 ON pr.pengaju_akun_id = u1.akun_id
		LEFT JOIN auth.master_akunpengguna u2 ON pr.penerima_akun_id = u2.akun_id
		WHERE pr.pengajuan_id = $1
	`
	var row PartnershipRow
	err := r.db.QueryRow(ctx, query, id).Scan(
		&row.ID, &row.RequestCode,
		&row.RequesterID, &row.ReceiverID,
		&row.Status,
		&row.ProposalText, &row.RejectionReason, &row.ContractDocumentID,
		&row.RequesterName, &row.ReceiverName,
		&row.SubmittedAt, &row.DecidedAt,
		&row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, newError(http.StatusNotFound, "Pengajuan kemitraan tidak ditemukan")
		}
		return nil, fmt.Errorf("find by id (no auth): %w", err)
	}
	return &row, nil
}

// -------------------------------------------------------
// FindByRequesterID (outgoing / status view)
// -------------------------------------------------------

// FindByRequesterID returns paginated partnerships where the user is the requester.
func (r *Repository) FindByRequesterID(ctx context.Context, requesterID string, status *PartnershipStatus, limit, offset int) ([]PartnershipListRow, int, error) {
	var extraWhere string
	var args []interface{}
	args = append(args, requesterID)
	argIdx := 2

	if status != nil {
		extraWhere = fmt.Sprintf(" AND pr.status_pengajuan_id = $%d", argIdx)
		args = append(args, string(*status))
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT
			pr.pengajuan_id,
			pr.kode_pengajuan,
			COALESCE(u1.nama_lengkap, '') AS requester_name,
			COALESCE(u2.nama_lengkap, '') AS receiver_name,
			COALESCE(umkm.nama_umkm, '')  AS requester_business_name,
			COALESCE(mitra.nama_mitra, '') AS receiver_business_name,
			pr.pesan_pengajuan             AS proposal_title,
			pr.status_pengajuan_id,
			pr.tanggal_pengajuan,
			pr.tanggal_keputusan,
			COUNT(*) OVER()               AS total_count
		FROM partnership.transaksi_pengajuankerjasama pr
		LEFT JOIN auth.master_akunpengguna u1   ON pr.pengaju_akun_id = u1.akun_id
		LEFT JOIN auth.master_akunpengguna u2   ON pr.penerima_akun_id = u2.akun_id
		LEFT JOIN user_mgmt.master_umkm  umkm  ON pr.umkm_id = umkm.umkm_id
		LEFT JOIN user_mgmt.master_mitra mitra ON pr.mitra_id = mitra.mitra_id
		WHERE pr.pengaju_akun_id = $1%s
		ORDER BY pr.created_at DESC
		LIMIT $%d OFFSET $%d
	`, extraWhere, argIdx, argIdx+1)

	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("find by requester: %w", err)
	}
	defer rows.Close()

	return r.scanListRows(rows)
}

// -------------------------------------------------------
// FindByReceiverID (incoming / inbox view)
// -------------------------------------------------------

// FindByReceiverID returns paginated partnerships where the user is the receiver.
func (r *Repository) FindByReceiverID(ctx context.Context, receiverID string, status *PartnershipStatus, limit, offset int) ([]PartnershipListRow, int, error) {
	var extraWhere string
	var args []interface{}
	args = append(args, receiverID)
	argIdx := 2

	if status != nil {
		extraWhere = fmt.Sprintf(" AND pr.status_pengajuan_id = $%d", argIdx)
		args = append(args, string(*status))
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT
			pr.pengajuan_id,
			pr.kode_pengajuan,
			COALESCE(u1.nama_lengkap, '') AS requester_name,
			COALESCE(u2.nama_lengkap, '') AS receiver_name,
			COALESCE(umkm.nama_umkm, '')  AS requester_business_name,
			COALESCE(mitra.nama_mitra, '') AS receiver_business_name,
			pr.pesan_pengajuan             AS proposal_title,
			pr.status_pengajuan_id,
			pr.tanggal_pengajuan,
			pr.tanggal_keputusan,
			COUNT(*) OVER()               AS total_count
		FROM partnership.transaksi_pengajuankerjasama pr
		LEFT JOIN auth.master_akunpengguna u1   ON pr.pengaju_akun_id = u1.akun_id
		LEFT JOIN auth.master_akunpengguna u2   ON pr.penerima_akun_id = u2.akun_id
		LEFT JOIN user_mgmt.master_umkm  umkm  ON pr.umkm_id = umkm.umkm_id
		LEFT JOIN user_mgmt.master_mitra mitra ON pr.mitra_id = mitra.mitra_id
		WHERE pr.penerima_akun_id = $1%s
		ORDER BY pr.created_at DESC
		LIMIT $%d OFFSET $%d
	`, extraWhere, argIdx, argIdx+1)

	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("find by receiver: %w", err)
	}
	defer rows.Close()

	return r.scanListRows(rows)
}

func (r *Repository) scanListRows(rows pgx.Rows) ([]PartnershipListRow, int, error) {
	var list []PartnershipListRow
	var total int
	for rows.Next() {
		var item PartnershipListRow
		if err := rows.Scan(
			&item.ID, &item.RequestCode,
			&item.RequesterName, &item.ReceiverName,
			&item.RequesterBusinessName, &item.ReceiverBusinessName,
			&item.ProposalTitle, &item.Status,
			&item.SubmittedAt, &item.DecidedAt,
			&item.TotalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan list row: %w", err)
		}
		total = item.TotalCount
		list = append(list, item)
	}
	return list, total, rows.Err()
}

// -------------------------------------------------------
// UpdateStatus — with DB-enforced actor authorization
// -------------------------------------------------------

// UpdateStatus performs a conditional status transition.
// Authorization is enforced inside SQL (same approach as existing REST service):
//   - AKTIF/DITOLAK: only penerima_akun_id (receiver) may apply, from DIAJUKAN or DITINJAU
//   - DIBATALKAN: only pengaju_akun_id (requester) may apply, from DRAFT, DIAJUKAN, or DITINJAU
//
// Returns conflict error if no rows affected (wrong actor, wrong current status, or not found).
func (r *Repository) UpdateStatus(ctx context.Context, id, actorID string, newStatus PartnershipStatus, rejectionReason *string) error {
	var decidedAt *time.Time
	if newStatus == StatusActive || newStatus == StatusRejected {
		now := time.Now()
		decidedAt = &now
	}
	const query = `
		UPDATE partnership.transaksi_pengajuankerjasama
		SET status_pengajuan_id = $1,
		    catatan_keputusan   = $2,
		    tanggal_keputusan   = $3,
		    updated_at          = NOW()
		WHERE pengajuan_id = $4
		  AND (
		        ($1 IN ('AKTIF', 'DITOLAK')
		         AND penerima_akun_id = $5
		         AND status_pengajuan_id IN ('DIAJUKAN', 'DITINJAU'))
		     OR ($1 = 'DIBATALKAN'
		         AND pengaju_akun_id = $5
		         AND status_pengajuan_id IN ('DRAFT', 'DIAJUKAN', 'DITINJAU'))
		  )
	`
	tag, err := r.db.Exec(ctx, query, string(newStatus), rejectionReason, decidedAt, id, actorID)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return newError(http.StatusConflict,
			"Pengajuan tidak ditemukan, Anda tidak berwenang, atau status tidak dapat diubah")
	}
	return nil
}

// -------------------------------------------------------
// MarkAsRead — prototype workaround
// -------------------------------------------------------

// MarkAsRead transitions DIAJUKAN → DITINJAU for the receiver.
//
// Prototype design note:
// In the original REST service, PATCH /partnerships/:id/read only verifies
// that the caller is the receiver (penerima_akun_id) but does NOT change the status.
// Because the existing DB schema has no is_read column, this SOAP prototype maps
// the operation to DIAJUKAN → DITINJAU. This is an intentional workaround.
// The state machine confirms DITINJAU means "receiver has seen the application".
func (r *Repository) MarkAsRead(ctx context.Context, id, receiverID string) error {
	const query = `
		UPDATE partnership.transaksi_pengajuankerjasama
		SET status_pengajuan_id = 'DITINJAU',
		    updated_at           = NOW()
		WHERE pengajuan_id       = $1
		  AND penerima_akun_id   = $2
		  AND status_pengajuan_id = 'DIAJUKAN'
	`
	tag, err := r.db.Exec(ctx, query, id, receiverID)
	if err != nil {
		return fmt.Errorf("mark as read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Could mean: not found, not the receiver, or already read (status != DIAJUKAN).
		return newError(http.StatusConflict,
			"Pengajuan tidak ditemukan, bukan milik penerima, atau sudah ditandai dibaca")
	}
	return nil
}

// -------------------------------------------------------
// UpdateContract (SignPartnership)
// -------------------------------------------------------

// UpdateContract attaches a contract document to a partnership.
// Actor: pengaju_akun_id (requester) — confirmed from existing service.go.
// Valid source statuses: DIAJUKAN, DITINJAU, MENUNGGU_DOKUMEN_TTD.
// Prototype note: no document ownership validation against document-service.
func (r *Repository) UpdateContract(ctx context.Context, id, requesterID, documentID string) error {
	now := time.Now()
	const query = `
		UPDATE partnership.transaksi_pengajuankerjasama
		SET dokumen_perjanjian_id  = $1,
		    tanggal_upload_dokumen = $2,
		    updated_at              = NOW()
		WHERE pengajuan_id    = $3
		  AND pengaju_akun_id = $4
		  AND status_pengajuan_id IN ('DIAJUKAN', 'DITINJAU', 'MENUNGGU_DOKUMEN_TTD')
	`
	tag, err := r.db.Exec(ctx, query, documentID, now, id, requesterID)
	if err != nil {
		return fmt.Errorf("update contract: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return newError(http.StatusConflict,
			"Pengajuan tidak ditemukan, Anda tidak berwenang, atau status tidak memungkinkan upload dokumen")
	}
	return nil
}

// -------------------------------------------------------
// Summary queries
// -------------------------------------------------------

// GetSummary returns status counts for partnerships where the user is the requester.
func (r *Repository) GetSummary(ctx context.Context, userID string) (map[string]int, error) {
	const q = `
		SELECT status_pengajuan_id, COUNT(*) AS cnt
		FROM partnership.transaksi_pengajuankerjasama
		WHERE pengaju_akun_id = $1
		GROUP BY status_pengajuan_id
	`
	return r.querySummary(ctx, q, userID)
}

// GetIncomingSummary returns status counts for partnerships where the user is the receiver.
func (r *Repository) GetIncomingSummary(ctx context.Context, userID string) (map[string]int, error) {
	const q = `
		SELECT status_pengajuan_id, COUNT(*) AS cnt
		FROM partnership.transaksi_pengajuankerjasama
		WHERE penerima_akun_id = $1
		GROUP BY status_pengajuan_id
	`
	return r.querySummary(ctx, q, userID)
}

func (r *Repository) querySummary(ctx context.Context, query, userID string) (map[string]int, error) {
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("summary query: %w", err)
	}
	defer rows.Close()

	summary := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		summary[status] = count
	}
	return summary, rows.Err()
}

// -------------------------------------------------------
// Resolve business ID → akun_id
// -------------------------------------------------------

// FindReceiverAkunID resolves a mitra_id or umkm_id to the corresponding akun_id.
// receiverRole is the role of the *receiver* (opposite of requester).
func (r *Repository) FindReceiverAkunID(ctx context.Context, businessID, receiverRole string) (string, error) {
	var query string
	if receiverRole == "MITRA" {
		query = `SELECT akun_id FROM user_mgmt.master_mitra WHERE mitra_id = $1 LIMIT 1`
	} else {
		query = `SELECT akun_id FROM user_mgmt.master_umkm WHERE umkm_id = $1 LIMIT 1`
	}
	var akunID string
	err := r.db.QueryRow(ctx, query, businessID).Scan(&akunID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", newError(http.StatusBadRequest, "Penerima tidak ditemukan")
		}
		return "", fmt.Errorf("find receiver akun id: %w", err)
	}
	return akunID, nil
}
