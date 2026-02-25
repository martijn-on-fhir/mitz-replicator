package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthCheck handles HEAD /xacml — mTLS connectivity probe.
func HealthCheck(c *gin.Context) {
	c.Status(http.StatusOK)
}
