package controllers

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"englishkorat_go/database"
	"englishkorat_go/models"
	"englishkorat_go/services"
)

type CreateScheduleRequest struct {
	CourseID              uint      `json:"course_id" validate:"required"`
	AssignedToUserID      uint      `json:"assigned_to_teacher_id" validate:"required"`
	RoomID                uint      `json:"room_id" validate:"required"`
	ScheduleName          string    `json:"schedule_name" validate:"required"`
	ScheduleType          string    `json:"schedule_type" validate:"required,oneof=class meeting event holiday appointment"`
	RecurringPattern      string    `json:"recurring_pattern" validate:"required,oneof=daily weekly bi-weekly monthly yearly custom"`
	TotalHours            int       `json:"total_hours" validate:"required,min=1"`
	HoursPerSession       int       `json:"hours_per_session" validate:"required,min=1"`
	SessionPerWeek        int       `json:"session_per_week" validate:"required,min=1"`
	MaxStudents           int       `json:"max_students" validate:"required,min=1"`
	StartDate             time.Time `json:"start_date" validate:"required"`
	EstimatedEndDate      time.Time `json:"estimated_end_date" validate:"required"`
	AutoRescheduleHoliday bool      `json:"auto_reschedule"`
	Notes                 string    `json:"notes"`
	UserInCourseIDs       []uint    `json:"user_in_course_ids" validate:"required"`
	SessionStartTime      string    `json:"session_start_time" validate:"required"` // เวลาเริ่มต้นของแต่ละ session เช่น "09:00"
	CustomRecurringDays   []int     `json:"custom_recurring_days,omitempty"`        // สำหรับ custom pattern [0=วันอาทิตย์, 1=วันจันทร์, ...]
}

type ConfirmScheduleRequest struct {
	Status string `json:"status" validate:"required,oneof=scheduled"`
}

type CreateMakeupSessionRequest struct {
	OriginalSessionID uint      `json:"original_session_id" validate:"required"`
	NewSessionDate    time.Time `json:"new_session_date" validate:"required"`
	NewStartTime      string    `json:"new_start_time" validate:"required"`
	CancellingReason  string    `json:"cancelling_reason" validate:"required"`
	NewSessionStatus  string    `json:"new_session_status" validate:"required,oneof=cancelled rescheduled no-show"`
}

type ScheduleController struct{}

// CreateSchedule - สร้าง schedule ใหม่ (เฉพาะ admin และ owner)
func (sc *ScheduleController) CreateSchedule(c *fiber.Ctx) error {
	// ตรวจสอบ role
	userRole := c.Locals("role")
	if userRole != "admin" && userRole != "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admin and owner can create schedules"})
	}

	var req CreateScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// ตรวจสอบว่า assigned teacher มีอยู่จริง
	var assignedUser models.User
	if err := database.DB.Where("id = ? AND role IN ?", req.AssignedToUserID, []string{"teacher", "admin", "owner"}).First(&assignedUser).Error; err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Assigned user not found or not authorized to teach"})
	}

	// ตรวจสอบ room availability สำหรับ type = class
	if req.ScheduleType == "class" {
		conflictExists, err := services.CheckRoomConflict(req.RoomID, req.StartDate, req.EstimatedEndDate, req.SessionStartTime, req.HoursPerSession, req.RecurringPattern, req.CustomRecurringDays)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to check room conflict"})
		}
		if conflictExists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Room conflict detected for the specified time slots"})
		}
	}

	// เริ่ม transaction ก่อนใช้งาน
	tx := database.DB.Begin()

	// Ensure a primary User_inCourse exists for the assigned teacher/student list
	// Create or get user_in_course for the first provided user id
	var primaryUIC models.User_inCourse
	if len(req.UserInCourseIDs) == 0 {
		tx.Rollback()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_in_course_ids must not be empty"})
	}
	primaryUserID := req.UserInCourseIDs[0]
	if err := tx.Where("user_id = ? AND course_id = ?", primaryUserID, req.CourseID).First(&primaryUIC).Error; err != nil {
		// create it
		var primaryUser models.User
		if err := tx.First(&primaryUser, primaryUserID).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("User ID %d not found", primaryUserID)})
		}
		role := "student"
		if primaryUser.Role == "teacher" || primaryUser.Role == "admin" || primaryUser.Role == "owner" {
			role = "teacher"
		}
		primaryUIC = models.User_inCourse{UserID: primaryUserID, CourseID: req.CourseID, Role: role, Status: "enrolled"}
		if err := tx.Create(&primaryUIC).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create primary user_in_course"})
		}
	}

	// สร้าง schedule
	schedule := models.Schedules{
		CourseID:                req.CourseID,
		User_inCourseID:         primaryUIC.ID,
		AssignedToUserID:        req.AssignedToUserID,
		RoomID:                  req.RoomID,
		ScheduleName:            req.ScheduleName,
		ScheduleType:            req.ScheduleType,
		Recurring_pattern:       req.RecurringPattern,
		Total_hours:             req.TotalHours,
		Hours_per_session:       req.HoursPerSession,
		Session_per_week:        req.SessionPerWeek,
		Max_students:            req.MaxStudents,
		Current_students:        len(req.UserInCourseIDs),
		Start_date:              req.StartDate,
		Estimated_end_date:      req.EstimatedEndDate,
		Status:                  "assigned", // เริ่มต้นเป็น assigned
		Auto_Reschedule_holiday: req.AutoRescheduleHoliday,
		Notes:                   req.Notes,
		Admin_assigned:          assignedUser.Username,
	}

	// บันทึก schedule
	if err := tx.Create(&schedule).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create schedule"})
	}

	// Ensure User_inCourse records exist for all provided users
	for _, userID := range req.UserInCourseIDs {
		var user models.User
		if err := tx.First(&user, userID).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("User ID %d not found", userID)})
		}

		var uic models.User_inCourse
		if err := tx.Where("user_id = ? AND course_id = ?", userID, req.CourseID).First(&uic).Error; err != nil {
			role := "student"
			if user.Role == "teacher" || user.Role == "admin" || user.Role == "owner" {
				role = "teacher"
			}
			uic = models.User_inCourse{
				UserID:   userID,
				CourseID: req.CourseID,
				Role:     role,
				Status:   "enrolled",
			}
			if err := tx.Create(&uic).Error; err != nil {
				tx.Rollback()
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to add users to course"})
			}
		}
	}

	// สร้าง sessions ตาม recurring pattern
	sessions, err := services.GenerateScheduleSessions(schedule, req.SessionStartTime, req.CustomRecurringDays)
	if err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate sessions: " + err.Error()})
	}

	// ดึงวันหยุดถ้าเปิด auto reschedule
	var holidays []time.Time
	if req.AutoRescheduleHoliday {
		holidays, err = services.GetThaiHolidays(req.StartDate.Year(), req.EstimatedEndDate.Year())
		if err != nil {
			// ไม่ rollback เพราะไม่ใช่ error ร้ายแรง
			fmt.Printf("Warning: Failed to fetch holidays: %v\n", err)
		}
	}

	// Reschedule sessions ที่ตรงกับวันหยุด
	if len(holidays) > 0 {
		sessions = services.RescheduleSessions(sessions, holidays)
	}

	// บันทึก sessions
	for _, session := range sessions {
		session.ScheduleID = schedule.ID
		if err := tx.Create(&session).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create sessions"})
		}
	}

	// สร้าง notification สำหรับ assigned teacher
	notification := models.Notification{
		UserID:    req.AssignedToUserID,
		Title:     "New Schedule Assignment",
		TitleTh:   "การมอบหมายตารางเรียนใหม่",
		Message:   fmt.Sprintf("You have been assigned to schedule: %s. Please confirm to activate.", req.ScheduleName),
		MessageTh: fmt.Sprintf("คุณได้รับมอบหมายตารางเรียน: %s กรุณายืนยันเพื่อเปิดใช้งาน", req.ScheduleName),
		Type:      "info",
	}

	if err := tx.Create(&notification).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create notification"})
	}

	tx.Commit()

	// โหลดข้อมูลสมบูรณ์เพื่อส่งกลับ
	database.DB.Preload("Course").Preload("AssignedTo").Preload("Room").First(&schedule, schedule.ID)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":  "Schedule created successfully",
		"schedule": schedule,
	})
}

// ConfirmSchedule - ยืนยัน schedule (เฉพาะคนที่ถูก assign)
func (sc *ScheduleController) ConfirmSchedule(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	currentUserID := userID.(uint)

	scheduleID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid schedule ID"})
	}

	var schedule models.Schedules
	if err := database.DB.First(&schedule, scheduleID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
	}

	// ตรวจสอบว่าเป็นคนที่ถูก assign
	if schedule.AssignedToUserID != currentUserID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You are not assigned to this schedule"})
	}

	// ตรวจสอบสถานะปัจจุบัน
	if schedule.Status != "assigned" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schedule is not in assigned status"})
	}

	var req ConfirmScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// อัพเดทสถานะ
	if err := database.DB.Model(&schedule).Update("status", req.Status).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to confirm schedule"})
	}

	// สร้าง notification สำหรับ students ที่อยู่ใน course
	go services.NotifyStudentsScheduleConfirmed(schedule.ID)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":  "Schedule confirmed successfully",
		"schedule": schedule,
	})
}

// GetMySchedules - ดู schedule ของตัวเอง (teacher และ student)
func (sc *ScheduleController) GetMySchedules(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	currentUserID := userID.(uint)
	userRole := c.Locals("role")

	var schedules []models.Schedules

	if userRole == "teacher" || userRole == "admin" || userRole == "owner" {
		// ครูดู schedule ที่ตัวเองถูก assign
		err := database.DB.Where("assigned_to_user_id = ?", currentUserID).
			Preload("Course").
			Preload("Room").
			Find(&schedules).Error
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schedules"})
		}
	} else {
		// นักเรียนดู schedule ที่ตัวเองเข้าร่วม
		err := database.DB.Table("schedules").
			Joins("JOIN user_in_courses ON user_in_courses.course_id = schedules.course_id").
			Where("user_in_courses.user_id = ? AND schedules.status = ?", currentUserID, "scheduled").
			Preload("Course").
			Preload("Room").
			Find(&schedules).Error
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schedules"})
		}
	}

	// ปรับแต่งข้อมูลสำหรับ student (แสดงแค่ nickname, level, grade)
	if userRole == "student" {
		for i := range schedules {
			schedules[i].Course = models.Course{
				Name:  schedules[i].Course.Name,
				Level: schedules[i].Course.Level,
			}
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"schedules": schedules,
	})
}

// CreateMakeupSession - สร้าง makeup session (เฉพาะ class type)
func (sc *ScheduleController) CreateMakeupSession(c *fiber.Ctx) error {
	userRole := c.Locals("role")
	if userRole != "admin" && userRole != "owner" && userRole != "teacher" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admin, owner, and teacher can create makeup sessions"})
	}

	var req CreateMakeupSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// ดึง session เก่า
	var originalSession models.Schedule_Sessions
	if err := database.DB.First(&originalSession, req.OriginalSessionID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Original session not found"})
	}

	// ดึง schedule เพื่อตรวจสอบ type
	var schedule models.Schedules
	if err := database.DB.First(&schedule, originalSession.ScheduleID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
	}

	// ตรวจสอบว่าเป็น class type
	if schedule.ScheduleType != "class" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Makeup sessions can only be created for class type schedules"})
	}

	// ตรวจสอบสิทธิ์ (teacher ต้องเป็นคนที่ถูก assign)
	if userRole == "teacher" {
		userID := c.Locals("user_id")
		currentUserID := userID.(uint)
		if schedule.AssignedToUserID != currentUserID {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You are not assigned to this schedule"})
		}
	}

	tx := database.DB.Begin()

	// อัพเดท session เก่า
	if err := tx.Model(&originalSession).Updates(map[string]interface{}{
		"status":            req.NewSessionStatus,
		"cencelling_reason": req.CancellingReason,
	}).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update original session"})
	}

	// แปลง time string เป็น time.Time
	startTime, err := time.Parse("15:04", req.NewStartTime)
	if err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid start time format"})
	}

	// สร้าง makeup session ใหม่
	makeupSession := models.Schedule_Sessions{
		ScheduleID:            originalSession.ScheduleID,
		Session_date:          req.NewSessionDate,
		Start_time:            time.Date(req.NewSessionDate.Year(), req.NewSessionDate.Month(), req.NewSessionDate.Day(), startTime.Hour(), startTime.Minute(), 0, 0, req.NewSessionDate.Location()),
		End_time:              time.Date(req.NewSessionDate.Year(), req.NewSessionDate.Month(), req.NewSessionDate.Day(), startTime.Hour()+schedule.Hours_per_session, startTime.Minute(), 0, 0, req.NewSessionDate.Location()),
		Session_number:        originalSession.Session_number,
		Week_number:           originalSession.Week_number,
		Status:                "scheduled",
		Is_makeup:             true,
		Makeup_for_session_id: func(v uint) *uint { return &v }(originalSession.ID),
		Notes:                 fmt.Sprintf("Makeup session for cancelled session on %s", originalSession.Session_date.Format("2006-01-02")),
	}

	if err := tx.Create(&makeupSession).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create makeup session"})
	}

	tx.Commit()

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":        "Makeup session created successfully",
		"makeup_session": makeupSession,
	})
}

// GetScheduleSessions - ดู sessions ของ schedule
func (sc *ScheduleController) GetScheduleSessions(c *fiber.Ctx) error {
	scheduleID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid schedule ID"})
	}

	var sessions []models.Schedule_Sessions
	if err := database.DB.Where("schedule_id = ?", scheduleID).Order("session_date ASC").Find(&sessions).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch sessions"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"sessions": sessions,
	})
}

// GetTeachersSchedules - ดูตารางของ teacher ทั้งหมด ตามช่วงวันที่
// Query params:
//
//	date_filter: one of day, week, month, year
//	date: reference date (YYYY-MM-DD or RFC3339)
func (sc *ScheduleController) GetTeachersSchedules(c *fiber.Ctx) error {
	// Tolerate duplicate query keys; use the first non-empty
	dateFilter := c.Query("date_filter", "day")
	dateStr := c.Query("date")
	if dateStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "date query parameter is required (YYYY-MM-DD)"})
	}

	// Try parsing as YYYY-MM-DD first, then RFC3339
	var refDate time.Time
	var err error
	refDate, err = time.Parse("2006-01-02", dateStr)
	if err != nil {
		refDate, err = time.Parse(time.RFC3339, dateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid date format; use YYYY-MM-DD or RFC3339"})
		}
	}

	// Normalize to UTC for API consistency
	refDate = refDate.UTC()
	loc := time.UTC
	// Compute start (inclusive) and end (exclusive)
	var start time.Time
	var end time.Time
	switch dateFilter {
	case "day":
		start = time.Date(refDate.Year(), refDate.Month(), refDate.Day(), 0, 0, 0, 0, loc)
		end = start.AddDate(0, 0, 1)
	case "week":
		// week starting Monday
		wd := int(refDate.Weekday())
		// convert Sunday(0) to 7
		if wd == 0 {
			wd = 7
		}
		// offset to Monday
		offset := wd - 1
		start = time.Date(refDate.Year(), refDate.Month(), refDate.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -offset)
		end = start.AddDate(0, 0, 7)
	case "month":
		start = time.Date(refDate.Year(), refDate.Month(), 1, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 1, 0)
	case "year":
		start = time.Date(refDate.Year(), 1, 1, 0, 0, 0, 0, loc)
		end = start.AddDate(1, 0, 0)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid date_filter; allowed: day, week, month, year"})
	}

	// Query sessions in range
	var sessions []models.Schedule_Sessions
	if err := database.DB.Preload("Schedule").Preload("Schedule.AssignedTo").Preload("Schedule.Room").Where("session_date >= ? AND session_date < ?", start, end).Order("session_date ASC").Find(&sessions).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch sessions"})
	}

	// Fetch all teachers (users with role=teacher) to include even if no sessions
	var teachers []models.User
	if err := database.DB.Where("role = ?", "teacher").Preload("Teacher").Find(&teachers).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch teachers"})
	}

	// Group sessions by teacher (AssignedToUserID) and return a light DTO for sessions
	type sessionDTO struct {
		ScheduleID  uint      `json:"schedule_id"`
		SessionDate time.Time `json:"session_date"`
		StartTime   time.Time `json:"start_time"`
		EndTime     time.Time `json:"end_time"`
		Status      string    `json:"status"`
	}
	type teacherResp struct {
		TeacherID   uint         `json:"teacher_id"`
		TeacherName string       `json:"teacher_name"`
		Sessions    []sessionDTO `json:"sessions"`
	}

	grouped := make(map[uint]*teacherResp)
	// Initialize all teachers with empty sessions
	for _, t := range teachers {
		name := t.Username
		if t.Teacher != nil {
			// prefer nickname + last name TH/EN if available
			if t.Teacher.NicknameTh != "" || t.Teacher.NicknameEn != "" {
				if t.Teacher.NicknameTh != "" {
					name = t.Teacher.NicknameTh
				} else {
					name = t.Teacher.NicknameEn
				}
			}
		}
		grouped[t.ID] = &teacherResp{TeacherID: t.ID, TeacherName: name, Sessions: []sessionDTO{}}
	}

	// Assign sessions into groups; unassigned sessions go under teacher_id=0
	if _, ok := grouped[0]; !ok {
		grouped[0] = &teacherResp{TeacherID: 0, TeacherName: "unassigned", Sessions: []sessionDTO{}}
	}
	for _, s := range sessions {
		tID := s.Schedule.AssignedTo.ID
		if _, ok := grouped[tID]; !ok {
			// In case assigned user is not role 'teacher' but admin/owner; still include
			name := s.Schedule.AssignedTo.Username
			grouped[tID] = &teacherResp{TeacherID: tID, TeacherName: name, Sessions: []sessionDTO{}}
		}
		grouped[tID].Sessions = append(grouped[tID].Sessions, sessionDTO{
			ScheduleID:  s.ScheduleID,
			SessionDate: s.Session_date.UTC(),
			StartTime:   s.Start_time.UTC(),
			EndTime:     s.End_time.UTC(),
			Status:      s.Status,
		})
	}

	// Convert to slice and sort by teacher_name; unassigned (id=0) last
	resp := make([]teacherResp, 0, len(grouped))
	for _, v := range grouped {
		resp = append(resp, *v)
	}
	// simple sort
	sort.SliceStable(resp, func(i, j int) bool {
		if resp[i].TeacherID == 0 {
			return false
		}
		if resp[j].TeacherID == 0 {
			return true
		}
		return resp[i].TeacherName < resp[j].TeacherName
	})

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"teachers": resp, "start": start, "end": end})
}

// GetCalendarView - calendar data for sessions (and optional holidays/students)
// Query: view=day|week|month|year, date=YYYY-MM-DD or RFC3339, include_students=true|false, include_holidays=true|false
func (sc *ScheduleController) GetCalendarView(c *fiber.Ctx) error {
	view := c.Query("view", "week")
	dateStr := c.Query("date")
	if dateStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "date query parameter is required"})
	}
	includeStudents := c.Query("include_students") == "true"
	includeHolidays := c.Query("include_holidays") == "true"

	// Parse date
	refDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		refDate, err = time.Parse(time.RFC3339, dateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid date format; use YYYY-MM-DD or RFC3339"})
		}
	}
	loc := refDate.Location()

	// Compute range
	var start, end time.Time
	switch view {
	case "day":
		start = time.Date(refDate.Year(), refDate.Month(), refDate.Day(), 0, 0, 0, 0, loc)
		end = start.AddDate(0, 0, 1)
	case "week":
		wd := int(refDate.Weekday())
		if wd == 0 {
			wd = 7
		}
		offset := wd - 1
		start = time.Date(refDate.Year(), refDate.Month(), refDate.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -offset)
		end = start.AddDate(0, 0, 7)
	case "month":
		start = time.Date(refDate.Year(), refDate.Month(), 1, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 1, 0)
	case "year":
		start = time.Date(refDate.Year(), 1, 1, 0, 0, 0, 0, loc)
		end = start.AddDate(1, 0, 0)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid view; allowed: day, week, month, year"})
	}

	// Load sessions in range
	var sessions []models.Schedule_Sessions
	if err := database.DB.Preload("Schedule").
		Preload("Schedule.Course").
		Preload("Schedule.AssignedTo").
		Preload("Schedule.Room").
		Where("session_date >= ? AND session_date < ?", start, end).
		Order("session_date ASC").
		Find(&sessions).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch sessions"})
	}

	// Optional: gather students per course if requested
	courseIDsSet := map[uint]struct{}{}
	if includeStudents {
		for _, s := range sessions {
			courseIDsSet[s.Schedule.CourseID] = struct{}{}
		}
	}
	type enrolledStudent struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	studentsByCourse := map[uint][]enrolledStudent{}
	if includeStudents && len(courseIDsSet) > 0 {
		// Build slice of course IDs
		courseIDs := make([]uint, 0, len(courseIDsSet))
		for id := range courseIDsSet {
			courseIDs = append(courseIDs, id)
		}
		// Join user_in_courses with users
		type row struct {
			CourseID uint
			UserID   uint
			Username string
			Role     string
		}
		var rows []row
		if err := database.DB.Table("user_in_courses").
			Select("user_in_courses.course_id as course_id, users.id as user_id, users.username as username, users.role as role").
			Joins("JOIN users ON users.id = user_in_courses.user_id").
			Where("user_in_courses.course_id IN ? AND user_in_courses.role = ?", courseIDs, "student").
			Scan(&rows).Error; err == nil {
			for _, r := range rows {
				studentsByCourse[r.CourseID] = append(studentsByCourse[r.CourseID], enrolledStudent{ID: r.UserID, Username: r.Username, Role: r.Role})
			}
		}
	}

	// Build events
	type calendarEvent struct {
		Type        string            `json:"type"` // session or holiday
		ID          uint              `json:"id,omitempty"`
		ScheduleID  uint              `json:"schedule_id,omitempty"`
		CourseID    uint              `json:"course_id,omitempty"`
		CourseName  string            `json:"course_name,omitempty"`
		TeacherID   uint              `json:"teacher_id,omitempty"`
		TeacherName string            `json:"teacher_name,omitempty"`
		RoomID      uint              `json:"room_id,omitempty"`
		RoomName    string            `json:"room_name,omitempty"`
		Date        time.Time         `json:"date"`
		StartTime   time.Time         `json:"start_time,omitempty"`
		EndTime     time.Time         `json:"end_time,omitempty"`
		Status      string            `json:"status,omitempty"`
		Students    []enrolledStudent `json:"students,omitempty"`
		Title       string            `json:"title,omitempty"`
	}

	events := make([]calendarEvent, 0, len(sessions))
	for _, s := range sessions {
		tID := s.Schedule.AssignedTo.ID
		tName := s.Schedule.AssignedTo.Username
		roomName := s.Schedule.Room.RoomName
		courseName := s.Schedule.Course.Name
		ev := calendarEvent{
			Type:        "session",
			ID:          s.ID,
			ScheduleID:  s.ScheduleID,
			CourseID:    s.Schedule.CourseID,
			CourseName:  courseName,
			TeacherID:   tID,
			TeacherName: tName,
			RoomID:      s.Schedule.RoomID,
			RoomName:    roomName,
			Date:        s.Session_date,
			StartTime:   s.Start_time,
			EndTime:     s.End_time,
			Status:      s.Status,
			Title:       s.Schedule.ScheduleName,
		}
		if includeStudents {
			ev.Students = studentsByCourse[s.Schedule.CourseID]
		}
		events = append(events, ev)
	}

	// Optional: add holidays
	if includeHolidays {
		years := map[int]struct{}{start.Year(): {}, end.AddDate(0, 0, -1).Year(): {}}
		holidayDates := []time.Time{}
		for y := range years {
			h, err := services.GetThaiHolidays(y, y)
			if err == nil {
				holidayDates = append(holidayDates, h...)
			}
		}
		for _, d := range holidayDates {
			if !d.Before(start) && d.Before(end) {
				events = append(events, calendarEvent{Type: "holiday", Date: d, Title: "Holiday"})
			}
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"start":  start,
		"end":    end,
		"view":   view,
		"events": events,
	})
}

// AddComment - เพิ่ม comment ใน schedule หรือ session
func (sc *ScheduleController) AddComment(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	currentUserID := userID.(uint)

	var req struct {
		ScheduleID *uint  `json:"schedule_id"`
		SessionID  *uint  `json:"session_id"`
		Comment    string `json:"comment" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if req.ScheduleID == nil && req.SessionID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Either schedule_id or session_id must be provided"})
	}

	comment := models.Schedules_or_Sessions_Comment{
		UserID:  currentUserID,
		Comment: req.Comment,
	}

	if req.ScheduleID != nil {
		comment.ScheduleID = *req.ScheduleID
	}
	if req.SessionID != nil {
		comment.SessionID = *req.SessionID
	}

	if err := database.DB.Create(&comment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to add comment"})
	}

	// โหลดข้อมูลสมบูรณ์
	database.DB.Preload("User").First(&comment, comment.ID)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Comment added successfully",
		"comment": comment,
	})
}

// GetComments - ดู comments ของ schedule หรือ session
func (sc *ScheduleController) GetComments(c *fiber.Ctx) error {
	scheduleID := c.Query("schedule_id")
	sessionID := c.Query("session_id")

	if scheduleID == "" && sessionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Either schedule_id or session_id query parameter must be provided"})
	}

	var comments []models.Schedules_or_Sessions_Comment
	query := database.DB.Preload("User")

	if scheduleID != "" {
		query = query.Where("schedule_id = ?", scheduleID)
	}
	if sessionID != "" {
		query = query.Where("session_id = ?", sessionID)
	}

	if err := query.Order("created_at ASC").Find(&comments).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch comments"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"comments": comments,
	})
}

// GetSchedules - ดู schedules ทั้งหมด (สำหรับ admin และ owner)
func (sc *ScheduleController) GetSchedules(c *fiber.Ctx) error {
	userRole := c.Locals("role")
	if userRole != "admin" && userRole != "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admin and owner can view all schedules"})
	}

	var schedules []models.Schedules
	query := database.DB.Preload("Course").Preload("AssignedTo").Preload("Room")

	// Filter parameters
	status := c.Query("status")
	scheduleType := c.Query("type")
	branchID := c.Query("branch_id")

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if scheduleType != "" {
		query = query.Where("schedule_type = ?", scheduleType)
	}
	if branchID != "" {
		query = query.Joins("JOIN rooms ON rooms.id = schedules.room_id").
			Where("rooms.branch_id = ?", branchID)
	}

	if err := query.Find(&schedules).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schedules"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"schedules": schedules,
	})
}

// UpdateSessionStatus - อัพเดทสถานะของ session (สำหรับครู)
func (sc *ScheduleController) UpdateSessionStatus(c *fiber.Ctx) error {
	userRole := c.Locals("role")
	userID := c.Locals("user_id")
	currentUserID := userID.(uint)

	sessionID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid session ID"})
	}

	var req struct {
		Status string `json:"status" validate:"required,oneof=confirmed completed cancelled no-show"`
		Notes  string `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// ดึง session
	var session models.Schedule_Sessions
	if err := database.DB.First(&session, sessionID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
	}

	// ดึง schedule เพื่อตรวจสอบสิทธิ์
	var schedule models.Schedules
	if err := database.DB.First(&schedule, session.ScheduleID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
	}

	// ตรวจสอบสิทธิ์
	if userRole == "teacher" && schedule.AssignedToUserID != currentUserID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You are not assigned to this schedule"})
	}

	// อัพเดท session
	updates := map[string]interface{}{
		"status": req.Status,
	}
	if req.Notes != "" {
		updates["notes"] = req.Notes
	}

	if err := database.DB.Model(&session).Updates(updates).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update session status"})
	}

	// ถ้าเป็นการ confirm session ให้ส่ง notification ให้นักเรียน
	if req.Status == "confirmed" {
		go services.NotifyUpcomingClass(session.ID, 30) // แจ้งเตือนก่อน 30 นาที
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Session status updated successfully",
		"session": session,
	})
}
