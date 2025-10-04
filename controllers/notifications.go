package controllers //nolint:goconst

import (
	"englishkorat_go/config"
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"englishkorat_go/services"
	notifsvc "englishkorat_go/services/notifications"
	"log"
	"strconv"
	"time"

	"englishkorat_go/utils"

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

	// Ensure GORM has the model/table set so Count/Find work (avoid "Table not set" errors)
	query := database.DB.Model(&models.Notification{}).Where("user_id = ?", user.ID)

	// Filter by read status if specified (quote reserved column `read`)
	if read := c.Query("read"); read == "true" {
		query = query.Where("`read` = ?", true)
	} else if read == "false" {
		query = query.Where("`read` = ?", false)
	}

	// Filter by type if specified
	if notificationType := c.Query("type"); notificationType != "" {
		// quote column name `type` to avoid conflicts with reserved words in some MySQL modes
		query = query.Where("`type` = ?", notificationType)
	}

	// Get total count (handle potential SQL errors upfront)
	if err := query.Count(&total).Error; err != nil {
		// Log the underlying DB error for debugging
		log.Printf("notifications: count error: %v", err)
		if config.AppConfig.AppEnv == "development" {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Failed to fetch notifications",
				"details": err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch notifications",
		})
	}

	// Preload related user/student/branch to build compact DTOs
	if err := query.Preload("User").Preload("User.Student").Preload("User.Teacher").Preload("User.Branch").
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&notifications).Error; err != nil {
		log.Printf("notifications: find error: %v", err)
		if config.AppConfig.AppEnv == "development" {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Failed to fetch notifications",
				"details": err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch notifications",
		})
	}

	// Map to DTOs
	var dtos []utils.NotificationDTO
	for _, n := range notifications {
		dtos = append(dtos, utils.ToNotificationDTO(n))
	}

	settingsService := services.NewSettingsService()
	settings, settingsErr := settingsService.GetOrCreate(user.ID)
	if settingsErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load user settings"})
	}
	settingsResponse := settingsService.BuildSettingsResponse(settings)

	return c.JSON(fiber.Map{
		"notifications": dtos,
		"pagination": fiber.Map{
			"page":  page,
			"limit": limit,
			"total": total,
		},
		"settings":          settingsResponse.Settings,
		"available_sounds":  settingsResponse.AvailableSounds,
		"settings_metadata": settingsResponse.Metadata,
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
		Preload("User").Preload("User.Student").Preload("User.Teacher").Preload("User.Branch").
		First(&notification).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Notification not found", //nolint:goconst
		})
	}

	dto := utils.ToNotificationDTO(notification)
	settingsService := services.NewSettingsService()
	settings, settingsErr := settingsService.GetOrCreate(user.ID)
	if settingsErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load user settings"})
	}
	settingsResponse := settingsService.BuildSettingsResponse(settings)

	return c.JSON(fiber.Map{
		"notification":      dto,
		"settings":          settingsResponse.Settings,
		"available_sounds":  settingsResponse.AvailableSounds,
		"settings_metadata": settingsResponse.Metadata,
	})
}

// CreateNotification creates a new notification (admin only)
func (nc *NotificationController) CreateNotification(c *fiber.Ctx) error { //nolint:gocognit,gocyclo
	var req struct {
		UserID    uint     `json:"user_id"`
		UserIDs   []uint   `json:"user_ids"`  // For multiple users
		Role      string   `json:"role"`      // For all users with specific role
		BranchID  uint     `json:"branch_id"` // For all users in branch
		Title     string   `json:"title" validate:"required"`
		TitleTh   string   `json:"title_th"`
		Message   string   `json:"message" validate:"required"`
		MessageTh string   `json:"message_th"`
		Type      string   `json:"type" validate:"required"`
		Channels  []string `json:"channels"` // e.g., ["normal","popup","line"]
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
	service := notifsvc.NewService()
	q := notifsvc.QueuedForController(req.Title, req.TitleTh, req.Message, req.MessageTh, req.Type, req.Channels...)
	if err := service.EnqueueOrCreate(userIDs, q); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create notifications"})
	}

	// Optional LINE delivery when requested
	if containsChannel(req.Channels, "line") {
		lineSvc := services.NewLineMessagingService()
		// fetch users to get LineID
		var users []models.User
		database.DB.Where("id IN ?", userIDs).Find(&users)
		for _, u := range users {
			if u.LineID != "" {
				if err := lineSvc.SendLineMessageToUser(u.LineID, buildLineMessage(req.Title, req.TitleTh, req.Message, req.MessageTh)); err != nil {
					log.Printf("LINE push failed for user %d: %v", u.ID, err)
				}
			}
		}
	}

	// Log activity
	middleware.LogActivity(c, "CREATE", "notifications", 0, fiber.Map{
		"target_users": len(userIDs),
		"type":         req.Type,
		"title":        req.Title,
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":      "Notifications accepted",
		"queued":       true,
		"target_users": len(userIDs),
	})
}

// containsChannel checks if list contains the given channel
func containsChannel(channels []string, target string) bool {
	for _, c := range channels {
		if c == target {
			return true
		}
	}
	return false
}

// buildLineMessage builds a simple message combining Thai/English if present
func buildLineMessage(titleEn, titleTh, msgEn, msgTh string) string {
	if titleTh != "" || msgTh != "" {
		if titleEn != "" || msgEn != "" {
			return titleTh + "\n" + msgTh + "\n\n" + titleEn + "\n" + msgEn
		}
		return titleTh + "\n" + msgTh
	}
	return titleEn + "\n" + msgEn
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
			"error": "Notification not found", //nolint:goconst
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
		Where("user_id = ? AND `read` = ?", user.ID, false).
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
			"error": "Notification not found", //nolint:goconst
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
		Where("user_id = ? AND `read` = ?", user.ID, false).
		Count(&count)

	return c.JSON(fiber.Map{
		"unread_count": count,
	})
}

// GetNotificationStats returns notification statistics (admin only)
func (nc *NotificationController) GetNotificationStats(c *fiber.Ctx) error {
	var stats struct {
		Total  int64            `json:"total"`
		Read   int64            `json:"read"`
		Unread int64            `json:"unread"`
		ByType map[string]int64 `json:"by_type"`
	}

	// Total notifications
	database.DB.Model(&models.Notification{}).Count(&stats.Total)

	// Read notifications
	database.DB.Model(&models.Notification{}).Where("`read` = ?", true).Count(&stats.Read)

	// Unread notifications
	database.DB.Model(&models.Notification{}).Where("`read` = ?", false).Count(&stats.Unread)

	// By type
	stats.ByType = make(map[string]int64)
	types := []string{"info", "warning", "error", "success"}
	for _, notType := range types {
		var count int64
		database.DB.Model(&models.Notification{}).Where("`type` = ?", notType).Count(&count)
		stats.ByType[notType] = count
	}

	return c.JSON(fiber.Map{
		"stats": stats,
	})
}

// TestWebSocketPopup sends test popup notifications with various scenarios
// GET /api/notifications/test/popup?user_id=X&case=scenario
func (nc *NotificationController) TestWebSocketPopup(c *fiber.Ctx) error {
	if config.AppConfig == nil || config.AppConfig.AppEnv == "production" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Test endpoints disabled in production"})
	}

	// Get target user
	userIDParam := c.Query("user_id")
	var targetUserID uint
	if userIDParam != "" {
		id, err := strconv.ParseUint(userIDParam, 10, 32)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user_id"})
		}
		targetUserID = uint(id)
	} else {
		// Default to current user
		user, err := middleware.GetCurrentUser(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not found or specify user_id"})
		}
		targetUserID = user.ID
	}

	// Verify target user exists
	var targetUser models.User
	if err := database.DB.First(&targetUser, targetUserID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Target user not found"})
	}

	testCase := c.Query("case", "basic")
	service := notifsvc.NewService()

	// Map numeric shortcuts
	shortcuts := map[string]string{
		"1": "basic", "2": "schedule", "3": "warning", "4": "success",
		"5": "error", "6": "normal_only", "7": "daily_reminder", "8": "payment_due",
		"9": "makeup_session", "10": "absence_approved", "11": "custom_sound", "12": "long_message",
		"13": "invitation",
	}
	if mapped, ok := shortcuts[testCase]; ok {
		testCase = mapped
	}

	// Send notification directly in each case (cannot store queuedNotification type)
	var desc string

	switch testCase {
	case "basic":
		desc = "Basic info popup"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Test Notification", "ทดสอบการแจ้งเตือน",
			"This is a basic test popup notification", "นี่คือการแจ้งเตือนแบบป๊อปอัพทดสอบพื้นฐาน",
			"info", fiber.Map{"test_case": "basic", "timestamp": time.Now().Unix()}, "popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "schedule":
		desc = "Schedule reminder (15 min)"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Upcoming Class", "คาบเรียนใกล้ถึงแล้ว",
			"Your class starts in 15 minutes", "คาบเรียนของคุณจะเริ่มใน 15 นาที", "info",
			fiber.Map{"action": "open_schedule", "schedule_id": 999, "session_id": 8888,
				"starts_at": time.Now().Add(15 * time.Minute).Format(time.RFC3339), "action_label": "View Schedule"},
			"popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "warning":
		desc = "Schedule conflict warning"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Schedule Conflict", "ตารางซ้อนกัน",
			"You have two sessions overlapping. Please resolve.", "คุณมี 2 คาบเรียนเวลาทับกัน กรุณาแก้ไข", "warning",
			fiber.Map{"action": "resolve_conflict", "conflicts": []fiber.Map{
				{"session_id": 1001, "starts_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339)},
				{"session_id": 1005, "starts_at": time.Now().Add(24*time.Hour + 30*time.Minute).Format(time.RFC3339)}}},
			"popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "success":
		desc = "Success message"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Schedule Approved", "อนุมัติตารางแล้ว",
			"Your schedule #A-1044 has been approved successfully", "ตาราง #A-1044 ของคุณได้รับการอนุมัติแล้ว",
			"success", fiber.Map{"schedule_code": "A-1044", "approved_by": "Admin"}, "popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "error":
		desc = "Error notification"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Upload Failed", "อัปโหลดไฟล์ล้มเหลว",
			"Failed to upload file. Please try again.", "ไม่สามารถอัปโหลดไฟล์ได้ กรุณาลองอีกครั้ง",
			"error", fiber.Map{"retry": true, "error_code": "FILE_TOO_LARGE"}, "popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "normal_only":
		desc = "Normal channel only (no popup)"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Background Sync Complete", "ซิงค์ข้อมูลเสร็จแล้ว",
			"Data has been synchronized successfully.", "ข้อมูลถูกซิงค์เรียบร้อยแล้ว", "success", nil, "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "daily_reminder":
		desc = "Daily schedule reminder"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Daily Schedule Reminder", "เตือนตารางเรียนประจำวัน",
			"You have 3 classes scheduled for today. First class starts at 09:00 AM.",
			"คุณมีคาบเรียน 3 คาบวันนี้ คาบแรกเริ่มเวลา 09:00 น.", "info",
			fiber.Map{"action": "view_daily_schedule", "date": time.Now().Format("2006-01-02"), "session_count": 3, "first_session": "09:00"},
			"popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "payment_due":
		desc = "Payment due warning"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Payment Due Soon", "ใกล้ถึงกำหนดชำระเงิน",
			"Your payment is due in 3 days. Amount: ฿5,000", "กำหนดชำระเงินของคุณใกล้ถึงแล้ว (อีก 3 วัน) จำนวน ฿5,000",
			"warning", fiber.Map{"action": "view_invoice", "invoice_id": "INV-2025-001", "amount": 5000,
				"due_date": time.Now().Add(3 * 24 * time.Hour).Format("2006-01-02")},
			"popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "makeup_session":
		desc = "Makeup session created"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Makeup Session Scheduled", "นัดชดเชยคาบเรียน",
			"A makeup session has been scheduled for 2025-10-05 at 14:00",
			"มีการนัดชดเชยคาบเรียนในวันที่ 5 ต.ค. 2025 เวลา 14:00 น.", "info",
			fiber.Map{"action": "open_schedule", "schedule_id": 888, "session_id": 7777,
				"session_type": "makeup", "scheduled_at": "2025-10-05T14:00:00Z"},
			"popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "absence_approved":
		desc = "Absence request approved"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Absence Approved", "อนุมัติการลา",
			"Your absence request for 2025-10-10 has been approved.",
			"คำขอลาของคุณสำหรับวันที่ 10 ต.ค. 2025 ได้รับการอนุมัติแล้ว",
			"success", fiber.Map{"absence_id": 555, "absence_date": "2025-10-10"}, "popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "custom_sound":
		desc = "Test custom sound (ensure user uploaded custom sound first)"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Custom Sound Test", "ทดสอบเสียงแจ้งเตือนแบบกำหนดเอง",
			"This notification should play your custom sound if enabled.",
			"การแจ้งเตือนนี้จะเล่นเสียงที่คุณกำหนดเอง (ถ้าเปิดใช้งาน)",
			"info", fiber.Map{"test": "custom_sound"}, "popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "long_message":
		desc = "Long message test"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Important Announcement", "ประกาศสำคัญ",
			"This is a long message to test how the popup handles extensive content. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.",
			"นี่คือข้อความยาวเพื่อทดสอบว่าป๊อปอัพจัดการกับเนื้อหาจำนวนมากได้อย่างไร ข้อความนี้มีหลายบรรทัดและมีรายละเอียดมากเพื่อทดสอบการแสดงผล และตรวจสอบว่าอินเทอร์เฟซรองรับได้ดีหรือไม่",
			"info", fiber.Map{"message_length": "long"}, "popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	case "invitation":
		desc = "Schedule invitation (requires response)"
		if err := service.EnqueueOrCreate([]uint{targetUserID}, notifsvc.QueuedWithData(
			"Schedule Invitation", "คำเชิญเข้าร่วมตาราง",
			"You have been invited to join a schedule. Please confirm your participation.",
			"คุณได้รับคำเชิญให้เข้าร่วมตาราง กรุณายืนยันการเข้าร่วม",
			"info", fiber.Map{
				"action": "respond_invitation", "schedule_id": 777, "invited_by": "Admin",
				"schedule_date":     time.Now().Add(48 * time.Hour).Format("2006-01-02"),
				"requires_response": true, "action_label": "Respond to Invitation"},
			"popup", "normal")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send test notification"})
		}
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Unknown test case", "available_cases": []string{
				"basic (1)", "schedule (2)", "warning (3)", "success (4)", "error (5)", "normal_only (6)",
				"daily_reminder (7)", "payment_due (8)", "makeup_session (9)", "absence_approved (10)",
				"custom_sound (11)", "long_message (12)", "invitation (13)"}})
	}

	return c.JSON(fiber.Map{
		"message": "Test notification sent via WebSocket", "target_user": targetUserID,
		"username": targetUser.Username, "test_case": testCase, "description": desc,
		"timestamp": time.Now().Format(time.RFC3339)})
}
