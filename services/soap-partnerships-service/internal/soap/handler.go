package soap

import (
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/partnerships"
)

// Handler is the SOAP HTTP handler that routes all 9 partnership operations.
type Handler struct {
	svc  *partnerships.Service
	wsdl string
}

// NewHandler creates a new SOAP handler.
func NewHandler(svc *partnerships.Service, wsdl string) *Handler {
	return &Handler{svc: svc, wsdl: wsdl}
}

// ServeHTTP routes WSDL queries and SOAP POST requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Serve WSDL when ?wsdl is in the query string
	if _, ok := r.URL.Query()["wsdl"]; ok {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		io.WriteString(w, h.wsdl) //nolint:errcheck
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed — use POST for SOAP requests", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteServerFault(w, "Gagal membaca request body")
		return
	}
	defer r.Body.Close()

	var env Envelope
	if err := xml.Unmarshal(body, &env); err != nil {
		WriteClientFault(w, "SOAP Envelope tidak valid: "+err.Error())
		return
	}

	operation := h.detectOperation(r, env.Body.Content)
	if operation == "" {
		WriteClientFault(w, "Operasi tidak dikenali. Tambahkan SOAPAction header atau pastikan tag XML sesuai WSDL.")
		return
	}

	log.Printf("[SOAP] operation=%s remote=%s", operation, r.RemoteAddr)
	h.dispatch(w, r, operation, env.Body.Content)
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
func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request, operation string, bodyContent []byte) {
	ctx := r.Context()

	switch operation {

	case "CreatePartnershipApplication":
		var req partnerships.CreatePartnershipApplicationRequest
		if err := xml.Unmarshal(bodyContent, &req); err != nil {
			WriteClientFault(w, "Gagal parse request: "+err.Error())
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
			WriteClientFault(w, "Gagal parse request: "+err.Error())
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
			WriteClientFault(w, "Gagal parse request: "+err.Error())
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
			WriteClientFault(w, "Gagal parse request: "+err.Error())
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
			WriteClientFault(w, "Gagal parse request: "+err.Error())
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
			WriteClientFault(w, "Gagal parse request: "+err.Error())
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
			WriteClientFault(w, "Gagal parse request: "+err.Error())
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
			WriteClientFault(w, "Gagal parse request: "+err.Error())
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
			WriteClientFault(w, "Gagal parse request: "+err.Error())
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
		WriteClientFault(w, "Operasi tidak dikenali: "+operation)
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
	if appErr, ok := err.(*partnerships.AppError); ok {
		switch appErr.Code {
		case 404:
			WriteNotFoundFault(w, appErr.Message)
		case 409:
			WriteConflictFault(w, appErr.Message)
		case 422:
			WriteUnprocessableFault(w, appErr.Message)
		case 400, 403:
			WriteClientFault(w, appErr.Message)
		default:
			WriteServerFault(w, appErr.Message)
		}
		return
	}
	WriteServerFault(w, "Internal server error: "+err.Error())
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
