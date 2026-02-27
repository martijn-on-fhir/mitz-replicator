package main

import (
	"crypto/tls"
	"crypto/x509"
	"embed"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"mitz-replicator/auth"
	"mitz-replicator/handlers"
)

//go:embed templates/*.xml
var templateFS embed.FS

func main() {
	port := getEnv("PORT", "8443")
	serverCert := getEnv("SERVER_CERT", "certs/server.crt")
	serverKey := getEnv("SERVER_KEY", "certs/server.key")
	caCert := getEnv("CA_CERT", "certs/ca.crt")
	mtlsEnabled := getEnv("MTLS_ENABLED", "false")

	// SAML validation config
	samlEnabled := getEnv("SAML_VALIDATION_ENABLED", "false") == "true"
	samlCertPath := getEnv("SAML_SIGNING_CERT", "certs/client.crt")
	samlExpectedIssuer := getEnv("SAML_EXPECTED_ISSUER", "")
	samlClockSkewSec, _ := strconv.Atoi(getEnv("SAML_CLOCK_SKEW_SECONDS", "5"))

	var samlValidator *auth.SamlValidator
	if samlEnabled {
		certPEM, err := os.ReadFile(samlCertPath)
		if err != nil {
			log.Fatalf("Failed to read SAML signing certificate %s: %v", samlCertPath, err)
		}

		samlValidator, err = auth.NewSamlValidator(auth.SamlValidatorConfig{
			Enabled:        true,
			SigningCert:    certPEM,
			ExpectedIssuer: samlExpectedIssuer,
			ClockSkew:      time.Duration(samlClockSkewSec) * time.Second,
		})
		if err != nil {
			log.Fatalf("Failed to create SAML validator: %v", err)
		}

		log.Printf("SAML validation enabled — cert=%s issuer=%q clockSkew=%ds",
			samlCertPath, samlExpectedIssuer, samlClockSkewSec)
	} else {
		samlValidator, _ = auth.NewSamlValidator(auth.SamlValidatorConfig{Enabled: false})
		log.Println("SAML validation disabled — FHIR endpoints accept any Authorization header")
	}

	handlers.InitSamlValidator(samlValidator)

	// Load embedded templates
	initTemplates()

	// Configure Gin
	router := gin.Default()
	router.Use(requestLogger())

	// SOAP endpoints
	router.HEAD("/xacml", handlers.HealthCheck)
	router.POST("/xacml", handlers.HandleXACML)
	router.POST("/xcpd", handlers.HandleXCPD)

	// FHIR endpoints (configure MITZ_FHIR_ENDPOINT=https://localhost:8443/fhir)
	fhir := router.Group("/fhir")
	{
		fhir.POST("/Subscription", auth.SamlAuthMiddleware(samlValidator), handlers.HandleFhirSubscriptionCreate)
		fhir.DELETE("/Subscription/:id", auth.SamlAuthMiddleware(samlValidator), handlers.HandleFhirSubscriptionDelete)
		fhir.GET("/Subscription/$processingStatus", handlers.HandleFhirProcessingStatus)
		fhir.GET("/Consent/$processingStatus", handlers.HandleFhirProcessingStatus)
		fhir.POST("/", handlers.HandleFhirBundle) // SAML checked inside handler (migration only)
	}

	// Configure TLS
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if mtlsEnabled == "true" {
		caCertPEM, err := os.ReadFile(caCert)
		if err != nil {
			log.Fatalf("Failed to read CA certificate: %v", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCertPEM) {
			log.Fatal("Failed to parse CA certificate")
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		log.Println("mTLS enabled — client certificates will be verified")
	} else {
		log.Println("mTLS disabled — any client can connect")
	}

	server := &http.Server{
		Addr:      ":" + port,
		Handler:   router,
		TLSConfig: tlsConfig,
	}

	log.Printf("Mitz Replicator starting on https://localhost:%s", port)
	log.Printf("  SOAP endpoints:")
	log.Printf("    HEAD /xacml  — health check")
	log.Printf("    POST /xacml  — gesloten autorisatievraag")
	log.Printf("    POST /xcpd   — open autorisatievraag")
	log.Printf("  FHIR endpoints:")
	log.Printf("    POST   /fhir/Subscription              — create subscription (OTV-TR-0120)")
	log.Printf("    DELETE /fhir/Subscription/:id           — cancel subscription (OTV-TR-0130)")
	log.Printf("    POST   /fhir/                           — Bundle transaction (OTV-TR-0150/0160)")
	log.Printf("    GET    /fhir/Subscription/$processingStatus — query processing status")
	log.Printf("    GET    /fhir/Consent/$processingStatus      — query processing status")

	if err := server.ListenAndServeTLS(serverCert, serverKey); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func initTemplates() {
	xacmlResponse := mustReadTemplate("templates/xacml_response.xml")
	xacmlFault := mustReadTemplate("templates/xacml_fault.xml")
	handlers.InitXACMLTemplates(xacmlResponse, xacmlFault)

	xcpdFound := mustReadTemplate("templates/xcpd_found.xml")
	xcpdEmpty := mustReadTemplate("templates/xcpd_empty.xml")
	xcpdFault := mustReadTemplate("templates/xcpd_fault.xml")
	handlers.InitXCPDTemplates(xcpdFound, xcpdEmpty, xcpdFault)

	fhirSubscription := mustReadTemplate("templates/fhir_subscription.xml")
	fhirBundleResponse := mustReadTemplate("templates/fhir_bundle_response.xml")
	fhirProcessingStatus := mustReadTemplate("templates/fhir_processing_status.xml")
	fhirOperationOutcome := mustReadTemplate("templates/fhir_operation_outcome.xml")
	handlers.InitFhirTemplates(fhirSubscription, fhirBundleResponse, fhirProcessingStatus, fhirOperationOutcome)
}

func mustReadTemplate(path string) string {
	data, err := templateFS.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read embedded template %s: %v", path, err)
	}
	return string(data)
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := c.GetHeader("X-Request-Id")

		c.Next()

		log.Printf("%s %s %d %s RequestId=%s",
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			time.Since(start),
			requestID,
		)
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

