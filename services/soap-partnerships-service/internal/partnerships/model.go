package partnerships

import (
	"encoding/xml"
	"time"
)

// -------------------------------------------------------
// Status constants (matching DB ref.ref_statuspengajuan exactly)
// -------------------------------------------------------

// PartnershipStatus represents partnership application statuses stored in the DB.
//
// IMPORTANT: There is NO "APPROVED" status in the database.
// When a receiver approves, the REST handler sets status to "AKTIF".
// This was confirmed by reading handler.go: req.Status = "AKTIF" in ApprovePartnership.
//
// Full state machine (derived from existing REST code + SQL):
//
//	DRAFT
//	  ↓ (submitted)
//	DIAJUKAN
//	  ↓ (MarkAsRead — prototype workaround: SOAP maps this to DIAJUKAN→DITINJAU.
//	  |  In REST, /read only verifies receiver identity, does NOT change status,
//	  |  because the DB has no is_read column.)
//	DITINJAU
//	  ├── DITOLAK     (receiver rejects, requires rejectionReason)
//	  ├── DIBATALKAN  (requester cancels)
//	  └── AKTIF       (receiver approves)
//	         ↓ (SignPartnership — upload signed contract document)
//	  MENUNGGU_DOKUMEN_TTD
//	         ↓ (future flow, outside SOAP prototype scope)
//	       SELESAI
type PartnershipStatus string

const (
	StatusDraft      PartnershipStatus = "DRAFT"
	StatusSubmitted  PartnershipStatus = "DIAJUKAN"
	StatusReviewed   PartnershipStatus = "DITINJAU"           // Marked as read by receiver
	StatusActive     PartnershipStatus = "AKTIF"              // Approved by receiver
	StatusRejected   PartnershipStatus = "DITOLAK"            // Rejected by receiver
	StatusCancelled  PartnershipStatus = "DIBATALKAN"         // Cancelled by requester
	StatusWaitingDoc PartnershipStatus = "MENUNGGU_DOKUMEN_TTD" // Waiting for signed contract
	StatusCompleted  PartnershipStatus = "SELESAI"            // Partnership completed
)

// IsValidStatus checks if a status string is one of the known DB statuses.
func IsValidStatus(s string) bool {
	switch PartnershipStatus(s) {
	case StatusDraft, StatusSubmitted, StatusReviewed,
		StatusActive, StatusRejected, StatusCancelled,
		StatusWaitingDoc, StatusCompleted:
		return true
	}
	return false
}

// IsValidUpdateStatus checks statuses accepted by UpdatePartnershipStatus.
// Derived from REST handler:
//   - AKTIF     → receiver approves (DIAJUKAN|DITINJAU → AKTIF)
//   - DITOLAK   → receiver rejects  (DIAJUKAN|DITINJAU → DITOLAK)
//   - DIBATALKAN → requester cancels (DRAFT|DIAJUKAN|DITINJAU → DIBATALKAN)
func IsValidUpdateStatus(s string) bool {
	switch PartnershipStatus(s) {
	case StatusActive, StatusRejected, StatusCancelled:
		return true
	}
	return false
}

// -------------------------------------------------------
// Authorization rules — derived from existing REST code
// -------------------------------------------------------

// ActorFor defines which actor (by DB column) is authorized for each status transition.
// Derived from authorization.go and handler.go of the existing REST service.
//
//	Approve (→AKTIF)   : penerima_akun_id (receiver)
//	Reject  (→DITOLAK) : penerima_akun_id (receiver)
//	Cancel  (→DIBATALKAN): pengaju_akun_id (requester)
//	MarkRead: penerima_akun_id (receiver)
//	Sign:     pengaju_akun_id (requester) — confirmed from service.go SignPartnership

// -------------------------------------------------------
// Internal / DB row models
// -------------------------------------------------------

// PartnershipRow is the internal row returned from FindByID.
type PartnershipRow struct {
	ID                 string
	RequestCode        string
	RequesterID        string
	ReceiverID         string
	Status             PartnershipStatus
	ProposalText       string  // pesan_pengajuan
	RejectionReason    *string // catatan_keputusan
	ContractDocumentID *string // dokumen_perjanjian_id
	RequesterName      string
	ReceiverName       string
	SubmittedAt        *time.Time
	DecidedAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// PartnershipListRow is a lighter row for paginated list queries.
type PartnershipListRow struct {
	ID                    string
	RequestCode           string
	RequesterName         string
	ReceiverName          string
	RequesterBusinessName string
	ReceiverBusinessName  string
	ProposalTitle         string
	Status                PartnershipStatus
	SubmittedAt           *time.Time
	DecidedAt             *time.Time
	TotalCount            int
}

// -------------------------------------------------------
// SOAP Request types (XML input)
// -------------------------------------------------------

// CreatePartnershipApplicationRequest is the SOAP CreatePartnershipApplication body.
type CreatePartnershipApplicationRequest struct {
	XMLName             xml.Name `xml:"CreatePartnershipApplication"`
	UserID              string   `xml:"userId"`
	UserRole            string   `xml:"userRole"`
	ReceiverID          string   `xml:"receiverId"`
	ProposalTitle       string   `xml:"proposalTitle"`
	ProposalDescription string   `xml:"proposalDescription"`
}

// GetPartnershipApplicationRequest is the SOAP GetPartnershipApplication body.
type GetPartnershipApplicationRequest struct {
	XMLName       xml.Name `xml:"GetPartnershipApplication"`
	UserID        string   `xml:"userId"`
	UserRole      string   `xml:"userRole"`
	ApplicationID string   `xml:"applicationId"`
}

// GetPartnershipApplicationsRequest is the SOAP GetPartnershipApplications body.
type GetPartnershipApplicationsRequest struct {
	XMLName  xml.Name `xml:"GetPartnershipApplications"`
	UserID   string   `xml:"userId"`
	UserRole string   `xml:"userRole"`
	Status   string   `xml:"status,omitempty"`
	Page     int      `xml:"page,omitempty"`
	Limit    int      `xml:"limit,omitempty"`
}

// GetIncomingPartnershipsRequest is the SOAP GetIncomingPartnerships body.
type GetIncomingPartnershipsRequest struct {
	XMLName  xml.Name `xml:"GetIncomingPartnerships"`
	UserID   string   `xml:"userId"`
	UserRole string   `xml:"userRole"`
	Status   string   `xml:"status,omitempty"`
	Page     int      `xml:"page,omitempty"`
	Limit    int      `xml:"limit,omitempty"`
}

// UpdatePartnershipStatusRequest is the SOAP UpdatePartnershipStatus body.
// Accepted status values: AKTIF (approve), DITOLAK (reject), DIBATALKAN (cancel).
type UpdatePartnershipStatusRequest struct {
	XMLName         xml.Name `xml:"UpdatePartnershipStatus"`
	UserID          string   `xml:"userId"`
	UserRole        string   `xml:"userRole"`
	ApplicationID   string   `xml:"applicationId"`
	Status          string   `xml:"status"`
	RejectionReason string   `xml:"rejectionReason,omitempty"`
}

// MarkPartnershipAsReadRequest is the SOAP MarkPartnershipAsRead body.
// NOTE: In the existing REST service, PATCH /partnerships/:id/read does NOT change
// the partnership status — it only checks that the caller is the receiver.
// Because the DB schema has no is_read column, this SOAP prototype maps
// MarkPartnershipAsRead to a status transition: DIAJUKAN → DITINJAU.
// This is an explicit workaround for the prototype, not a replica of REST behavior.
type MarkPartnershipAsReadRequest struct {
	XMLName       xml.Name `xml:"MarkPartnershipAsRead"`
	UserID        string   `xml:"userId"`
	UserRole      string   `xml:"userRole"`
	ApplicationID string   `xml:"applicationId"`
}

// SignPartnershipRequest is the SOAP SignPartnership body.
// The requester uploads a contract document ID.
// Actor: pengaju_akun_id (requester) — confirmed from service.go SignPartnership.
// Valid source statuses: DIAJUKAN, DITINJAU, MENUNGGU_DOKUMEN_TTD.
// Prototype note: no cross-service document ownership validation (document-service not called).
type SignPartnershipRequest struct {
	XMLName       xml.Name `xml:"SignPartnership"`
	UserID        string   `xml:"userId"`
	UserRole      string   `xml:"userRole"`
	ApplicationID string   `xml:"applicationId"`
	DocumentID    string   `xml:"documentId"`
}

// GetPartnershipSummaryRequest is the SOAP GetPartnershipSummary body.
type GetPartnershipSummaryRequest struct {
	XMLName  xml.Name `xml:"GetPartnershipSummary"`
	UserID   string   `xml:"userId"`
	UserRole string   `xml:"userRole"`
}

// GetIncomingPartnershipSummaryRequest is the SOAP GetIncomingPartnershipSummary body.
type GetIncomingPartnershipSummaryRequest struct {
	XMLName  xml.Name `xml:"GetIncomingPartnershipSummary"`
	UserID   string   `xml:"userId"`
	UserRole string   `xml:"userRole"`
}

// -------------------------------------------------------
// SOAP Response types (XML output)
// -------------------------------------------------------

// CreatePartnershipApplicationResponse is the SOAP response for CreatePartnershipApplication.
type CreatePartnershipApplicationResponse struct {
	XMLName       xml.Name `xml:"tns:CreatePartnershipApplicationResponse"`
	Success       bool     `xml:"success"`
	ApplicationID string   `xml:"applicationId"`
	RequestCode   string   `xml:"requestCode"`
	Status        string   `xml:"status"`
	Message       string   `xml:"message"`
}

// PartnershipDetail is the full detail returned by GetPartnershipApplication.
type PartnershipDetail struct {
	XMLName            xml.Name `xml:"tns:GetPartnershipApplicationResponse"`
	ApplicationID      string   `xml:"applicationId"`
	RequestCode        string   `xml:"requestCode"`
	RequesterID        string   `xml:"requesterId"`
	ReceiverID         string   `xml:"receiverId"`
	RequesterName      string   `xml:"requesterName"`
	ReceiverName       string   `xml:"receiverName"`
	ProposalText       string   `xml:"proposalText"`
	Status             string   `xml:"status"`
	RejectionReason    string   `xml:"rejectionReason,omitempty"`
	ContractDocumentID string   `xml:"contractDocumentId,omitempty"`
	SubmittedAt        string   `xml:"submittedAt,omitempty"`
	DecidedAt          string   `xml:"decidedAt,omitempty"`
	CreatedAt          string   `xml:"createdAt"`
	UpdatedAt          string   `xml:"updatedAt"`
}

// PartnershipListItem is a single item in list responses.
type PartnershipListItem struct {
	XMLName               xml.Name `xml:"application"`
	ApplicationID         string   `xml:"applicationId"`
	RequestCode           string   `xml:"requestCode"`
	RequesterName         string   `xml:"requesterName"`
	ReceiverName          string   `xml:"receiverName"`
	RequesterBusinessName string   `xml:"requesterBusinessName"`
	ReceiverBusinessName  string   `xml:"receiverBusinessName"`
	ProposalTitle         string   `xml:"proposalTitle"`
	Status                string   `xml:"status"`
	SubmittedAt           string   `xml:"submittedAt,omitempty"`
	DecidedAt             string   `xml:"decidedAt,omitempty"`
}

// GetPartnershipApplicationsResponse is the outgoing list response.
type GetPartnershipApplicationsResponse struct {
	XMLName      xml.Name              `xml:"tns:GetPartnershipApplicationsResponse"`
	TotalCount   int                   `xml:"totalCount"`
	Applications []PartnershipListItem `xml:"applications>application"`
}

// GetIncomingPartnershipsResponse is the incoming list response.
type GetIncomingPartnershipsResponse struct {
	XMLName      xml.Name              `xml:"tns:GetIncomingPartnershipsResponse"`
	TotalCount   int                   `xml:"totalCount"`
	Applications []PartnershipListItem `xml:"applications>application"`
}

// UpdatePartnershipStatusResponse is the response for status updates.
type UpdatePartnershipStatusResponse struct {
	XMLName       xml.Name `xml:"tns:UpdatePartnershipStatusResponse"`
	Success       bool     `xml:"success"`
	ApplicationID string   `xml:"applicationId"`
	Status        string   `xml:"status"`
	Message       string   `xml:"message"`
}

// MarkPartnershipAsReadResponse is the response for mark-as-read.
type MarkPartnershipAsReadResponse struct {
	XMLName       xml.Name `xml:"tns:MarkPartnershipAsReadResponse"`
	Success       bool     `xml:"success"`
	ApplicationID string   `xml:"applicationId"`
	Message       string   `xml:"message"`
}

// SignPartnershipResponse is the response for signing.
type SignPartnershipResponse struct {
	XMLName       xml.Name `xml:"tns:SignPartnershipResponse"`
	Success       bool     `xml:"success"`
	ApplicationID string   `xml:"applicationId"`
	Message       string   `xml:"message"`
}

// SummaryItem represents one status count in a summary.
type SummaryItem struct {
	XMLName xml.Name `xml:"item"`
	Status  string   `xml:"status"`
	Count   int      `xml:"count"`
}

// GetPartnershipSummaryResponse is the summary response.
type GetPartnershipSummaryResponse struct {
	XMLName xml.Name      `xml:"tns:GetPartnershipSummaryResponse"`
	Items   []SummaryItem `xml:"summary>item"`
}

// GetIncomingPartnershipSummaryResponse is the incoming summary response.
type GetIncomingPartnershipSummaryResponse struct {
	XMLName xml.Name      `xml:"tns:GetIncomingPartnershipSummaryResponse"`
	Items   []SummaryItem `xml:"summary>item"`
}
