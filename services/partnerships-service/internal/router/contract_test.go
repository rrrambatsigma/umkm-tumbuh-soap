package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/apperror"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/auth"
	p "github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/partnerships"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/rest"
)

// The HTTP adapter is real; only the business boundary is recorded here.
// Existing service tests separately exercise authorization and persistence calls.
type contractService struct {
	p.Service
	err                    error
	calls                  int
	op, id, search, filter string
	actor                  auth.Actor
	ctx                    context.Context
	page, limit            int
	status                 *p.PartnershipStatus
	create                 p.CreatePartnershipRequest
	update                 p.UpdatePartnershipStatus
	sign                   p.SignPartnershipRequest
	role                   p.UserRole
}

func (s *contractService) record(ctx context.Context, op, id string) {
	s.calls++
	s.ctx, s.op, s.id = ctx, op, id
	s.actor, _ = auth.ActorFromContext(ctx)
}
func (s *contractService) CreatePartnership(ctx context.Context, id string, role p.UserRole, req p.CreatePartnershipRequest) (*p.PartnershipResponse, error) {
	s.record(ctx, "create", id)
	s.create, s.role = req, role
	return &p.PartnershipResponse{PartnershipRequest: p.PartnershipRequest{ID: "P1"}}, s.err
}
func (s *contractService) GetPartnershipsByRequester(ctx context.Context, id string, status *p.PartnershipStatus, page, limit int) ([]p.PartnershipListResponse, int, error) {
	s.record(ctx, "sent", id)
	s.status, s.page, s.limit = status, page, limit
	return contractList(), 7, s.err
}
func (s *contractService) GetPartnershipsByReceiver(ctx context.Context, id string, status *p.PartnershipStatus, page, limit int) ([]p.PartnershipListResponse, int, error) {
	s.record(ctx, "received", id)
	s.status, s.page, s.limit = status, page, limit
	return contractList(), 7, s.err
}
func contractList() []p.PartnershipListResponse {
	return []p.PartnershipListResponse{{ID: "P1", RequesterName: "Sender", ReceiverName: "Receiver", RequesterBusinessName: " Sender business ", ReceiverBusinessName: "Receiver business", ProposalTitle: "Stored title", Status: p.StatusSubmitted}}
}
func (s *contractService) GetPartnershipSummary(ctx context.Context, id string) (map[string]int, error) {
	s.record(ctx, "outgoing summary", id)
	return contractSummary(), s.err
}
func (s *contractService) GetIncomingPartnershipSummary(ctx context.Context, id string) (map[string]int, error) {
	s.record(ctx, "incoming summary", id)
	return contractSummary(), s.err
}
func contractSummary() map[string]int {
	return map[string]int{"AKTIF": 1, "SELESAI": 2, "DIAJUKAN": 3, "DITINJAU": 4, "MENUNGGU_DOKUMEN_TTD": 5, "APPROVED": 6, "DITOLAK": 7, "DIBATALKAN": 8}
}
func (s *contractService) GetPartnershipByID(ctx context.Context, id string) (*p.PartnershipResponse, error) {
	s.record(ctx, "detail", id)
	contract := "contract-1"
	return &p.PartnershipResponse{PartnershipRequest: p.PartnershipRequest{ID: "P1", RequesterID: "account", ReceiverID: "receiver", ProposalTitle: "Stored title", ProposalDescription: "Stored description", ContractDocumentID: &contract}, Attachments: []p.PartnershipAttachment{{DocumentID: "doc-1", Type: "proposal", FileName: "stored.pdf", OriginalFilename: "proposal.pdf", ContentType: "application/pdf", SizeBytes: 123}}}, s.err
}
func (s *contractService) MarkPartnershipAsRead(ctx context.Context, id string) error {
	s.record(ctx, "read", id)
	return s.err
}
func (s *contractService) UpdatePartnershipStatus(ctx context.Context, id string, req p.UpdatePartnershipStatus) error {
	s.record(ctx, "update", id)
	s.update = req
	return s.err
}
func (s *contractService) SignPartnership(ctx context.Context, id string, req p.SignPartnershipRequest) error {
	s.record(ctx, "sign", id)
	s.sign = req
	return s.err
}
func (s *contractService) GetUMKMList(ctx context.Context, q, filter string, page, limit int) ([]p.UMKMListItem, int, error) {
	s.record(ctx, "umkm list", "")
	s.search, s.filter, s.page, s.limit = q, filter, page, limit
	return []p.UMKMListItem{{ID: "U1", Name: "Shop", Type: "Food", City: "City", Province: "Province", Description: "Description", OperationalArea: "Area", LogoURL: "logo", FotoCoverURL: "cover"}}, 7, s.err
}
func (s *contractService) GetMitraList(ctx context.Context, q, filter string, page, limit int) ([]p.MitraListItem, int, error) {
	s.record(ctx, "mitra list", "")
	s.search, s.filter, s.page, s.limit = q, filter, page, limit
	return []p.MitraListItem{{ID: "M1", Name: "Partner", Type: "Retail", City: "City", Province: "Province", Description: "Description", OperationalArea: "Area"}}, 7, s.err
}
func (s *contractService) GetUMKMDetail(ctx context.Context, id string) (*p.UMKMDetail, error) {
	s.record(ctx, "umkm detail", id)
	return &p.UMKMDetail{ID: "U1", Name: "Shop", FeaturedProducts: []p.FeaturedProduct{{ID: "product-1", Name: "Product", Price: 12, Stock: 3}}}, s.err
}
func (s *contractService) GetMitraDetail(ctx context.Context, id string) (*p.MitraDetail, error) {
	s.record(ctx, "mitra detail", id)
	return &p.MitraDetail{ID: "M1", Name: "Partner", NIB: "nib", NPWP: "npwp", ContactPerson: "Contact"}, s.err
}

type contractRoute struct{ method, path, body, role, op, id, message, data string }

var contractRoutes = []contractRoute{
	{"POST", "/partnerships", `{"receiver_id":"business","proposal_title":"Valid proposal title","proposal_description":"A sufficiently long proposal description.","attachment_files":["doc-1"]}`, "UMKM", "create", "account", "Pengajuan kemitraan berhasil dikirim.", `{"pengajuanID":"P1"}`},
	{"GET", "/partnerships/status?page=3&limit=10&status=diajukan", "", "UMKM", "sent", "account", "", `{"pagination":{"page":3,"limit":10,"total":7,"totalPages":1},"pengajuan":[{"pengajuanID":"P1","statusPengajuan":"DIAJUKAN","tanggalPengajuan":null,"mitraUmkmTujuan":"Receiver","mitraUmkmUsaha":"Receiver business","pengirim":"Sender","pengirimUsaha":" Sender business ","proposalTitle":"Pengajuan Kemitraan - Sender business"}]}`},
	{"GET", "/partnerships/summary", "", "UMKM", "outgoing summary", "account", "", `{"summary":{"bermitra":3,"menunggu":12,"ditolak":15}}`},
	{"GET", "/partnerships/incoming?page=3&limit=10&status=diajukan", "", "MITRA", "received", "account", "", `{"pagination":{"page":3,"limit":10,"total":7,"totalPages":1},"pengajuan_masuk":[{"pengajuanID":"P1","pengirim":"Sender","proposal_title":"Pengajuan Kemitraan - Sender business","tanggalPengajuan":null,"status":"DIAJUKAN"}]}`},
	{"GET", "/partnerships/incoming/summary", "", "MITRA", "incoming summary", "account", "", `{"summary":{"menunggu":7,"disetujui":14,"ditolak":7,"dibatalkan":8,"total":36}}`},
	{"GET", "/partnerships/P1", "", "UMKM", "detail", "P1", "", ""},
	{"PATCH", "/partnerships/P1/read", "", "MITRA", "read", "P1", "Status dibaca berhasil diperbarui.", `null`},
	{"PATCH", "/partnerships/P1/approve", `{"status":"DIBATALKAN"}`, "MITRA", "update", "P1", "Pengajuan disetujui. Kemitraan aktif.", `null`},
	{"PATCH", "/partnerships/P1/reject", `{"status":"AKTIF","rejection_reason":"Not suitable"}`, "MITRA", "update", "P1", "Pengajuan ditolak.", `null`},
	{"PATCH", "/partnerships/P1/cancel", `{"status":"AKTIF"}`, "UMKM", "update", "P1", "Pengajuan dibatalkan.", `null`},
	{"POST", "/partnerships/P1/sign", `{"dokumen_kontrak":"contract-1"}`, "UMKM", "sign", "P1", "Dokumen berhasil diunggah. Pengajuan siap disetujui.", `null`},
	{"GET", "/umkm?q=coffee&filterType=Food&page=2&limit=99", "", "MITRA", "umkm list", "", "", `{"pagination":{"page":2,"limit":50,"total":7,"totalPages":1},"umkm":[{"id":"U1","name":"Shop","type":"Food","city":"City","province":"Province","description":"Description","operational_area":"Area","logo_url":"logo","foto_cover_url":"cover"}]}`},
	{"GET", "/umkm/U1", "", "MITRA", "umkm detail", "U1", "", ""},
	{"GET", "/mitra?q=coffee&filterType=Food&page=2&limit=99", "", "UMKM", "mitra list", "", "", `{"pagination":{"page":2,"limit":50,"total":7,"totalPages":1},"mitra":[{"id":"M1","name":"Partner","type":"Retail","city":"City","province":"Province","description":"Description","operational_area":"Area"}]}`},
	{"GET", "/mitra/M1", "", "UMKM", "mitra detail", "M1", "", ""},
}

func callContract(t *testing.T, s *contractService, tc contractRoute, role string) *httptest.ResponseRecorder {
	t.Helper()
	r := NewRouter(rest.NewHandler(s), "http://localhost:5173", "contract-secret")
	req := httptest.NewRequest(tc.method, "/api/v1"+tc.path, strings.NewReader(tc.body))
	ctx, cancel := context.WithTimeout(req.Context(), time.Minute)
	t.Cleanup(cancel)
	req = req.WithContext(ctx)
	req.Header.Set("X-User-ID", "victim")
	req.Header.Set("X-User-Role", "ADMIN")
	if role != "" {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "account", "role": role, "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte("contract-secret"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}
func decodeObject(t *testing.T, body string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestRESTRouteContracts(t *testing.T) {
	for _, tc := range contractRoutes {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			s := &contractService{Service: p.NewService(nil)}
			rr := callContract(t, s, tc, tc.role)
			if rr.Code != 200 || rr.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("status=%d headers=%v body=%s", rr.Code, rr.Header(), rr.Body.String())
			}
			v := decodeObject(t, rr.Body.String())
			if len(v) != 3 || v["success"] != true || v["message"] != tc.message {
				t.Fatal(v)
			}
			if tc.data != "" {
				var want any
				if err := json.Unmarshal([]byte(tc.data), &want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(v["data"], want) {
					t.Fatalf("data=%v want=%v", v["data"], want)
				}
			}
			if s.calls != 1 || s.op != tc.op || s.id != tc.id || s.actor != (auth.Actor{UserID: "account", Role: tc.role}) {
				t.Fatalf("wrong adapter call: %+v", s)
			}
			if _, ok := s.ctx.Deadline(); !ok {
				t.Fatal("request deadline lost")
			}
			switch tc.op {
			case "create":
				if s.role != p.RoleUMKM || s.create.ReceiverID != "business" || s.create.ProposalTitle != "Valid proposal title" || s.create.ProposalDescription != "A sufficiently long proposal description." || !reflect.DeepEqual(s.create.AttachmentFiles, []string{"doc-1"}) {
					t.Fatal(s.create)
				}
			case "sent", "received":
				if s.page != 3 || s.limit != 10 || s.status == nil || *s.status != p.StatusSubmitted {
					t.Fatalf("bad query: %+v", s)
				}
			case "umkm list", "mitra list":
				if s.page != 2 || s.limit != 50 || s.search != "coffee" || s.filter != "Food" {
					t.Fatalf("bad directory query: %+v", s)
				}
			case "update":
				want := p.StatusActive
				if strings.HasSuffix(tc.path, "reject") {
					want = p.StatusRejected
					if s.update.RejectionReason == nil || *s.update.RejectionReason != "Not suitable" {
						t.Fatal(s.update)
					}
				}
				if strings.HasSuffix(tc.path, "cancel") {
					want = p.StatusCancelled
				}
				if s.update.Status != want {
					t.Fatal(s.update)
				}
			case "sign":
				if s.sign.DokumenKontrak != "contract-1" {
					t.Fatal(s.sign)
				}
			case "detail":
				data := v["data"].(map[string]any)
				if len(data) != 1 {
					t.Fatal(data)
				}
				row := data["pengajuan"].(map[string]any)
				for key, want := range map[string]any{"id": "P1", "requester_id": "account", "receiver_id": "receiver", "proposal_title": "Stored title", "proposal_description": "Stored description", "contract_document_id": "contract-1"} {
					if row[key] != want {
						t.Fatalf("%s=%v", key, row[key])
					}
				}
				attachments := row["attachments"].([]any)
				want := decodeObject(t, `{"document_id":"doc-1","type":"proposal","file_name":"stored.pdf","original_filename":"proposal.pdf","content_type":"application/pdf","size_bytes":123,"created_at":"0001-01-01T00:00:00Z"}`)
				if len(attachments) != 1 || !reflect.DeepEqual(attachments[0], want) {
					t.Fatal(attachments)
				}
			case "umkm detail":
				row := v["data"].(map[string]any)["umkm"].(map[string]any)
				if row["id"] != "U1" || row["name"] != "Shop" {
					t.Fatal(row)
				}
				product := row["featured_products"].([]any)[0].(map[string]any)
				if product["id"] != "product-1" || product["price"] != float64(12) || product["stock"] != float64(3) {
					t.Fatal(product)
				}
			case "mitra detail":
				row := v["data"].(map[string]any)["mitra"].(map[string]any)
				if row["id"] != "M1" || row["name"] != "Partner" || row["nib"] != "nib" || row["npwp"] != "npwp" || row["contact_person"] != "Contact" {
					t.Fatal(row)
				}
			}
		})
	}
}

func TestRESTRouteErrorContracts(t *testing.T) {
	for _, tc := range contractRoutes {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			for _, role := range []string{"", "ADMIN"} {
				s := &contractService{Service: p.NewService(nil)}
				rr := callContract(t, s, tc, role)
				want := 401
				if role == "ADMIN" {
					want = 403
				}
				if rr.Code != want || s.calls != 0 {
					t.Fatalf("role=%q status=%d calls=%d", role, rr.Code, s.calls)
				}
			}
			for _, code := range []int{403, 404, 409, 500} {
				s := &contractService{Service: p.NewService(nil), err: apperror.New(code, "public error")}
				rr := callContract(t, s, tc, tc.role)
				if rr.Code != code || rr.Body.String() != "{\"message\":\"public error\",\"success\":false}\n" {
					t.Fatalf("%d: %d %s", code, rr.Code, rr.Body.String())
				}
			}
			s := &contractService{Service: p.NewService(nil), err: errors.New("private SQLSTATE details")}
			rr := callContract(t, s, tc, tc.role)
			if rr.Code != 500 || strings.Contains(rr.Body.String(), "SQLSTATE") || decodeObject(t, rr.Body.String())["success"] != false {
				t.Fatalf("%d %s", rr.Code, rr.Body.String())
			}
			wrong := tc
			wrong.method = "DELETE"
			s = &contractService{Service: p.NewService(nil)}
			rr = callContract(t, s, wrong, tc.role)
			if rr.Code != 405 || s.calls != 0 {
				t.Fatalf("wrong method: %d calls=%d", rr.Code, s.calls)
			}
		})
	}
}

func TestRESTMalformedAndDefaultInputs(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/partnerships", "{", 400}, {"POST", "/partnerships", "{}", 422},
		{"POST", "/partnerships/P1/sign", "{", 400},
		{"PATCH", "/partnerships/P1/reject", "{", 400}, {"PATCH", "/partnerships/P1/reject", "{}", 422},
		{"PATCH", "/partnerships/P1/cancel", "{", 400},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			s := &contractService{Service: p.NewService(nil)}
			rr := callContract(t, s, contractRoute{method: tc.method, path: tc.path, body: tc.body}, "UMKM")
			if rr.Code != tc.want || s.calls != 0 {
				t.Fatalf("%d calls=%d %s", rr.Code, s.calls, rr.Body.String())
			}
		})
	}
	// Existing approval behavior ignores an absent/malformed body and fixes AKTIF.
	for _, body := range []string{"", "{"} {
		s := &contractService{Service: p.NewService(nil)}
		rr := callContract(t, s, contractRoute{method: "PATCH", path: "/partnerships/P1/approve", body: body}, "MITRA")
		if rr.Code != 200 || s.update.Status != p.StatusActive {
			t.Fatalf("%d %+v", rr.Code, s.update)
		}
	}
	for _, tc := range []struct{ path, role string }{{"/partnerships/status", "UMKM"}, {"/partnerships/incoming", "MITRA"}, {"/umkm", "MITRA"}, {"/mitra", "UMKM"}} {
		s := &contractService{Service: p.NewService(nil)}
		rr := callContract(t, s, contractRoute{method: "GET", path: tc.path + "?page=-1&limit=bad"}, tc.role)
		if rr.Code != 200 || s.page != 1 || s.limit != 10 || s.status != nil {
			t.Fatalf("%s %d page=%d limit=%d", tc.path, rr.Code, s.page, s.limit)
		}
	}
}

func TestBusinessCoreHasNoTransportDependencies(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", "../partnerships").CombinedOutput()
	if err != nil {
		t.Fatalf("dependency inspection failed: %v: %s", err, output)
	}
	for _, dep := range strings.Fields(string(output)) {
		for _, forbidden := range []string{"/internal/rest", "/internal/router", "/internal/middleware"} {
			if strings.HasSuffix(dep, forbidden) {
				t.Errorf("business core depends on transport: %s", dep)
			}
		}
	}
}
