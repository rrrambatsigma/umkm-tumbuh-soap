package soap

import "encoding/xml"

const (
	NSEnvelope = "http://schemas.xmlsoap.org/soap/envelope/"
	NSPartner  = "http://umkm-tumbuh.example.com/partnerships"
)

// Envelope is the root SOAP 1.1 element (for incoming requests).
type Envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Header  *Header  `xml:"Header,omitempty"`
	Body    Body     `xml:"Body"`
}

// Header is an optional SOAP header.
type Header struct {
	Content []byte `xml:",innerxml"`
}

// Body holds the SOAP payload as raw bytes for flexible dispatch.
type Body struct {
	Content []byte `xml:",innerxml"`
}

// ResponseEnvelope wraps any response struct in a SOAP envelope.
type ResponseEnvelope struct {
	XMLName xml.Name     `xml:"soap:Envelope"`
	XMLNS   string       `xml:"xmlns:soap,attr"`
	XMLNSP  string       `xml:"xmlns:tns,attr"`
	Body    ResponseBody `xml:"soap:Body"`
}

// ResponseBody holds the response content and implements xml.Marshaler.
type ResponseBody struct {
	Content interface{}
}

// MarshalXML encodes the body content into XML.
func (b ResponseBody) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	if err := e.Encode(b.Content); err != nil {
		return err
	}
	return e.EncodeToken(start.End())
}

// NewResponseEnvelope creates a properly namespaced SOAP response envelope.
func NewResponseEnvelope(content interface{}) ResponseEnvelope {
	return ResponseEnvelope{
		XMLNS:  NSEnvelope,
		XMLNSP: NSPartner,
		Body:   ResponseBody{Content: content},
	}
}
