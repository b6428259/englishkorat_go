package controllers

import (
	"englishkorat_go/services"

	"github.com/gofiber/fiber/v2"
)

// HealthController exposes comprehensive health endpoints.
type HealthController struct {
	service *services.HealthService
}

// NewHealthController constructs a controller backed by the provided service.
func NewHealthController(service *services.HealthService) *HealthController {
	if service == nil {
		service = services.NewHealthService("", "")
	}
	return &HealthController{service: service}
}

// GetHealthStatus returns the aggregated health report.
func (hc *HealthController) GetHealthStatus(c *fiber.Ctx) error {
	report := hc.service.GetHealthReport()
	statusCode := hc.service.HTTPStatusForOverall(report.Status)
	return c.Status(statusCode).JSON(report)
}
