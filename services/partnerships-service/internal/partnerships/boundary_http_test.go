package partnerships_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/apperror"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/middleware"
	. "github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/partnerships"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/rest"
)

type readService struct {
	Service
	called bool
}

func (s *readService) MarkPartnershipAsRead(context.Context, string) error {
	s.called = true
	return nil
}
func (s *readService) GetPartnershipByID(context.Context, string) (*PartnershipResponse, error) {
	return nil, apperror.New(404, "old path")
}

func serveBoundaryHTTP(t *testing.T, svc Service, method, path, body, actor string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Use(middleware.AuthMiddleware("boundary-secret"))
	r.Use(middleware.RequireRoles("UMKM", "MITRA"))
	h := rest.NewHandler(svc)
	r.Post("/partnerships", h.CreatePartnership)
	r.Patch("/partnerships/{id}/read", h.MarkAsRead)
	r.Get("/partnerships/status", h.GetPartnershipStatus)
	r.Get("/partnerships/incoming", h.GetIncomingPartnerships)
	r.Get("/partnerships/summary", h.GetPartnershipSummary)
	r.Get("/partnerships/incoming/summary", h.GetIncomingPartnershipSummary)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if actor != "" {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": actor, "role": "UMKM", "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte("boundary-secret"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-User-ID", "victim")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestReadHandlerDelegates(t *testing.T) {
	svc := &readService{}
	rr := serveBoundaryHTTP(t, svc, "PATCH", "/partnerships/P1/read", "", "receiver")
	if rr.Code != 200 || !svc.called || rr.Body.String() != "{\"data\":null,\"message\":\"Status dibaca berhasil diperbarui.\",\"success\":true}\n" {
		t.Fatalf("status=%d called=%v body=%s", rr.Code, svc.called, rr.Body.String())
	}
}

func TestReadHTTPBoundary(t *testing.T) {
	for _, tc := range []struct {
		actor       string
		want, calls int
	}{
		{"receiver", 200, 1}, {"requester", 403, 1}, {"outsider", 403, 1}, {"", 401, 0},
	} {
		t.Run(tc.actor, func(t *testing.T) {
			repo := &boundaryRepository{row: PartnershipResponse{PartnershipRequest: PartnershipRequest{ID: "P1", RequesterID: "requester", ReceiverID: "receiver", Status: StatusSubmitted}}}
			rr := serveBoundaryHTTP(t, NewService(repo), "PATCH", "/partnerships/P1/read", "", tc.actor)
			if rr.Code != tc.want || repo.calls != tc.calls || repo.row.Status != StatusSubmitted {
				t.Fatalf("status=%d calls=%d body=%s", rr.Code, repo.calls, rr.Body.String())
			}
		})
	}
}

func TestScopedHTTPBoundary(t *testing.T) {
	for _, path := range []string{"/partnerships/status", "/partnerships/incoming", "/partnerships/summary", "/partnerships/incoming/summary"} {
		for _, actor := range []string{"", "account"} {
			t.Run(path+"/"+actor, func(t *testing.T) {
				repo := &boundaryRepository{}
				rr := serveBoundaryHTTP(t, NewService(repo), "GET", path+"?user_id=victim&page=3&limit=10&status=diajukan", "", actor)
				if actor == "" {
					if rr.Code != 401 || repo.calls != 0 {
						t.Fatalf("unauthenticated: %d calls=%d", rr.Code, repo.calls)
					}
					return
				}
				if rr.Code != 200 || repo.calls != 1 || repo.account != actor {
					t.Fatalf("status=%d account=%s body=%s", rr.Code, repo.account, rr.Body.String())
				}
				var body struct {
					Success bool
					Data    struct {
						Pagination map[string]int
						Pengajuan  []map[string]any
						Incoming   []map[string]any `json:"pengajuan_masuk"`
						Summary    map[string]int
					}
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if !body.Success {
					t.Fatal(rr.Body.String())
				}
				if strings.HasSuffix(path, "summary") {
					key := "bermitra"
					if strings.Contains(path, "incoming") {
						key = "disetujui"
					}
					if body.Data.Summary[key] != 3 {
						t.Fatal(rr.Body.String())
					}
				} else {
					if strings.Contains(path, "incoming") {
						body.Data.Pengajuan = body.Data.Incoming
					}
					if repo.limit != 10 || repo.offset != 20 || repo.status == nil || *repo.status != StatusSubmitted || body.Data.Pagination["total"] != 7 || body.Data.Pagination["page"] != 3 || len(body.Data.Pengajuan) != 1 || body.Data.Pengajuan[0]["pengajuanID"] != "P1" {
						t.Fatal(rr.Body.String())
					}
				}
			})
		}
	}
}

func TestCreateHTTPCompatibilityAndSafeErrors(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		body                 string
		lookupErr, createErr error
		want                 int
	}{
		{"valid", `{"receiver_id":"receiver-business","proposal_title":"Valid proposal title","proposal_description":"A sufficiently long proposal description."}`, nil, nil, 200},
		{"validation", `{}`, nil, nil, 422},
		{"malformed", `{`, nil, nil, 400},
		{"lookup infrastructure", `{"receiver_id":"receiver-business","proposal_title":"Valid proposal title","proposal_description":"A sufficiently long proposal description."}`, errors.New("SQLSTATE password=secret private_table"), nil, 500},
		{"create infrastructure", `{"receiver_id":"receiver-business","proposal_title":"Valid proposal title","proposal_description":"A sufficiently long proposal description."}`, nil, errors.New("SQLSTATE password=secret private_table"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &creationRepository{lookupErr: tc.lookupErr, createErr: tc.createErr}
			rr := serveBoundaryHTTP(t, NewService(repo), "POST", "/partnerships", tc.body, "requester")
			if rr.Code != tc.want {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tc.want == 422 {
				want := map[string]string{"receiver_id": "receiver_id wajib diisi", "proposal_title": "proposal_title minimal 10 karakter", "proposal_description": "proposal_description minimal 30 karakter"}
				details, ok := body["details"].(map[string]any)
				if !ok || len(details) != len(want) || body["message"] != "Validation failed" {
					t.Fatal(body)
				}
				for k, v := range want {
					if details[k] != v {
						t.Fatal(body)
					}
				}
				if repo.calls != 0 {
					t.Fatal("invalid HTTP request reached repository")
				}
			}
			if tc.want == 200 {
				data, ok := body["data"].(map[string]any)
				if !ok || len(data) != 1 || data["pengajuanID"] != repo.created.ID || body["success"] != true {
					t.Fatal(body)
				}
			}
			for _, marker := range []string{"SQLSTATE", "password=", "private_table"} {
				if strings.Contains(rr.Body.String(), marker) {
					t.Fatalf("leaked %s", rr.Body.String())
				}
			}
		})
	}
}
