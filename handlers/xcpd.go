package handlers

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"mitz-replicator/parser"
)

// XCPDLocation represents a single location in the XCPD response.
type XCPDLocation struct {
	PatientID    string
	SourceID     string
	CustodianOID string
	EventCodes   []string
}

// XCPDFoundData is the template data for xcpd_found.xml.
type XCPDFoundData struct {
	ResponseID   string
	Timestamp    string
	RequestedBSN string
	Locations    []XCPDLocation
}

var (
	xcpdFoundTmpl *template.Template
	xcpdEmptyTmpl *template.Template
	xcpdFaultTmpl *template.Template
)

// InitXCPDTemplates loads the XCPD response templates.
func InitXCPDTemplates(foundXML, emptyXML, faultXML string) {
	xcpdFoundTmpl = template.Must(template.New("xcpd_found").Parse(foundXML))
	xcpdEmptyTmpl = template.Must(template.New("xcpd_empty").Parse(emptyXML))
	xcpdFaultTmpl = template.Must(template.New("xcpd_fault").Parse(faultXML))
}

// HandleXCPD handles POST /xcpd — open autorisatievraag.
func HandleXCPD(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		log.Printf("[XCPD] Failed to read request body: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}

	req, err := parser.ParseXCPDRequest(body)
	if err != nil {
		log.Printf("[XCPD] Failed to parse request: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}

	requestID := c.GetHeader("X-Request-Id")
	log.Printf("[XCPD] RequestId=%s BSN=%s SenderOrg=%s", requestID, req.BSN, req.SenderOrg)

	switch req.BSN {
	case "000000001":
		renderXCPDFound(c, req.BSN, twoLocationsMultipleEvents())
	case "000000002":
		renderXCPDFound(c, req.BSN, oneLocationOneEvent())
	case "000000003":
		renderXCPDEmpty(c)
	case "000000004", "000000005":
		renderXCPDFault(c)
	default:
		if strings.HasPrefix(req.BSN, "999") {
			renderXCPDFound(c, req.BSN, defaultLocation())
		} else {
			renderXCPDFound(c, req.BSN, defaultLocation())
		}
	}
}

func twoLocationsMultipleEvents() []XCPDLocation {
	return []XCPDLocation{
		{
			PatientID:    "123456789",
			SourceID:     "1.2.3.4.5.6.7",
			CustodianOID: "urn:oid:2.16.840.1.113883.2.4.6.6",
			EventCodes:   []string{"huisartsgegevens", "medicatiegegevens"},
		},
		{
			PatientID:    "987654321",
			CustodianOID: "urn:oid:2.16.840.1.113883.2.4.3.11",
			EventCodes:   []string{"medicatiegegevens"},
		},
	}
}

func oneLocationOneEvent() []XCPDLocation {
	return []XCPDLocation{
		{
			PatientID:    "111222333",
			CustodianOID: "urn:oid:2.16.840.1.113883.2.4.6.6",
			EventCodes:   []string{"huisartsgegevens"},
		},
	}
}

func defaultLocation() []XCPDLocation {
	return []XCPDLocation{
		{
			PatientID:    "555666777",
			SourceID:     "1.2.3.4.5.6.8",
			CustodianOID: "urn:oid:2.16.840.1.113883.2.4.6.6",
			EventCodes:   []string{"huisartsgegevens", "medicatiegegevens"},
		},
	}
}

func renderXCPDFound(c *gin.Context, bsn string, locations []XCPDLocation) {
	data := XCPDFoundData{
		ResponseID:   uuid.New().String(),
		Timestamp:    time.Now().Format("20060102150405"),
		RequestedBSN: bsn,
		Locations:    locations,
	}

	var buf bytes.Buffer
	if err := xcpdFoundTmpl.Execute(&buf, data); err != nil {
		log.Printf("[XCPD] Template error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, soapContentType, buf.Bytes())
}

func renderXCPDEmpty(c *gin.Context) {
	var buf bytes.Buffer
	if err := xcpdEmptyTmpl.Execute(&buf, nil); err != nil {
		log.Printf("[XCPD] Empty template error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, soapContentType, buf.Bytes())
}

func renderXCPDFault(c *gin.Context) {
	data := FaultData{
		FaultCode:    "soap:Sender",
		FaultSubcode: "mitz:InvalidRequest",
		FaultReason:  "Patient BSN not found in register",
		FaultDetail:  fmt.Sprintf("RequestId: %s", c.GetHeader("X-Request-Id")),
	}

	var buf bytes.Buffer
	if err := xcpdFaultTmpl.Execute(&buf, data); err != nil {
		log.Printf("[XCPD] Fault template error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, soapContentType, buf.Bytes())
}
