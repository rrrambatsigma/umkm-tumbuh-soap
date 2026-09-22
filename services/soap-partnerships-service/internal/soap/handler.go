package soap

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/partnerships"
)

const (
	DefaultMaxRequestBytes int64 = 1 << 20
	DefaultRequestTimeout        = 10 * time.Second
)

type partnershipService interface {
	CreatePartnershipApplication(context.Context, *partnerships.CreatePartnershipApplicationRequest) (*partnerships.CreateResult, error)
	GetPartnershipApplication(context.Context, *partnerships.GetPartnershipApplicationRequest) (*partnerships.PartnershipRow, error)
	GetPartnershipApplications(context.Context, *partnerships.GetPartnershipApplicationsRequest) (*partnerships.GetPartnershipApplicationsResult, error)
	GetIncomingPartnerships(context.Context, *partnerships.GetIncomingPartnershipsRequest) (*partnerships.GetIncomingPartnershipsResult, error)
	UpdatePartnershipStatus(context.Context, *partnerships.UpdatePartnershipStatusRequest) error
	MarkPartnershipAsRead(context.Context, *partnerships.MarkPartnershipAsReadRequest) error
	SignPartnership(context.Context, *partnerships.SignPartnershipRequest) error
	GetPartnershipSummary(context.Context, *partnerships.GetPartnershipSummaryRequest) (map[string]int, error)
	GetIncomingPartnershipSummary(context.Context, *partnerships.GetIncomingPartnershipSummaryRequest) (map[string]int, error)
}

// HandlerConfig controls transport-level security limits.
type HandlerConfig struct {
	JWTSecret       string
	MaxRequestBytes int64
	RequestTimeout  time.Duration
	Logger          *log.Logger
}

// Handler is the SOAP HTTP handler that routes all 9 partnership operations.
type Handler struct {
	svc             partnershipService
	wsdl            string
	jwtSecret       string
	maxRequestBytes int64
	requestTimeout  time.Duration
	logger          *log.Logger
}

// NewHandler creates a new SOAP handler.
func NewHandler(svc partnershipService, wsdl string) *Handler {
	return NewHandlerWithConfig(svc, wsdl, HandlerConfig{JWTSecret: os.Getenv("JWT_SECRET")})
}

// NewHandlerWithConfig creates a SOAP handler with explicit transport configuration.
func NewHandlerWithConfig(svc partnershipService, wsdl string, cfg HandlerConfig) *Handler {
	if cfg.MaxRequestBytes <= 0 {
		cfg.MaxRequestBytes = DefaultMaxRequestBytes
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = DefaultRequestTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	return &Handler{
		svc:             svc,
		wsdl:            wsdl,
		jwtSecret:       cfg.JWTSecret,
		maxRequestBytes: cfg.MaxRequestBytes,
		requestTimeout:  cfg.RequestTimeout,
		logger:          cfg.Logger,
	}
}

// ServeHTTP routes WSDL queries and SOAP POST requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// WSDL is public only for GET requests.
	if r.Method == http.MethodGet {
		if _, ok := r.URL.Query()["wsdl"]; !ok {
			http.Error(w, "Method Not Allowed — use POST for SOAP requests", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		io.WriteString(w, h.wsdl) //nolint:errcheck
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed — use POST for SOAP requests", http.StatusMethodNotAllowed)
		return
	}
	defer h.recoverPanic(w)

	actor, err := authenticateBearer(r.Header.Get("Authorization"), h.jwtSecret)
	if err != nil {
		var authErr *authenticationError
		if errors.As(err, &authErr) && authErr.serverMisconfigured {
			h.logError("authentication configuration", err)
			WriteServerFault(w, "Internal server error")
			return
		}
		WriteUnauthorizedFault(w, "Token autentikasi tidak valid")
		return
	}
	if actor.Role == "ADMIN" {
		WriteForbiddenFault(w, "Role tidak diizinkan untuk operasi kemitraan")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()
	r = r.WithContext(ctx)

	r.Body = http.MaxBytesReader(w, r.Body, h.maxRequestBytes)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			WritePayloadTooLargeFault(w, "Request SOAP melebihi batas ukuran")
			return
		}
		h.logError("read request body", err)
		WriteServerFault(w, "Gagal membaca request body")
		return
	}

	var env Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		h.logError("decode SOAP envelope", err)
		WriteClientFault(w, "SOAP Envelope tidak valid")
		return
	}

	operation := h.detectOperation(r, env.Body.Content)
	if operation == "" {
		WriteClientFault(w, "Operasi tidak dikenali. Tambahkan SOAPAction header atau pastikan tag XML sesuai WSDL.")
		return
	}

	h.logger.Printf("[SOAP] operation=%q remote=%q", operation, r.RemoteAddr)
	h.dispatch(w, r, operation, env.Body.Content, actor)
}

// detectOperation determines which SOAP operation is being invoked.
// Priority: SOAPAction header → root XML element name in the body.
func (h *Handler) detectOperation(r *http.Request, bodyContent []byte) string {
	action := strings.Trim(r.Header.Get("SOAPAction"), "\"")
	if action != "" {
		parts := strings.Split(action, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	d := xml.NewDecoder(strings.NewReader(string(bodyContent)))
	for {
		tok, err := d.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local
		}
	}
}

// dispatch routes to the correct handler method based on the operation name.
func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request, operation string, bodyContent []byte, actor authenticatedActor) {
	ctx := r.Context()

	switch operation {

	case "CreatePartnershipApplication":
		var req partnerships.CreatePartnershipApplicationRequest
		if err := xml.Unmarshal(bodyContent, &req); err != nil {
			h.writeInvalidRequest(w, operation, err)
			return
		}
		if !bindAuthenticatedIdentity(w, &req.UserID, &req.UserRole, actor) {
			return
		}
		result, err := h.svc.CreatePartnershipApplication(ctx, &req)
		if err != nil {
			h.writeError(w, err)
			return
		}
		h.writeResponse(w, partnerships.CreatePartnershipApplicationResponse{
			Success:       true,
			ApplicationID: result.ApplicationID,
			RequestCode:   result.RequestCode,
			Status:        result.Status,
			Message:       "Pengajuan kemitraan berhasil dibuat",
		})

	case "GetPartnershipApplication":
		var req partnerships.GetPartnershipApplicationRequest
		if err := xml.Unmarshal(bodyContent, &req); err != nil {
			h.writeInvalidRequest(w, operation, err)
			return
		}
		if !bindAuthenticatedIdentity(w, &req.UserID, &req.UserRole, actor) {
			return
		}
		row, err := h.svc.GetPartnershipApplication(ctx, &req)
		if err != nil {
			h.writeError(w, err)
			return
		}
		h.writeResponse(w, partnerships.PartnershipDetail{
			ApplicationID:      row.ID,
			RequestCode:        row.RequestCode,
			RequesterID:        row.RequesterID,
			ReceiverID:         row.ReceiverID,
			RequesterName:      row.RequesterName,
			ReceiverName:       row.ReceiverName,
			ProposalText:       row.ProposalText,
			Status:             string(row.Status),
			RejectionReason:    derefStr(row.RejectionReason),
			ContractDocumentID: derefStr(row.ContractDocumentID),
			SubmittedAt:        fmtTime(row.SubmittedAt),
			DecidedAt:          fmtTime(row.DecidedAt),
			CreatedAt:          row.CreatedAt.Format(time.RFC3339),
			UpdatedAt:          row.UpdatedAt.Format(time.RFC3339),
		})

	case "GetPartnershipApplications":
		var req partnerships.GetPartnershipApplicationsRequest
		if err := xml.Unmarshal(bodyContent, &req); err != nil {
			h.writeInvalidRequest(w, operation, err)
			return
		}
		if !bindAuthenticatedIdentity(w, &req.UserID, &req.UserRole, actor) {
			return
		}
		result, err := h.svc.GetPartnershipApplications(ctx, &req)
		if err != nil {
			h.writeError(w, err)
			return
		}
		h.writeResponse(w, partnerships.GetPartnershipApplicationsResponse{
			TotalCount:   result.TotalCount,
			Applications: toListItems(result.Applications),
		})

	case "GetIncomingPartnerships":
		var req partnerships.GetIncomingPartnershipsRequest
		if err := xml.Unmarshal(bodyContent, &req); err != nil {
			h.writeInvalidRequest(w, operation, err)
			return
		}
		if !bindAuthenticatedIdentity(w, &req.UserID, &req.UserRole, actor) {
			return
		}
		result, err := h.svc.GetIncomingPartnerships(ctx, &req)
		if err != nil {
			h.writeError(w, err)
			return
		}
		h.writeResponse(w, partnerships.GetIncomingPartnershipsResponse{
			TotalCount:   result.TotalCount,
			Applications: toListItems(result.Applications),
		})

	case "UpdatePartnershipStatus":
		var req partnerships.UpdatePartnershipStatusRequest
		if err := xml.Unmarshal(bodyContent, &req); err != nil {
			h.writeInvalidRequest(w, operation, err)
			return
		}
		if !bindAuthenticatedIdentity(w, &req.UserID, &req.UserRole, actor) {
			return
		}
		if err := h.svc.UpdatePartnershipStatus(ctx, &req); err != nil {
			h.writeError(w, err)
			return
		}
		h.writeResponse(w, partnerships.UpdatePartnershipStatusResponse{
			Success:       true,
			ApplicationID: req.ApplicationID,
			Status:        req.Status,
			Message:       "Status pengajuan berhasil diperbarui",
		})

	case "MarkPartnershipAsRead":
		var req partnerships.MarkPartnershipAsReadRequest
		if err := xml.Unmarshal(bodyContent, &req); err != nil {
			h.writeInvalidRequest(w, operation, err)
			return
		}
		if !bindAuthenticatedIdentity(w, &req.UserID, &req.UserRole, actor) {
			return
		}
		if err := h.svc.MarkPartnershipAsRead(ctx, &req); err != nil {
			h.writeError(w, err)
			return
		}
		h.writeResponse(w, partnerships.MarkPartnershipAsReadResponse{
			Success:       true,
			ApplicationID: req.ApplicationID,
			Message:       "Pengajuan berhasil ditandai sebagai sudah dibaca (status: DITINJAU)",
		})

	case "SignPartnership":
		var req partnerships.SignPartnershipRequest
		if err := xml.Unmarshal(bodyContent, &req); err != nil {
			h.writeInvalidRequest(w, operation, err)
			return
		}
		if !bindAuthenticatedIdentity(w, &req.UserID, &req.UserRole, actor) {
			return
		}
		if err := h.svc.SignPartnership(ctx, &req); err != nil {
			h.writeError(w, err)
			return
		}
		h.writeResponse(w, partnerships.SignPartnershipResponse{
			Success:       true,
			ApplicationID: req.ApplicationID,
			Message:       "Dokumen kontrak berhasil diunggah",
		})

	case "GetPartnershipSummary":
		var req partnerships.GetPartnershipSummaryRequest
		if err := xml.Unmarshal(bodyContent, &req); err != nil {
			h.writeInvalidRequest(w, operation, err)
			return
		}
		if !bindAuthenticatedIdentity(w, &req.UserID, &req.UserRole, actor) {
			return
		}
		summary, err := h.svc.GetPartnershipSummary(ctx, &req)
		if err != nil {
			h.writeError(w, err)
			return
		}
		h.writeResponse(w, partnerships.GetPartnershipSummaryResponse{
			Items: toSummaryItems(summary),
		})

	case "GetIncomingPartnershipSummary":
		var req partnerships.GetIncomingPartnershipSummaryRequest
		if err := xml.Unmarshal(bodyContent, &req); err != nil {
			h.writeInvalidRequest(w, operation, err)
			return
		}
		if !bindAuthenticatedIdentity(w, &req.UserID, &req.UserRole, actor) {
			return
		}
		summary, err := h.svc.GetIncomingPartnershipSummary(ctx, &req)
		if err != nil {
			h.writeError(w, err)
			return
		}
		h.writeResponse(w, partnerships.GetIncomingPartnershipSummaryResponse{
			Items: toSummaryItems(summary),
		})

	default:
		WriteClientFault(w, "Operasi SOAP tidak dikenali")
	}
}

// writeResponse serializes content into a SOAP response envelope.
func (h *Handler) writeResponse(w http.ResponseWriter, content interface{}) {
	env := NewResponseEnvelope(content)
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(xml.Header)) //nolint:errcheck
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	enc.Encode(env) //nolint:errcheck
}

// writeError maps an AppError to the appropriate SOAP Fault.
func (h *Handler) writeError(w http.ResponseWriter, err error) {
	var appErr *partnerships.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case 404:
			WriteNotFoundFault(w, appErr.Message)
		case 409:
			WriteConflictFault(w, appErr.Message)
		case 422:
			WriteUnprocessableFault(w, appErr.Message)
		case 400:
			WriteClientFault(w, appErr.Message)
		case 403:
			WriteForbiddenFault(w, appErr.Message)
		default:
			h.logError("service", err)
			WriteServerFault(w, "Internal server error")
		}
		return
	}
	h.logError("service", err)
	WriteServerFault(w, "Internal server error")
}

func (h *Handler) writeInvalidRequest(w http.ResponseWriter, operation string, err error) {
	h.logError("decode "+operation, err)
	WriteClientFault(w, "Request SOAP tidak valid")
}

func (h *Handler) logError(event string, err error) {
	if h.logger != nil {
		h.logger.Printf("[SOAP] event=%q error=%v", event, err)
	}
}

func (h *Handler) recoverPanic(w http.ResponseWriter) {
	if recovered := recover(); recovered != nil {
		if h.logger != nil {
			h.logger.Printf("[SOAP] event=%q recovered_type=%T", "panic", recovered)
		}
		WriteServerFault(w, "Internal server error")
	}
}

func bindAuthenticatedIdentity(w http.ResponseWriter, userID, userRole *string, actor authenticatedActor) bool {
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*userRole) == "" {
		WriteClientFault(w, "userId dan userRole wajib diisi sesuai kontrak WSDL")
		return false
	}
	if *userID != actor.UserID || *userRole != actor.Role {
		WriteForbiddenFault(w, "Identitas request tidak sesuai token autentikasi")
		return false
	}

	// XML identity fields remain for WSDL compatibility only. The values passed
	// to business logic always come from the verified token.
	*userID = actor.UserID
	*userRole = actor.Role
	return true
}

// -------------------------------------------------------
// Helpers
// -------------------------------------------------------

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func toListItems(rows []partnerships.PartnershipListRow) []partnerships.PartnershipListItem {
	items := make([]partnerships.PartnershipListItem, 0, len(rows))
	for _, r := range rows {
		item := partnerships.PartnershipListItem{
			ApplicationID:         r.ID,
			RequestCode:           r.RequestCode,
			RequesterName:         r.RequesterName,
			ReceiverName:          r.ReceiverName,
			RequesterBusinessName: r.RequesterBusinessName,
			ReceiverBusinessName:  r.ReceiverBusinessName,
			ProposalTitle:         r.ProposalTitle,
			Status:                string(r.Status),
			SubmittedAt:           fmtTime(r.SubmittedAt),
			DecidedAt:             fmtTime(r.DecidedAt),
		}
		items = append(items, item)
	}
	return items
}

func toSummaryItems(m map[string]int) []partnerships.SummaryItem {
	items := make([]partnerships.SummaryItem, 0, len(m))
	for status, count := range m {
		items = append(items, partnerships.SummaryItem{Status: status, Count: count})
	}
	return items
}
