package parser

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
)

// boolAttrRe matches HTML-style boolean attributes (e.g. IncludeInResult>) that lack a value,
// which are invalid in XML. We normalise them to IncludeInResult="true"> so Go's strict
// encoding/xml parser can handle them.
var boolAttrRe = regexp.MustCompile(`(\s)(IncludeInResult)([\s/>])`)

func sanitizeXML(body []byte) []byte {
	return boolAttrRe.ReplaceAll(body, []byte(`${1}${2}="true"${3}`))
}

// XACMLRequest holds the extracted fields from a SOAP/XACML authorization query.
type XACMLRequest struct {
	BSN        string
	Categories []string
}

// XCPDRequest holds the extracted fields from a SOAP/XCPD patient discovery query.
type XCPDRequest struct {
	BSN       string
	SenderOrg string
}

// --- XACML XML structs (minimal, just what we need) ---

type xacmlEnvelope struct {
	XMLName xml.Name  `xml:"Envelope"`
	Body    xacmlBody `xml:"Body"`
}

type xacmlBody struct {
	Query xacmlQuery `xml:",any"`
}

type xacmlQuery struct {
	Request xacmlRequest `xml:"Request"`
}

type xacmlRequest struct {
	Attributes []xacmlAttributes `xml:"Attributes"`
}

type xacmlAttributes struct {
	Category  string           `xml:"Category,attr"`
	Attribute []xacmlAttribute `xml:"Attribute"`
}

type xacmlAttribute struct {
	AttributeId    string `xml:"AttributeId,attr"`
	AttributeValue string `xml:"AttributeValue"`
}

// ParseXACMLRequest extracts the patient BSN and gegevenscategorieen from an XACML request body.
func ParseXACMLRequest(body []byte) (*XACMLRequest, error) {
	var env xacmlEnvelope
	if err := xml.Unmarshal(sanitizeXML(body), &env); err != nil {
		return nil, fmt.Errorf("failed to parse XACML request: %w", err)
	}

	req := &XACMLRequest{}

	for _, attrs := range env.Body.Query.Request.Attributes {
		switch {
		case strings.HasSuffix(attrs.Category, ":resource"):
			for _, attr := range attrs.Attribute {
				if strings.HasSuffix(attr.AttributeId, "resource-id") {
					req.BSN = strings.TrimSpace(attr.AttributeValue)
				}
			}
		case strings.HasSuffix(attrs.Category, ":action"):
			for _, attr := range attrs.Attribute {
				if strings.HasSuffix(attr.AttributeId, "event-code") {
					val := strings.TrimSpace(attr.AttributeValue)
					// Strip OID prefix (e.g. "2.16.840.1.113883.2.4.3.111.5.10.1^1" → "1")
					if idx := strings.LastIndex(val, "^"); idx >= 0 {
						val = val[idx+1:]
					}
					req.Categories = append(req.Categories, val)
				}
			}
		}
	}

	if req.BSN == "" {
		return nil, fmt.Errorf("no patient BSN found in XACML request")
	}

	return req, nil
}

// --- XCPD XML structs (minimal) ---

type xcpdEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    xcpdBody `xml:"Body"`
}

type xcpdBody struct {
	Message xcpdMessage `xml:",any"`
}

type xcpdMessage struct {
	Sender            xcpdSender            `xml:"sender"`
	ControlActProcess xcpdControlActProcess `xml:"controlActProcess"`
}

type xcpdSender struct {
	Device xcpdDevice `xml:"device"`
}

type xcpdDevice struct {
	ID xcpdID `xml:"id"`
}

type xcpdID struct {
	Root      string `xml:"root,attr"`
	Extension string `xml:"extension,attr"`
}

type xcpdControlActProcess struct {
	QueryByParameter xcpdQueryByParameter `xml:"queryByParameter"`
}

type xcpdQueryByParameter struct {
	ParameterList xcpdParameterList `xml:"parameterList"`
}

type xcpdParameterList struct {
	LivingSubjectId xcpdLivingSubjectId `xml:"livingSubjectId"`
}

type xcpdLivingSubjectId struct {
	Value xcpdID `xml:"value"`
}

// ParseXCPDRequest extracts the patient BSN and sender org from an XCPD request body.
func ParseXCPDRequest(body []byte) (*XCPDRequest, error) {
	var env xcpdEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("failed to parse XCPD request: %w", err)
	}

	req := &XCPDRequest{}
	req.BSN = env.Body.Message.ControlActProcess.QueryByParameter.ParameterList.LivingSubjectId.Value.Extension
	req.SenderOrg = env.Body.Message.Sender.Device.ID.Root

	if req.BSN == "" {
		return nil, fmt.Errorf("no patient BSN found in XCPD request")
	}

	return req, nil
}
