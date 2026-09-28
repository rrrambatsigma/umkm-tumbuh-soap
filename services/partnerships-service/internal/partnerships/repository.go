package partnerships

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/apperror"
)

// ErrBusinessNotFound distinguishes an absent business from a database failure.
var ErrBusinessNotFound = errors.New("business not found")

type Repository interface {
	OwnsDocument(ctx context.Context, actorID, documentID string, legacy bool) (bool, error)
	Create(ctx context.Context, req *PartnershipRequest) error
	CountAll(ctx context.Context) (int, error)
	FindByID(ctx context.Context, id string) (*PartnershipResponse, error)
	FindByRequesterID(ctx context.Context, requesterID string, status *PartnershipStatus, limit, offset int) ([]PartnershipListResponse, int, error)
	FindByReceiverID(ctx context.Context, receiverID string, status *PartnershipStatus, limit, offset int) ([]PartnershipListResponse, int, error)
	UpdateStatus(ctx context.Context, id, actorID string, expectedStatus, status PartnershipStatus, rejectionReason *string, decidedAt time.Time) error
	UpdateContract(ctx context.Context, id, actorID string, expectedStatus PartnershipStatus, dokumenKontrak string, signedAt time.Time) error
	GenerateRequestCode(ctx context.Context) (string, error)
	GetSummary(ctx context.Context, userID string) (map[string]int, error)
	GetIncomingSummary(ctx context.Context, userID string) (map[string]int, error)

	FindUMKMList(ctx context.Context, search string, filterType string, limit, offset int) ([]UMKMListItem, int, error)
	FindMitraList(ctx context.Context, search string, filterType string, limit, offset int) ([]MitraListItem, int, error)
	FindAkunIDByBusinessID(ctx context.Context, businessID string, role UserRole) (string, error)
	FindBusinessIDByAkunID(ctx context.Context, akunID string, role UserRole) (string, error)
	FindUMKMDetail(ctx context.Context, umkmID string) (*UMKMDetail, error)
	FindMitraDetail(ctx context.Context, mitraID string) (*MitraDetail, error)

	CreateAttachments(ctx context.Context, partnershipID string, documentIDs []string) error
	FindAttachmentsByPartnershipID(ctx context.Context, partnershipID string) ([]PartnershipAttachment, error)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, req *PartnershipRequest) error {
	// Determine umkm_id and mitra_id based on requester role
	// Use nil for empty business IDs so they become SQL NULL (FK allows NULL)
	var umkmID, mitraID *string
	if req.RequesterRole == RoleUMKM {
		if req.RequesterBusinessID != "" {
			umkmID = &req.RequesterBusinessID
		}
		if req.ReceiverBusinessID != "" {
			mitraID = &req.ReceiverBusinessID
		}
	} else {
		if req.RequesterBusinessID != "" {
			mitraID = &req.RequesterBusinessID
		}
		if req.ReceiverBusinessID != "" {
			umkmID = &req.ReceiverBusinessID
		}
	}

	query := `
		INSERT INTO partnership.transaksi_pengajuankerjasama (
			pengajuan_id, kode_pengajuan, umkm_id, mitra_id, pengaju_akun_id, penerima_akun_id,
			status_pengajuan_id, pesan_pengajuan, catatan_keputusan, dokumen_perjanjian_id,
			tanggal_pengajuan, tanggal_keputusan, tanggal_upload_dokumen,
			tanggal_mulai_kerjasama, tanggal_selesai_kerjasama, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW()
		)
	`

	_, err := r.db.Exec(ctx, query,
		req.ID, req.RequestCode, umkmID, mitraID, req.RequesterID, req.ReceiverID,
		req.Status, req.ProposalDescription, req.RejectionReason, req.ContractDocumentID,
		req.SubmittedAt, req.DecidedAt, req.ContractSignedAt,
		req.PartnershipStart, req.PartnershipEnd,
	)

	if err != nil {
		return fmt.Errorf("failed to create partnership request: %w", err)
	}

	return nil
}

func (r *repository) CountAll(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM partnership.transaksi_pengajuankerjasama").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count partnerships: %w", err)
	}
	return count, nil
}

func (r *repository) FindByID(ctx context.Context, id string) (*PartnershipResponse, error) {
	query := `
		SELECT 
			pr.pengajuan_id as id,
			pr.kode_pengajuan as request_code,
			pr.pengaju_akun_id as requester_id,
			pr.penerima_akun_id as receiver_id,
			pr.status_pengajuan_id as status,
			pr.tanggal_pengajuan as submitted_at,
			pr.tanggal_keputusan as decided_at,
			pr.pesan_pengajuan as proposal_description,
			pr.catatan_keputusan as rejection_reason,
			pr.dokumen_perjanjian_id as contract_document_id,
			u1.nama_lengkap as requester_name,
			u2.nama_lengkap as receiver_name,
			pr.created_at,
			pr.updated_at
		FROM partnership.transaksi_pengajuankerjasama pr
		LEFT JOIN auth.master_akunpengguna u1 ON pr.pengaju_akun_id = u1.akun_id
		LEFT JOIN auth.master_akunpengguna u2 ON pr.penerima_akun_id = u2.akun_id
		WHERE pr.pengajuan_id = $1
	`

	var resp PartnershipResponse
	err := r.db.QueryRow(ctx, query, id).Scan(
		&resp.ID, &resp.RequestCode, &resp.RequesterID, &resp.ReceiverID,
		&resp.Status, &resp.SubmittedAt, &resp.DecidedAt,
		&resp.ProposalDescription, &resp.RejectionReason, &resp.ContractDocumentID,
		&resp.RequesterName, &resp.ReceiverName, &resp.CreatedAt, &resp.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperror.New(http.StatusNotFound, "Pengajuan kemitraan tidak ditemukan")
		}
		return nil, fmt.Errorf("failed to find partnership request: %w", err)
	}

	attachments, err := r.FindAttachmentsByPartnershipID(ctx, id)
	if err != nil {
		return nil, err
	}
	if attachments == nil {
		attachments = []PartnershipAttachment{}
	}

	resp.Attachments = attachments

	return &resp, nil
}

func (r *repository) FindByRequesterID(ctx context.Context, requesterID string, status *PartnershipStatus, limit, offset int) ([]PartnershipListResponse, int, error) {
	var whereClause string
	var args []interface{}
	args = append(args, requesterID)
	argIndex := 2

	if status != nil {
		whereClause = fmt.Sprintf(" AND pr.status_pengajuan_id = $%d", argIndex)
		args = append(args, *status)
		argIndex++
	}

	query := fmt.Sprintf(`
		SELECT 
			pr.pengajuan_id as id,
			pr.kode_pengajuan as request_code,
			COALESCE(u1.nama_lengkap, '') as requester_name,
			COALESCE(u2.nama_lengkap, '') as receiver_name,
			COALESCE(umkm.nama_umkm, '') as requester_business_name,
			COALESCE(mitra.nama_mitra, '') as receiver_business_name,
			pr.pesan_pengajuan as proposal_title,
			pr.status_pengajuan_id as status,
			pr.tanggal_pengajuan as submitted_at,
			pr.tanggal_keputusan as decided_at,
			COUNT(*) OVER() as total_count
		FROM partnership.transaksi_pengajuankerjasama pr
		LEFT JOIN auth.master_akunpengguna u1 ON pr.pengaju_akun_id = u1.akun_id
		LEFT JOIN auth.master_akunpengguna u2 ON pr.penerima_akun_id = u2.akun_id
		LEFT JOIN user_mgmt.master_umkm umkm ON pr.umkm_id = umkm.umkm_id
		LEFT JOIN user_mgmt.master_mitra mitra ON pr.mitra_id = mitra.mitra_id
		WHERE pr.pengaju_akun_id = $1%s
		ORDER BY pr.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch partnership requests: %w", err)
	}
	defer rows.Close()

	var partnerships []PartnershipListResponse
	var totalCount int

	for rows.Next() {
		var p PartnershipListResponse
		err := rows.Scan(
			&p.ID, &p.RequestCode, &p.RequesterName, &p.ReceiverName,
			&p.RequesterBusinessName, &p.ReceiverBusinessName,
			&p.ProposalTitle, &p.Status, &p.SubmittedAt, &p.DecidedAt,
			&totalCount,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan partnership request: %w", err)
		}
		partnerships = append(partnerships, p)
	}

	return partnerships, totalCount, nil
}

func (r *repository) FindByReceiverID(ctx context.Context, receiverID string, status *PartnershipStatus, limit, offset int) ([]PartnershipListResponse, int, error) {
	var whereClause string
	var args []interface{}
	args = append(args, receiverID)
	argIndex := 2

	if status != nil {
		whereClause = fmt.Sprintf(" AND pr.status_pengajuan_id = $%d", argIndex)
		args = append(args, *status)
		argIndex++
	}

	query := fmt.Sprintf(`
		SELECT 
			pr.pengajuan_id as id,
			pr.kode_pengajuan as request_code,
			COALESCE(u1.nama_lengkap, '') as requester_name,
			COALESCE(u2.nama_lengkap, '') as receiver_name,
			COALESCE(umkm.nama_umkm, '') as requester_business_name,
			COALESCE(mitra.nama_mitra, '') as receiver_business_name,
			pr.pesan_pengajuan as proposal_title,
			pr.status_pengajuan_id as status,
			pr.tanggal_pengajuan as submitted_at,
			pr.tanggal_keputusan as decided_at,
			COUNT(*) OVER() as total_count
		FROM partnership.transaksi_pengajuankerjasama pr
		LEFT JOIN auth.master_akunpengguna u1 ON pr.pengaju_akun_id = u1.akun_id
		LEFT JOIN auth.master_akunpengguna u2 ON pr.penerima_akun_id = u2.akun_id
		LEFT JOIN user_mgmt.master_umkm umkm ON pr.umkm_id = umkm.umkm_id
		LEFT JOIN user_mgmt.master_mitra mitra ON pr.mitra_id = mitra.mitra_id
		WHERE pr.penerima_akun_id = $1%s
		ORDER BY pr.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch partnership requests: %w", err)
	}
	defer rows.Close()

	var partnerships []PartnershipListResponse
	var totalCount int

	for rows.Next() {
		var p PartnershipListResponse
		err := rows.Scan(
			&p.ID, &p.RequestCode, &p.RequesterName, &p.ReceiverName,
			&p.RequesterBusinessName, &p.ReceiverBusinessName,
			&p.ProposalTitle, &p.Status, &p.SubmittedAt, &p.DecidedAt,
			&totalCount,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan partnership request: %w", err)
		}
		partnerships = append(partnerships, p)
	}

	return partnerships, totalCount, nil
}

func (r *repository) UpdateStatus(ctx context.Context, id, actorID string, expectedStatus, status PartnershipStatus, rejectionReason *string, decidedAt time.Time) error {
	const query = `
		UPDATE partnership.transaksi_pengajuankerjasama
		SET status_pengajuan_id = $1,
			catatan_keputusan = $2,
			tanggal_keputusan = $3,
			updated_at = NOW()
		WHERE pengajuan_id = $4 AND status_pengajuan_id = $6
		  AND (($1 IN ('AKTIF', 'DITOLAK') AND penerima_akun_id = $5
		        AND status_pengajuan_id IN ('DIAJUKAN', 'DITINJAU'))
		    OR ($1 = 'DIBATALKAN' AND pengaju_akun_id = $5
		        AND status_pengajuan_id IN ('DRAFT', 'DIAJUKAN', 'DITINJAU')))
	`
	tag, err := r.db.Exec(ctx, query, status, rejectionReason, decidedAt, id, actorID, expectedStatus)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperror.New(http.StatusConflict, "Pengajuan telah berubah. Muat ulang sebelum mencoba lagi")
	}
	return nil
}

func (r *repository) UpdateContract(ctx context.Context, id, actorID string, expectedStatus PartnershipStatus, dokumenKontrak string, signedAt time.Time) error {
	const query = `
		UPDATE partnership.transaksi_pengajuankerjasama
		SET dokumen_perjanjian_id = $1, tanggal_upload_dokumen = $2, updated_at = NOW()
		WHERE pengajuan_id = $3 AND pengaju_akun_id = $4 AND status_pengajuan_id = $5
		  AND status_pengajuan_id IN ('DIAJUKAN', 'DITINJAU', 'MENUNGGU_DOKUMEN_TTD')
		  AND EXISTS (
		      SELECT 1 FROM document.transaksi_dokumenterunggah d
		      WHERE d.dokumen_id = $1 AND d.uploader_akun_id = $4
		  )
	`
	tag, err := r.db.Exec(ctx, query, dokumenKontrak, signedAt, id, actorID, expectedStatus)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperror.New(http.StatusConflict, "Pengajuan atau dokumen telah berubah. Muat ulang sebelum mencoba lagi")
	}
	return nil
}

// Kontrak masih memakai tabel dokumen lama; lampiran memakai metadata upload saat ini.
func (r *repository) OwnsDocument(ctx context.Context, actorID, documentID string, legacy bool) (bool, error) {
	query := `SELECT EXISTS (
		SELECT 1 FROM documents.master_dokumen
		WHERE dokumen_id = $1 AND uploader_akun_id = $2 AND status = 'AKTIF'
	)`
	if legacy {
		query = `SELECT EXISTS (
			SELECT 1 FROM document.transaksi_dokumenterunggah
			WHERE dokumen_id = $1 AND uploader_akun_id = $2
		)`
	}
	var owned bool
	err := r.db.QueryRow(ctx, query, documentID, actorID).Scan(&owned)
	return owned, err
}

func (r *repository) GetSummary(ctx context.Context, userID string) (map[string]int, error) {
	query := `
		SELECT status_pengajuan_id, COUNT(*) as cnt
		FROM partnership.transaksi_pengajuankerjasama
		WHERE pengaju_akun_id = $1
		GROUP BY status_pengajuan_id
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get summary: %w", err)
	}
	defer rows.Close()

	summary := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan summary row: %w", err)
		}
		summary[status] = count
	}

	return summary, nil
}

func (r *repository) GetIncomingSummary(ctx context.Context, userID string) (map[string]int, error) {
	query := `
		SELECT status_pengajuan_id, COUNT(*) as cnt
		FROM partnership.transaksi_pengajuankerjasama
		WHERE penerima_akun_id = $1
		GROUP BY status_pengajuan_id
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get incoming summary: %w", err)
	}
	defer rows.Close()

	summary := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan incoming summary row: %w", err)
		}
		summary[status] = count
	}

	return summary, nil
}

func (r *repository) GenerateRequestCode(ctx context.Context) (string, error) {
	// Generate request code with format: PKS-YYYY-XXXXXX
	var count int
	query := `SELECT COUNT(*) FROM partnership.transaksi_pengajuankerjasama WHERE kode_pengajuan LIKE $1`
	err := r.db.QueryRow(ctx, query, "PKS-"+time.Now().Format("2006")+"-%").Scan(&count)
	if err != nil {
		return "", fmt.Errorf("failed to generate request code: %w", err)
	}

	// Format: PKS-2024-000001
	requestCode := fmt.Sprintf("PKS-%s-%06d", time.Now().Format("2006"), count+1)
	return requestCode, nil
}

// ============================================================
// NEW IMPLEMENTATIONS FOR UMKM AND MITRA LISTS
// ============================================================

// / FindUMKMList retrieves a paginated list of verified UMKM
// Used by MITRA to find potential partners
func (r *repository) FindUMKMList(ctx context.Context, search string, filterType string, limit, offset int) ([]UMKMListItem, int, error) {
	var whereClause string
	var args []interface{}
	argIndex := 1

	// Build search condition if search term is provided
	if search != "" {
		whereClause = fmt.Sprintf(" AND u.nama_umkm ILIKE $%d", argIndex)
		args = append(args, "%"+search+"%")
		argIndex++
	}

	// Build filter by type if filterType is provided
	if filterType != "" && filterType != "all" {
		whereClause += fmt.Sprintf(" AND juk.nama_jenis_umkm ILIKE $%d", argIndex)
		args = append(args, "%"+filterType+"%")
		argIndex++
	}

	query := fmt.Sprintf(`
		SELECT 
			u.umkm_id as id,
			u.nama_umkm as name,
			COALESCE(juk.nama_jenis_umkm, '') as type,
			COALESCE(l.kabupaten_kota, '') as city,
			COALESCE(l.provinsi, '') as province,
			COALESCE(u.deskripsi_usaha, '') as description,
			'' as operational_area,
			COALESCE(u.logo_url, '') as logo_url,
			COALESCE(u.foto_cover_url, '') as foto_cover_url,
			COUNT(*) OVER() as total_count
		FROM user_mgmt.master_umkm u
		LEFT JOIN ref.ref_jenisumkm juk ON u.jenis_umkm_id = juk.jenis_umkm_id
		LEFT JOIN user_mgmt.master_lokasi l ON u.lokasi_id = l.lokasi_id
		WHERE u.status_verified = true 
		AND u.is_deleted = false
		%s
		ORDER BY u.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch UMKM list: %w", err)
	}
	defer rows.Close()

	var umkmList []UMKMListItem
	var totalCount int

	for rows.Next() {
		var item UMKMListItem
		err := rows.Scan(
			&item.ID, &item.Name, &item.Type, &item.City,
			&item.Province, &item.Description, &item.OperationalArea,
			&item.LogoURL, &item.FotoCoverURL,
			&totalCount,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan UMKM row: %w", err)
		}
		umkmList = append(umkmList, item)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration error: %w", err)
	}

	return umkmList, totalCount, nil
}

// FindMitraList retrieves a paginated list of verified Mitra
// Used by UMKM to find potential partners
func (r *repository) FindMitraList(ctx context.Context, search string, filterType string, limit, offset int) ([]MitraListItem, int, error) {
	var whereClause string
	var args []interface{}
	argIndex := 1

	// Build search condition if search term is provided
	if search != "" {
		whereClause = fmt.Sprintf(" AND m.nama_mitra ILIKE $%d", argIndex)
		args = append(args, "%"+search+"%")
		argIndex++
	}

	// Build filter by type if filterType is provided
	if filterType != "" && filterType != "all" {
		whereClause += fmt.Sprintf(" AND jm.nama_jenis_mitra ILIKE $%d", argIndex)
		args = append(args, "%"+filterType+"%")
		argIndex++
	}

	query := fmt.Sprintf(`
		SELECT 
			m.mitra_id as id,
			m.nama_mitra as name,
			COALESCE(jm.nama_jenis_mitra, '') as type,
			COALESCE(l.kabupaten_kota, '') as city,
			COALESCE(l.provinsi, '') as province,
			COALESCE(m.deskripsi_dukungan, '') as description,
			COALESCE(m.wilayah_operasional, '') as operational_area,
			COUNT(*) OVER() as total_count
		FROM user_mgmt.master_mitra m
		LEFT JOIN ref.ref_jenismitra jm ON m.jenis_mitra_id = jm.jenis_mitra_id
		LEFT JOIN user_mgmt.master_lokasi l ON m.lokasi_id = l.lokasi_id
		WHERE m.status_verified = true 
		AND m.is_deleted = false
		%s
		ORDER BY m.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch Mitra list: %w", err)
	}
	defer rows.Close()

	var mitraList []MitraListItem
	var totalCount int

	for rows.Next() {
		var item MitraListItem
		err := rows.Scan(
			&item.ID, &item.Name, &item.Type, &item.City,
			&item.Province, &item.Description, &item.OperationalArea, &totalCount,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan Mitra row: %w", err)
		}
		mitraList = append(mitraList, item)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration error: %w", err)
	}

	return mitraList, totalCount, nil
}

// FindAkunIDByBusinessID converts a business ID (mitra_id/umkm_id) to the corresponding akun_id
func (r *repository) FindAkunIDByBusinessID(ctx context.Context, businessID string, role UserRole) (string, error) {
	var akunID string
	var query string

	if role == RoleMitra {
		query = `SELECT akun_id FROM user_mgmt.master_mitra WHERE mitra_id = $1`
	} else {
		query = `SELECT p.akun_id FROM user_mgmt.master_umkm u
				JOIN user_mgmt.master_pelakuumkm p ON u.pelaku_umkm_id = p.pelaku_umkm_id
				WHERE u.umkm_id = $1`
	}

	err := r.db.QueryRow(ctx, query, businessID).Scan(&akunID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrBusinessNotFound
		}
		return "", fmt.Errorf("failed to find akun_id for business ID %s: %w", businessID, err)
	}

	return akunID, nil
}

// FindBusinessIDByAkunID converts an akun_id to the corresponding business ID (mitra_id/umkm_id)
func (r *repository) FindBusinessIDByAkunID(ctx context.Context, akunID string, role UserRole) (string, error) {
	var businessID string
	var query string

	if role == RoleMitra {
		query = `SELECT mitra_id FROM user_mgmt.master_mitra WHERE akun_id = $1`
	} else {
		query = `SELECT u.umkm_id FROM user_mgmt.master_umkm u
				JOIN user_mgmt.master_pelakuumkm p ON u.pelaku_umkm_id = p.pelaku_umkm_id
				WHERE p.akun_id = $1`
	}

	err := r.db.QueryRow(ctx, query, akunID).Scan(&businessID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrBusinessNotFound
		}
		return "", fmt.Errorf("failed to find business ID for akun_id %s: %w", akunID, err)
	}

	return businessID, nil
}

// FindUMKMDetail retrieves full detail of an UMKM by its umkm_id
func (r *repository) FindUMKMDetail(ctx context.Context, umkmID string) (*UMKMDetail, error) {
	query := `
		SELECT 
			u.umkm_id,
			u.nama_umkm,
			COALESCE(juk.nama_jenis_umkm, '') as type,
			COALESCE(l.kabupaten_kota, '') as city,
			COALESCE(l.provinsi, '') as province,
			COALESCE(u.deskripsi_usaha, '') as description,
			'' as operational_area,
			COALESCE(p.nama_pelaku, '') as owner_name,
			COALESCE(p.no_hp, '') as phone_number,
			COALESCE(p.email::text, '') as email,
			COALESCE(p.alamat, '') as address,
			COALESCE(u.produk_utama, '') as products,
			COALESCE(u.tahun_berdiri, 0) as year_established,
			COALESCE(u.media_sosial_marketplace, '') as social_media_marketplace,
			COALESCE(u.logo_url, '') as logo_url,
			COALESCE(u.foto_cover_url, '') as foto_cover_url
		FROM user_mgmt.master_umkm u
		LEFT JOIN ref.ref_jenisumkm juk ON u.jenis_umkm_id = juk.jenis_umkm_id
		LEFT JOIN user_mgmt.master_lokasi l ON u.lokasi_id = l.lokasi_id
		LEFT JOIN user_mgmt.master_pelakuumkm p ON u.pelaku_umkm_id = p.pelaku_umkm_id
		WHERE u.umkm_id = $1 AND u.is_deleted = false
	`

	var d UMKMDetail
	err := r.db.QueryRow(ctx, query, umkmID).Scan(
		&d.ID, &d.Name, &d.Type, &d.City, &d.Province,
		&d.Description, &d.OperationalArea,
		&d.OwnerName, &d.PhoneNumber, &d.Email, &d.Address,
		&d.Products, &d.YearEstablished, &d.SocialMediaMarketplace,
		&d.LogoURL, &d.FotoCoverURL,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("UMKM not found: %s", umkmID)
		}
		return nil, fmt.Errorf("failed to find UMKM detail: %w", err)
	}

	products, err := r.FindFeaturedProductsByUMKMID(ctx, umkmID)
	if err != nil {
		return nil, err
	}

	d.FeaturedProducts = products

	return &d, nil
}

func (r *repository) FindFeaturedProductsByUMKMID(ctx context.Context, umkmID string) ([]FeaturedProduct, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			p.produk_id,
			p.nama_produk,
			COALESCE(k.nama_kategori_produk, '') AS kategori,
			COALESCE(p.deskripsi_produk, '') AS deskripsi,
			p.harga,
			p.stok_saat_ini,
			COALESCE(p.thumbnail_url, '') AS thumbnail_url,
			COALESCE(p.legalitas_produk, '') AS legalitas
		FROM user_mgmt.master_produkumkm p
		LEFT JOIN ref.ref_kategoriproduk k
			ON k.kategori_produk_id = p.kategori_produk_id
		WHERE p.umkm_id = $1
		  AND p.tampil_di_profil = TRUE
		  AND p.status_produk = 'AKTIF'
		  AND p.is_deleted = FALSE
		ORDER BY
		  COALESCE(p.urutan_tampil_profil, 999),
		  p.featured_at DESC,
		  p.updated_at DESC
		LIMIT 5
	`, umkmID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := []FeaturedProduct{}

	for rows.Next() {
		var item FeaturedProduct
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.CategoryName,
			&item.Description,
			&item.Price,
			&item.Stock,
			&item.ThumbnailURL,
			&item.Legalitas,
		); err != nil {
			return nil, err
		}

		products = append(products, item)
	}

	return products, rows.Err()
}

// FindMitraDetail retrieves full detail of a Mitra by its mitra_id
func (r *repository) FindMitraDetail(ctx context.Context, mitraID string) (*MitraDetail, error) {
	query := `
		SELECT 
			m.mitra_id,
			m.nama_mitra,
			COALESCE(jm.nama_jenis_mitra, '') as type,
			COALESCE(l.kabupaten_kota, '') as city,
			COALESCE(l.provinsi, '') as province,
			COALESCE(m.deskripsi_dukungan, '') as description,
			COALESCE(m.wilayah_operasional, '') as operational_area,
			COALESCE(m.nama_pic, '') as contact_person,
			COALESCE(m.jabatan_pic, '') as contact_title,
			COALESCE(m.kontak_pic, '') as phone_number,
			COALESCE(m.email_pic::text, '') as email,
			COALESCE(m.alamat_mitra, '') as address,
			COALESCE(m.nama_badan_hukum, '') as legal_name,
			COALESCE(m.nib, '') as nib,
			COALESCE(m.npwp, '') as npwp,
			COALESCE(sks.nama_skala_kerjasama, '') as cooperation_scale
		FROM user_mgmt.master_mitra m
		LEFT JOIN ref.ref_jenismitra jm ON m.jenis_mitra_id = jm.jenis_mitra_id
		LEFT JOIN user_mgmt.master_lokasi l ON m.lokasi_id = l.lokasi_id
		LEFT JOIN ref.ref_skalakerjasama sks ON m.skala_kerjasama_id = sks.skala_kerjasama_id
		WHERE m.mitra_id = $1 AND m.is_deleted = false
	`

	var d MitraDetail
	err := r.db.QueryRow(ctx, query, mitraID).Scan(
		&d.ID, &d.Name, &d.Type, &d.City, &d.Province,
		&d.Description, &d.OperationalArea,
		&d.ContactPerson, &d.ContactTitle, &d.PhoneNumber, &d.Email,
		&d.Address, &d.LegalName, &d.NIB, &d.NPWP, &d.CooperationScale,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("Mitra not found: %s", mitraID)
		}
		return nil, fmt.Errorf("failed to find Mitra detail: %w", err)
	}

	return &d, nil
}

func (r *repository) CreateAttachments(ctx context.Context, partnershipID string, documentIDs []string) error {
	if len(documentIDs) == 0 {
		return nil
	}

	for index, documentID := range documentIDs {
		if documentID == "" {
			continue
		}

		attachmentType := "LAINNYA"
		switch index {
		case 0:
			attachmentType = "NIB_KTP"
		case 1:
			attachmentType = "PROPOSAL_KEMITRAAN"
		case 2:
			attachmentType = "SERTIFIKAT"
		}

		const query = `
			INSERT INTO partnership.transaksi_pengajuankerjasama_lampiran (
				pengajuan_id,
				dokumen_id,
				jenis_lampiran,
				nama_file,
				urutan,
				created_at
			)
			SELECT
				$1,
				d.dokumen_id,
				$2,
				d.original_filename,
				$3,
				NOW()
			FROM documents.master_dokumen d
			WHERE d.dokumen_id = $4
			  AND d.status = 'AKTIF'
			ON CONFLICT (pengajuan_id, dokumen_id) DO NOTHING
		`

		result, err := r.db.Exec(ctx, query, partnershipID, attachmentType, index+1, documentID)
		if err != nil {
			return fmt.Errorf("failed to create partnership attachment: %w", err)
		}

		if result.RowsAffected() == 0 {
			return fmt.Errorf("document attachment not found or inactive: %s", documentID)
		}
	}

	return nil
}

func (r *repository) FindAttachmentsByPartnershipID(ctx context.Context, partnershipID string) ([]PartnershipAttachment, error) {
	const query = `
		SELECT
			l.dokumen_id,
			l.jenis_lampiran,
			COALESCE(l.nama_file, ''),
			COALESCE(d.original_filename, ''),
			COALESCE(d.content_type, ''),
			COALESCE(d.size_bytes, 0),
			l.created_at
		FROM partnership.transaksi_pengajuankerjasama_lampiran l
		JOIN documents.master_dokumen d
		  ON d.dokumen_id = l.dokumen_id
		WHERE l.pengajuan_id = $1
		  AND d.status = 'AKTIF'
		ORDER BY l.urutan ASC, l.created_at ASC
	`

	rows, err := r.db.Query(ctx, query, partnershipID)
	if err != nil {
		return nil, fmt.Errorf("failed to find partnership attachments: %w", err)
	}
	defer rows.Close()

	attachments := make([]PartnershipAttachment, 0)

	for rows.Next() {
		var item PartnershipAttachment
		if err := rows.Scan(
			&item.DocumentID,
			&item.Type,
			&item.FileName,
			&item.OriginalFilename,
			&item.ContentType,
			&item.SizeBytes,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan partnership attachment: %w", err)
		}

		attachments = append(attachments, item)
	}

	return attachments, nil
}
