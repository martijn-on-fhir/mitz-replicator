package auth

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/gin-gonic/gin"
	dsig "github.com/russellhaering/goxmldsig"
)

// SamlValidatorConfig holds the configuration for SAML assertion validation.
type SamlValidatorConfig struct {
	Enabled       bool
	SigningCert   []byte // PEM-encoded certificate
	ExpectedIssuer string
	ClockSkew     time.Duration
}

// SamlValidator validates SAML assertions extracted from Authorization headers.
type SamlValidator struct {
	config    SamlValidatorConfig
	certStore dsig.MemoryX509CertificateStore
}

// NewSamlValidator creates a validator from the given config.
// Returns an error if the PEM certificate cannot be parsed.
func NewSamlValidator(config SamlValidatorConfig) (*SamlValidator, error) {

	v := &SamlValidator{
		config: config,
	}

	if !config.Enabled {
		return v, nil
	}

	block, _ := pem.Decode(config.SigningCert)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from SAML signing certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SAML signing certificate: %w", err)
	}

	v.certStore = dsig.MemoryX509CertificateStore{
		Roots: []*x509.Certificate{cert},
	}

	return v, nil
}

// IsEnabled returns whether SAML validation is active.
func (v *SamlValidator) IsEnabled() bool {

	return v.config.Enabled
}

// ValidateFromHeader extracts a SAML assertion from the Authorization header
// ("SAML <base64>") and validates it.
func (v *SamlValidator) ValidateFromHeader(authHeader string) error {

	if authHeader == "" {
		return fmt.Errorf("missing Authorization header")
	}

	if !strings.HasPrefix(authHeader, "SAML ") {
		return fmt.Errorf("unsupported Authorization scheme (expected 'SAML <base64>')")
	}

	b64 := strings.TrimPrefix(authHeader, "SAML ")
	if b64 == "" {
		return fmt.Errorf("empty SAML assertion payload")
	}

	xmlBytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("invalid base64 in SAML assertion: %w", err)
	}

	return v.validateAssertion(xmlBytes)
}

// validateAssertion performs full SAML assertion validation:
// 1. Parse XML document
// 2. Find Assertion element
// 3. Verify XML-DSig signature
// 4. Check Issuer (if configured)
// 5. Check Conditions NotBefore/NotOnOrAfter with clock skew
func (v *SamlValidator) validateAssertion(xmlBytes []byte) error {

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(xmlBytes); err != nil {
		return fmt.Errorf("failed to parse SAML assertion XML: %w", err)
	}

	// Find the Assertion element — handles both "Assertion" and "saml:Assertion" (namespace-prefixed)
	assertion := findElementByLocalName(doc.Root(), "Assertion")
	if assertion == nil {
		return fmt.Errorf("no Assertion element found in SAML XML")
	}

	// Verify XML-DSig signature
	validationCtx := dsig.NewDefaultValidationContext(&v.certStore)
	validationCtx.Clock = dsig.NewFakeClockAt(time.Now())

	_, err := validationCtx.Validate(assertion)
	if err != nil {
		return fmt.Errorf("XML-DSig signature verification failed: %w", err)
	}

	// Check Issuer (if configured)
	if v.config.ExpectedIssuer != "" {
		issuerEl := findChildByLocalName(assertion, "Issuer")
		if issuerEl == nil {
			return fmt.Errorf("no Issuer element in SAML assertion")
		}

		issuer := strings.TrimSpace(issuerEl.Text())
		if issuer != v.config.ExpectedIssuer {
			return fmt.Errorf("SAML Issuer mismatch: got %q, expected %q", issuer, v.config.ExpectedIssuer)
		}
	}

	// Check temporal Conditions
	conditions := findChildByLocalName(assertion, "Conditions")
	if conditions != nil {
		now := time.Now()

		notBefore := conditions.SelectAttrValue("NotBefore", "")
		if notBefore != "" {
			nb, err := time.Parse(time.RFC3339, notBefore)
			if err != nil {
				return fmt.Errorf("failed to parse Conditions/@NotBefore: %w", err)
			}
			if now.Add(v.config.ClockSkew).Before(nb) {
				return fmt.Errorf("SAML assertion is not yet valid (NotBefore=%s)", notBefore)
			}
		}

		notOnOrAfter := conditions.SelectAttrValue("NotOnOrAfter", "")
		if notOnOrAfter != "" {
			noa, err := time.Parse(time.RFC3339, notOnOrAfter)
			if err != nil {
				return fmt.Errorf("failed to parse Conditions/@NotOnOrAfter: %w", err)
			}
			if now.Add(-v.config.ClockSkew).After(noa) {
				return fmt.Errorf("SAML assertion has expired (NotOnOrAfter=%s)", notOnOrAfter)
			}
		}
	}

	return nil
}

// SamlAuthMiddleware returns a Gin middleware that validates SAML assertions
// on incoming requests. Returns 401 with a FHIR OperationOutcome on failure.
func SamlAuthMiddleware(validator *SamlValidator) gin.HandlerFunc {

	return func(c *gin.Context) {

		if validator == nil || !validator.IsEnabled() {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if err := validator.ValidateFromHeader(authHeader); err != nil {
			log.Printf("[SAML] Validation failed: %v", err)
			abortWithFhirUnauthorized(c, err.Error())
			return
		}

		c.Next()
	}
}

// abortWithFhirUnauthorized sends a 401 response with a FHIR OperationOutcome body.
func abortWithFhirUnauthorized(c *gin.Context, reason string) {

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<OperationOutcome xmlns="http://hl7.org/fhir">
  <issue>
    <severity value="error"/>
    <code value="security"/>
    <diagnostics value="SAML validation failed: %s"/>
  </issue>
</OperationOutcome>`, escapeXml(reason))

	c.Data(http.StatusUnauthorized, "application/fhir+xml; charset=utf-8", []byte(body))
	c.Abort()
}

// escapeXml escapes special characters for safe XML attribute/text inclusion.
func escapeXml(s string) string {

	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// localName strips the namespace prefix from an element tag (e.g. "saml:Assertion" → "Assertion").
func localName(tag string) string {

	if idx := strings.LastIndex(tag, ":"); idx >= 0 {
		return tag[idx+1:]
	}
	return tag
}

// findElementByLocalName searches el and its descendants for the first element matching the local name.
func findElementByLocalName(el *etree.Element, name string) *etree.Element {

	if el == nil {
		return nil
	}

	if localName(el.Tag) == name {
		return el
	}

	for _, child := range el.ChildElements() {
		if found := findElementByLocalName(child, name); found != nil {
			return found
		}
	}

	return nil
}

// findChildByLocalName finds a direct child element matching the local name.
func findChildByLocalName(el *etree.Element, name string) *etree.Element {

	for _, child := range el.ChildElements() {
		if localName(child.Tag) == name {
			return child
		}
	}

	return nil
}
