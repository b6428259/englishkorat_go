package services

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"englishkorat_go/database"
	"englishkorat_go/models"
	notifsvc "englishkorat_go/services/notifications"
)

// NotificationScheduler จัดการการส่ง notification อัตโนมัติ
type NotificationScheduler struct {
	db *gorm.DB
	ns *notifsvc.Service
}

// NewNotificationScheduler สร้าง NotificationScheduler ใหม่
func NewNotificationScheduler() *NotificationScheduler {
	return &NotificationScheduler{
		db: database.DB,
		ns: notifsvc.NewService(),
	}
}

// StartScheduler เริ่มต้น scheduler สำหรับ notification
func (ns *NotificationScheduler) StartScheduler() {
	// ตั้งค่าให้ทำงานทุก 15 นาที
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	fmt.Println("Notification scheduler started...")

	for range ticker.C {
		ns.CheckUpcomingSessions()
	}
}

// CheckUpcomingSessions ตรวจสอบ sessions ที่จะเกิดขึ้นเร็วๆ นี้
func (ns *NotificationScheduler) CheckUpcomingSessions() {

	now := time.Now()

	// กำหนดช่วงเวลาที่จะแจ้งเตือน (ภายใน 5 นาทีจากตอนนี้)
	startRange := now
	endRange := now.Add(5 * time.Minute)

	var sessions []models.Schedule_Sessions
	err := ns.db.Where("start_time BETWEEN ? AND ? AND status = ?", startRange, endRange, "confirmed").
		Preload("Schedule").
		Find(&sessions).Error

	if err != nil {
		fmt.Printf("Error checking upcoming sessions: %v\n", err)
		return
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
	
	// โหลด schedule
	var schedule models.Schedules
	if err := ns.db.Preload("Group.Course").First(&schedule, session.ScheduleID).Error; err != nil {
		fmt.Printf("Error fetching schedule for session %d: %v\n", session.ID, err)
		return
	}

	// ดึงรายชื่อผู้เข้าร่วม
	var users []models.User
	
	// For class schedules - get users from group members
	if schedule.GroupID != nil {
		var groupMembers []models.GroupMember
		err := ns.db.Preload("Student.User").Where("group_id = ?", *schedule.GroupID).Find(&groupMembers).Error
		if err != nil {
			fmt.Printf("Error fetching group members for group %d: %v\n", *schedule.GroupID, err)
			return
		}
		
		for _, member := range groupMembers {
			if member.Student.UserID != nil {
				var user models.User
				if err := ns.db.First(&user, *member.Student.UserID).Error; err == nil {
					users = append(users, user)
				}
			}
		}
	} else {
		// For event/appointment schedules - get participants
		var participants []models.ScheduleParticipant
		err := ns.db.Preload("User").Where("schedule_id = ?", schedule.ID).Find(&participants).Error
		if err != nil {
			fmt.Printf("Error fetching participants for schedule %d: %v\n", schedule.ID, err)
			return
		}
		
		for _, participant := range participants {
			users = append(users, participant.User)
		}
	}

	// เพิ่มครูที่ถูก assign (ใช้ default teacher หรือ teacher specific สำหรับ session)
	teacherID := session.AssignedTeacherID
	if teacherID == nil {
		teacherID = schedule.DefaultTeacherID
	}
	
	if teacherID != nil {
		var assignedTeacher models.User
		if err := ns.db.First(&assignedTeacher, *teacherID).Error; err == nil {
			users = append(users, assignedTeacher)
		}
	}

	// ส่ง notification ผ่าน service (รองรับ queue และ websocket broadcast)
	if len(users) > 0 {
		userIDs := make([]uint, 0, len(users))
		for _, u := range users {
			userIDs = append(userIDs, u.ID)
		}
		title := "Upcoming Class"
		titleTh := "เรียนจะเริ่มเร็วๆ นี้"
		msg := fmt.Sprintf("Your class '%s' will start in %s at %s", schedule.ScheduleName, timeLabel, session.Start_time.Format("15:04"))
		msgTh := fmt.Sprintf("คลาส '%s' ของคุณจะเริ่มในอีก %s เวลา %s", schedule.ScheduleName, ns.translateTimeLabel(timeLabel), session.Start_time.Format("15:04"))
		q := notifsvc.QueuedForController(title, titleTh, msg, msgTh, "info")
		if err := ns.ns.EnqueueOrCreate(userIDs, q); err != nil {
			fmt.Printf("Error creating notifications for session %d: %v\n", session.ID, err)
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
		
		// For class schedules - get users from group members
		if session.Schedule.GroupID != nil {
			var groupMembers []models.GroupMember
			err := ns.db.Preload("Student.User").Where("group_id = ?", *session.Schedule.GroupID).Find(&groupMembers).Error
			if err == nil {
				for _, member := range groupMembers {
					if member.Student.UserID != nil {
						var user models.User
						if err := ns.db.First(&user, *member.Student.UserID).Error; err == nil {
							users = append(users, user)
						}
					}
				}
			}
		} else {
			// For event/appointment schedules - get participants
			var participants []models.ScheduleParticipant
			err := ns.db.Preload("User").Where("schedule_id = ?", session.Schedule.ID).Find(&participants).Error
			if err == nil {
				for _, participant := range participants {
					users = append(users, participant.User)
				}
			}
		}

		// เพิ่มครูที่ถูก assign (ใช้ default teacher หรือ teacher specific สำหรับ session)
		teacherID := session.AssignedTeacherID
		if teacherID == nil {
			teacherID = session.Schedule.DefaultTeacherID
		}
		
		if teacherID != nil {
			var assignedTeacher models.User
			if err := ns.db.First(&assignedTeacher, *teacherID).Error; err == nil {
				users = append(users, assignedTeacher)
			}
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

	q := notifsvc.QueuedForController("Daily Schedule Reminder", "เตือนตารางเรียนประจำวัน", messageEn, messageTh, "info")
	if err := ns.ns.EnqueueOrCreate([]uint{userID}, q); err != nil {
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

	if len(admins) > 0 {
		userIDs := make([]uint, 0, len(admins))
		for _, a := range admins {
			userIDs = append(userIDs, a.ID)
		}
		title := "Missed Session Alert"
		titleTh := "แจ้งเตือน Session พลาด"
		msg := fmt.Sprintf("Session '%s' on %s was missed (no-show)", session.Schedule.ScheduleName, session.Session_date.Format("2006-01-02"))
		msgTh := fmt.Sprintf("Session '%s' วันที่ %s พลาด (no-show)", session.Schedule.ScheduleName, session.Session_date.Format("2006-01-02"))
		q := notifsvc.QueuedForController(title, titleTh, msg, msgTh, "warning")
		if err := ns.ns.EnqueueOrCreate(userIDs, q); err != nil {
			fmt.Printf("Error creating missed-session notifications: %v\n", err)
		}
	}
}
