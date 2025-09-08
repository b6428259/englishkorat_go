package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"englishkorat_go/database"
	"englishkorat_go/models"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type LogController struct{}

// LogResponse represents a log entry response
type LogResponse struct {
	ID         uint                   `json:"id"`
	UserID     uint                   `json:"user_id"`
	Action     string                 `json:"action"`
	Resource   string                 `json:"resource"`
	ResourceID uint                   `json:"resource_id"`
	Details    map[string]interface{} `json:"details"`
	IPAddress  string                 `json:"ip_address"`
	UserAgent  string                 `json:"user_agent"`
	CreatedAt  time.Time              `json:"created_at"`
	User       *UserBasicInfo         `json:"user,omitempty"`
}

type UserBasicInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type LogsStatsResponse struct {
	Total             int64                 `json:"total"`
	TotalToday        int64                 `json:"total_today"`
	TotalThisWeek     int64                 `json:"total_this_week"`
	TotalThisMonth    int64                 `json:"total_this_month"`
	ActionBreakdown   map[string]int64      `json:"action_breakdown"`
	ResourceBreakdown map[string]int64      `json:"resource_breakdown"`
	HourlyActivity    map[string]int64      `json:"hourly_activity"`
	TopUsers          []UserActivitySummary `json:"top_users"`
	RecentActivity    []LogResponse         `json:"recent_activity"`
}

type UserActivitySummary struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Count    int64  `json:"count"`
}

// GetLogs retrieves paginated activity logs with filters
func (lc *LogController) GetLogs(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	// Validation
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}

	offset := (page - 1) * limit

	// Build query with filters
	query := database.DB.Model(&models.ActivityLog{}).Preload("User")

	// Filters
	if userID := c.Query("user_id"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	if action := c.Query("action"); action != "" {
		query = query.Where("action = ?", action)
	}

	if resource := c.Query("resource"); resource != "" {
		query = query.Where("resource = ?", resource)
	}

	if ipAddress := c.Query("ip_address"); ipAddress != "" {
		query = query.Where("ip_address = ?", ipAddress)
	}

	// Date range filters
	if startDate := c.Query("start_date"); startDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", startDate); err == nil {
			query = query.Where("created_at >= ?", parsedDate)
		}
	}

	if endDate := c.Query("end_date"); endDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", endDate); err == nil {
			query = query.Where("created_at <= ?", parsedDate.Add(24*time.Hour))
		}
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		logrus.WithError(err).Error("Failed to count logs")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve logs count",
		})
	}

	// Get logs with pagination
	var activityLogs []models.ActivityLog
	if err := query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&activityLogs).Error; err != nil {
		logrus.WithError(err).Error("Failed to retrieve logs")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve logs",
		})
	}

	// Convert to response format
	logs := make([]LogResponse, len(activityLogs))
	for i, log := range activityLogs {
		logs[i] = LogResponse{
			ID:         log.ID,
			UserID:     log.UserID,
			Action:     log.Action,
			Resource:   log.Resource,
			ResourceID: log.ResourceID,
			IPAddress:  log.IPAddress,
			UserAgent:  log.UserAgent,
			CreatedAt:  log.CreatedAt,
		}

		// Parse details if available
		if log.Details != nil && len(log.Details) > 0 {
			var details map[string]interface{}
			if err := json.Unmarshal(log.Details, &details); err == nil {
				logs[i].Details = details
			}
		}

		// Add user info if available
		if log.User.ID > 0 {
			logs[i].User = &UserBasicInfo{
				ID:       log.User.ID,
				Username: log.User.Username,
				Role:     log.User.Role,
			}
		}
	}

	response := fiber.Map{
		"logs":        logs,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": (total + int64(limit) - 1) / int64(limit),
	}

	return c.JSON(response)
}

// GetLogStats provides comprehensive logging statistics
func (lc *LogController) GetLogStats(c *fiber.Ctx) error {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	thisWeek := today.AddDate(0, 0, -int(today.Weekday()))
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	stats := LogsStatsResponse{
		ActionBreakdown:   make(map[string]int64),
		ResourceBreakdown: make(map[string]int64),
		HourlyActivity:    make(map[string]int64),
	}

	// Total logs
	database.DB.Model(&models.ActivityLog{}).Count(&stats.Total)

	// Today's logs
	database.DB.Model(&models.ActivityLog{}).
		Where("created_at >= ?", today).
		Count(&stats.TotalToday)

	// This week's logs
	database.DB.Model(&models.ActivityLog{}).
		Where("created_at >= ?", thisWeek).
		Count(&stats.TotalThisWeek)

	// This month's logs
	database.DB.Model(&models.ActivityLog{}).
		Where("created_at >= ?", thisMonth).
		Count(&stats.TotalThisMonth)

	// Action breakdown
	var actionStats []struct {
		Action string `json:"action"`
		Count  int64  `json:"count"`
	}
	database.DB.Model(&models.ActivityLog{}).
		Select("action, COUNT(*) as count").
		Group("action").
		Find(&actionStats)

	for _, stat := range actionStats {
		stats.ActionBreakdown[stat.Action] = stat.Count
	}

	// Resource breakdown
	var resourceStats []struct {
		Resource string `json:"resource"`
		Count    int64  `json:"count"`
	}
	database.DB.Model(&models.ActivityLog{}).
		Select("resource, COUNT(*) as count").
		Group("resource").
		Find(&resourceStats)

	for _, stat := range resourceStats {
		stats.ResourceBreakdown[stat.Resource] = stat.Count
	}

	// Hourly activity for today
	for i := 0; i < 24; i++ {
		hour := fmt.Sprintf("%02d:00", i)
		stats.HourlyActivity[hour] = 0
	}

	var hourlyStats []struct {
		Hour  int   `json:"hour"`
		Count int64 `json:"count"`
	}
	database.DB.Model(&models.ActivityLog{}).
		Select("EXTRACT(hour FROM created_at) as hour, COUNT(*) as count").
		Where("created_at >= ?", today).
		Group("hour").
		Find(&hourlyStats)

	for _, stat := range hourlyStats {
		hour := fmt.Sprintf("%02d:00", stat.Hour)
		stats.HourlyActivity[hour] = stat.Count
	}

	// Top users by activity
	var topUserStats []struct {
		UserID   uint   `json:"user_id"`
		Username string `json:"username"`
		Role     string `json:"role"`
		Count    int64  `json:"count"`
	}
	database.DB.Model(&models.ActivityLog{}).
		Select("activity_logs.user_id, users.username, users.role, COUNT(*) as count").
		Joins("LEFT JOIN users ON activity_logs.user_id = users.id").
		Where("activity_logs.created_at >= ?", thisWeek).
		Group("activity_logs.user_id, users.username, users.role").
		Order("count DESC").
		Limit(10).
		Find(&topUserStats)

	for _, stat := range topUserStats {
		stats.TopUsers = append(stats.TopUsers, UserActivitySummary{
			UserID:   stat.UserID,
			Username: stat.Username,
			Role:     stat.Role,
			Count:    stat.Count,
		})
	}

	// Recent activity (last 10)
	var recentLogs []models.ActivityLog
	database.DB.Preload("User").
		Order("created_at DESC").
		Limit(10).
		Find(&recentLogs)

	for _, log := range recentLogs {
		logResponse := LogResponse{
			ID:         log.ID,
			UserID:     log.UserID,
			Action:     log.Action,
			Resource:   log.Resource,
			ResourceID: log.ResourceID,
			IPAddress:  log.IPAddress,
			UserAgent:  log.UserAgent,
			CreatedAt:  log.CreatedAt,
		}

		if log.Details != nil && len(log.Details) > 0 {
			var details map[string]interface{}
			if err := json.Unmarshal(log.Details, &details); err == nil {
				logResponse.Details = details
			}
		}

		if log.User.ID > 0 {
			logResponse.User = &UserBasicInfo{
				ID:       log.User.ID,
				Username: log.User.Username,
				Role:     log.User.Role,
			}
		}

		stats.RecentActivity = append(stats.RecentActivity, logResponse)
	}

	return c.JSON(stats)
}

// GetLog retrieves a single log entry by ID
func (lc *LogController) GetLog(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid log ID",
		})
	}

	var activityLog models.ActivityLog
	if err := database.DB.Preload("User").First(&activityLog, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Log not found",
			})
		}
		logrus.WithError(err).Error("Failed to retrieve log")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve log",
		})
	}

	response := LogResponse{
		ID:         activityLog.ID,
		UserID:     activityLog.UserID,
		Action:     activityLog.Action,
		Resource:   activityLog.Resource,
		ResourceID: activityLog.ResourceID,
		IPAddress:  activityLog.IPAddress,
		UserAgent:  activityLog.UserAgent,
		CreatedAt:  activityLog.CreatedAt,
	}

	// Parse details if available
	if activityLog.Details != nil && len(activityLog.Details) > 0 {
		var details map[string]interface{}
		if err := json.Unmarshal(activityLog.Details, &details); err == nil {
			response.Details = details
		}
	}

	// Add user info if available
	if activityLog.User.ID > 0 {
		response.User = &UserBasicInfo{
			ID:       activityLog.User.ID,
			Username: activityLog.User.Username,
			Role:     activityLog.User.Role,
		}
	}

	return c.JSON(response)
}

// DeleteOldLogs removes logs older than specified days (Admin only)
func (lc *LogController) DeleteOldLogs(c *fiber.Ctx) error {
	days, err := strconv.Atoi(c.Query("days", "30"))
	if err != nil || days < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid days parameter",
		})
	}

	cutoffDate := time.Now().AddDate(0, 0, -days)

	result := database.DB.Where("created_at < ?", cutoffDate).Delete(&models.ActivityLog{})
	if result.Error != nil {
		logrus.WithError(result.Error).Error("Failed to delete old logs")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete old logs",
		})
	}

	return c.JSON(fiber.Map{
		"message":       "Old logs deleted successfully",
		"deleted_count": result.RowsAffected,
		"cutoff_date":   cutoffDate,
	})
}

// ExportLogs exports logs to CSV format (Admin only)
func (lc *LogController) ExportLogs(c *fiber.Ctx) error {
	// Set response headers for CSV download
	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", "attachment; filename=activity_logs.csv")

	// Get date range from query params
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := database.DB.Model(&models.ActivityLog{}).Preload("User")

	if startDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", startDate); err == nil {
			query = query.Where("created_at >= ?", parsedDate)
		}
	}

	if endDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", endDate); err == nil {
			query = query.Where("created_at <= ?", parsedDate.Add(24*time.Hour))
		}
	}

	var logs []models.ActivityLog
	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		logrus.WithError(err).Error("Failed to retrieve logs for export")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve logs for export",
		})
	}

	// Build CSV content
	csvContent := "ID,User ID,Username,Role,Action,Resource,Resource ID,IP Address,User Agent,Created At,Details\n"

	for _, log := range logs {
		username := ""
		role := ""
		if log.User.ID > 0 {
			username = log.User.Username
			role = log.User.Role
		}

		details := ""
		if log.Details != nil && len(log.Details) > 0 {
			details = string(log.Details)
		}

		csvContent += fmt.Sprintf("%d,%d,%s,%s,%s,%s,%d,%s,%s,%s,\"%s\"\n",
			log.ID,
			log.UserID,
			username,
			role,
			log.Action,
			log.Resource,
			log.ResourceID,
			log.IPAddress,
			log.UserAgent,
			log.CreatedAt.Format("2006-01-02 15:04:05"),
			details,
		)
	}

	return c.SendString(csvContent)
}

// Redis operations for caching logs
func (lc *LogController) GetRedisClient() *redis.Client {
	return database.GetRedisClient()
}

// FlushCachedLogs manually flushes cached logs to database (Admin only)
func (lc *LogController) FlushCachedLogs(c *fiber.Ctx) error {
	ctx := context.Background()
	redisClient := lc.GetRedisClient()

	// Get all cached log keys
	keys, err := redisClient.Keys(ctx, "log:*").Result()
	if err != nil {
		logrus.WithError(err).Error("Failed to get cached log keys")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to retrieve cached logs",
		})
	}

	var processedCount int
	var errorCount int

	// Process each cached log
	for _, key := range keys {
		logData, err := redisClient.Get(ctx, key).Result()
		if err != nil {
			errorCount++
			continue
		}

		var activityLog models.ActivityLog
		if err := json.Unmarshal([]byte(logData), &activityLog); err != nil {
			errorCount++
			continue
		}

		// Save to database
		if err := database.DB.Create(&activityLog).Error; err != nil {
			logrus.WithError(err).Error("Failed to save cached log to database")
			errorCount++
			continue
		}

		// Remove from cache
		if err := redisClient.Del(ctx, key).Err(); err != nil {
			logrus.WithError(err).Error("Failed to remove cached log")
		}

		processedCount++
	}

	return c.JSON(fiber.Map{
		"message":         "Cached logs flushing completed",
		"processed_count": processedCount,
		"error_count":     errorCount,
		"total_keys":      len(keys),
	})
}
