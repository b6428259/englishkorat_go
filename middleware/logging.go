package middleware

import (
	"englishkorat_go/database"
	"englishkorat_go/models"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
)

// LoggerMiddleware logs HTTP requests
func LoggerMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Process request
		err := c.Next()

		// Log request
		duration := time.Since(start)
		status := c.Response().StatusCode()

		logrus.WithFields(logrus.Fields{
			"method":    c.Method(),
			"path":      c.Path(),
			"status":    status,
			"duration":  duration.String(),
			"ip":        c.IP(),
			"user_agent": c.Get("User-Agent"),
		}).Info("HTTP Request")

		return err
	}
}

// ActivityLogger logs user activities to database
func LogActivity(c *fiber.Ctx, action, resource string, resourceID uint, details interface{}) {
	// Get current user
	user, err := GetCurrentUser(c)
	if err != nil {
		// If no authenticated user, log as system action
		user = &models.User{BaseModel: models.BaseModel{ID: 0}}
	}

	// Convert details to JSON
	var detailsJSON models.JSON
	if details != nil {
		if detailsBytes, err := json.Marshal(details); err == nil {
			detailsJSON = detailsBytes
		}
	}

	// Create activity log
	activityLog := models.ActivityLog{
		UserID:     user.ID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Details:    detailsJSON,
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	}

	// Save to database (in goroutine to not block request)
	go func() {
		if err := database.DB.Create(&activityLog).Error; err != nil {
			logrus.WithError(err).Error("Failed to save activity log")
		}
	}()
}

// LogActivityMiddleware automatically logs CRUD operations
func LogActivityMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip logging for GET requests and auth endpoints
		if c.Method() == "GET" || strings.Contains(c.Path(), "/auth/") {
			return c.Next()
		}

		// Process request
		err := c.Next()

		// Determine action based on method
		var action string
		switch c.Method() {
		case "POST":
			action = "CREATE"
		case "PUT", "PATCH":
			action = "UPDATE"
		case "DELETE":
			action = "DELETE"
		default:
			return err
		}

		// Extract resource from path
		pathParts := strings.Split(strings.Trim(c.Path(), "/"), "/")
		var resource string
		if len(pathParts) >= 2 {
			resource = pathParts[1] // assumes /api/resource format
		}

		// Extract resource ID from params if available
		var resourceID uint
		if id := c.Params("id"); id != "" {
			// Convert string ID to uint if needed
			// This is a simplified version - you might want more robust parsing
			if parsedID, parseErr := parseUint(id); parseErr == nil {
				resourceID = parsedID
			}
		}

		// Log only if request was successful
		if c.Response().StatusCode() < 400 {
			LogActivity(c, action, resource, resourceID, nil)
		}

		return err
	}
}

// parseUint converts string to uint
func parseUint(s string) (uint, error) {
	// Simple implementation - in real app you might want to use strconv.ParseUint
	var result uint
	for _, char := range s {
		if char < '0' || char > '9' {
			return 0, fiber.NewError(fiber.StatusBadRequest, "Invalid ID format")
		}
		result = result*10 + uint(char-'0')
	}
	return result, nil
}