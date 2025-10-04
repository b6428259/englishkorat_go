package services

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"englishkorat_go/database"
	"englishkorat_go/models"
	notifsvc "englishkorat_go/services/notifications"

	"github.com/robfig/cron/v3"
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
// อิง incoming: ยิงแจ้งเตือนแบบเป็นช่วงเวลา และกันยิงซ้ำด้วย hasNotificationBeenSent
// เพิ่มเติมจาก current: รองรับแจ้งเตือน 5 นาทีด้วย
func (ns *NotificationScheduler) CheckUpcomingSessions() {
	now := time.Now()

	// กำหนดช่วงเวลาที่จะแจ้งเตือน (5, 30, 60 นาที)
	notificationTimes := []struct {
		minutes int
		label   string
	}{
		{5, "5 minutes"},
		{30, "30 minutes"},
		{60, "1 hour"},
	}

	for _, nt := range notificationTimes {
		targetTime := now.Add(time.Duration(nt.minutes) * time.Minute)

		// หา sessions ที่จะเริ่มในช่วงเวลาที่กำหนด (±5 นาที เพื่อเผื่อสั่นนาฬิกา/สโครล cron)
		startRange := targetTime.Add(-5 * time.Minute)
		endRange := targetTime.Add(5 * time.Minute)

		var sessions []models.Schedule_Sessions
		err := ns.db.
			Where("start_time BETWEEN ? AND ? AND status = ?", startRange, endRange, "scheduled").
			Find(&sessions).Error
		if err != nil {
			fmt.Printf("Error checking upcoming sessions: %v\n", err)
			continue
		}

		for _, session := range sessions {
			// ให้ฟังก์ชันส่งแจ้งเตือนเป็นผู้รับผิดชอบการกันซ้ำ (หลังดึง schedule แล้ว)
			ns.sendUpcomingClassNotification(session, nt.minutes, nt.label)
		}
	}
}

// hasNotificationBeenSent ตรวจสอบว่าได้ส่ง notification แล้วหรือยัง
// ใช้ข้อความภาษาอังกฤษเป็น anchor แบบ incoming เพื่อความสม่ำเสมอ
func (ns *NotificationScheduler) hasNotificationBeenSent(scheduleName, timeLabel, startAt string) bool {
	// ใช้ anchor 3 ส่วน: ชื่อคลาส, ช่วงเวลา (เช่น 5 minutes/1 hour), และเวลาเริ่ม HH:MM
	// จำกัดช่วงเวลา 3 ชั่วโมงเพื่อกันซ้ำรอบๆ cron windows
	var count int64
	cutoff := time.Now().Add(-3 * time.Hour)
	// ตัวอย่างข้อความ: Your class 'ABC' will start in 1 hour at 14:30
	if err := ns.db.Model(&models.Notification{}).
		Where("message LIKE ?", fmt.Sprintf("%%class '%s'%%", scheduleName)).
		Where("message LIKE ?", fmt.Sprintf("%%will start in %s%%", timeLabel)).
		Where("message LIKE ?", fmt.Sprintf("%%at %s%%", startAt)).
		Where("created_at > ?", cutoff).
		Count(&count).Error; err != nil {
		// หาก query มีปัญหา ให้ถือว่ายังไม่ส่ง (เพื่อไม่บล็อกการแจ้งเตือนโดยไม่ตั้งใจ)
		return false
	}
	return count > 0
}

// sendUpcomingClassNotification ส่ง notification สำหรับ class ที่จะเริ่มเร็วๆ นี้
// อิง incoming เป็นหลัก: ดึงผู้เรียนจาก group/participants, เลือกครูจาก AssignedTeacherID/DefaultTeacherID
// เพิ่มเติมความเข้ากันได้กับ current: เผื่อกรณี schedule.AssignedToUserID (สคีมาเก่า)
func (ns *NotificationScheduler) sendUpcomingClassNotification(session models.Schedule_Sessions, minutes int, timeLabel string) {
	// ดึงข้อมูล schedule
	var schedule models.Schedules
	if err := ns.db.Preload("Group.Course").First(&schedule, session.ScheduleID).Error; err != nil {
		fmt.Printf("Error fetching schedule for session %d: %v\n", session.ID, err)
		return
	}

	// กันยิงซ้ำ: ตรวจจากข้อความภาษาอังกฤษที่เราจะส่งจริง ๆ
	startHHMM := ""
	if session.Start_time != nil {
		startHHMM = session.Start_time.Format("15:04")
	}
	if ns.hasNotificationBeenSent(schedule.ScheduleName, timeLabel, startHHMM) {
		return
	}

	// ดึงรายชื่อผู้เข้าร่วม
	var users []models.User

	// ถ้าเป็นคลาสแบบ Group -> ดึงจากสมาชิกกลุ่ม
	if schedule.GroupID != nil {
		var groupMembers []models.GroupMember
		if err := ns.db.Preload("Student.User").Where("group_id = ?", *schedule.GroupID).Find(&groupMembers).Error; err != nil {
			fmt.Printf("Error fetching group members for group %d: %v\n", *schedule.GroupID, err)
			return
		}
		for _, member := range groupMembers {
			if member.Student.UserID != nil {
				var u models.User
				if err := ns.db.First(&u, *member.Student.UserID).Error; err == nil {
					users = append(users, u)
				}
			}
		}
	} else {
		// ถ้าเป็น event/appointment -> ดึงจาก participants
		var participants []models.ScheduleParticipant
		if err := ns.db.Preload("User").Where("schedule_id = ?", schedule.ID).Find(&participants).Error; err != nil {
			fmt.Printf("Error fetching participants for schedule %d: %v\n", schedule.ID, err)
			return
		}
		for _, p := range participants {
			users = append(users, p.User)
		}
	}

	// เพิ่มครูที่ถูก assign: ใช้ teacher ของ session ก่อน ถ้าไม่มีใช้ default ของ schedule
	teacherID := session.AssignedTeacherID
	if teacherID == nil {
		teacherID = schedule.DefaultTeacherID
	}
	if teacherID != nil {
		var teacher models.User
		if err := ns.db.First(&teacher, *teacherID).Error; err == nil {
			users = append(users, teacher)
		}
	}

	// หมายเหตุ: ตัดการรองรับฟิลด์ legacy schedule.AssignedToUserID เนื่องจากไม่มีในโมเดลปัจจุบัน

	// ส่ง notification ผ่าน service (รองรับ queue และ websocket broadcast)
	if len(users) > 0 {
		// dedupe ผู้รับเพื่อกันยิงซ้ำต่อคน
		unique := make(map[uint]struct{}, len(users))
		userIDs := make([]uint, 0, len(users))
		for _, u := range users {
			if _, ok := unique[u.ID]; !ok {
				unique[u.ID] = struct{}{}
				userIDs = append(userIDs, u.ID)
			}
		}

		startLabel := ""
		if session.Start_time != nil {
			startLabel = session.Start_time.Format("15:04")
		}
		title := "Upcoming Class"
		titleTh := "เรียนจะเริ่มเร็วๆ นี้"
		msg := fmt.Sprintf("Your class '%s' will start in %s at %s",
			schedule.ScheduleName, timeLabel, startLabel)
		msgTh := fmt.Sprintf("คลาส '%s' ของคุณจะเริ่มในอีก %s เวลา %s",
			schedule.ScheduleName, ns.translateTimeLabel(timeLabel), startLabel)

		data := map[string]interface{}{
			"link": map[string]interface{}{
				"href":   fmt.Sprintf("/api/schedules/sessions/%d", session.ID),
				"method": "GET",
			},
			"action":                  "open-session",
			"session_id":              session.ID,
			"schedule_id":             schedule.ID,
			"reminder_before_minutes": minutes,
		}
		q := notifsvc.QueuedWithData(title, titleTh, msg, msgTh, "info", data, "normal", "popup")
		if err := ns.ns.EnqueueOrCreate(userIDs, q); err != nil {
			fmt.Printf("Error creating notifications for session %d: %v\n", session.ID, err)
		}
	}

	fmt.Printf("Sent upcoming class notifications for session %d (%s before)\n", session.ID, timeLabel)
}

// translateTimeLabel แปลงเวลาเป็นภาษาไทย
func (ns *NotificationScheduler) translateTimeLabel(timeLabel string) string {
	switch timeLabel {
	case "5 minutes":
		return "5 นาที"
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
		today.Format("2006-01-02"), tomorrow.Format("2006-01-02"), "scheduled").
		Preload("Schedule").
		Preload("Schedule.Group").
		Preload("Schedule.Group.Course").
		Find(&sessions).Error
	if err != nil {
		fmt.Printf("Error fetching daily sessions: %v\n", err)
		return
	}

	// จัดกลุ่ม sessions ตาม user
	userSessions := make(map[uint][]models.Schedule_Sessions)

	for _, session := range sessions {
		var users []models.User

		// สำหรับคลาสกลุ่ม
		if session.Schedule.GroupID != nil {
			var groupMembers []models.GroupMember
			if err := ns.db.Preload("Student.User").Where("group_id = ?", *session.Schedule.GroupID).Find(&groupMembers).Error; err == nil {
				for _, member := range groupMembers {
					if member.Student.UserID != nil {
						var u models.User
						if err := ns.db.First(&u, *member.Student.UserID).Error; err == nil {
							users = append(users, u)
						}
					}
				}
			}
		} else {
			// สำหรับ event/appointment
			var participants []models.ScheduleParticipant
			if err := ns.db.Preload("User").Where("schedule_id = ?", session.Schedule.ID).Find(&participants).Error; err == nil {
				for _, p := range participants {
					users = append(users, p.User)
				}
			}
		}

		// ครูที่ถูก assign (session > default)
		teacherID := session.AssignedTeacherID
		if teacherID == nil {
			teacherID = session.Schedule.DefaultTeacherID
		}
		if teacherID != nil {
			var teacher models.User
			if err := ns.db.First(&teacher, *teacherID).Error; err == nil {
				users = append(users, teacher)
			}
		}

		// หมายเหตุ: ตัดการรองรับฟิลด์ legacy schedule.AssignedToUserID เนื่องจากไม่มีในโมเดลปัจจุบัน

		for _, u := range users {
			userSessions[u.ID] = append(userSessions[u.ID], session)
		}
	}

	// ส่ง notification สำหรับแต่ละ user
	for userID, ss := range userSessions {
		if len(ss) > 0 {
			ns.sendDailyReminderNotification(userID, ss)
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

	data := map[string]interface{}{
		"action": "open-today-schedule",
	}
	q := notifsvc.QueuedWithData("Daily Schedule Reminder", "เตือนตารางเรียนประจำวัน", messageEn, messageTh, "info", data, "normal", "popup")
	if err := ns.ns.EnqueueOrCreate([]uint{userID}, q); err != nil {
		fmt.Printf("Error creating daily reminder for user %d: %v\n", userID, err)
	}
}

// CheckMissedSessions ตรวจสอบ sessions ที่พลาดไป (no-show)
func (ns *NotificationScheduler) CheckMissedSessions() {
	now := time.Now()
	pastTime := now.Add(-30 * time.Minute) // ตรวจสอบ sessions ที่ผ่านมา 30 นาที

	// อัพเดท sessions ที่ได้รับการยืนยันแล้วและจบไปเป็น completed
	if tx := ns.db.Model(&models.Schedule_Sessions{}).
		Where("end_time IS NOT NULL AND end_time <= ? AND status = ?", now, "confirmed").
		Update("status", "completed"); tx.Error != nil {
		fmt.Printf("Error marking confirmed sessions as completed: %v\n", tx.Error)
	} else if tx.RowsAffected > 0 {
		fmt.Printf("Marked %d confirmed sessions as completed\n", tx.RowsAffected)
	}

	var sessions []models.Schedule_Sessions
	err := ns.db.Where("start_time < ? AND status = ?", pastTime, "scheduled").
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
	if err := ns.db.Where("role IN ?", []string{"admin", "owner"}).Find(&admins).Error; err != nil {
		return
	}

	if len(admins) == 0 {
		return
	}

	userIDs := make([]uint, 0, len(admins))
	for _, a := range admins {
		userIDs = append(userIDs, a.ID)
	}

	dateLabel := ""
	if session.Session_date != nil {
		dateLabel = session.Session_date.Format("2006-01-02")
	}
	title := "Missed Session Alert"
	titleTh := "แจ้งเตือน Session พลาด"
	msg := fmt.Sprintf("Session '%s' on %s was missed (no-show)",
		session.Schedule.ScheduleName, dateLabel)
	msgTh := fmt.Sprintf("Session '%s' วันที่ %s พลาด (no-show)",
		session.Schedule.ScheduleName, dateLabel)

	data := map[string]interface{}{
		"link": map[string]interface{}{
			"href":   fmt.Sprintf("/api/schedules/sessions/%d", session.ID),
			"method": "GET",
		},
		"action":      "review-missed-session",
		"session_id":  session.ID,
		"schedule_id": session.ScheduleID,
	}
	q := notifsvc.QueuedWithData(title, titleTh, msg, msgTh, "warning", data, "normal", "popup")
	if err := ns.ns.EnqueueOrCreate(userIDs, q); err != nil {
		fmt.Printf("Error creating missed-session notifications: %v\n", err)
	}
}

// NEW CODE FOR DAILY NOTI LINE CLASS : JAH

func (ns *NotificationScheduler) StartDailyScheduler() {
	loc, _ := time.LoadLocation("Asia/Bangkok")
	c := cron.New(cron.WithLocation(loc))

	// ตั้ง job ให้รันทุกวันเวลา 01:15 น.
	_, err := c.AddFunc("50 11 * * *", func() {
		log.Println("⏰ Running daily LINE group reminder job...")

		matcher := NewLineGroupMatcher()
		matcher.MatchLineGroupsToGroups() // ✅ แมทช์ LineGroup ↔ Group ก่อน

		ns.sendDailyLineGroupReminders()
	})

	if err != nil {
		log.Fatalf("❌ Failed to schedule daily LINE group reminders: %v", err)
	}

	c.Start()
}

// sendDailyLineGroupReminders ดึง schedule ของพรุ่งนี้ และส่งแจ้งเตือนเข้าไลน์กลุ่ม
func (ns *NotificationScheduler) sendDailyLineGroupReminders() {
	db := database.DB
	//tomorrow := time.Now().AddDate(0, 0, 1)
	loc, _ := time.LoadLocation("Asia/Bangkok")
	now := time.Now().In(loc)
	startOfTomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	endOfTomorrow := startOfTomorrow.Add(24*time.Hour - time.Nanosecond)

	var sessions []models.Schedule_Sessions
	if err := db.
		Preload("Schedule").
		Preload("Schedule.Group").
		Preload("Schedule.DefaultTeacher.Teacher").
		Where("session_date BETWEEN ? AND ?", startOfTomorrow, endOfTomorrow).
		//Where("status IN ?", []string{"scheduled", "confirmed"}).
		Order("start_time ASC").
		Find(&sessions).Error; err != nil {
		log.Printf("❌ Error fetching tomorrow's sessions: %v", err)
		return
	}

	log.Printf("📅 Found %d sessions scheduled for tomorrow (%s)", len(sessions), startOfTomorrow.Format("2006-01-02"))
	if len(sessions) == 0 {
		log.Println("ℹ️ No sessions found for tomorrow")
		return
	}

	lineSvc := NewLineMessagingService()

	for _, sess := range sessions {
		if sess.Schedule == nil || sess.Schedule.Group == nil {
			log.Printf("⚠️ Session ID=%d ไม่มี group", sess.ID)
			continue
		}

		// หา LineGroup ที่แมทช์กับ Group นี้
		var lineGroup models.LineGroup
		if err := db.Where("matched_group_id = ? AND is_active = ?", sess.Schedule.Group.ID, true).
			First(&lineGroup).Error; err != nil {
			log.Printf("⚠️ No LineGroup found for Group '%s' (ID=%d)", sess.Schedule.Group.GroupName, sess.Schedule.Group.ID)
			continue
		}

		teacherName := "-"
		if sess.Schedule.DefaultTeacher != nil && sess.Schedule.DefaultTeacher.Teacher != nil {
			t := sess.Schedule.DefaultTeacher.Teacher
			if t.NicknameEn != "" {
				teacherName = fmt.Sprintf("T.%s", t.NicknameEn)
			}
		} else if sess.Schedule.DefaultTeacher != nil {
			// Fallback to User.username or phone if Teacher profile missing
			if sess.Schedule.DefaultTeacher.Username != "" {
				teacherName = sess.Schedule.DefaultTeacher.Username
			}
		}

		// ✅ format เวลาเรียน
		start := ""
		end := ""
		if sess.Start_time != nil {
			start = sess.Start_time.Format("15:04")
		}
		if sess.End_time != nil {
			end = sess.End_time.Format("15:04")
		}

		// Hardcode branch/room ชั่วครัว
		//branch := "สาขาโคราช"

		// ✅ ดึงสาขาจาก ScheduleName
		branch := "-"
		if sess.Schedule.ScheduleName != "" {
			parts := strings.SplitN(sess.Schedule.ScheduleName, "-", 2) // แบ่งออกเป็น 2 ส่วนเท่านั้น
			if len(parts) > 0 {
				branch = strings.TrimSpace(parts[0]) // เอาส่วนก่อนขีดแรก + trim space
			}
		}
		room := ""
		AbsenceLink := "https://www.englishkorat.site/students/absence"

		msg := fmt.Sprintf("📢 แจ้งเตือนตารางเรียนพรุ่งนี้\nกลุ่ม: %s\nสาขา: %s\nคลาส: %s\nเวลา: %s - %s\nครู: %s\nห้องเรียน: %s\nกรณีแจ้งลา กรุณาแจ้งลาผ่านระบบล่วงหน้าก่อนวันเรียน และภายใน 18.00 น.\nหากแจ้งหลังจากนี้ ระบบจะหักชั่วโมงเรียนอัตโนมัติ\nแจ้งลาที่นี่: %s",

			sess.Schedule.Group.GroupName,
			branch,
			sess.Schedule.ScheduleName,
			start,
			end,
			teacherName,
			room,
			AbsenceLink,
		)

		if err := lineSvc.SendLineMessageToGroup(lineGroup.GroupID, msg); err != nil {
			log.Printf("❌ Failed to send message to group '%s': %v", lineGroup.GroupName, err)
		} else {
			log.Printf("✅ Sent reminder to LineGroup '%s' (%s)", lineGroup.GroupName, lineGroup.GroupID)
		}
	}
}
