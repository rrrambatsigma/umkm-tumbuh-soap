package soap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/partnerships"
)

func TestSOAPServicePanicBecomesSafeFault(t *testing.T) {
	const panicMarker = "password=secret relation=private_table"
	var logs bytes.Buffer
	spy := &partnershipServiceSpy{panicValue: panicMarker}
	handler := NewHandlerWithConfig(spy, "test wsdl", HandlerConfig{
		JWTSecret: testJWTSecret,
		Logger:    log.New(&logs, "", 0),
	})
	req := authenticatedOperationRequest(t, soapOperationCase{name: "GetPartnershipSummary"})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertSOAPFault(t, rr, http.StatusInternalServerError)
	if strings.Contains(rr.Body.String(), panicMarker) {
		t.Fatalf("panic details leaked to client: %s", rr.Body.String())
	}
	if !strings.Contains(logs.String(), "panic") {
		t.Fatalf("panic was not logged internally: %q", logs.String())
	}
}

func TestSOAPDatabaseErrorIsLoggedButNotExposed(t *testing.T) {
	const internalMarker = "SQLSTATE 42P01 relation private_table password=secret"
	var logs bytes.Buffer
	spy := &partnershipServiceSpy{err: errors.New(internalMarker)}
	handler := NewHandlerWithConfig(spy, "test wsdl", HandlerConfig{
		JWTSecret: testJWTSecret,
		Logger:    log.New(&logs, "", 0),
	})
	req := authenticatedOperationRequest(t, soapOperationCase{name: "GetPartnershipSummary"})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertSOAPFault(t, rr, http.StatusInternalServerError)
	if strings.Contains(rr.Body.String(), internalMarker) || strings.Contains(rr.Body.String(), "private_table") {
		t.Fatalf("database details leaked to client: %s", rr.Body.String())
	}
	if !strings.Contains(logs.String(), internalMarker) {
		t.Fatalf("underlying database error was not logged: %q", logs.String())
	}
}

func TestSOAPBusinessFaultStatusesRemainCompatible(t *testing.T) {
	tests := []struct {
		code int
		want int
	}{
		{code: http.StatusBadRequest, want: http.StatusBadRequest},
		{code: http.StatusForbidden, want: http.StatusForbidden},
		{code: http.StatusNotFound, want: http.StatusNotFound},
		{code: http.StatusConflict, want: http.StatusConflict},
		{code: http.StatusUnprocessableEntity, want: http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status %d", tt.code), func(t *testing.T) {
			spy := &partnershipServiceSpy{err: &partnerships.AppError{Code: tt.code, Message: "business fault"}}
			handler := newTestHandler(spy, HandlerConfig{JWTSecret: testJWTSecret})
			req := authenticatedOperationRequest(t, soapOperationCase{name: "GetPartnershipSummary"})
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assertSOAPFault(t, rr, tt.want)
			if !strings.Contains(rr.Body.String(), "business fault") {
				t.Fatalf("business fault message was not preserved: %s", rr.Body.String())
			}
		})
	}
}

func TestSOAPMalformedRequestsReturnGenericFaultsWithoutServiceCall(t *testing.T) {
	token := validTestToken(t, "AKUN-UMKM", "UMKM")
	tests := []struct {
		name       string
		body       string
		soapAction string
	}{
		{name: "malformed envelope", body: `<soap:Envelope><`},
		{
			name:       "SOAPAction body mismatch",
			body:       operationEnvelope(allSOAPOperationCases()[0], identityElements(stringPointer("AKUN-UMKM"), stringPointer("UMKM"))),
			soapAction: `"http://umkm-tumbuh.example.com/partnerships/GetPartnershipSummary"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &partnershipServiceSpy{}
			handler := newTestHandler(spy, HandlerConfig{JWTSecret: testJWTSecret})
			req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+token)
			if tt.soapAction != "" {
				req.Header.Set("SOAPAction", tt.soapAction)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assertSOAPFault(t, rr, http.StatusBadRequest)
			if spy.calls != 0 {
				t.Fatalf("service calls = %d, want 0", spy.calls)
			}
			for _, marker := range []string{"syntax error", "expected element type", "CreatePartnershipApplication"} {
				if strings.Contains(rr.Body.String(), marker) {
					t.Fatalf("parser or operation detail %q leaked: %s", marker, rr.Body.String())
				}
			}
		})
	}
}

func TestSOAPBodyLimitHandlesExactBoundaryAndContentLengthSpoof(t *testing.T) {
	operation := soapOperationCase{name: "GetPartnershipSummary"}
	body := operationEnvelope(operation, identityElements(stringPointer("AKUN-UMKM"), stringPointer("UMKM")))
	token := validTestToken(t, "AKUN-UMKM", "UMKM")

	t.Run("exact limit accepted", func(t *testing.T) {
		spy := &partnershipServiceSpy{}
		handler := newTestHandler(spy, HandlerConfig{JWTSecret: testJWTSecret, MaxRequestBytes: int64(len(body))})
		req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK || spy.calls != 1 {
			t.Fatalf("status/calls = %d/%d, want 200/1; body=%q", rr.Code, spy.calls, rr.Body.String())
		}
	})

	for _, tt := range []struct {
		name          string
		contentLength int64
	}{
		{name: "one byte over limit", contentLength: int64(len(body))},
		{name: "lying content length", contentLength: 1},
		{name: "unknown content length", contentLength: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spy := &partnershipServiceSpy{}
			handler := newTestHandler(spy, HandlerConfig{JWTSecret: testJWTSecret, MaxRequestBytes: int64(len(body) - 1)})
			req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(body))
			req.ContentLength = tt.contentLength
			req.Header.Set("Authorization", "Bearer "+token)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assertSOAPFault(t, rr, http.StatusRequestEntityTooLarge)
			if spy.calls != 0 {
				t.Fatalf("service calls = %d, want 0", spy.calls)
			}
		})
	}
}

func TestInvalidAuthenticationDoesNotReadRequestBody(t *testing.T) {
	spy := &partnershipServiceSpy{}
	handler := newTestHandler(spy, HandlerConfig{JWTSecret: testJWTSecret})
	body := &trackingReadCloser{}
	req := httptest.NewRequest(http.MethodPost, "/partnership", nil)
	req.Body = body
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertSOAPFault(t, rr, http.StatusUnauthorized)
	if body.reads != 0 {
		t.Fatalf("body read calls = %d, want 0", body.reads)
	}
	if spy.calls != 0 {
		t.Fatalf("service calls = %d, want 0", spy.calls)
	}
}

func TestSOAPRequestDeadlineBoundsServiceWork(t *testing.T) {
	spy := &partnershipServiceSpy{waitForContext: true}
	handler := newTestHandler(spy, HandlerConfig{
		JWTSecret:      testJWTSecret,
		RequestTimeout: 10 * time.Millisecond,
	})
	req := authenticatedOperationRequest(t, soapOperationCase{name: "GetPartnershipSummary"})
	rr := httptest.NewRecorder()
	started := time.Now()

	handler.ServeHTTP(rr, req)

	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("deadline did not bound service work; elapsed=%s", elapsed)
	}
	assertSOAPFault(t, rr, http.StatusInternalServerError)
	if strings.Contains(rr.Body.String(), context.DeadlineExceeded.Error()) {
		t.Fatalf("deadline internals leaked: %s", rr.Body.String())
	}
}

func authenticatedOperationRequest(t *testing.T, operation soapOperationCase) *http.Request {
	t.Helper()
	token := validTestToken(t, "AKUN-UMKM", "UMKM")
	req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(operationEnvelope(
		operation, identityElements(stringPointer("AKUN-UMKM"), stringPointer("UMKM")),
	)))
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func validTestToken(t *testing.T, userID, role string) string {
	t.Helper()
	return signedTestJWT(t, map[string]any{
		"sub": userID, "role": role, "exp": time.Now().Add(time.Hour).Unix(),
	}, testJWTSecret, "HS256")
}

type trackingReadCloser struct {
	reads int
}

func (b *trackingReadCloser) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func (b *trackingReadCloser) Close() error { return nil }
