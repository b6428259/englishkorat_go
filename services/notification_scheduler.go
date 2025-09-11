package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"englishkorat_go/database"
	"englishkorat_go/models"
)

// NotificationScheduler จัดการการส่ง notification อัตโนมัติ
type NotificationScheduler struct {
	db *gorm.DB
}

// NewNotificationScheduler สร้าง NotificationScheduler ใหม่
func NewNotificationScheduler() *NotificationScheduler {
	return &NotificationScheduler{
		db: database.DB,
	}
}

// StartScheduler เริ่มต้น scheduler สำหรับ notification
func (ns *NotificationScheduler) StartScheduler() {
	// ตั้งค่าให้ทำงานทุก 15 นาที
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	fmt.Println("Notification scheduler started...")

	for {
		select {
		case <-ticker.C:
			ns.CheckUpcomingSessions()
		}
	}
}

// CheckUpcomingSessions ตรวจสอบ sessions ที่จะเกิดขึ้นเร็วๆ นี้
func (ns *NotificationScheduler) CheckUpcomingSessions() {
	now := time.Now()

	// กำหนดช่วงเวลาที่จะแจ้งเตือน (30 นาที และ 60 นาที)
	notificationTimes := []struct {
		minutes int
		label   string
	}{
		{30, "30 minutes"},
		{60, "1 hour"},
	}

	for _, notifTime := range notificationTimes {
		targetTime := now.Add(time.Duration(notifTime.minutes) * time.Minute)

		// หา sessions ที่จะเริ่มในช่วงเวลาที่กำหนด (±5 นาที)
		startRange := targetTime.Add(-5 * time.Minute)
		endRange := targetTime.Add(5 * time.Minute)

		var sessions []models.Schedule_Sessions
		err := ns.db.Where("start_time BETWEEN ? AND ? AND status = ?",
			startRange, endRange, "confirmed").
			Find(&sessions).Error

		if err != nil {
			fmt.Printf("Error checking upcoming sessions: %v\n", err)
			continue
		}

		for _, session := range sessions {
			// ตรวจสอบว่าได้ส่ง notification แล้วหรือยัง
			if !ns.hasNotificationBeenSent(session.ID, notifTime.minutes) {
				ns.sendUpcomingClassNotification(session, notifTime.minutes, notifTime.label)
			}
		}
	}
}

// hasNotificationBeenSent ตรวจสอบว่าได้ส่ง notification แล้วหรือยัง
func (ns *NotificationScheduler) hasNotificationBeenSent(sessionID uint, minutes int) bool {
	var count int64
	err := ns.db.Model(&models.Notification{}).
		Where("message LIKE ? AND created_at > ?",
			fmt.Sprintf("%%will start in %d minutes%%", minutes),
			time.Now().Add(-2*time.Hour)).
		Count(&count).Error

	if err != nil {
		return false
	}

	return count > 0
}

// sendUpcomingClassNotification ส่ง notification สำหรับ class ที่จะเริ่มเร็วๆ นี้
func (ns *NotificationScheduler) sendUpcomingClassNotification(session models.Schedule_Sessions, minutes int, timeLabel string) {
	// ดึงข้อมูล schedule
	var schedule models.Schedules
	if err := ns.db.Preload("Course").First(&schedule, session.ScheduleID).Error; err != nil {
		fmt.Printf("Error fetching schedule for session %d: %v\n", session.ID, err)
		return
	}

	// ดึงรายชื่อผู้เข้าร่วม
	var users []models.User
	err := ns.db.Table("users").
		Joins("JOIN user_in_courses ON user_in_courses.user_id = users.id").
		Where("user_in_courses.course_id = ?", schedule.CourseID).
		Find(&users).Error
	if err != nil {
		fmt.Printf("Error fetching users for course %d: %v\n", schedule.CourseID, err)
		return
	}

	// เพิ่มครูที่ถูก assign
	var assignedTeacher models.User
	if err := ns.db.First(&assignedTeacher, schedule.AssignedToUserID).Error; err == nil {
		users = append(users, assignedTeacher)
	}

	// สร้าง notification สำหรับแต่ละคน
	for _, user := range users {
		notification := models.Notification{
			UserID:    user.ID,
			Title:     "Upcoming Class",
			TitleTh:   "เรียนจะเริ่มเร็วๆ นี้",
			Message:   fmt.Sprintf("Your class '%s' will start in %s at %s", schedule.ScheduleName, timeLabel, session.Start_time.Format("15:04")),
			MessageTh: fmt.Sprintf("คลาส '%s' ของคุณจะเริ่มในอีก %s เวลา %s", schedule.ScheduleName, ns.translateTimeLabel(timeLabel), session.Start_time.Format("15:04")),
			Type:      "info",
		}

		if err := ns.db.Create(&notification).Error; err != nil {
			fmt.Printf("Error creating notification for user %d: %v\n", user.ID, err)
		}
	}

	fmt.Printf("Sent upcoming class notifications for session %d (%s before)\n", session.ID, timeLabel)
}

// translateTimeLabel แปลงเวลาเป็นภาษาไทย
func (ns *NotificationScheduler) translateTimeLabel(timeLabel string) string {
	switch timeLabel {
	case "30 minutes":
		return "30 นาที"
	case "1 hour":
		return "1 ชั่วโมง"
	default:
		return timeLabel
	}
}

// SendDailyScheduleReminder ส่งเตือนตารางเรียนประจำวัน (เรียกจาก cron job ตอนเช้า)
func (ns *NotificationScheduler) SendDailyScheduleReminder() {
	today := time.Now()
	tomorrow := today.AddDate(0, 0, 1)

	// หา sessions ที่จะเกิดขึ้นวันนี้และพรุ่งนี้
	var sessions []models.Schedule_Sessions
	err := ns.db.Where("session_date BETWEEN ? AND ? AND status = ?",
		today.Format("2006-01-02"), tomorrow.Format("2006-01-02"), "confirmed").
		Preload("Schedule").
		Preload("Schedule.Course").
		Find(&sessions).Error

	if err != nil {
		fmt.Printf("Error fetching daily sessions: %v\n", err)
		return
	}

	// จัดกลุ่ม sessions ตาม user
	userSessions := make(map[uint][]models.Schedule_Sessions)

	for _, session := range sessions {
		// ดึงรายชื่อผู้เข้าร่วม
		var users []models.User
		err := ns.db.Table("users").
			Joins("JOIN user_in_courses ON user_in_courses.user_id = users.id").
			Where("user_in_courses.course_id = ?", session.Schedule.CourseID).
			Find(&users).Error

		if err != nil {
			continue
		}

		// เพิ่มครูที่ถูก assign
		var assignedTeacher models.User
		if err := ns.db.First(&assignedTeacher, session.Schedule.AssignedToUserID).Error; err == nil {
			users = append(users, assignedTeacher)
		}

		for _, user := range users {
			userSessions[user.ID] = append(userSessions[user.ID], session)
		}
	}

	// ส่ง notification สำหรับแต่ละ user
	for userID, sessions := range userSessions {
		if len(sessions) > 0 {
			ns.sendDailyReminderNotification(userID, sessions)
		}
	}
}

// sendDailyReminderNotification ส่ง notification สรุปตารางเรียนประจำวัน
func (ns *NotificationScheduler) sendDailyReminderNotification(userID uint, sessions []models.Schedule_Sessions) {
	messageEn := "Today's schedule:\n"
	messageTh := "ตารางเรียนวันนี้:\n"

	for _, session := range sessions {
		messageEn += fmt.Sprintf("- %s at %s\n",
			session.Schedule.ScheduleName,
			session.Start_time.Format("15:04"))
		messageTh += fmt.Sprintf("- %s เวลา %s\n",
			session.Schedule.ScheduleName,
			session.Start_time.Format("15:04"))
	}

	notification := models.Notification{
		UserID:    userID,
		Title:     "Daily Schedule Reminder",
		TitleTh:   "เตือนตารางเรียนประจำวัน",
		Message:   messageEn,
		MessageTh: messageTh,
		Type:      "info",
	}

	if err := ns.db.Create(&notification).Error; err != nil {
		fmt.Printf("Error creating daily reminder for user %d: %v\n", userID, err)
	}
}

// CheckMissedSessions ตรวจสอบ sessions ที่พลาดไป (no-show)
func (ns *NotificationScheduler) CheckMissedSessions() {
	now := time.Now()
	pastTime := now.Add(-30 * time.Minute) // ตรวจสอบ sessions ที่ผ่านมา 30 นาที

	var sessions []models.Schedule_Sessions
	err := ns.db.Where("start_time < ? AND status = ?", pastTime, "confirmed").
		Preload("Schedule").
		Find(&sessions).Error

	if err != nil {
		fmt.Printf("Error checking missed sessions: %v\n", err)
		return
	}

	for _, session := range sessions {
		// อัพเดทสถานะเป็น no-show
		ns.db.Model(&session).Update("status", "no-show")

		// ส่ง notification ให้ admin/owner
		ns.sendMissedSessionNotification(session)
	}
}

// sendMissedSessionNotification ส่ง notification เมื่อมี session no-show
func (ns *NotificationScheduler) sendMissedSessionNotification(session models.Schedule_Sessions) {
	// หา admin และ owner
	var admins []models.User
	err := ns.db.Where("role IN ?", []string{"admin", "owner"}).Find(&admins).Error
	if err != nil {
		return
	}

	for _, admin := range admins {
		notification := models.Notification{
			UserID:    admin.ID,
			Title:     "Missed Session Alert",
			TitleTh:   "แจ้งเตือน Session พลาด",
			Message:   fmt.Sprintf("Session '%s' on %s was missed (no-show)", session.Schedule.ScheduleName, session.Session_date.Format("2006-01-02")),
			MessageTh: fmt.Sprintf("Session '%s' วันที่ %s พลาด (no-show)", session.Schedule.ScheduleName, session.Session_date.Format("2006-01-02")),
			Type:      "warning",
		}

		ns.db.Create(&notification)
	}
}
