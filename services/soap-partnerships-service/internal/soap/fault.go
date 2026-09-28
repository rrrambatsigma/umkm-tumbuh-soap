package soap

import (
	"encoding/xml"
	"net/http"
)

// FaultCode represents standard SOAP 1.1 fault codes.
type FaultCode string

const (
	FaultClient FaultCode = "soap:Client"
	FaultServer FaultCode = "soap:Server"
)

// Fault is a SOAP 1.1 Fault element.
type Fault struct {
	XMLName     xml.Name  `xml:"soap:Fault"`
	FaultCode   FaultCode `xml:"faultcode"`
	FaultString string    `xml:"faultstring"`
	Detail      string    `xml:"detail,omitempty"`
}

// FaultEnvelope is a SOAP envelope containing a Fault.
type FaultEnvelope struct {
	XMLName xml.Name  `xml:"soap:Envelope"`
	XMLNS   string    `xml:"xmlns:soap,attr"`
	Body    FaultBody `xml:"soap:Body"`
}

// FaultBody wraps the Fault.
type FaultBody struct {
	Fault Fault
}

// WriteFault writes a complete SOAP fault response.
func WriteFault(w http.ResponseWriter, httpStatus int, code FaultCode, message string) {
	env := FaultEnvelope{
		XMLNS: NSEnvelope,
		Body: FaultBody{
			Fault: Fault{
				FaultCode:   code,
				FaultString: message,
			},
		},
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(httpStatus)
	w.Write([]byte(xml.Header)) //nolint:errcheck
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	enc.Encode(env) //nolint:errcheck
}

// WriteClientFault writes a SOAP Client fault (400 Bad Request).
func WriteClientFault(w http.ResponseWriter, message string) {
	WriteFault(w, http.StatusBadRequest, FaultClient, message)
}

// WriteUnauthorizedFault writes an authentication SOAP fault (401).
func WriteUnauthorizedFault(w http.ResponseWriter, message string) {
	WriteFault(w, http.StatusUnauthorized, FaultClient, message)
}

// WriteForbiddenFault writes an authorization SOAP fault (403).
func WriteForbiddenFault(w http.ResponseWriter, message string) {
	WriteFault(w, http.StatusForbidden, FaultClient, message)
}

// WritePayloadTooLargeFault writes an oversized-request SOAP fault (413).
func WritePayloadTooLargeFault(w http.ResponseWriter, message string) {
	WriteFault(w, http.StatusRequestEntityTooLarge, FaultClient, message)
}

// WriteServerFault writes a SOAP Server fault (500 Internal Server Error).
func WriteServerFault(w http.ResponseWriter, message string) {
	WriteFault(w, http.StatusInternalServerError, FaultServer, message)
}

// WriteNotFoundFault writes a not-found SOAP fault (404).
func WriteNotFoundFault(w http.ResponseWriter, message string) {
	WriteFault(w, http.StatusNotFound, FaultClient, message)
}

// WriteConflictFault writes a conflict SOAP fault (409).
func WriteConflictFault(w http.ResponseWriter, message string) {
	WriteFault(w, http.StatusConflict, FaultClient, message)
}

// WriteUnprocessableFault writes an unprocessable entity SOAP fault (422).
func WriteUnprocessableFault(w http.ResponseWriter, message string) {
	WriteFault(w, http.StatusUnprocessableEntity, FaultClient, message)
}
