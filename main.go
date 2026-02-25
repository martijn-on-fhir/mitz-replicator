package main

import (
	"crypto/tls"
	"crypto/x509"
	"embed"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

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

	// Load embedded templates
	initTemplates()

	// Configure Gin
	router := gin.Default()
	router.Use(requestLogger())

	router.HEAD("/xacml", handlers.HealthCheck)
	router.POST("/xacml", handlers.HandleXACML)
	router.POST("/xcpd", handlers.HandleXCPD)

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
	log.Printf("  HEAD /xacml  — health check")
	log.Printf("  POST /xacml  — gesloten autorisatievraag")
	log.Printf("  POST /xcpd   — open autorisatievraag")

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

