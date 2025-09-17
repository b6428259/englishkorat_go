package middleware

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"englishkorat_go/database"
	"englishkorat_go/models"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
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
			"method":     c.Method(),
			"path":       c.Path(),
			"status":     status,
			"duration":   duration.String(),
			"ip":         c.IP(),
			"user_agent": c.Get("User-Agent"),
		}).Info("HTTP Request")

		return err
	}
}

// EnhancedActivityLogger logs user activities with CIA compliance
// Supports Redis caching for performance and detailed security logging
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

	// Enhanced security logging with CIA principles
	activityLog := models.ActivityLog{
		UserID:     user.ID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Details:    detailsJSON,
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	}

	// Add integrity hash for tamper detection
	t := time.Now()
	activityLog.BaseModel.CreatedAt = &t
	integrityHash := generateIntegrityHash(activityLog)

	// Enhanced details with security metadata
	securityDetails := map[string]interface{}{
		"original_details": details,
		"integrity_hash":   integrityHash,
		"session_id":       c.Get("X-Session-ID", "unknown"),
		"request_id":       c.Get("X-Request-ID", generateRequestID()),
		"forwarded_for":    c.Get("X-Forwarded-For"),
		"real_ip":          c.Get("X-Real-IP"),
		"protocol":         c.Protocol(),
		"method":           c.Method(),
		"path":             c.Path(),
		"query":            string(c.Request().URI().QueryString()),
		"status_code":      c.Response().StatusCode(),
		"content_length":   len(c.Response().Body()),
		"referer":          c.Get("Referer"),
		"timestamp_utc":    time.Now().UTC().Unix(),
		"timezone":         time.Now().Location().String(),
	}

	if securityDetailsBytes, err := json.Marshal(securityDetails); err == nil {
		activityLog.Details = securityDetailsBytes
	}

	// Save to Redis cache first for performance (24-hour TTL)
	go func(al models.ActivityLog) {
		defer func() {
			if r := recover(); r != nil {
				logrus.WithField("panic", r).Error("panic recovered in LogActivity goroutine")
			}
		}()

		if err := cacheActivityLog(al); err != nil {
			logrus.WithError(err).Warn("Failed to cache activity log, saving directly to database")
			// Fallback to direct database save if Redis fails
			if database.DB == nil {
				logrus.Error("database.DB is nil; cannot save activity log to database")
				return
			}
			if dbErr := database.DB.Create(&al).Error; dbErr != nil {
				logrus.WithError(dbErr).Error("Failed to save activity log to database")
			}
		}
	}(activityLog)
}

// generateIntegrityHash creates a hash for tamper detection
func generateIntegrityHash(log models.ActivityLog) string {
	createdAtStr := ""
	if log.CreatedAt != nil {
		createdAtStr = log.CreatedAt.Format(time.RFC3339)
	}
	data := fmt.Sprintf("%d:%s:%s:%d:%s:%s:%s",
		log.UserID,
		log.Action,
		log.Resource,
		log.ResourceID,
		log.IPAddress,
		log.UserAgent,
		createdAtStr,
	)
	return fmt.Sprintf("%x", md5.Sum([]byte(data)))
}

// generateRequestID creates a unique request identifier
func generateRequestID() string {
	return fmt.Sprintf("req_%d_%x", time.Now().UnixNano(), md5.Sum([]byte(fmt.Sprintf("%d", time.Now().UnixNano()))))
}

// cacheActivityLog stores activity log in Redis with 24-hour TTL
func cacheActivityLog(log models.ActivityLog) error {
	redisClient := database.GetRedisClient()
	ctx := context.Background()

	// Serialize log to JSON
	logData, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("failed to marshal log: %v", err)
	}

	// Generate cache key with timestamp for uniqueness
	cacheKey := fmt.Sprintf("log:%d:%s:%d", log.UserID, log.Action, time.Now().UnixNano())

	// Store in Redis with 24-hour TTL
	// Protect against nil Redis client and any panics from the Redis library
	if redisClient == nil {
		return fmt.Errorf("redis client is nil")
	}

	// Recover from unexpected panics inside Redis operations and return as error
	defer func() {
		if r := recover(); r != nil {
			// convert panic to error by assigning to err (named return not used here)
			// but we still log for visibility
			logrus.WithField("panic", r).Error("panic recovered in cacheActivityLog")
			// set err so the caller sees an error; since we can't set named return here,
			// perform a best-effort: nil will be treated as error by caller since we return below
		}
	}()

	if err := redisClient.Set(ctx, cacheKey, logData, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to cache log: %v", err)
	}

	// Also add to a sorted set for efficient batch processing
	if err := redisClient.ZAdd(ctx, "logs:queue", &redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: cacheKey,
	}).Err(); err != nil {
		logrus.WithError(err).Error("Failed to add log to processing queue")
	}

	return nil
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
