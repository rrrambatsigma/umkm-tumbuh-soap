package soap

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/partnerships"
)

type soapOperationCase struct {
	name     string
	extraXML string
}

func TestSOAPAuthenticationStopsAllOperationsBeforeService(t *testing.T) {
	now := time.Now()
	validClaims := map[string]any{
		"sub":  "AKUN-AUTHENTICATED",
		"role": "UMKM",
		"exp":  now.Add(time.Hour).Unix(),
	}
	authCases := []struct {
		name          string
		authorization string
	}{
		{name: "missing authorization"},
		{name: "invalid bearer", authorization: "Bearer not-a-jwt"},
		{name: "wrong scheme", authorization: "Basic credentials"},
		{name: "forged signature", authorization: "Bearer " + signedTestJWT(t, validClaims, "forged-secret", "HS256")},
		{name: "expired", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "AKUN-AUTHENTICATED", "role": "UMKM", "exp": now.Add(-time.Minute).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "missing subject", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"role": "UMKM", "exp": now.Add(time.Hour).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "non-string subject", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": 42, "role": "UMKM", "exp": now.Add(time.Hour).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "blank subject", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "  ", "role": "UMKM", "exp": now.Add(time.Hour).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "missing role", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "AKUN-AUTHENTICATED", "exp": now.Add(time.Hour).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "invalid role", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "AKUN-AUTHENTICATED", "role": "OWNER", "exp": now.Add(time.Hour).Unix(),
		}, testJWTSecret, "HS256")},
	}

	for _, operation := range allSOAPOperationCases() {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			for _, authCase := range authCases {
				authCase := authCase
				t.Run(authCase.name, func(t *testing.T) {
					spy := &partnershipServiceSpy{}
					handler := newTestHandler(spy, HandlerConfig{JWTSecret: testJWTSecret})
					req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(operationEnvelope(
						operation, identityElements(stringPointer("AKUN-VICTIM"), stringPointer("MITRA")),
					)))
					if authCase.authorization != "" {
						req.Header.Set("Authorization", authCase.authorization)
					}
					rr := httptest.NewRecorder()

					handler.ServeHTTP(rr, req)

					assertSOAPFault(t, rr, http.StatusUnauthorized)
					if spy.calls != 0 {
						t.Fatalf("service calls = %d, want 0", spy.calls)
					}
				})
			}
		})
	}
}

func TestSOAPIdentityContractStopsAllOperationsBeforeService(t *testing.T) {
	token := signedTestJWT(t, map[string]any{
		"sub":  "AKUN-AUTHENTICATED",
		"role": "UMKM",
		"exp":  time.Now().Add(time.Hour).Unix(),
	}, testJWTSecret, "HS256")

	identityCases := []struct {
		name       string
		identity   string
		wantStatus int
	}{
		{
			name:       "conflicting user id",
			identity:   identityElements(stringPointer("AKUN-VICTIM"), stringPointer("UMKM")),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "conflicting user role",
			identity:   identityElements(stringPointer("AKUN-AUTHENTICATED"), stringPointer("MITRA")),
			wantStatus: http.StatusForbidden,
		},
		{name: "missing user id", identity: identityElements(nil, stringPointer("UMKM")), wantStatus: http.StatusBadRequest},
		{name: "empty user id", identity: identityElements(stringPointer(""), stringPointer("UMKM")), wantStatus: http.StatusBadRequest},
		{name: "whitespace user id", identity: identityElements(stringPointer(" \t "), stringPointer("UMKM")), wantStatus: http.StatusBadRequest},
		{name: "missing user role", identity: identityElements(stringPointer("AKUN-AUTHENTICATED"), nil), wantStatus: http.StatusBadRequest},
		{name: "empty user role", identity: identityElements(stringPointer("AKUN-AUTHENTICATED"), stringPointer("")), wantStatus: http.StatusBadRequest},
		{name: "whitespace user role", identity: identityElements(stringPointer("AKUN-AUTHENTICATED"), stringPointer(" \n ")), wantStatus: http.StatusBadRequest},
	}

	for _, operation := range allSOAPOperationCases() {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			for _, identityCase := range identityCases {
				identityCase := identityCase
				t.Run(identityCase.name, func(t *testing.T) {
					spy := &partnershipServiceSpy{}
					handler := newTestHandler(spy, HandlerConfig{JWTSecret: testJWTSecret})
					req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(operationEnvelope(operation, identityCase.identity)))
					req.Header.Set("Authorization", "Bearer "+token)
					rr := httptest.NewRecorder()

					handler.ServeHTTP(rr, req)

					assertSOAPFault(t, rr, identityCase.wantStatus)
					if spy.calls != 0 {
						t.Fatalf("service calls = %d, want 0", spy.calls)
					}
				})
			}
		})
	}
}

func TestSOAPVerifiedIdentityReachesEveryServiceOperation(t *testing.T) {
	actors := []struct {
		userID string
		role   string
	}{
		{userID: "AKUN-UMKM", role: "UMKM"},
		{userID: "AKUN-MITRA", role: "MITRA"},
	}

	for _, operation := range allSOAPOperationCases() {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			for _, actor := range actors {
				actor := actor
				t.Run(actor.role, func(t *testing.T) {
					spy := &partnershipServiceSpy{}
					handler := newTestHandler(spy, HandlerConfig{JWTSecret: testJWTSecret})
					token := signedTestJWT(t, map[string]any{
						"sub": actor.userID, "role": actor.role, "exp": time.Now().Add(time.Hour).Unix(),
					}, testJWTSecret, "HS256")
					req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(operationEnvelope(
						operation, identityElements(&actor.userID, &actor.role),
					)))
					req.Header.Set("Authorization", "bearer  "+token)
					req.Header.Set("X-User-ID", "AKUN-VICTIM")
					req.Header.Set("X-User-Role", "ADMIN")
					rr := httptest.NewRecorder()

					handler.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Fatalf("status = %d, want %d; body=%q", rr.Code, http.StatusOK, rr.Body.String())
					}
					if spy.calls != 1 {
						t.Fatalf("service calls = %d, want 1", spy.calls)
					}
					if spy.operation != operation.name {
						t.Fatalf("operation = %q, want %q", spy.operation, operation.name)
					}
					gotUserID, gotRole := requestIdentity(t, spy.request)
					if gotUserID != actor.userID || gotRole != actor.role {
						t.Fatalf("service identity = (%q, %q), want (%q, %q)", gotUserID, gotRole, actor.userID, actor.role)
					}
				})
			}
		})
	}
}

func TestSOAPRequestDeadlineReachesServiceAndIsCancelled(t *testing.T) {
	spy := &partnershipServiceSpy{}
	handler := newTestHandler(spy, HandlerConfig{
		JWTSecret:      testJWTSecret,
		RequestTimeout: 250 * time.Millisecond,
	})
	token := signedTestJWT(t, map[string]any{
		"sub": "AKUN-UMKM", "role": "UMKM", "exp": time.Now().Add(time.Hour).Unix(),
	}, testJWTSecret, "HS256")
	operation := soapOperationCase{name: "GetPartnershipSummary"}
	req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(operationEnvelope(
		operation, identityElements(stringPointer("AKUN-UMKM"), stringPointer("UMKM")),
	)))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}
	if spy.ctx == nil {
		t.Fatal("service did not receive a context")
	}
	deadline, ok := spy.ctx.Deadline()
	if !ok {
		t.Fatal("service context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > 250*time.Millisecond {
		t.Fatalf("service deadline remaining = %s, want within (0, 250ms]", remaining)
	}
	select {
	case <-spy.ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("service context was not cancelled when the request completed")
	}
}

func newTestHandler(svc partnershipService, cfg HandlerConfig) *Handler {
	if cfg.Logger == nil {
		cfg.Logger = log.New(io.Discard, "", 0)
	}
	return NewHandlerWithConfig(svc, "test wsdl", cfg)
}

func allSOAPOperationCases() []soapOperationCase {
	return []soapOperationCase{
		{
			name: "CreatePartnershipApplication",
			extraXML: "<receiverId>MITRA-001</receiverId>" +
				"<proposalTitle>Proposal kemitraan</proposalTitle>" +
				"<proposalDescription>Deskripsi proposal kemitraan yang cukup panjang.</proposalDescription>",
		},
		{name: "GetPartnershipApplication", extraXML: "<applicationId>PGJ000001</applicationId>"},
		{name: "GetPartnershipApplications", extraXML: "<status>DIAJUKAN</status><page>1</page><limit>20</limit>"},
		{name: "GetIncomingPartnerships", extraXML: "<status>DIAJUKAN</status><page>1</page><limit>20</limit>"},
		{name: "UpdatePartnershipStatus", extraXML: "<applicationId>PGJ000001</applicationId><status>AKTIF</status>"},
		{name: "MarkPartnershipAsRead", extraXML: "<applicationId>PGJ000001</applicationId>"},
		{name: "SignPartnership", extraXML: "<applicationId>PGJ000001</applicationId><documentId>DOK001</documentId>"},
		{name: "GetPartnershipSummary"},
		{name: "GetIncomingPartnershipSummary"},
	}
}

func operationEnvelope(operation soapOperationCase, identityXML string) string {
	return `<?xml version="1.0"?>` +
		`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">` +
		`<soap:Header><userId>AKUN-HEADER-SPOOF</userId><userRole>ADMIN</userRole></soap:Header>` +
		`<soap:Body><tns:` + operation.name + ` xmlns:tns="http://umkm-tumbuh.example.com/partnerships">` +
		identityXML + operation.extraXML +
		`</tns:` + operation.name + `></soap:Body></soap:Envelope>`
}

func identityElements(userID, role *string) string {
	var builder strings.Builder
	if userID != nil {
		builder.WriteString("<userId>")
		builder.WriteString(*userID)
		builder.WriteString("</userId>")
	}
	if role != nil {
		builder.WriteString("<userRole>")
		builder.WriteString(*role)
		builder.WriteString("</userRole>")
	}
	return builder.String()
}

func stringPointer(value string) *string { return &value }

func requestIdentity(t *testing.T, request any) (string, string) {
	t.Helper()
	switch req := request.(type) {
	case *partnerships.CreatePartnershipApplicationRequest:
		return req.UserID, req.UserRole
	case *partnerships.GetPartnershipApplicationRequest:
		return req.UserID, req.UserRole
	case *partnerships.GetPartnershipApplicationsRequest:
		return req.UserID, req.UserRole
	case *partnerships.GetIncomingPartnershipsRequest:
		return req.UserID, req.UserRole
	case *partnerships.UpdatePartnershipStatusRequest:
		return req.UserID, req.UserRole
	case *partnerships.MarkPartnershipAsReadRequest:
		return req.UserID, req.UserRole
	case *partnerships.SignPartnershipRequest:
		return req.UserID, req.UserRole
	case *partnerships.GetPartnershipSummaryRequest:
		return req.UserID, req.UserRole
	case *partnerships.GetIncomingPartnershipSummaryRequest:
		return req.UserID, req.UserRole
	default:
		t.Fatalf("unexpected request type %T", request)
		return "", ""
	}
}

type partnershipServiceSpy struct {
	calls          int
	operation      string
	request        any
	ctx            context.Context
	err            error
	panicValue     any
	waitForContext bool
}

func (s *partnershipServiceSpy) record(ctx context.Context, operation string, request any) error {
	s.calls++
	s.operation = operation
	s.request = request
	s.ctx = ctx
	if s.panicValue != nil {
		panic(s.panicValue)
	}
	if s.waitForContext {
		<-ctx.Done()
		return ctx.Err()
	}
	return s.err
}

func (s *partnershipServiceSpy) CreatePartnershipApplication(ctx context.Context, req *partnerships.CreatePartnershipApplicationRequest) (*partnerships.CreateResult, error) {
	if err := s.record(ctx, "CreatePartnershipApplication", req); err != nil {
		return nil, err
	}
	return &partnerships.CreateResult{ApplicationID: "PGJ000001", RequestCode: "PKS-2026-000001", Status: "DIAJUKAN"}, nil
}

func (s *partnershipServiceSpy) GetPartnershipApplication(ctx context.Context, req *partnerships.GetPartnershipApplicationRequest) (*partnerships.PartnershipRow, error) {
	if err := s.record(ctx, "GetPartnershipApplication", req); err != nil {
		return nil, err
	}
	return &partnerships.PartnershipRow{ID: "PGJ000001", CreatedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0)}, nil
}

func (s *partnershipServiceSpy) GetPartnershipApplications(ctx context.Context, req *partnerships.GetPartnershipApplicationsRequest) (*partnerships.GetPartnershipApplicationsResult, error) {
	if err := s.record(ctx, "GetPartnershipApplications", req); err != nil {
		return nil, err
	}
	return &partnerships.GetPartnershipApplicationsResult{}, nil
}

func (s *partnershipServiceSpy) GetIncomingPartnerships(ctx context.Context, req *partnerships.GetIncomingPartnershipsRequest) (*partnerships.GetIncomingPartnershipsResult, error) {
	if err := s.record(ctx, "GetIncomingPartnerships", req); err != nil {
		return nil, err
	}
	return &partnerships.GetIncomingPartnershipsResult{}, nil
}

func (s *partnershipServiceSpy) UpdatePartnershipStatus(ctx context.Context, req *partnerships.UpdatePartnershipStatusRequest) error {
	return s.record(ctx, "UpdatePartnershipStatus", req)
}

func (s *partnershipServiceSpy) MarkPartnershipAsRead(ctx context.Context, req *partnerships.MarkPartnershipAsReadRequest) error {
	return s.record(ctx, "MarkPartnershipAsRead", req)
}

func (s *partnershipServiceSpy) SignPartnership(ctx context.Context, req *partnerships.SignPartnershipRequest) error {
	return s.record(ctx, "SignPartnership", req)
}

func (s *partnershipServiceSpy) GetPartnershipSummary(ctx context.Context, req *partnerships.GetPartnershipSummaryRequest) (map[string]int, error) {
	if err := s.record(ctx, "GetPartnershipSummary", req); err != nil {
		return nil, err
	}
	return map[string]int{"DIAJUKAN": 1}, nil
}

func (s *partnershipServiceSpy) GetIncomingPartnershipSummary(ctx context.Context, req *partnerships.GetIncomingPartnershipSummaryRequest) (map[string]int, error) {
	if err := s.record(ctx, "GetIncomingPartnershipSummary", req); err != nil {
		return nil, err
	}
	return map[string]int{"DIAJUKAN": 1}, nil
}
