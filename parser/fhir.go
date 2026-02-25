package parser

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// FhirSubscriptionRequest holds extracted fields from a FHIR Subscription creation request.
type FhirSubscriptionRequest struct {
	BSN         string
	ProviderID  string
	Criteria    string
	Endpoint    string
	PayloadType string
}

// FhirBundleRequest holds extracted fields from a FHIR Bundle transaction request.
type FhirBundleRequest struct {
	BSN             string
	BundleType      string
	HasConsent      bool
	HasProvenance   bool
	HasOrganization bool
	EntryCount      int
}

// --- FHIR XML structs (namespace-stripped) ---

var fhirNsRe = regexp.MustCompile(`\s+xmlns="[^"]*"`)

func stripFhirNamespace(body []byte) []byte {
	return fhirNsRe.ReplaceAll(body, nil)
}

type fhirValueAttr struct {
	Value string `xml:"value,attr"`
}

type fhirSubscriptionXML struct {
	XMLName  xml.Name       `xml:"Subscription"`
	Criteria fhirValueAttr  `xml:"criteria"`
	Channel  fhirChannelXML `xml:"channel"`
}

type fhirChannelXML struct {
	Type     fhirValueAttr `xml:"type"`
	Endpoint fhirValueAttr `xml:"endpoint"`
	Payload  fhirValueAttr `xml:"payload"`
}

// ParseFhirSubscription extracts BSN, provider ID, and channel info from a FHIR Subscription request.
func ParseFhirSubscription(body []byte) (*FhirSubscriptionRequest, error) {
	cleaned := stripFhirNamespace(body)

	var sub fhirSubscriptionXML
	if err := xml.Unmarshal(cleaned, &sub); err != nil {
		return nil, fmt.Errorf("failed to parse FHIR Subscription: %w", err)
	}

	req := &FhirSubscriptionRequest{
		Criteria:    sub.Criteria.Value,
		Endpoint:    sub.Channel.Endpoint.Value,
		PayloadType: sub.Channel.Payload.Value,
	}

	// Parse BSN and provider ID from criteria query string
	// Format: Consent?_query=otv&patientid={bsn}&providerid={ura}&providertype={type}
	if idx := strings.Index(sub.Criteria.Value, "?"); idx >= 0 {
		params, _ := url.ParseQuery(sub.Criteria.Value[idx+1:])
		req.BSN = params.Get("patientid")
		req.ProviderID = params.Get("providerid")
	}

	return req, nil
}

// --- FHIR Bundle parsing ---

type fhirBundleXML struct {
	XMLName xml.Name       `xml:"Bundle"`
	Type    fhirValueAttr  `xml:"type"`
	Entry   []fhirEntryXML `xml:"entry"`
}

type fhirEntryXML struct {
	Resource fhirResourceXML `xml:"resource"`
}

type fhirResourceXML struct {
	Patient      *fhirPatientXML `xml:"Patient"`
	Consent      *fhirAnyXML     `xml:"Consent"`
	Provenance   *fhirAnyXML     `xml:"Provenance"`
	Organization *fhirAnyXML     `xml:"Organization"`
}

// fhirAnyXML is a placeholder for any FHIR resource we only need to detect.
type fhirAnyXML struct {
	XMLName xml.Name `xml:",any"`
}

type fhirPatientXML struct {
	Identifier fhirIdentifierXML `xml:"identifier"`
}

type fhirIdentifierXML struct {
	System fhirValueAttr `xml:"system"`
	Value  fhirValueAttr `xml:"value"`
}

// ParseFhirBundle extracts BSN and bundle metadata from a FHIR Bundle transaction request.
func ParseFhirBundle(body []byte) (*FhirBundleRequest, error) {
	cleaned := stripFhirNamespace(body)

	var bundle fhirBundleXML
	if err := xml.Unmarshal(cleaned, &bundle); err != nil {
		return nil, fmt.Errorf("failed to parse FHIR Bundle: %w", err)
	}

	req := &FhirBundleRequest{
		BundleType: bundle.Type.Value,
		EntryCount: len(bundle.Entry),
	}

	for _, entry := range bundle.Entry {
		if entry.Resource.Patient != nil {
			req.BSN = entry.Resource.Patient.Identifier.Value.Value
		}
		if entry.Resource.Consent != nil {
			req.HasConsent = true
		}
		if entry.Resource.Provenance != nil {
			req.HasProvenance = true
		}
		if entry.Resource.Organization != nil {
			req.HasOrganization = true
		}
	}

	return req, nil
}