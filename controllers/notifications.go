package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

type NotificationController struct{}

// GetNotifications returns notifications for the current user
func (nc *NotificationController) GetNotifications(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	var notifications []models.Notification
	var total int64

	query := database.DB.Where("user_id = ?", user.ID)

	// Filter by read status if specified
	if read := c.Query("read"); read == "true" {
		query = query.Where("read = ?", true)
	} else if read == "false" {
		query = query.Where("read = ?", false)
	}

	// Filter by type if specified
	if notificationType := c.Query("type"); notificationType != "" {
		query = query.Where("type = ?", notificationType)
	}

	// Get total count
	query.Count(&total)

	// Get notifications with pagination
	if err := query.Order("created_at DESC").
		Offset(offset).Limit(limit).Find(&notifications).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch notifications",
		})
	}

	return c.JSON(fiber.Map{
		"notifications": notifications,
		"pagination": fiber.Map{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetNotification returns a specific notification
func (nc *NotificationController) GetNotification(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid notification ID",
		})
	}

	var notification models.Notification
	if err := database.DB.Where("id = ? AND user_id = ?", uint(id), user.ID).
		First(&notification).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Notification not found",
		})
	}

	return c.JSON(fiber.Map{
		"notification": notification,
	})
}

// CreateNotification creates a new notification (admin only)
func (nc *NotificationController) CreateNotification(c *fiber.Ctx) error {
	var req struct {
		UserID    uint   `json:"user_id"`
		UserIDs   []uint `json:"user_ids"`    // For multiple users
		Role      string `json:"role"`        // For all users with specific role
		BranchID  uint   `json:"branch_id"`   // For all users in branch
		Title     string `json:"title" validate:"required"`
		TitleTh   string `json:"title_th"`
		Message   string `json:"message" validate:"required"`
		MessageTh string `json:"message_th"`
		Type      string `json:"type" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate type
	validTypes := []string{"info", "warning", "error", "success"}
	isValidType := false
	for _, validType := range validTypes {
		if req.Type == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid notification type. Must be: info, warning, error, or success",
		})
	}

	var userIDs []uint

	// Determine target users
	if req.UserID != 0 {
		// Single user
		userIDs = []uint{req.UserID}
	} else if len(req.UserIDs) > 0 {
		// Multiple specific users
		userIDs = req.UserIDs
	} else if req.Role != "" {
		// All users with specific role
		var users []models.User
		query := database.DB.Where("role = ? AND status = ?", req.Role, "active")
		if req.BranchID != 0 {
			query = query.Where("branch_id = ?", req.BranchID)
		}
		query.Find(&users)
		
		for _, user := range users {
			userIDs = append(userIDs, user.ID)
		}
	} else if req.BranchID != 0 {
		// All users in specific branch
		var users []models.User
		database.DB.Where("branch_id = ? AND status = ?", req.BranchID, "active").Find(&users)
		
		for _, user := range users {
			userIDs = append(userIDs, user.ID)
		}
	} else {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Must specify user_id, user_ids, role, or branch_id",
		})
	}

	if len(userIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No target users found",
		})
	}

	// Create notifications
	var notifications []models.Notification
	for _, userID := range userIDs {
		notification := models.Notification{
			UserID:    userID,
			Title:     req.Title,
			TitleTh:   req.TitleTh,
			Message:   req.Message,
			MessageTh: req.MessageTh,
			Type:      req.Type,
			Read:      false,
		}
		notifications = append(notifications, notification)
	}

	if err := database.DB.Create(&notifications).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create notifications",
		})
	}

	// Log activity
	middleware.LogActivity(c, "CREATE", "notifications", 0, fiber.Map{
		"target_users": len(userIDs),
		"type":         req.Type,
		"title":        req.Title,
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":       "Notifications created successfully",
		"notifications": len(notifications),
		"target_users":  len(userIDs),
	})
}

// MarkAsRead marks a notification as read
func (nc *NotificationController) MarkAsRead(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid notification ID",
		})
	}

	var notification models.Notification
	if err := database.DB.Where("id = ? AND user_id = ?", uint(id), user.ID).
		First(&notification).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Notification not found",
		})
	}

	now := time.Now()
	if err := database.DB.Model(&notification).Updates(map[string]interface{}{
		"read":    true,
		"read_at": &now,
	}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to mark notification as read",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Notification marked as read",
	})
}

// MarkAllAsRead marks all notifications as read for the current user
func (nc *NotificationController) MarkAllAsRead(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	now := time.Now()
	if err := database.DB.Model(&models.Notification{}).
		Where("user_id = ? AND read = ?", user.ID, false).
		Updates(map[string]interface{}{
			"read":    true,
			"read_at": &now,
		}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to mark notifications as read",
		})
	}

	return c.JSON(fiber.Map{
		"message": "All notifications marked as read",
	})
}

// DeleteNotification deletes a notification
func (nc *NotificationController) DeleteNotification(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid notification ID",
		})
	}

	var notification models.Notification
	if err := database.DB.Where("id = ? AND user_id = ?", uint(id), user.ID).
		First(&notification).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Notification not found",
		})
	}

	if err := database.DB.Delete(&notification).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete notification",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Notification deleted successfully",
	})
}

// GetUnreadCount returns the count of unread notifications for the current user
func (nc *NotificationController) GetUnreadCount(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var count int64
	database.DB.Model(&models.Notification{}).
		Where("user_id = ? AND read = ?", user.ID, false).
		Count(&count)

	return c.JSON(fiber.Map{
		"unread_count": count,
	})
}

// GetNotificationStats returns notification statistics (admin only)
func (nc *NotificationController) GetNotificationStats(c *fiber.Ctx) error {
	var stats struct {
		Total   int64 `json:"total"`
		Read    int64 `json:"read"`
		Unread  int64 `json:"unread"`
		ByType  map[string]int64 `json:"by_type"`
	}

	// Total notifications
	database.DB.Model(&models.Notification{}).Count(&stats.Total)

	// Read notifications
	database.DB.Model(&models.Notification{}).Where("read = ?", true).Count(&stats.Read)

	// Unread notifications
	database.DB.Model(&models.Notification{}).Where("read = ?", false).Count(&stats.Unread)

	// By type
	stats.ByType = make(map[string]int64)
	types := []string{"info", "warning", "error", "success"}
	for _, notType := range types {
		var count int64
		database.DB.Model(&models.Notification{}).Where("type = ?", notType).Count(&count)
		stats.ByType[notType] = count
	}

	return c.JSON(fiber.Map{
		"stats": stats,
	})
}