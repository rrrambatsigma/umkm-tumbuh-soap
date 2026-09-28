package partnerships

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/apperror"
)

var (
	pgjMu      sync.Mutex
	pgjCounter int
	pgjOnce    sync.Once
)

type Service interface {
	MarkPartnershipAsRead(ctx context.Context, id string) error
	GetPartnershipSummary(
		ctx context.Context,
		userID string,
	) (map[string]int, error)

	CreatePartnership(
		ctx context.Context,
		userID string,
		userRole UserRole,
		req CreatePartnershipRequest,
	) (*PartnershipResponse, error)

	GetPartnershipByID(
		ctx context.Context,
		id string,
	) (*PartnershipResponse, error)

	GetPartnershipsByRequester(
		ctx context.Context,
		requesterID string,
		status *PartnershipStatus,
		page,
		limit int,
	) ([]PartnershipListResponse, int, error)

	GetPartnershipsByReceiver(
		ctx context.Context,
		receiverID string,
		status *PartnershipStatus,
		page,
		limit int,
	) ([]PartnershipListResponse, int, error)

	UpdatePartnershipStatus(
		ctx context.Context,
		id string,
		req UpdatePartnershipStatus,
	) error

	SignPartnership(
		ctx context.Context,
		id string,
		req SignPartnershipRequest,
	) error

	ValidatePartnershipRequest(
		req CreatePartnershipRequest,
	) map[string]string

	GetUMKMList(
		ctx context.Context,
		search string,
		filterType string,
		page, limit int,
	) ([]UMKMListItem, int, error)

	GetMitraList(
		ctx context.Context,
		search string,
		filterType string,
		page, limit int,
	) ([]MitraListItem, int, error)

	GetUMKMDetail(
		ctx context.Context,
		umkmID string,
	) (*UMKMDetail, error)

	GetMitraDetail(
		ctx context.Context,
		mitraID string,
	) (*MitraDetail, error)

	GetIncomingPartnershipSummary(
		ctx context.Context,
		userID string,
	) (map[string]int, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{
		repo: repo,
	}
}

func (s *service) GetIncomingPartnershipSummary(
	ctx context.Context,
	userID string,
) (map[string]int, error) {
	actorID, err := partnershipScope(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.repo.GetIncomingSummary(ctx, actorID)
}

func (s *service) ValidatePartnershipRequest(
	req CreatePartnershipRequest,
) map[string]string {

	errs := make(map[string]string)

	if req.ReceiverID == "" {
		errs["receiver_id"] = "receiver_id wajib diisi"
	}

	if len(req.ProposalTitle) < 10 {
		errs["proposal_title"] = "proposal_title minimal 10 karakter"
	}

	if len(req.ProposalTitle) > 200 {
		errs["proposal_title"] = "proposal_title maksimal 200 karakter"
	}

	if len(req.ProposalDescription) < 30 {
		errs["proposal_description"] = "proposal_description minimal 30 karakter"
	}

	if len(req.ProposalDescription) > 1000 {
		errs["proposal_description"] = "proposal_description maksimal 1000 karakter"
	}

	return errs
}

func (s *service) CreatePartnership(
	ctx context.Context,
	userID string,
	userRole UserRole,
	req CreatePartnershipRequest,
) (*PartnershipResponse, error) {

	actorID, actorRole, err := partnershipActor(ctx)
	if err != nil {
		return nil, err
	}
	if actorID != userID || actorRole != userRole {
		return nil, apperror.New(http.StatusForbidden, "Identitas pengaju tidak sesuai sesi")
	}
	if validation := s.ValidatePartnershipRequest(req); len(validation) > 0 {
		return nil, apperror.New(http.StatusUnprocessableEntity, "Validation failed")
	}
	for _, documentID := range req.AttachmentFiles {
		owned, err := s.repo.OwnsDocument(ctx, actorID, documentID, false)
		if err != nil {
			return nil, err
		}
		if !owned {
			return nil, apperror.New(http.StatusForbidden, "Lampiran tidak tersedia atau bukan milik Anda")
		}
	}

	requestCode, err := s.repo.GenerateRequestCode(ctx)
	if err != nil {
		return nil, apperror.New(500, "failed to generate request code")
	}

	receiverRole := RoleMitra
	if userRole == RoleMitra {
		receiverRole = RoleUMKM
	}

	// Convert business ID (mitra_id/umkm_id) to akun_id
	receiverAkunID, err := s.repo.FindAkunIDByBusinessID(ctx, req.ReceiverID, receiverRole)
	if err != nil {
		if errors.Is(err, ErrBusinessNotFound) {
			return nil, apperror.New(400, "Penerima tidak ditemukan")
		}
		return nil, apperror.New(500, "Gagal mencari penerima")
	}

	// Look up requester's business ID and receiver's business ID for FK constraints
	// These may be empty for API-registered accounts without business profiles
	requesterBusinessID, err := s.repo.FindBusinessIDByAkunID(ctx, actorID, actorRole)
	if err != nil && !errors.Is(err, ErrBusinessNotFound) {
		return nil, apperror.New(500, "Gagal mencari profil pengaju")
	}
	receiverBusinessID := req.ReceiverID

	now := time.Now()

	partnership := &PartnershipRequest{
		ID:                   generatePGJID(ctx, s.repo),
		RequestCode:          requestCode,
		RequesterID:          userID,
		ReceiverID:           receiverAkunID,
		RequesterBusinessID:  requesterBusinessID,
		ReceiverBusinessID:   receiverBusinessID,
		RequesterRole:        userRole,
		ReceiverRole:         receiverRole,
		Category:             "default",
		ProposalTitle:        req.ProposalTitle,
		ProposalDescription:  req.ProposalDescription,
		BusinessName:         "",
		ContactPerson:        "",
		ProductDescription:   "",
		ReasonForPartnership: "",
		NIBKTPFile:           "",
		ProposalFile:         "",
		CertificateFile:      nil,
		Status:               StatusSubmitted,
		SubmittedAt:          &now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := s.repo.Create(ctx, partnership); err != nil {
		return nil, apperror.New(
			500,
			"failed to create partnership request",
		)
	}

	if len(req.AttachmentFiles) > 0 {
		if err := s.repo.CreateAttachments(ctx, partnership.ID, req.AttachmentFiles); err != nil {
			return nil, apperror.New(
				500,
				"failed to save partnership attachments",
			)
		}
	}

	created, err := s.repo.FindByID(ctx, partnership.ID)
	if err != nil {
		return nil, apperror.New(500, "Gagal mengambil pengajuan kemitraan")
	}
	return created, nil
}

func generatePGJID(ctx context.Context, repo Repository) string {
	pgjOnce.Do(func() {
		count, err := repo.CountAll(ctx)
		if err != nil {
			pgjCounter = 0
			return
		}
		pgjCounter = count
	})

	pgjMu.Lock()
	defer pgjMu.Unlock()
	pgjCounter++
	return fmt.Sprintf("PGJ%06d", pgjCounter)
}

func (s *service) GetPartnershipByID(
	ctx context.Context,
	id string,
) (*PartnershipResponse, error) {

	actorID, _, err := partnershipActor(ctx)
	if err != nil {
		return nil, err
	}
	partnership, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if partnership.RequesterID != actorID && partnership.ReceiverID != actorID {
		return nil, apperror.New(http.StatusForbidden, "Anda bukan pihak dalam pengajuan ini")
	}
	return partnership, nil
}

func (s *service) GetPartnershipsByRequester(
	ctx context.Context,
	requesterID string,
	status *PartnershipStatus,
	page,
	limit int,
) ([]PartnershipListResponse, int, error) {
	actorID, err := partnershipScope(ctx, requesterID)
	if err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}

	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit

	return s.repo.FindByRequesterID(
		ctx,
		actorID,
		status,
		limit,
		offset,
	)
}

func (s *service) GetPartnershipsByReceiver(
	ctx context.Context,
	receiverID string,
	status *PartnershipStatus,
	page,
	limit int,
) ([]PartnershipListResponse, int, error) {
	actorID, err := partnershipScope(ctx, receiverID)
	if err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}

	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit

	return s.repo.FindByReceiverID(
		ctx,
		actorID,
		status,
		limit,
		offset,
	)
}

func (s *service) UpdatePartnershipStatus(
	ctx context.Context,
	id string,
	req UpdatePartnershipStatus,
) error {
	partnership, err := s.GetPartnershipByID(ctx, id)
	if err != nil {
		return err
	}
	actorID, _, err := partnershipActor(ctx)
	if err != nil {
		return err
	}
	if err := authorizeStatusChange(partnership, actorID, req.Status); err != nil {
		return err
	}
	if req.Status == StatusRejected && (req.RejectionReason == nil || strings.TrimSpace(*req.RejectionReason) == "") {
		return apperror.New(http.StatusUnprocessableEntity, "Alasan penolakan wajib diisi")
	}
	return s.repo.UpdateStatus(ctx, id, actorID, partnership.Status, req.Status, req.RejectionReason, time.Now())
}

func (s *service) SignPartnership(
	ctx context.Context,
	id string,
	req SignPartnershipRequest,
) error {
	partnership, err := s.GetPartnershipByID(ctx, id)
	if err != nil {
		return err
	}
	actorID, _, err := partnershipActor(ctx)
	if err != nil {
		return err
	}
	if partnership.RequesterID != actorID {
		return apperror.New(http.StatusForbidden, "Hanya pengaju yang dapat mengunggah dokumen kontrak")
	}
	if partnership.Status != StatusSubmitted && partnership.Status != StatusReviewed && partnership.Status != StatusWaitingDoc {
		return apperror.New(http.StatusConflict, "Dokumen kontrak tidak dapat diubah pada status ini")
	}
	if strings.TrimSpace(req.DokumenKontrak) == "" {
		return apperror.New(http.StatusBadRequest, "dokumen_kontrak wajib diisi")
	}
	owned, err := s.repo.OwnsDocument(ctx, actorID, req.DokumenKontrak, true)
	if err != nil {
		return err
	}
	if !owned {
		return apperror.New(http.StatusForbidden, "Dokumen kontrak tidak tersedia atau bukan milik Anda")
	}
	return s.repo.UpdateContract(ctx, id, actorID, partnership.Status, req.DokumenKontrak, time.Now())
}

func (s *service) GetPartnershipSummary(ctx context.Context, userID string) (map[string]int, error) {
	actorID, err := partnershipScope(ctx, userID)
	if err != nil {
		return nil, err
	}
	summary, err := s.repo.GetSummary(ctx, actorID)
	if err != nil {
		return nil, apperror.New(500, "failed to get partnership summary: "+err.Error())
	}
	return summary, nil
}

// MarkPartnershipAsRead preserves the REST authorization-only behavior.
// There is deliberately no read-state or partnership-status mutation.
func (s *service) MarkPartnershipAsRead(ctx context.Context, id string) error {
	partnership, err := s.GetPartnershipByID(ctx, id)
	if err != nil {
		return err
	}
	actorID, _, err := partnershipActor(ctx)
	if err != nil {
		return err
	}
	if partnership.ReceiverID != actorID {
		return apperror.New(http.StatusForbidden, "Hanya penerima yang dapat menandai pengajuan dibaca")
	}
	return nil
}

// ============================================================
// NEW IMPLEMENTATIONS FOR UMKM AND MITRA LISTS
// ============================================================

// GetUMKMList retrieves a paginated list of UMKM
// This is called by MITRA to see available UMKM for partnership
func (s *service) GetUMKMList(
	ctx context.Context,
	search string,
	filterType string,
	page, limit int,
) ([]UMKMListItem, int, error) {
	// Validate and normalize pagination parameters
	if page < 1 {
		page = 1
	}

	if limit < 1 {
		limit = 10
	}

	if limit > 100 {
		limit = 100
	}

	// Calculate offset for database query
	offset := (page - 1) * limit

	// Fetch UMKM list from repository
	umkmList, totalCount, err := s.repo.FindUMKMList(ctx, search, filterType, limit, offset)
	if err != nil {
		return nil, 0, apperror.New(500, "failed to fetch UMKM list: "+err.Error())
	}

	return umkmList, totalCount, nil
}

// GetMitraList retrieves a paginated list of Mitra
// This is called by UMKM to see available Mitra for partnership
func (s *service) GetMitraList(
	ctx context.Context,
	search string,
	filterType string,
	page, limit int,
) ([]MitraListItem, int, error) {
	// Validate and normalize pagination parameters
	if page < 1 {
		page = 1
	}

	if limit < 1 {
		limit = 10
	}

	if limit > 100 {
		limit = 100
	}

	// Calculate offset for database query
	offset := (page - 1) * limit

	// Fetch Mitra list from repository
	mitraList, totalCount, err := s.repo.FindMitraList(ctx, search, filterType, limit, offset)
	if err != nil {
		return nil, 0, apperror.New(500, "failed to fetch Mitra list: "+err.Error())
	}

	return mitraList, totalCount, nil
}

// GetUMKMDetail retrieves full detail of an UMKM
func (s *service) GetUMKMDetail(ctx context.Context, umkmID string) (*UMKMDetail, error) {
	detail, err := s.repo.FindUMKMDetail(ctx, umkmID)
	if err != nil {
		return nil, apperror.New(404, err.Error())
	}
	return detail, nil
}

// GetMitraDetail retrieves full detail of a Mitra
func (s *service) GetMitraDetail(ctx context.Context, mitraID string) (*MitraDetail, error) {
	detail, err := s.repo.FindMitraDetail(ctx, mitraID)
	if err != nil {
		return nil, apperror.New(404, err.Error())
	}
	return detail, nil
}
