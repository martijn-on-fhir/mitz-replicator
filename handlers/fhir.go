package handlers

import (
	"bytes"
	"log"
	"net/http"
	"strings"
	"text/template"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"mitz-replicator/parser"
)

const fhirContentType = "application/fhir+xml; charset=utf-8"

// --- Template data types ---

// FhirSubscriptionData is the template data for fhir_subscription.xml.
type FhirSubscriptionData struct {
	SubscriptionID string
	Criteria       string
	Endpoint       string
	PayloadType    string
}

// FhirBundleResponseEntry represents one entry in a Bundle transaction-response.
type FhirBundleResponseEntry struct {
	Status   string
	Location string
}

// FhirBundleResponseData is the template data for fhir_bundle_response.xml.
type FhirBundleResponseData struct {
	BundleID string
	Entries  []FhirBundleResponseEntry
}

// FhirProcessingStatusData is the template data for fhir_processing_status.xml.
type FhirProcessingStatusData struct {
	Count int
}

// FhirOperationOutcomeData is the template data for fhir_operation_outcome.xml.
type FhirOperationOutcomeData struct {
	Severity    string
	Code        string
	Diagnostics string
}

// --- Template variables ---

var (
	fhirSubscriptionTmpl     *template.Template
	fhirBundleResponseTmpl   *template.Template
	fhirProcessingStatusTmpl *template.Template
	fhirOperationOutcomeTmpl *template.Template
)

// InitFhirTemplates loads the FHIR response templates.
func InitFhirTemplates(subscriptionXML, bundleResponseXML, processingStatusXML, operationOutcomeXML string) {
	fhirSubscriptionTmpl = template.Must(template.New("fhir_subscription").Parse(subscriptionXML))
	fhirBundleResponseTmpl = template.Must(template.New("fhir_bundle_response").Parse(bundleResponseXML))
	fhirProcessingStatusTmpl = template.Must(template.New("fhir_processing_status").Parse(processingStatusXML))
	fhirOperationOutcomeTmpl = template.Must(template.New("fhir_operation_outcome").Parse(operationOutcomeXML))
}

// HandleFhirSubscriptionCreate handles POST /fhir/Subscription — create consent subscription (OTV-TR-0120).
func HandleFhirSubscriptionCreate(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		log.Printf("[FHIR] Failed to read Subscription request body: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}

	req, err := parser.ParseFhirSubscription(body)
	if err != nil {
		log.Printf("[FHIR] Failed to parse Subscription: %v", err)
		renderFhirError(c, http.StatusBadRequest, "invalid", "processing", "Failed to parse Subscription request")
		return
	}

	requestID := c.GetHeader("X-Request-Id")
	log.Printf("[FHIR] POST /Subscription RequestId=%s BSN=%s ProviderID=%s", requestID, req.BSN, req.ProviderID)

	// BSN-based routing
	switch req.BSN {
	case "000000003":
		renderFhirError(c, http.StatusBadRequest, "error", "processing", "Patient BSN not found in register")
		return
	case "000000004":
		c.Header("Retry-After", "30")
		renderFhirError(c, http.StatusTooManyRequests, "error", "throttled", "Rate limit exceeded — retry after 30s")
		return
	case "000000005":
		renderFhirError(c, http.StatusInternalServerError, "fatal", "exception", "Internal server error")
		return
	}

	// Success: return 202 Accepted with Subscription resource
	data := FhirSubscriptionData{
		SubscriptionID: uuid.New().String(),
		Criteria:       req.Criteria,
		Endpoint:       req.Endpoint,
		PayloadType:    req.PayloadType,
	}

	var buf bytes.Buffer
	if err := fhirSubscriptionTmpl.Execute(&buf, data); err != nil {
		log.Printf("[FHIR] Subscription template error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusAccepted, fhirContentType, buf.Bytes())
}

// HandleFhirSubscriptionDelete handles DELETE /fhir/Subscription/:id — cancel subscription (OTV-TR-0130).
func HandleFhirSubscriptionDelete(c *gin.Context) {
	subID := c.Param("id")
	requestID := c.GetHeader("X-Request-Id")
	log.Printf("[FHIR] DELETE /Subscription/%s RequestId=%s", subID, requestID)

	// Specific IDs that return errors
	switch subID {
	case "00000000-0000-0000-0000-000000000004":
		renderFhirError(c, http.StatusNotFound, "error", "not-found", "Subscription not found")
		return
	case "00000000-0000-0000-0000-000000000005":
		renderFhirError(c, http.StatusInternalServerError, "fatal", "exception", "Internal server error")
		return
	}

	c.Status(http.StatusNoContent)
}

// HandleFhirProcessingStatus handles GET /fhir/{Subscription|Consent}/$processingStatus.
func HandleFhirProcessingStatus(c *gin.Context) {
	providerID := c.Query("providerid")
	requestID := c.GetHeader("X-Request-Id")

	resourceType := "Subscription"
	if strings.Contains(c.Request.URL.Path, "/Consent/") {
		resourceType = "Consent"
	}

	log.Printf("[FHIR] GET %s/$processingStatus RequestId=%s ProviderID=%s", resourceType, requestID, providerID)

	// Provider-based routing
	switch providerID {
	case "00000003":
		renderProcessingStatus(c, 5)
		return
	case "00000004":
		renderProcessingStatus(c, 42)
		return
	case "00000005":
		renderFhirError(c, http.StatusBadRequest, "error", "processing", "Provider not found in register")
		return
	}

	// Default: all processed
	renderProcessingStatus(c, 0)
}

// HandleFhirBundle handles POST /fhir/ — Bundle transaction (migration OTV-TR-0150, toestemmingsknop OTV-TR-0160).
func HandleFhirBundle(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		log.Printf("[FHIR] Failed to read Bundle request body: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}

	req, err := parser.ParseFhirBundle(body)
	if err != nil {
		log.Printf("[FHIR] Failed to parse Bundle: %v", err)
		renderFhirError(c, http.StatusBadRequest, "error", "processing", "Failed to parse Bundle request")
		return
	}

	requestID := c.GetHeader("X-Request-Id")
	txType := "migration"
	if req.HasProvenance {
		txType = "toestemmingsknop"
	}
	log.Printf("[FHIR] POST / Bundle RequestId=%s BSN=%s Type=%s Entries=%d",
		requestID, req.BSN, txType, req.EntryCount)

	// BSN-based routing
	switch req.BSN {
	case "000000003":
		renderFhirError(c, http.StatusBadRequest, "error", "processing", "Patient BSN not found in register")
		return
	case "000000004":
		c.Header("Retry-After", "30")
		renderFhirError(c, http.StatusTooManyRequests, "error", "throttled", "Rate limit exceeded — retry after 30s")
		return
	case "000000005":
		renderFhirError(c, http.StatusInternalServerError, "fatal", "exception", "Internal server error")
		return
	}

	// Build response entries matching the input resources
	entries := []FhirBundleResponseEntry{
		{Status: "201 Created", Location: "Patient/" + uuid.New().String()},
	}
	if req.HasOrganization {
		entries = append(entries, FhirBundleResponseEntry{
			Status:   "201 Created",
			Location: "Organization/" + uuid.New().String(),
		})
	}
	if req.HasConsent {
		entries = append(entries, FhirBundleResponseEntry{
			Status:   "201 Created",
			Location: "Consent/" + uuid.New().String(),
		})
	}
	if req.HasProvenance {
		entries = append(entries, FhirBundleResponseEntry{
			Status:   "201 Created",
			Location: "Provenance/" + uuid.New().String(),
		})
	}

	data := FhirBundleResponseData{
		BundleID: uuid.New().String(),
		Entries:  entries,
	}

	var buf bytes.Buffer
	if err := fhirBundleResponseTmpl.Execute(&buf, data); err != nil {
		log.Printf("[FHIR] Bundle response template error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, fhirContentType, buf.Bytes())
}

// --- Rendering helpers ---

func renderProcessingStatus(c *gin.Context, count int) {
	data := FhirProcessingStatusData{Count: count}

	var buf bytes.Buffer
	if err := fhirProcessingStatusTmpl.Execute(&buf, data); err != nil {
		log.Printf("[FHIR] Processing status template error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, fhirContentType, buf.Bytes())
}

func renderFhirError(c *gin.Context, status int, severity, code, diagnostics string) {
	data := FhirOperationOutcomeData{
		Severity:    severity,
		Code:        code,
		Diagnostics: diagnostics,
	}

	var buf bytes.Buffer
	if err := fhirOperationOutcomeTmpl.Execute(&buf, data); err != nil {
		log.Printf("[FHIR] OperationOutcome template error: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(status, fhirContentType, buf.Bytes())
}