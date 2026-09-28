package partnerships

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	counterMu  sync.Mutex
	pgjCounter int
	pgjOnce    sync.Once
)

// Service contains all business logic for the 9 SOAP partnership operations.
//
// Authorization model:
//   - The SOAP transport verifies JWT authentication and replaces the legacy
//     XML userID + userRole fields with the authenticated actor before entry.
//   - userRole ("UMKM" or "MITRA") must be one of the two valid roles.
//   - Actor-level authorization (who can approve/reject/cancel/sign) is enforced
//     by matching userID against the DB's pengaju_akun_id or penerima_akun_id.
//     This is done in SQL (UpdateStatus, MarkAsRead, UpdateContract) for defence-in-depth.
type Service struct {
	repo *Repository
}

// NewService creates a new Service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// -------------------------------------------------------
// Helpers
// -------------------------------------------------------

// validateRole ensures the given role is UMKM or MITRA.
func validateRole(role string) error {
	if role != "UMKM" && role != "MITRA" {
		return newError(400, "userRole harus 'UMKM' atau 'MITRA'")
	}
	return nil
}

// oppositeRole returns MITRA for UMKM and vice versa.
func oppositeRole(role string) string {
	if role == "UMKM" {
		return "MITRA"
	}
	return "UMKM"
}

// generateID creates a sequential PGJ-format application ID.
func (s *Service) generateID(ctx context.Context) string {
	pgjOnce.Do(func() {
		if count, err := s.repo.CountAll(ctx); err == nil {
			pgjCounter = count
		}
	})
	counterMu.Lock()
	defer counterMu.Unlock()
	pgjCounter++
	return fmt.Sprintf("PGJ%06d", pgjCounter)
}

// generateRequestCode generates PKS-YYYY-NNNNNN format code.
func (s *Service) generateRequestCode(ctx context.Context) (string, error) {
	year := time.Now().Format("2006")
	count, err := s.repo.CountByYear(ctx, year)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("PKS-%s-%06d", year, count+1), nil
}

// -------------------------------------------------------
// 1. CreatePartnershipApplication
// -------------------------------------------------------

// CreateResult is returned by CreatePartnershipApplication.
type CreateResult struct {
	ApplicationID string
	RequestCode   string
	Status        string
}

// CreatePartnershipApplication creates a new partnership request.
// Either UMKM or MITRA can initiate — the receiver role is the opposite.
func (s *Service) CreatePartnershipApplication(ctx context.Context, req *CreatePartnershipApplicationRequest) (*CreateResult, error) {
	if err := validateRole(req.UserRole); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.ReceiverID) == "" {
		return nil, newError(400, "receiverId wajib diisi")
	}
	if len(strings.TrimSpace(req.ProposalTitle)) < 10 {
		return nil, newError(400, "proposalTitle minimal 10 karakter")
	}
	if len(strings.TrimSpace(req.ProposalTitle)) > 200 {
		return nil, newError(400, "proposalTitle maksimal 200 karakter")
	}
	if len(strings.TrimSpace(req.ProposalDescription)) < 30 {
		return nil, newError(400, "proposalDescription minimal 30 karakter")
	}
	if len(strings.TrimSpace(req.ProposalDescription)) > 1000 {
		return nil, newError(400, "proposalDescription maksimal 1000 karakter")
	}

	// receiverID in the SOAP request is the business ID (mitra_id or umkm_id).
	// Resolve it to the internal akun_id using the receiver's role.
	receiverRole := oppositeRole(req.UserRole)
	receiverAkunID, err := s.repo.FindReceiverAkunID(ctx, req.ReceiverID, receiverRole)
	if err != nil {
		return nil, err
	}

	id := s.generateID(ctx)
	requestCode, err := s.generateRequestCode(ctx)
	if err != nil {
		return nil, newError(500, "Gagal membuat kode pengajuan: "+err.Error())
	}

	// Combine proposal title + description into pesan_pengajuan.
	// The DB column pesan_pengajuan stores the full proposal text.
	proposalText := req.ProposalTitle + "\n\n" + req.ProposalDescription

	now := time.Now()
	if err := s.repo.Create(ctx, CreateInput{
		ID:                 id,
		RequestCode:        requestCode,
		RequesterID:        req.UserID,
		ReceiverID:         receiverAkunID,
		RequesterRole:      req.UserRole,
		ReceiverBusinessID: req.ReceiverID, // original business ID for FK
		ProposalText:       proposalText,
		SubmittedAt:        now,
	}); err != nil {
		return nil, newError(500, "Gagal menyimpan pengajuan: "+err.Error())
	}

	return &CreateResult{
		ApplicationID: id,
		RequestCode:   requestCode,
		Status:        string(StatusSubmitted),
	}, nil
}

// -------------------------------------------------------
// 2. GetPartnershipApplication
// -------------------------------------------------------

// GetPartnershipApplication returns the full detail of one partnership.
// Authorization: the caller must be either the requester or the receiver.
// This is enforced in the SQL query (FindByID with actorID parameter).
func (s *Service) GetPartnershipApplication(ctx context.Context, req *GetPartnershipApplicationRequest) (*PartnershipRow, error) {
	if err := validateRole(req.UserRole); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.UserID) == "" {
		return nil, newError(400, "userId wajib diisi")
	}
	if strings.TrimSpace(req.ApplicationID) == "" {
		return nil, newError(400, "applicationId wajib diisi")
	}
	// FindByID enforces: WHERE pengajuan_id=$1 AND (pengaju_akun_id=$2 OR penerima_akun_id=$2)
	return s.repo.FindByID(ctx, req.ApplicationID, req.UserID)
}

// -------------------------------------------------------
// 3. GetPartnershipApplications (outgoing / requester view)
// -------------------------------------------------------

// GetPartnershipApplicationsResult is returned by GetPartnershipApplications.
type GetPartnershipApplicationsResult struct {
	Applications []PartnershipListRow
	TotalCount   int
}

// GetPartnershipApplications returns paginated outgoing applications for a user.
// The user only sees their own applications (WHERE pengaju_akun_id = userID).
func (s *Service) GetPartnershipApplications(ctx context.Context, req *GetPartnershipApplicationsRequest) (*GetPartnershipApplicationsResult, error) {
	if err := validateRole(req.UserRole); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.UserID) == "" {
		return nil, newError(400, "userId wajib diisi")
	}

	page, limit := normalizePagination(req.Page, req.Limit)
	offset := (page - 1) * limit

	var statusPtr *PartnershipStatus
	if req.Status != "" {
		if !IsValidStatus(req.Status) {
			return nil, newError(400, "status tidak valid: "+req.Status)
		}
		s := PartnershipStatus(req.Status)
		statusPtr = &s
	}

	list, total, err := s.repo.FindByRequesterID(ctx, req.UserID, statusPtr, limit, offset)
	if err != nil {
		return nil, newError(500, "Gagal mengambil daftar pengajuan: "+err.Error())
	}
	return &GetPartnershipApplicationsResult{Applications: list, TotalCount: total}, nil
}

// -------------------------------------------------------
// 4. GetIncomingPartnerships (receiver / inbox view)
// -------------------------------------------------------

// GetIncomingPartnershipsResult is returned by GetIncomingPartnerships.
type GetIncomingPartnershipsResult struct {
	Applications []PartnershipListRow
	TotalCount   int
}

// GetIncomingPartnerships returns paginated incoming applications for a receiver.
// The user only sees applications directed to them (WHERE penerima_akun_id = userID).
func (s *Service) GetIncomingPartnerships(ctx context.Context, req *GetIncomingPartnershipsRequest) (*GetIncomingPartnershipsResult, error) {
	if err := validateRole(req.UserRole); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.UserID) == "" {
		return nil, newError(400, "userId wajib diisi")
	}

	page, limit := normalizePagination(req.Page, req.Limit)
	offset := (page - 1) * limit

	var statusPtr *PartnershipStatus
	if req.Status != "" {
		if !IsValidStatus(req.Status) {
			return nil, newError(400, "status tidak valid: "+req.Status)
		}
		s := PartnershipStatus(req.Status)
		statusPtr = &s
	}

	list, total, err := s.repo.FindByReceiverID(ctx, req.UserID, statusPtr, limit, offset)
	if err != nil {
		return nil, newError(500, "Gagal mengambil pengajuan masuk: "+err.Error())
	}
	return &GetIncomingPartnershipsResult{Applications: list, TotalCount: total}, nil
}

// -------------------------------------------------------
// 5. UpdatePartnershipStatus
// -------------------------------------------------------

// UpdatePartnershipStatus changes the status of a partnership.
//
// Authorization (enforced in SQL, matching existing REST service):
//   - AKTIF   → receiver (penerima_akun_id) from DIAJUKAN or DITINJAU
//   - DITOLAK → receiver (penerima_akun_id) from DIAJUKAN or DITINJAU; rejectionReason required
//   - DIBATALKAN → requester (pengaju_akun_id) from DRAFT, DIAJUKAN, or DITINJAU
//
// If the userID is not the correct actor for the requested transition,
// the SQL UPDATE affects 0 rows → returns a conflict error (not a forbidden error,
// to avoid leaking whether the record exists).
func (s *Service) UpdatePartnershipStatus(ctx context.Context, req *UpdatePartnershipStatusRequest) error {
	if err := validateRole(req.UserRole); err != nil {
		return err
	}
	if strings.TrimSpace(req.ApplicationID) == "" {
		return newError(400, "applicationId wajib diisi")
	}
	if !IsValidUpdateStatus(req.Status) {
		return newError(400, "status harus salah satu dari: AKTIF (setuju), DITOLAK (tolak), DIBATALKAN (batal)")
	}

	newStatus := PartnershipStatus(req.Status)

	// Rejection requires a reason — same rule as existing REST service.
	if newStatus == StatusRejected && strings.TrimSpace(req.RejectionReason) == "" {
		return newError(422, "rejectionReason wajib diisi saat menolak pengajuan")
	}

	var rejectionReason *string
	if req.RejectionReason != "" {
		r := req.RejectionReason
		rejectionReason = &r
	}

	return s.repo.UpdateStatus(ctx, req.ApplicationID, req.UserID, newStatus, rejectionReason)
}

// -------------------------------------------------------
// 6. MarkPartnershipAsRead
// -------------------------------------------------------

// MarkPartnershipAsRead marks a partnership as read by the receiver.
//
// Prototype design note:
// In the original REST service, PATCH /partnerships/:id/read does NOT change status.
// Because the DB has no is_read column, this SOAP prototype maps the operation to
// DIAJUKAN → DITINJAU (a legitimate next state). Only the receiver may do this.
// Authorization is enforced in SQL: AND penerima_akun_id = $receiverID.
func (s *Service) MarkPartnershipAsRead(ctx context.Context, req *MarkPartnershipAsReadRequest) error {
	if err := validateRole(req.UserRole); err != nil {
		return err
	}
	if strings.TrimSpace(req.ApplicationID) == "" {
		return newError(400, "applicationId wajib diisi")
	}
	return s.repo.MarkAsRead(ctx, req.ApplicationID, req.UserID)
}

// -------------------------------------------------------
// 7. SignPartnership
// -------------------------------------------------------

// SignPartnership attaches a contract document to a partnership.
//
// Authorization: only the requester (pengaju_akun_id) may sign.
// Derived from existing service.go: "Hanya pengaju yang dapat mengunggah dokumen kontrak".
// Valid source statuses: DIAJUKAN, DITINJAU, MENUNGGU_DOKUMEN_TTD.
//
// Prototype note: document ownership is NOT validated against document-service
// to keep the prototype self-contained.
func (s *Service) SignPartnership(ctx context.Context, req *SignPartnershipRequest) error {
	if err := validateRole(req.UserRole); err != nil {
		return err
	}
	if strings.TrimSpace(req.ApplicationID) == "" {
		return newError(400, "applicationId wajib diisi")
	}
	if strings.TrimSpace(req.DocumentID) == "" {
		return newError(400, "documentId wajib diisi")
	}
	// Authorization is enforced in SQL: AND pengaju_akun_id = $requesterID
	return s.repo.UpdateContract(ctx, req.ApplicationID, req.UserID, req.DocumentID)
}

// -------------------------------------------------------
// 8. GetPartnershipSummary
// -------------------------------------------------------

// GetPartnershipSummary returns counts per status for a user's outgoing applications.
// Only returns data for the requesting user (WHERE pengaju_akun_id = userID).
func (s *Service) GetPartnershipSummary(ctx context.Context, req *GetPartnershipSummaryRequest) (map[string]int, error) {
	if err := validateRole(req.UserRole); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.UserID) == "" {
		return nil, newError(400, "userId wajib diisi")
	}
	return s.repo.GetSummary(ctx, req.UserID)
}

// -------------------------------------------------------
// 9. GetIncomingPartnershipSummary
// -------------------------------------------------------

// GetIncomingPartnershipSummary returns counts per status for a user's incoming applications.
// Only returns data for the requesting user (WHERE penerima_akun_id = userID).
func (s *Service) GetIncomingPartnershipSummary(ctx context.Context, req *GetIncomingPartnershipSummaryRequest) (map[string]int, error) {
	if err := validateRole(req.UserRole); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.UserID) == "" {
		return nil, newError(400, "userId wajib diisi")
	}
	return s.repo.GetIncomingSummary(ctx, req.UserID)
}

// -------------------------------------------------------
// Shared helpers
// -------------------------------------------------------

func normalizePagination(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return page, limit
}
