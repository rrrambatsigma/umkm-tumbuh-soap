package soap

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/partnerships"
)

const testJWTSecret = "stage-1-test-secret"

func TestSOAPAuthenticationRejectsInvalidCredentials(t *testing.T) {
	now := time.Now()
	validClaims := map[string]any{
		"sub":  "AKUN-001",
		"role": "UMKM",
		"exp":  now.Add(time.Hour).Unix(),
	}

	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing authorization"},
		{name: "wrong scheme", authorization: "Basic credentials"},
		{name: "bearer without token", authorization: "Bearer"},
		{name: "too many header fields", authorization: "Bearer token extra"},
		{name: "bad signature", authorization: "Bearer " + signedTestJWT(t, validClaims, "different-secret", "HS256")},
		{name: "expired", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "AKUN-001", "role": "UMKM", "exp": now.Add(-time.Minute).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "missing expiration", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "AKUN-001", "role": "UMKM",
		}, testJWTSecret, "HS256")},
		{name: "future not before", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "AKUN-001", "role": "UMKM", "exp": now.Add(time.Hour).Unix(), "nbf": now.Add(time.Minute).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "missing subject", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"role": "UMKM", "exp": now.Add(time.Hour).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "blank subject", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "   ", "role": "UMKM", "exp": now.Add(time.Hour).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "missing role", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "AKUN-001", "exp": now.Add(time.Hour).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "unknown role", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "AKUN-001", "role": "OWNER", "exp": now.Add(time.Hour).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "lowercase role", authorization: "Bearer " + signedTestJWT(t, map[string]any{
			"sub": "AKUN-001", "role": "umkm", "exp": now.Add(time.Hour).Unix(),
		}, testJWTSecret, "HS256")},
		{name: "wrong algorithm", authorization: "Bearer " + signedTestJWT(t, validClaims, testJWTSecret, "HS384")},
		{name: "unsigned token", authorization: "Bearer " + signedTestJWT(t, validClaims, "", "none")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", testJWTSecret)
			handler := NewHandler(nil, "test wsdl")
			req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(unknownOperationEnvelope()))
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assertSOAPFault(t, rr, http.StatusUnauthorized)
		})
	}
}

func TestSOAPAuthenticationRejectsDisallowedRole(t *testing.T) {
	t.Setenv("JWT_SECRET", testJWTSecret)
	token := signedTestJWT(t, map[string]any{
		"sub":  "AKUN-ADMIN",
		"role": "ADMIN",
		"exp":  time.Now().Add(time.Hour).Unix(),
	}, testJWTSecret, "HS256")
	handler := NewHandler(nil, "test wsdl")
	req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(unknownOperationEnvelope()))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertSOAPFault(t, rr, http.StatusForbidden)
}

func TestSOAPAuthenticationFailsClosedWithoutServerSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	token := signedTestJWT(t, map[string]any{
		"sub":  "AKUN-001",
		"role": "UMKM",
		"exp":  time.Now().Add(time.Hour).Unix(),
	}, testJWTSecret, "HS256")
	handler := NewHandler(nil, "test wsdl")
	req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(unknownOperationEnvelope()))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertSOAPFault(t, rr, http.StatusInternalServerError)
}

func TestWSDLIsAnonymousOnlyForGET(t *testing.T) {
	t.Setenv("JWT_SECRET", testJWTSecret)
	handler := NewHandler(nil, "test wsdl")

	t.Run("GET serves WSDL", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/partnership?wsdl", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		if rr.Body.String() != "test wsdl" {
			t.Fatalf("body = %q, want WSDL", rr.Body.String())
		}
	})

	tests := []struct {
		method string
		want   int
	}{
		{method: http.MethodPost, want: http.StatusUnauthorized},
		{method: http.MethodPut, want: http.StatusMethodNotAllowed},
		{method: http.MethodDelete, want: http.StatusMethodNotAllowed},
		{method: http.MethodHead, want: http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.method+" cannot fetch WSDL", func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/partnership?wsdl", strings.NewReader(unknownOperationEnvelope()))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%q", rr.Code, tt.want, rr.Body.String())
			}
			if rr.Body.String() == "test wsdl" {
				t.Fatal("non-GET request bypassed method validation and received the WSDL")
			}
		})
	}
}

func TestSOAPRequestBodyLimit(t *testing.T) {
	t.Setenv("JWT_SECRET", testJWTSecret)
	token := signedTestJWT(t, map[string]any{
		"sub":  "AKUN-001",
		"role": "UMKM",
		"exp":  time.Now().Add(time.Hour).Unix(),
	}, testJWTSecret, "HS256")
	handler := NewHandler(nil, "test wsdl")
	body := `<?xml version="1.0"?><soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><Unknown>` +
		strings.Repeat("x", (1<<20)+1) +
		`</Unknown></soap:Body></soap:Envelope>`
	req := httptest.NewRequest(http.MethodPost, "/partnership", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertSOAPFault(t, rr, http.StatusRequestEntityTooLarge)
}

func TestSOAPInternalErrorsAreSanitized(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "ordinary error", err: fmt.Errorf("password=secret SQLSTATE 42P01")},
		{name: "application 500", err: &partnerships.AppError{Code: http.StatusInternalServerError, Message: "relation private_table does not exist"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{}
			rr := httptest.NewRecorder()

			handler.writeError(rr, tt.err)

			assertSOAPFault(t, rr, http.StatusInternalServerError)
			for _, marker := range []string{"password=secret", "SQLSTATE", "private_table"} {
				if strings.Contains(rr.Body.String(), marker) {
					t.Fatalf("SOAP fault leaked internal marker %q: %s", marker, rr.Body.String())
				}
			}
		})
	}
}

func signedTestJWT(t *testing.T, claims map[string]any, secret, algorithm string) string {
	t.Helper()
	header := map[string]any{"alg": algorithm, "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)

	var signature []byte
	switch algorithm {
	case "HS256":
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(unsigned))
		signature = mac.Sum(nil)
	case "HS384":
		mac := hmac.New(sha512.New384, []byte(secret))
		_, _ = mac.Write([]byte(unsigned))
		signature = mac.Sum(nil)
	case "none":
		return unsigned + "."
	default:
		t.Fatalf("unsupported test algorithm %q", algorithm)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func unknownOperationEnvelope() string {
	return `<?xml version="1.0"?>` +
		`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">` +
		`<soap:Body><UnknownOperation/></soap:Body></soap:Envelope>`
}

func assertSOAPFault(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%q", rr.Code, wantStatus, rr.Body.String())
	}
	if contentType := rr.Header().Get("Content-Type"); contentType != "text/xml; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want SOAP XML", contentType)
	}
	var envelope struct {
		XMLName xml.Name   `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
		Attrs   []xml.Attr `xml:",any,attr"`
		Body    struct {
			Fault struct {
				Code    string `xml:"faultcode"`
				Message string `xml:"faultstring"`
			} `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault"`
		} `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
	}
	if err := xml.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid SOAP Fault XML: %v; body=%s", err, rr.Body.String())
	}
	prefix, local, qualified := strings.Cut(envelope.Body.Fault.Code, ":")
	wantCode := "Client"
	if wantStatus >= 500 {
		wantCode = "Server"
	}
	if !qualified || local != wantCode {
		t.Fatalf("faultcode = %q, want namespace-qualified %s", envelope.Body.Fault.Code, wantCode)
	}
	bound := false
	for _, attr := range envelope.Attrs {
		if attr.Name.Space == "xmlns" && attr.Name.Local == prefix && attr.Value == "http://schemas.xmlsoap.org/soap/envelope/" {
			bound = true
		}
	}
	if !bound || strings.TrimSpace(envelope.Body.Fault.Message) == "" {
		t.Fatalf("SOAP Fault lacks a bound code namespace or faultstring: %s", rr.Body.String())
	}
}
