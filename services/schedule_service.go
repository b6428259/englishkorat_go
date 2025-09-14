package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"englishkorat_go/database"
	"englishkorat_go/models"
)

// HolidayResponse represents the Thai holiday API response
type HolidayResponse struct {
	VCALENDAR []struct {
		VEVENT []struct {
			DTStart string `json:"DTSTART"`
			Summary string `json:"SUMMARY"`
		} `json:"VEVENT"`
	} `json:"VCALENDAR"`
}

// CheckRoomConflict ตรวจสอบการชนของห้องสำหรับ schedule ใหม่
func CheckRoomConflict(roomID uint, startDate, endDate time.Time, sessionStartTime string, hoursPerSession int, recurringPattern string, customDays []int) (bool, error) {
	// ดึง schedule ที่มีอยู่แล้วในห้องนี้ และเป็น type class
	var existingSchedules []models.Schedules
	err := database.DB.Where("room_id = ? AND schedule_type = ? AND status IN ?",
		roomID, "class", []string{"scheduled", "assigned"}).
		Find(&existingSchedules).Error
	if err != nil {
		return false, err
	}

	// แปลง session start time
	startTime, err := time.Parse("15:04", sessionStartTime)
	if err != nil {
		return false, fmt.Errorf("invalid session start time format")
	}

	// สร้าง sessions ชั่วคราวสำหรับ schedule ใหม่
	newSessions := generateSessionTimes(startDate, endDate, startTime, hoursPerSession, recurringPattern, customDays)

	// ตรวจสอบการชนกับ sessions ที่มีอยู่
	for _, schedule := range existingSchedules {
		var existingSessions []models.Schedule_Sessions
		err := database.DB.Where("schedule_id = ? AND status NOT IN ?",
			schedule.ID, []string{"cancelled", "no-show"}).
			Find(&existingSessions).Error
		if err != nil {
			return false, err
		}

		// เปรียบเทียบเวลา
		for _, newSession := range newSessions {
			for _, existingSession := range existingSessions {
				if sessionsOverlap(newSession, existingSession) {
					return true, nil // มีการชน
				}
			}
		}
	}

	return false, nil
}

// sessionsOverlap ตรวจสอบว่า session 2 session ทับกันหรือไม่
func sessionsOverlap(session1, session2 models.Schedule_Sessions) bool {
	// ตรวจสอบวันที่เดียวกัน
	date1 := session1.Session_date.Format("2006-01-02")
	date2 := session2.Session_date.Format("2006-01-02")
	if date1 != date2 {
		return false
	}

	// ตรวจสอบเวลาทับกัน
	start1 := session1.Start_time.Format("15:04")
	end1 := session1.End_time.Format("15:04")
	start2 := session2.Start_time.Format("15:04")
	end2 := session2.End_time.Format("15:04")

	return !(end1 <= start2 || end2 <= start1)
}

// generateSessionTimes สร้างเวลา session ชั่วคราวสำหรับตรวจสอบการชน
func generateSessionTimes(startDate, endDate time.Time, sessionStartTime time.Time, hoursPerSession int, recurringPattern string, customDays []int) []models.Schedule_Sessions {
	var sessions []models.Schedule_Sessions
	current := startDate
	sessionNumber := 1

	for current.Before(endDate) || current.Equal(endDate) {
		shouldCreateSession := false

		switch recurringPattern {
		case "daily":
			shouldCreateSession = true
		case "weekly":
			shouldCreateSession = current.Weekday() == startDate.Weekday()
		case "bi-weekly":
			weeksDiff := int(current.Sub(startDate).Hours() / (24 * 7))
			shouldCreateSession = weeksDiff%2 == 0 && current.Weekday() == startDate.Weekday()
		case "custom":
			weekday := int(current.Weekday())
			for _, day := range customDays {
				if day == weekday {
					shouldCreateSession = true
					break
				}
			}
		}

		if shouldCreateSession {
			sessionStart := time.Date(current.Year(), current.Month(), current.Day(),
				sessionStartTime.Hour(), sessionStartTime.Minute(), 0, 0, current.Location())
			sessionEnd := sessionStart.Add(time.Duration(hoursPerSession) * time.Hour)

			session := models.Schedule_Sessions{
				Session_date:          current,
				Start_time:            sessionStart,
				End_time:              sessionEnd,
				Session_number:        sessionNumber,
				Status:                "scheduled",
				Makeup_for_session_id: nil,
			}
			sessions = append(sessions, session)
			sessionNumber++
		}

		current = current.AddDate(0, 0, 1)
	}

	return sessions
}

// GenerateScheduleSessions สร้าง sessions ตาม recurring pattern
func GenerateScheduleSessions(schedule models.Schedules, sessionStartTime string, customDays []int) ([]models.Schedule_Sessions, error) {
	startTime, err := time.Parse("15:04", sessionStartTime)
	if err != nil {
		return nil, fmt.Errorf("invalid session start time format")
	}

	var sessions []models.Schedule_Sessions
	current := schedule.Start_date
	sessionNumber := 1
	weekNumber := 1

	// คำนวณ sessions ที่ต้องการทั้งหมด
	totalSessions := schedule.Total_hours / schedule.Hours_per_session

	for sessionNumber <= totalSessions && (current.Before(schedule.Estimated_end_date) || current.Equal(schedule.Estimated_end_date)) {
		shouldCreateSession := false

		switch schedule.Recurring_pattern {
		case "daily":
			shouldCreateSession = true
		case "weekly":
			shouldCreateSession = current.Weekday() == schedule.Start_date.Weekday()
		case "bi-weekly":
			weeksDiff := int(current.Sub(schedule.Start_date).Hours() / (24 * 7))
			shouldCreateSession = weeksDiff%2 == 0 && current.Weekday() == schedule.Start_date.Weekday()
		case "monthly":
			shouldCreateSession = current.Day() == schedule.Start_date.Day()
		case "custom":
			weekday := int(current.Weekday())
			for _, day := range customDays {
				if day == weekday {
					shouldCreateSession = true
					break
				}
			}
		}

		if shouldCreateSession {
			sessionStart := time.Date(current.Year(), current.Month(), current.Day(),
				startTime.Hour(), startTime.Minute(), 0, 0, current.Location())
			sessionEnd := sessionStart.Add(time.Duration(schedule.Hours_per_session) * time.Hour)

			session := models.Schedule_Sessions{
				Session_date:          current,
				Start_time:            sessionStart,
				End_time:              sessionEnd,
				Session_number:        sessionNumber,
				Week_number:           weekNumber,
				Status:                "scheduled",
				Makeup_for_session_id: nil,
			}
			sessions = append(sessions, session)
			sessionNumber++

			// เพิ่ม week number ตาม session per week
			if sessionNumber%schedule.Session_per_week == 1 && sessionNumber > 1 {
				weekNumber++
			}
		}

		current = current.AddDate(0, 0, 1)
	}

	return sessions, nil
}

// GetThaiHolidays ดึงวันหยุดไทยจาก API
func GetThaiHolidays(startYear, endYear int) ([]time.Time, error) {
	var allHolidays []time.Time

	for year := startYear; year <= endYear; year++ {
		// แปลงปี ค.ศ. เป็น พ.ศ.
		buddhistYear := year + 543
		url := fmt.Sprintf("https://www.myhora.com/calendar/ical/holiday.aspx?%d.json", buddhistYear)

		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch holidays for year %d: %v", year, err)
		}
		defer resp.Body.Close()

		var holidayResp HolidayResponse
		if err := json.NewDecoder(resp.Body).Decode(&holidayResp); err != nil {
			return nil, fmt.Errorf("failed to decode holiday response for year %d: %v", year, err)
		}

		// แปลงวันหยุดเป็น time.Time
		for _, calendar := range holidayResp.VCALENDAR {
			for _, event := range calendar.VEVENT {
				if dateStr := event.DTStart; dateStr != "" {
					if date, err := time.Parse("20060102", dateStr); err == nil {
						allHolidays = append(allHolidays, date)
					}
				}
			}
		}
	}

	return allHolidays, nil
}

// RescheduleSessions ปรับ sessions ที่ตรงกับวันหยุด
func RescheduleSessions(sessions []models.Schedule_Sessions, holidays []time.Time) []models.Schedule_Sessions {
	holidayMap := make(map[string]bool)
	for _, holiday := range holidays {
		holidayMap[holiday.Format("2006-01-02")] = true
	}

	var rescheduledSessions []models.Schedule_Sessions

	for _, session := range sessions {
		sessionDate := session.Session_date.Format("2006-01-02")

		if holidayMap[sessionDate] {
			// หาวันถัดไปที่ไม่ใช่วันหยุด
			newDate := session.Session_date.AddDate(0, 0, 1)
			for holidayMap[newDate.Format("2006-01-02")] {
				newDate = newDate.AddDate(0, 0, 1)
			}

			// อัพเดทวันที่ session
			session.Session_date = newDate
			session.Start_time = time.Date(newDate.Year(), newDate.Month(), newDate.Day(),
				session.Start_time.Hour(), session.Start_time.Minute(), 0, 0, newDate.Location())
			session.End_time = time.Date(newDate.Year(), newDate.Month(), newDate.Day(),
				session.End_time.Hour(), session.End_time.Minute(), 0, 0, newDate.Location())
			session.Notes = "Rescheduled due to holiday"
		}

		rescheduledSessions = append(rescheduledSessions, session)
	}

	return rescheduledSessions
}

// NotifyStudentsScheduleConfirmed ส่ง notification ให้นักเรียนเมื่อ schedule ถูกยืนยัน
func NotifyStudentsScheduleConfirmed(scheduleID uint) {
	var schedule models.Schedules
	if err := database.DB.Preload("Group.Course").First(&schedule, scheduleID).Error; err != nil {
		return
	}

	// ดึงรายชื่อนักเรียนจาก group members (สำหรับ class schedules)
	if schedule.GroupID != nil {
		var groupMembers []models.GroupMember
		if err := database.DB.Preload("Student.User").Where("group_id = ?", *schedule.GroupID).Find(&groupMembers).Error; err != nil {
			return
		}

		// สร้าง notification สำหรับนักเรียนแต่ละคน
		for _, member := range groupMembers {
			if member.Student.UserID != nil {
				notification := models.Notification{
					UserID:    *member.Student.UserID,
					Title:     "Schedule Confirmed",
					TitleTh:   "ตารางเรียนได้รับการยืนยันแล้ว",
					Message:   fmt.Sprintf("Your class schedule '%s' has been confirmed by the teacher.", schedule.ScheduleName),
					MessageTh: fmt.Sprintf("ตารางเรียน '%s' ของคุณได้รับการยืนยันจากครูแล้ว", schedule.ScheduleName),
					Type:      "success",
				}

				database.DB.Create(&notification)
			}
		}
	} else {
		// สำหรับ event/appointment schedules - ส่งให้ participants
		var participants []models.ScheduleParticipant
		if err := database.DB.Where("schedule_id = ?", scheduleID).Find(&participants).Error; err != nil {
			return
		}

		for _, participant := range participants {
			notification := models.Notification{
				UserID:    participant.UserID,
				Title:     "Schedule Confirmed",
				TitleTh:   "ตารางนัดหมายได้รับการยืนยันแล้ว",
				Message:   fmt.Sprintf("Your scheduled '%s' has been confirmed.", schedule.ScheduleName),
				MessageTh: fmt.Sprintf("การนัดหมาย '%s' ของคุณได้รับการยืนยันแล้ว", schedule.ScheduleName),
				Type:      "success",
			}

			database.DB.Create(&notification)
		}
	}
}

// NotifyUpcomingClass ส่ง notification เตือนก่อนเรียน
func NotifyUpcomingClass(sessionID uint, minutesBefore int) {
	var session models.Schedule_Sessions
	if err := database.DB.First(&session, sessionID).Error; err != nil {
		return
	}

	var schedule models.Schedules
	if err := database.DB.Preload("Group.Course").First(&schedule, session.ScheduleID).Error; err != nil {
		return
	}

	// ตรวจสอบว่า session ได้รับการยืนยันแล้ว
	if session.Status != "confirmed" {
		return
	}

	// ดึงรายชื่อผู้เข้าร่วม (ครูและนักเรียน)
	var users []models.User
	
	// For class schedules - get users from group members
	if schedule.GroupID != nil {
		var groupMembers []models.GroupMember
		err := database.DB.Preload("Student.User").Where("group_id = ?", *schedule.GroupID).Find(&groupMembers).Error
		if err == nil {
			for _, member := range groupMembers {
				if member.Student.UserID != nil {
					var user models.User
					if err := database.DB.First(&user, *member.Student.UserID).Error; err == nil {
						users = append(users, user)
					}
				}
			}
		}
	} else {
		// For event/appointment schedules - get participants
		var participants []models.ScheduleParticipant
		err := database.DB.Preload("User").Where("schedule_id = ?", schedule.ID).Find(&participants).Error
		if err == nil {
			for _, participant := range participants {
				users = append(users, participant.User)
			}
		}
	}

	// เพิ่มครูที่ถูก assign (ใช้ default teacher หรือ teacher specific สำหรับ session)
	teacherID := session.AssignedTeacherID
	if teacherID == nil {
		teacherID = schedule.DefaultTeacherID
	}
	
	if teacherID != nil {
		var assignedTeacher models.User
		if err := database.DB.First(&assignedTeacher, *teacherID).Error; err == nil {
			// สร้าง notification สำหรับครู
			notification := models.Notification{
				UserID:    assignedTeacher.ID,
				Title:     "Upcoming Class",
				TitleTh:   "เรียนจะเริ่มเร็วๆ นี้",
				Message:   fmt.Sprintf("Your class '%s' will start in %d minutes at %s", schedule.ScheduleName, minutesBefore, session.Start_time.Format("15:04")),
				MessageTh: fmt.Sprintf("คลาส '%s' ของคุณจะเริ่มในอีก %d นาที เวลา %s", schedule.ScheduleName, minutesBefore, session.Start_time.Format("15:04")),
				Type:      "info",
			}
			database.DB.Create(&notification)
		}
	}

	// สร้าง notification สำหรับนักเรียน
	for _, user := range users {
		if user.Role == "student" {
			notification := models.Notification{
				UserID:    user.ID,
				Title:     "Upcoming Class",
				TitleTh:   "เรียนจะเริ่มเร็วๆ นี้",
				Message:   fmt.Sprintf("Your class '%s' will start in %d minutes at %s", schedule.ScheduleName, minutesBefore, session.Start_time.Format("15:04")),
				MessageTh: fmt.Sprintf("คลาส '%s' ของคุณจะเริ่มในอีก %d นาที เวลา %s", schedule.ScheduleName, minutesBefore, session.Start_time.Format("15:04")),
				Type:      "info",
			}
			database.DB.Create(&notification)
		}
	}
}

// ScheduleNotifications ตั้งเวลา notification สำหรับ sessions ที่จะมาถึง
func ScheduleNotifications() {
	// ดึง sessions ที่จะเกิดขึ้นในอนาคต
	var sessions []models.Schedule_Sessions
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)

	err := database.DB.Where("session_date BETWEEN ? AND ? AND status = ?",
		now.Format("2006-01-02"), tomorrow.Format("2006-01-02"), "confirmed").
		Find(&sessions).Error
	if err != nil {
		return
	}

	for _, session := range sessions {
		// คำนวณเวลาก่อนเรียน (เช่น 30 นาที, 1 ชั่วโมง)
		notificationTimes := []int{30, 60} // นาที

		for _, minutes := range notificationTimes {
			notifyTime := session.Start_time.Add(-time.Duration(minutes) * time.Minute)

			if notifyTime.After(now) {
				// ตั้งเวลาส่ง notification (ในระบบจริงอาจใช้ cron job หรือ message queue)
				go func(sessionID uint, mins int, notifyAt time.Time) {
					time.Sleep(notifyAt.Sub(now))
					NotifyUpcomingClass(sessionID, mins)
				}(session.ID, minutes, notifyTime)
			}
		}
	}
}
