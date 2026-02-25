package handlers

import (
	"bytes"
	"log"
	"net/http"
	"strings"
	"text/template"

	"github.com/gin-gonic/gin"

	"mitz-replicator/parser"
)

// XACMLResult holds a single decision result for template rendering.
type XACMLResult struct {
	Decision  string
	EventCode string
}

// XACMLResponseData is the template data for xacml_response.xml.
type XACMLResponseData struct {
	Results []XACMLResult
}

// FaultData is the template data for SOAP fault responses.
type FaultData struct {
	FaultCode    string
	FaultSubcode string
	FaultReason  string
	FaultDetail  string
}

var (
	xacmlResponseTmpl *template.Template
	xacmlFaultTmpl    *template.Template
)

// InitXACMLTemplates loads the XACML response templates.
func InitXACMLTemplates(responseXML, faultXML string) {
	xacmlResponseTmpl = template.Must(template.New("xacml_response").Parse(responseXML))
	xacmlFaultTmpl = template.Must(template.New("xacml_fault").Parse(faultXML))
}

const soapContentType = "application/soap+xml; charset=utf-8"

// HandleXACML handles POST /xacml — gesloten autorisatievraag.
func HandleXACML(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		log.Printf("[XACML] Failed to read request body: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}

	req, err := parser.ParseXACMLRequest(body)
	if err != nil {
		log.Printf("[XACML] Failed to parse request: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}

	requestID := c.GetHeader("X-Request-Id")
	log.Printf("[XACML] RequestId=%s BSN=%s Categories=%v", requestID, req.BSN, req.Categories)

	// Route on BSN pattern
	switch req.BSN {
	case "000000005":
		renderXACMLFault(c)
		return
	}

	// Build results based on BSN
	results := buildXACMLResults(req.BSN, req.Categories)

	var buf bytes.Buffer
	if err := xacmlResponseTmpl.Execute(&buf, XACMLResponseData{Results: results}); err != nil {
		log.Printf("[XACML] Template error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, soapContentType, buf.Bytes())
}

func buildXACMLResults(bsn string, categories []string) []XACMLResult {
	results := make([]XACMLResult, len(categories))

	for i, cat := range categories {
		var decision string

		switch bsn {
		case "000000001":
			decision = "Permit"
		case "000000002":
			decision = "Deny"
		case "000000003":
			if i == 0 {
				decision = "Permit"
			} else {
				decision = "Deny"
			}
		case "000000004":
			decision = "Indeterminate"
		default:
			// 999* and anything else → all Permit
			if strings.HasPrefix(bsn, "999") {
				decision = "Permit"
			} else {
				decision = "Permit"
			}
		}

		results[i] = XACMLResult{
			Decision:  decision,
			EventCode: cat,
		}
	}

	return results
}

func renderXACMLFault(c *gin.Context) {
	data := FaultData{
		FaultCode:    "soap:Sender",
		FaultSubcode: "mitz:InvalidRequest",
		FaultReason:  "Patient BSN not found in register",
		FaultDetail:  "The requested BSN is not known in the Mitz consent register",
	}

	var buf bytes.Buffer
	if err := xacmlFaultTmpl.Execute(&buf, data); err != nil {
		log.Printf("[XACML] Fault template error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, soapContentType, buf.Bytes())
}
