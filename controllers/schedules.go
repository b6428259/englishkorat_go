package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/models"
	"englishkorat_go/services"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type CreateScheduleRequest struct {
	// Core schedule information
	ScheduleName string `json:"schedule_name" validate:"required"`
	ScheduleType string `json:"schedule_type" validate:"required,oneof=class meeting event holiday appointment"`

	// For class schedules
	GroupID *uint `json:"group_id"` // Required for class schedules

	// For event/appointment schedules
	ParticipantUserIDs []uint `json:"participant_user_ids"` // User IDs for events/appointments

	// Schedule timing
	RecurringPattern string    `json:"recurring_pattern" validate:"required,oneof=daily weekly bi-weekly monthly yearly custom"`
	TotalHours       int       `json:"total_hours" validate:"required,min=1"`
	HoursPerSession  int       `json:"hours_per_session" validate:"required,min=1"`
	SessionPerWeek   int       `json:"session_per_week" validate:"required,min=1"`
	StartDate        time.Time `json:"start_date" validate:"required"`
	EstimatedEndDate time.Time `json:"estimated_end_date" validate:"required"`

	// Default assignments
	DefaultTeacherID *uint `json:"default_teacher_id"`
	DefaultRoomID    *uint `json:"default_room_id"`

	// Settings
	AutoRescheduleHoliday bool   `json:"auto_reschedule"`
	Notes                 string `json:"notes"`
	SessionStartTime      string `json:"session_start_time" validate:"required"` // เวลาเริ่มต้นของแต่ละ session เช่น "09:00"
	CustomRecurringDays   []int  `json:"custom_recurring_days,omitempty"`        // สำหรับ custom pattern [0=วันอาทิตย์, 1=วันจันทร์, ...]
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

	userID := c.Locals("user_id").(uint)

	var req CreateScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Validate schedule type specific requirements
	if req.ScheduleType == "class" {
		if req.GroupID == nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "group_id is required for class schedules"})
		}

		// Validate that the group exists and has members with proper payment status
		var group models.Group
		if err := database.DB.Preload("Members").First(&group, *req.GroupID).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Group not found"})
		}

		// Check if group has members with appropriate payment status
		hasEligibleMembers := false
		for _, member := range group.Members {
			if member.PaymentStatus == "deposit_paid" || member.PaymentStatus == "fully_paid" {
				hasEligibleMembers = true
				break
			}
		}

		if !hasEligibleMembers {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Group must have at least one member with deposit paid or fully paid status"})
		}
	} else {
		// For event/appointment schedules
		if len(req.ParticipantUserIDs) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "participant_user_ids is required for event/appointment schedules"})
		}
	}

	// ตรวจสอบว่า default teacher มีอยู่จริง (ถ้าระบุ)
	if req.DefaultTeacherID != nil {
		var teacher models.User
		if err := database.DB.Where("id = ? AND role IN ?", *req.DefaultTeacherID, []string{"teacher", "admin", "owner"}).First(&teacher).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Default teacher not found or not authorized to teach"})
		}
	}

	// เริ่ม transaction
	tx := database.DB.Begin()

	// Get admin user for assignment tracking
	var assignedUser models.User
	if err := tx.First(&assignedUser, userID).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get user info"})
	}

	// สร้าง schedule
	schedule := models.Schedules{
		ScheduleName:            req.ScheduleName,
		ScheduleType:            req.ScheduleType,
		GroupID:                 req.GroupID,
		CreatedByUserID:         &userID,
		Recurring_pattern:       req.RecurringPattern,
		Total_hours:             req.TotalHours,
		Hours_per_session:       req.HoursPerSession,
		Session_per_week:        req.SessionPerWeek,
		Start_date:              req.StartDate,
		Estimated_end_date:      req.EstimatedEndDate,
		DefaultTeacherID:        req.DefaultTeacherID,
		DefaultRoomID:           req.DefaultRoomID,
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

	// For event/appointment schedules - create participant records
	if req.ScheduleType != "class" && len(req.ParticipantUserIDs) > 0 {
		for _, participantID := range req.ParticipantUserIDs {
			var user models.User
			if err := tx.First(&user, participantID).Error; err != nil {
				tx.Rollback()
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("Participant user ID %d not found", participantID)})
			}

			participant := models.ScheduleParticipant{
				ScheduleID: schedule.ID,
				UserID:     participantID,
				Role:       "participant",
				Status:     "invited",
			}

			// Set organizer role for the creator
			if participantID == userID {
				participant.Role = "organizer"
			}

			if err := tx.Create(&participant).Error; err != nil {
				tx.Rollback()
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to add participants to schedule"})
			}
		}
	}

	// สร้าง sessions ตาม recurring pattern
	sessions, err := services.GenerateScheduleSessions(schedule, req.SessionStartTime, req.CustomRecurringDays)
	if err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate sessions: " + err.Error()})
	}

	// บันทึก sessions
	for _, session := range sessions {
		session.ScheduleID = schedule.ID
		// Use default teacher and room if provided
		session.AssignedTeacherID = req.DefaultTeacherID
		session.RoomID = req.DefaultRoomID

		if err := tx.Create(&session).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create sessions"})
		}
	}

	// สร้าง notification สำหรับ assigned users
	var notificationUserIDs []uint

	if req.ScheduleType == "class" && req.DefaultTeacherID != nil {
		// For class schedules - notify the default teacher
		notificationUserIDs = append(notificationUserIDs, *req.DefaultTeacherID)
	} else if req.ScheduleType != "class" {
		// For event/appointment schedules - notify all participants
		notificationUserIDs = append(notificationUserIDs, req.ParticipantUserIDs...)
	}

	for _, notifUserID := range notificationUserIDs {
		notification := models.Notification{
			UserID:    notifUserID,
			Title:     "New Schedule Assignment",
			TitleTh:   "การมอบหมายตารางใหม่",
			Message:   fmt.Sprintf("You have been assigned to schedule: %s. Please confirm your sessions.", req.ScheduleName),
			MessageTh: fmt.Sprintf("คุณได้รับมอบหมายตาราง: %s กรุณายืนยัน sessions ของคุณ", req.ScheduleName),
			Type:      "info",
		}

		if err := tx.Create(&notification).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create notification"})
		}
	}

	tx.Commit()

	// โหลดข้อมูลสมบูรณ์เพื่อส่งกลับ
	database.DB.Preload("Group.Course").Preload("DefaultTeacher").Preload("DefaultRoom").Preload("CreatedBy").First(&schedule, schedule.ID)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":  "Schedule created successfully",
		"schedule": schedule,
	})
}

// GetSchedules - ดู schedule ทั้งหมด (เฉพาะ admin และ owner)
func (sc *ScheduleController) GetSchedules(c *fiber.Ctx) error {
	userRole := c.Locals("role")
	if userRole != "admin" && userRole != "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admin and owner can view all schedules"})
	}

	var schedules []models.Schedules
	if err := database.DB.Preload("Group.Course").Preload("DefaultTeacher").Preload("DefaultRoom").Find(&schedules).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schedules"})
	}

	return c.JSON(fiber.Map{
		"schedules": schedules,
	})
}

// GetMySchedules - ดู schedule ของตัวเอง
func (sc *ScheduleController) GetMySchedules(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	userRole := c.Locals("role").(string)

	var schedules []models.Schedules

	if userRole == "teacher" || userRole == "admin" || userRole == "owner" {
		// ครูดู schedule ที่ตัวเองถูก assign (ทั้ง default teacher และ session teacher)
		query := database.DB.Preload("Group.Course").Preload("DefaultRoom").Preload("DefaultTeacher")

		if userRole == "teacher" {
			query = query.Where("default_teacher_id = ? OR id IN (SELECT DISTINCT schedule_id FROM schedule_sessions WHERE assigned_teacher_id = ?)", userID, userID)
		}

		err := query.Find(&schedules).Error
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schedules"})
		}
	} else {
		// นักเรียนดู schedule ที่ตัวเองเข้าร่วม (จาก group members)
		err := database.DB.Table("schedules").
			Joins("JOIN groups ON groups.id = schedules.group_id").
			Joins("JOIN group_members ON group_members.group_id = groups.id").
			Joins("JOIN students ON students.id = group_members.student_id").
			Where("students.user_id = ? AND schedules.status = ?", userID, "scheduled").
			Preload("Group.Course").
			Preload("DefaultRoom").
			Preload("DefaultTeacher").
			Find(&schedules).Error
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch schedules"})
		}
	}

	// ปรับแต่งข้อมูลสำหรับ student (แสดงแค่ข้อมูลพื้นฐาน)
	if userRole == "student" {
		for i := range schedules {
			if schedules[i].Group != nil && schedules[i].Group.Course.Name != "" {
				schedules[i].Group.Course = models.Course{
					Name:  schedules[i].Group.Course.Name,
					Level: schedules[i].Group.Course.Level,
				}
			}
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"schedules": schedules,
	})
}

// GetTeachersSchedules - Enhanced implementation for Group-based model
func (sc *ScheduleController) GetTeachersSchedules(c *fiber.Ctx) error {
	// ----- Parse query parameters -----
	dateFilter := c.Query("date_filter") // "day" | "week" | ""
	dateStr := c.Query("date")           // e.g. "2025-09-15"
	startDate := c.Query("start_date")   // optional explicit range
	endDate := c.Query("end_date")
	branchID := c.Query("branch_id")

	// Timezone: Asia/Bangkok
	loc, _ := time.LoadLocation("Asia/Bangkok")
	const dLayout = "2006-01-02"

	// ----- Resolve date range -----
	var start time.Time
	var end time.Time
	var err error

	switch {
	// If explicit range provided: use it (and still turn into half-open [start, endNext))
	case startDate != "" && endDate != "":
		start, err = time.ParseInLocation(dLayout, startDate, loc)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid start_date"})
		}
		end, err = time.ParseInLocation(dLayout, endDate, loc)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid end_date"})
		}
		// make end exclusive by adding 1 day
		end = end.AddDate(0, 0, 1)

	// If date_filter=day with a single date
	case dateFilter == "day" && dateStr != "":
		day, err := time.ParseInLocation(dLayout, dateStr, loc)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid date"})
		}
		start = day
		end = day.AddDate(0, 0, 1) // next day (exclusive)

	// If date_filter=week with a single date (use ISO week: Monday start)
	case dateFilter == "week" && dateStr != "":
		day, err := time.ParseInLocation(dLayout, dateStr, loc)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid date"})
		}
		// Start of week (Monday)
		weekday := int(day.Weekday())
		// In Go, Sunday=0 ... Saturday=6; want Monday=0
		// shift so that Mon->0, Tue->1, ..., Sun->6
		shift := (weekday + 6) % 7
		start = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -shift)
		end = start.AddDate(0, 0, 7) // next Monday (exclusive)

	// Default: today (day filter)
	default:
		today := time.Now().In(loc).Truncate(24 * time.Hour)
		start = today
		end = today.AddDate(0, 0, 1)
	}

	// Normalize to ISO strings with time
	startISO := start.Format("2006-01-02 15:04:05")
	endISO := end.Format("2006-01-02 15:04:05")

	// ----- Query sessions with half-open range -----
	var sessions []models.Schedule_Sessions
	sessionsQuery := database.DB.Model(&models.Schedule_Sessions{}).
		Where("session_date >= ? AND session_date < ?", startISO, endISO).
		Where("status NOT IN ?", []string{"cancelled", "no-show"}).
		Where("schedule_sessions.deleted_at IS NULL").
		Preload("Schedule").
		Preload("AssignedTeacher").
		Preload("Schedule.DefaultTeacher").
		Preload("Room").
		Preload("Schedule.DefaultRoom").
		Order("session_date ASC, start_time ASC")

	if branchID != "" {
		// join to schedules/groups/courses to filter by branch
		sessionsQuery = sessionsQuery.
			Joins("JOIN schedules ON schedule_sessions.schedule_id = schedules.id").
			Joins("LEFT JOIN groups ON schedules.group_id = groups.id").
			Joins("LEFT JOIN courses ON groups.course_id = courses.id").
			Where("courses.branch_id = ?", branchID)
	}

	if err := sessionsQuery.Find(&sessions).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch sessions"})
	}

	// ----- Map teacher (by user ID) -> sessions -----
	// Build a map of serialized sessions keyed by teacher's user ID.
	// Use preloaded AssignedTeacher and Schedule.DefaultTeacher objects to determine the teacher's user ID
	// and serialize sessions into simple maps to avoid deep nested GORM structs which may not serialize as expected.
	teacherSessions := make(map[uint][]map[string]interface{})
	// cache to resolve legacy assigned_teacher_id that may reference Teacher.ID instead of User.ID
	legacyTeacherToUser := make(map[uint]uint)
	// Track sessions with no teacher assigned
	unassignedSessions := make([]map[string]interface{}, 0)
	for _, s := range sessions {
		// determine teacher's user id
		var teacherUserID uint
		if s.AssignedTeacher != nil && s.AssignedTeacher.ID != 0 {
			teacherUserID = s.AssignedTeacher.ID
		} else if s.AssignedTeacher == nil && s.AssignedTeacherID != nil {
			// Fallback: assigned_teacher_id might be a Teacher.ID; try to resolve to UserID
			if mappedUID, ok := legacyTeacherToUser[*s.AssignedTeacherID]; ok {
				teacherUserID = mappedUID
			} else {
				var t models.Teacher
				if err := database.DB.Select("id,user_id").First(&t, *s.AssignedTeacherID).Error; err == nil && t.UserID != 0 {
					legacyTeacherToUser[*s.AssignedTeacherID] = t.UserID
					teacherUserID = t.UserID
				}
			}
		} else if s.Schedule != nil && s.Schedule.DefaultTeacher != nil && s.Schedule.DefaultTeacher.ID != 0 {
			teacherUserID = s.Schedule.DefaultTeacher.ID
		} else {
			// no teacher info available for this session - add to unassigned
			// safe time formatting
			formatTime := func(t *time.Time, layout string) string {
				if t == nil {
					return ""
				}
				return t.In(time.FixedZone("Asia/Bangkok", 7*3600)).Format(layout)
			}

			sessMap := map[string]interface{}{
				"id":             s.ID,
				"schedule_id":    s.ScheduleID,
				"date":           formatTime(s.Session_date, "2006-01-02"),
				"start_time":     formatTime(s.Start_time, "15:04"),
				"end_time":       formatTime(s.End_time, "15:04"),
				"status":         s.Status,
				"session_number": s.Session_number,
				"week_number":    s.Week_number,
				"is_makeup":      s.Is_makeup,
				"notes":          s.Notes,
			}

			if s.Room != nil {
				sessMap["room"] = map[string]interface{}{"id": s.Room.ID, "name": s.Room.RoomName}
			} else if s.Schedule != nil && s.Schedule.DefaultRoom != nil {
				// Fallback to schedule's default room if session doesn't have room assigned
				sessMap["room"] = map[string]interface{}{"id": s.Schedule.DefaultRoom.ID, "name": s.Schedule.DefaultRoom.RoomName}
			} else {
				sessMap["room"] = map[string]interface{}{"id": nil, "name": ""}
			}

			unassignedSessions = append(unassignedSessions, sessMap)
			continue
		}

		// safe time formatting
		formatTime := func(t *time.Time, layout string) string {
			if t == nil {
				return ""
			}
			return t.In(time.FixedZone("Asia/Bangkok", 7*3600)).Format(layout)
		}

		sessMap := map[string]interface{}{
			"id":             s.ID,
			"schedule_id":    s.ScheduleID,
			"date":           formatTime(s.Session_date, "2006-01-02"),
			"start_time":     formatTime(s.Start_time, "15:04"),
			"end_time":       formatTime(s.End_time, "15:04"),
			"status":         s.Status,
			"session_number": s.Session_number,
			"week_number":    s.Week_number,
			"is_makeup":      s.Is_makeup,
			"notes":          s.Notes,
		}

		if s.Room != nil {
			sessMap["room"] = map[string]interface{}{"id": s.Room.ID, "name": s.Room.RoomName}
		} else if s.Schedule != nil && s.Schedule.DefaultRoom != nil {
			// Fallback to schedule's default room if session doesn't have room assigned
			sessMap["room"] = map[string]interface{}{"id": s.Schedule.DefaultRoom.ID, "name": s.Schedule.DefaultRoom.RoomName}
		} else {
			sessMap["room"] = map[string]interface{}{"id": nil, "name": ""}
		}

		teacherSessions[teacherUserID] = append(teacherSessions[teacherUserID], sessMap)
	}

	// Debug endpoint: expose diagnostics when debug=true
	if c.Query("debug") == "true" {
		sessInfos := make([]map[string]interface{}, 0, len(sessions))
		for _, s := range sessions {
			assignedID := uint(0)
			if s.AssignedTeacherID != nil {
				assignedID = *s.AssignedTeacherID
			}
			preloadedAssigned := uint(0)
			if s.AssignedTeacher != nil {
				preloadedAssigned = s.AssignedTeacher.ID
			}
			defaultTeacher := uint(0)
			if s.Schedule != nil && s.Schedule.DefaultTeacher != nil {
				defaultTeacher = s.Schedule.DefaultTeacher.ID
			}
			sessInfos = append(sessInfos, map[string]interface{}{
				"id":                        s.ID,
				"schedule_id":               s.ScheduleID,
				"session_date":              s.Session_date,
				"assigned_teacher_id_field": assignedID,
				"assigned_teacher_preload":  preloadedAssigned,
				"schedule_default_teacher":  defaultTeacher,
			})
		}

		// teacherSessions keys
		keys := make([]uint, 0, len(teacherSessions))
		for k := range teacherSessions {
			keys = append(keys, k)
		}

		return c.JSON(fiber.Map{
			"success":            true,
			"sessions_count":     len(sessions),
			"sessions":           sessInfos,
			"teacherSessionsKey": keys,
		})
	}

	// ----- Fetch all active teachers (respect branch filter if provided) -----
	var teachers []models.Teacher
	tQuery := database.DB.Model(&models.Teacher{}).
		Where("active = ?", true).
		Preload("User").
		Preload("Branch")

	if branchID != "" {
		tQuery = tQuery.Where("branch_id = ?", branchID)
	}

	if err := tQuery.Find(&teachers).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch teachers"})
	}

	// ----- Build response for known teachers -----
	resp := make([]map[string]interface{}, 0, len(teachers))
	// Track which user IDs have entries so we can backfill any orphan user IDs from sessions
	includedUserIDs := make(map[uint]bool, len(teachers))
	for _, t := range teachers {
		// fallback branch from user
		if t.BranchID == 0 && t.User.ID != 0 && t.User.Branch.ID != 0 {
			t.Branch = t.User.Branch
		}

		// pick up serialized sessions for this teacher (keyed by user's ID)
		sess := teacherSessions[t.UserID]
		if sess == nil {
			sess = make([]map[string]interface{}, 0)
		}

		item := map[string]interface{}{
			"id":      t.ID,
			"user_id": t.UserID,
			"name": map[string]interface{}{
				"first_en":    t.FirstNameEn,
				"last_en":     t.LastNameEn,
				"first_th":    t.FirstNameTh,
				"last_th":     t.LastNameTh,
				"nickname_en": t.NicknameEn,
				"nickname_th": t.NicknameTh,
			},
			"user": map[string]interface{}{
				"id":       t.User.ID,
				"username": t.User.Username,
				"avatar":   t.User.Avatar,
			},
			"branch":   map[string]interface{}{},
			"sessions": sess,
		}

		if t.Branch.ID != 0 {
			item["branch"] = map[string]interface{}{
				"id":      t.Branch.ID,
				"name_en": t.Branch.NameEn,
				"name_th": t.Branch.NameTh,
				"code":    t.Branch.Code,
			}
		}

		resp = append(resp, item)
		includedUserIDs[t.UserID] = true
	}

	// ----- Backfill: include any users referenced by sessions but missing in teachers list -----
	// Collect missing user IDs from teacherSessions
	missingUserIDs := make([]uint, 0)
	for uid := range teacherSessions {
		if !includedUserIDs[uid] {
			missingUserIDs = append(missingUserIDs, uid)
		}
	}

	if len(missingUserIDs) > 0 {
		var extraUsers []models.User
		uQuery := database.DB.Model(&models.User{}).
			Where("id IN ?", missingUserIDs)
		if branchID != "" {
			uQuery = uQuery.Where("branch_id = ?", branchID)
		}
		if err := uQuery.Preload("Teacher").Preload("Branch").Find(&extraUsers).Error; err == nil {
			for _, u := range extraUsers {
				// Name fallback: prefer Teacher profile fields if available
				name := map[string]interface{}{
					"first_en":    "",
					"last_en":     "",
					"first_th":    "",
					"last_th":     "",
					"nickname_en": u.Username,
					"nickname_th": "",
				}
				if u.Teacher != nil {
					if u.Teacher.FirstNameEn != "" || u.Teacher.NicknameEn != "" {
						name["first_en"] = u.Teacher.FirstNameEn
						name["last_en"] = u.Teacher.LastNameEn
						name["first_th"] = u.Teacher.FirstNameTh
						name["last_th"] = u.Teacher.LastNameTh
						name["nickname_en"] = u.Teacher.NicknameEn
						name["nickname_th"] = u.Teacher.NicknameTh
					}
				}

				// Default id to 0; replace with Teacher ID if exists
				teacherID := uint(0)
				if u.Teacher != nil {
					teacherID = u.Teacher.ID
				}

				item := map[string]interface{}{
					"id":      teacherID,
					"user_id": u.ID,
					"name":    name,
					"user": map[string]interface{}{
						"id":       u.ID,
						"username": u.Username,
						"avatar":   u.Avatar,
					},
					"branch":   map[string]interface{}{},
					"sessions": teacherSessions[u.ID],
				}

				if u.Branch.ID != 0 {
					item["branch"] = map[string]interface{}{
						"id":      u.Branch.ID,
						"name_en": u.Branch.NameEn,
						"name_th": u.Branch.NameTh,
						"code":    u.Branch.Code,
					}
				}

				resp = append(resp, item)
			}
		}
	}

	// ----- Add unassigned sessions under a special "unassigned" teacher entry -----
	if len(unassignedSessions) > 0 {
		unassignedItem := map[string]interface{}{
			"id":      0,
			"user_id": 0,
			"name": map[string]interface{}{
				"first_en":    "",
				"last_en":     "",
				"first_th":    "",
				"last_th":     "",
				"nickname_en": "Unassigned",
				"nickname_th": "",
			},
			"user": map[string]interface{}{
				"id":       0,
				"username": "unassigned",
				"avatar":   "",
			},
			"branch":   map[string]interface{}{},
			"sessions": unassignedSessions,
		}

		resp = append(resp, unassignedItem)
	}

	// expose filters back (use human-friendly dates)
	outStart := start.Format(dLayout)
	// end is exclusive; show end-1day for clarity if start and end differ more than 1 day
	outEnd := end.AddDate(0, 0, -1).Format(dLayout)

	return c.JSON(fiber.Map{
		"success": true,
		"data":    resp,
		"total":   len(resp),
		"filters": map[string]interface{}{
			"date_filter": dateFilter,
			"start_date":  outStart,
			"end_date":    outEnd,
			"branch_id":   branchID,
			"timezone":    "Asia/Bangkok",
		},
	})
}

// GetCalendarView - Enhanced implementation for Group-based model
func (sc *ScheduleController) GetCalendarView(c *fiber.Ctx) error {
	// Parse query parameters
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	viewType := c.Query("view", "month") // month, week, day
	userRole := c.Locals("role").(string)
	userID := c.Locals("user_id").(uint)
	branchID := c.Query("branch_id")
	includeHolidays := c.Query("include_holidays", "false") == "true"

	// Default date range if not provided
	if startDate == "" || endDate == "" {
		now := time.Now()
		switch viewType {
		case "week":
			startDate = now.AddDate(0, 0, -int(now.Weekday())).Format("2006-01-02")
			endDate = now.AddDate(0, 0, 6-int(now.Weekday())).Format("2006-01-02")
		case "day":
			startDate = now.Format("2006-01-02")
			endDate = now.Format("2006-01-02")
		default: // month
			startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
			endDate = time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		}
	}

	// Build query for sessions
	query := database.DB.Model(&models.Schedule_Sessions{}).
		Where("session_date BETWEEN ? AND ?", startDate, endDate).
		Where("status NOT IN ?", []string{"cancelled", "no-show"})

	// Apply role-based filtering
	if userRole == "teacher" {
		// Teachers see only their assigned sessions
		query = query.Joins("JOIN schedules ON schedule_sessions.schedule_id = schedules.id").
			Where("schedules.default_teacher_id = ? OR schedule_sessions.assigned_teacher_id = ?", userID, userID)
	} else if userRole == "student" {
		// Students see only their group sessions
		query = query.Joins("JOIN schedules ON schedule_sessions.schedule_id = schedules.id").
			Joins("JOIN groups ON schedules.group_id = groups.id").
			Joins("JOIN group_members ON group_members.group_id = groups.id").
			Joins("JOIN students ON students.id = group_members.student_id").
			Where("students.user_id = ?", userID)
	}

	// Admin/Owner can filter by branch
	if (userRole == "admin" || userRole == "owner") && branchID != "" {
		query = query.Joins("JOIN schedules ON schedule_sessions.schedule_id = schedules.id").
			Joins("LEFT JOIN groups ON schedules.group_id = groups.id").
			Joins("LEFT JOIN courses ON groups.course_id = courses.id").
			Where("courses.branch_id = ?", branchID)
	}

	// Get sessions with relationships
	var sessions []models.Schedule_Sessions
	if err := query.Preload("Schedule.Group.Course").
		Preload("Schedule.Group.Members.Student").
		Preload("AssignedTeacher").
		Preload("Schedule.DefaultTeacher").
		Preload("Room").
		Order("session_date ASC, start_time ASC").
		Find(&sessions).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch calendar sessions"})
	}

	// Build calendar events
	events := make([]map[string]interface{}, 0, len(sessions))
	for _, session := range sessions {
		// Determine teacher for the session
		var teacher *models.User
		if session.AssignedTeacher != nil {
			teacher = session.AssignedTeacher
		} else if session.Schedule.DefaultTeacher != nil {
			teacher = session.Schedule.DefaultTeacher
		}

		// Build participants list
		participants := make([]map[string]interface{}, 0)
		if session.Schedule.Group != nil {
			for _, member := range session.Schedule.Group.Members {
				participants = append(participants, map[string]interface{}{
					"id":             member.Student.ID,
					"name":           fmt.Sprintf("%s %s", member.Student.FirstName, member.Student.LastName),
					"nickname":       member.Student.NicknameEn,
					"payment_status": member.PaymentStatus,
				})
			}
		}

		// Safe format helpers for nullable times
		formatDate := func(t *time.Time, layout string) string {
			if t == nil {
				return ""
			}
			return t.Format(layout)
		}

		event := map[string]interface{}{
			"id":             session.ID,
			"schedule_id":    session.ScheduleID,
			"title":          session.Schedule.ScheduleName,
			"date":           formatDate(session.Session_date, "2006-01-02"),
			"start_time":     formatDate(session.Start_time, "15:04"),
			"end_time":       formatDate(session.End_time, "15:04"),
			"status":         session.Status,
			"session_number": session.Session_number,
			"week_number":    session.Week_number,
			"is_makeup":      session.Is_makeup,
			"type":           "class",
			"teacher": map[string]interface{}{
				"id":       nil,
				"name":     "",
				"username": "",
			},
			"room": map[string]interface{}{
				"id":   nil,
				"name": "",
			},
			"course": map[string]interface{}{
				"name":  "",
				"level": "",
			},
			"participants": participants,
		}

		// Add teacher info
		if teacher != nil {
			event["teacher"] = map[string]interface{}{
				"id":       teacher.ID,
				"name":     teacher.Username,
				"username": teacher.Username,
			}
		}

		// Add room info
		if session.Room != nil {
			event["room"] = map[string]interface{}{
				"id":   session.Room.ID,
				"name": session.Room.RoomName,
			}
		}

		// Add course info
		if session.Schedule.Group != nil && session.Schedule.Group.Course.Name != "" {
			event["course"] = map[string]interface{}{
				"name":  session.Schedule.Group.Course.Name,
				"level": session.Schedule.Group.Course.Level,
			}
		}

		events = append(events, event)
	}

	// Include holidays if requested
	var holidays []map[string]interface{}
	if includeHolidays {
		// Parse start and end dates
		startDateParsed, _ := time.Parse("2006-01-02", startDate)
		endDateParsed, _ := time.Parse("2006-01-02", endDate)

		// Get Thai holidays for the date range
		holidayDates, err := services.GetThaiHolidays(startDateParsed.Year(), endDateParsed.Year())
		if err == nil {
			for _, holiday := range holidayDates {
				if holiday.After(startDateParsed.AddDate(0, 0, -1)) && holiday.Before(endDateParsed.AddDate(0, 0, 1)) {
					holidays = append(holidays, map[string]interface{}{
						"date":  holiday.Format("2006-01-02"),
						"title": "Thai Holiday",
						"type":  "holiday",
					})
				}
			}
		}
	}

	// Build response
	response := map[string]interface{}{
		"events":    events,
		"holidays":  holidays,
		"view_type": viewType,
		"date_range": map[string]interface{}{
			"start": startDate,
			"end":   endDate,
		},
		"total_events": len(events),
		"user_context": map[string]interface{}{
			"role":      userRole,
			"user_id":   userID,
			"branch_id": branchID,
		},
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    response,
	})
}

// GetScheduleSessions - ดู sessions ของ schedule
func (sc *ScheduleController) GetScheduleSessions(c *fiber.Ctx) error {
	scheduleID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid schedule ID"})
	}

	var sessions []models.Schedule_Sessions
	if err := database.DB.Where("schedule_id = ?", scheduleID).
		Preload("AssignedTeacher").
		Preload("Room").
		Order("session_date ASC, start_time ASC").
		Find(&sessions).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch sessions"})
	}

	return c.JSON(fiber.Map{
		"sessions": sessions,
	})
}

// UpdateSessionStatus - อัพเดทสถานะ session
func (sc *ScheduleController) UpdateSessionStatus(c *fiber.Ctx) error {
	sessionID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid session ID"})
	}

	var req struct {
		Status string `json:"status" validate:"required,oneof=scheduled confirmed pending completed cancelled rescheduled no-show"`
		Notes  string `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Find the session
	var session models.Schedule_Sessions
	if err := database.DB.Preload("Schedule").First(&session, sessionID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
	}

	userID := c.Locals("user_id").(uint)
	userRole := c.Locals("role").(string)

	// ตรวจสอบสิทธิ์ (teacher ต้องเป็นคนที่ถูก assign หรือเป็น participant)
	if userRole == "teacher" {
		// ตรวจสอบสิทธิ์สำหรับ teacher
		hasPermission := false

		// สำหรับ class schedules - ตรวจสอบว่าเป็น default teacher หรือ assigned teacher
		if session.Schedule.ScheduleType == "class" {
			if (session.Schedule.DefaultTeacherID != nil && *session.Schedule.DefaultTeacherID == userID) ||
				(session.AssignedTeacherID != nil && *session.AssignedTeacherID == userID) {
				hasPermission = true
			}
		} else if session.Schedule.ScheduleType != "class" {
			// สำหรับ event/appointment - ตรวจสอบว่าเป็น participant
			var participant models.ScheduleParticipant
			if err := database.DB.Where("schedule_id = ? AND user_id = ?", session.Schedule.ID, userID).First(&participant).Error; err == nil {
				hasPermission = true
			}
		}

		if !hasPermission {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You are not assigned to this schedule"})
		}
	}

	// Update session status
	updates := map[string]interface{}{
		"status": req.Status,
		"notes":  req.Notes,
	}

	if req.Status == "confirmed" {
		now := time.Now()
		updates["confirmed_at"] = &now
		updates["confirmed_by_user_id"] = userID
	}

	if err := database.DB.Model(&session).Updates(updates).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update session status"})
	}

	return c.JSON(fiber.Map{
		"message": "Session status updated successfully",
	})
}

// ConfirmSession - Confirm a single session (assigned teacher, participant, admin/owner)
func (sc *ScheduleController) ConfirmSession(c *fiber.Ctx) error {
	sessionID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid session ID"})
	}

	var session models.Schedule_Sessions
	if err := database.DB.Preload("Schedule").First(&session, sessionID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
	}

	userID := c.Locals("user_id").(uint)
	userRole := c.Locals("role").(string)

	// Permission check: admin/owner can confirm any session
	canConfirm := false
	if userRole == "admin" || userRole == "owner" {
		canConfirm = true
	} else {
		// Assigned teacher for the session
		if session.AssignedTeacherID != nil && *session.AssignedTeacherID == userID {
			canConfirm = true
		}
		// For class schedules, default teacher on schedule can confirm
		if !canConfirm && session.Schedule != nil && session.Schedule.DefaultTeacherID != nil && *session.Schedule.DefaultTeacherID == userID {
			canConfirm = true
		}
		// Participant for non-class schedules
		if !canConfirm && session.Schedule != nil && session.Schedule.ScheduleType != "class" {
			var participant models.ScheduleParticipant
			if err := database.DB.Where("schedule_id = ? AND user_id = ?", session.ScheduleID, userID).First(&participant).Error; err == nil {
				canConfirm = true
			}
		}
	}

	if !canConfirm {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You are not authorized to confirm this session"})
	}

	// Update confirmation fields
	now := time.Now()
	updates := map[string]interface{}{
		"status":               "confirmed",
		"confirmed_at":         &now,
		"confirmed_by_user_id": userID,
	}

	if err := database.DB.Model(&session).Updates(updates).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to confirm session"})
	}

	return c.JSON(fiber.Map{"message": "Session confirmed successfully"})
}

// AddComment - เพิ่ม comment ให้ schedule หรือ session
func (sc *ScheduleController) AddComment(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var req struct {
		ScheduleID *uint  `json:"schedule_id"`
		SessionID  *uint  `json:"session_id"`
		Comment    string `json:"comment" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Validate comment content
	req.Comment = strings.TrimSpace(req.Comment)
	if req.Comment == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Comment cannot be empty"})
	}

	if len(req.Comment) > 1000 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Comment is too long (max 1000 characters)"})
	}

	if req.ScheduleID == nil && req.SessionID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Either schedule_id or session_id is required"})
	}

	// Validate that only one of schedule_id or session_id is provided
	if req.ScheduleID != nil && req.SessionID != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot provide both schedule_id and session_id"})
	}

	// Validate that the referenced schedule or session exists
	if req.ScheduleID != nil {
		var schedule models.Schedules
		if err := database.DB.First(&schedule, *req.ScheduleID).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schedule not found"})
		}
	}

	if req.SessionID != nil {
		var session models.Schedule_Sessions
		if err := database.DB.First(&session, *req.SessionID).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Session not found"})
		}
	}

	comment := models.Schedules_or_Sessions_Comment{
		UserID:  userID,
		Comment: req.Comment,
	}

	if req.ScheduleID != nil {
		comment.ScheduleID = req.ScheduleID
	}
	if req.SessionID != nil {
		comment.SessionID = req.SessionID
	}

	if err := database.DB.Create(&comment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create comment"})
	}

	// Load with user information and branch
	database.DB.Preload("User").Preload("User.Branch").Preload("Schedule").Preload("Session").First(&comment, comment.ID)

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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Either schedule_id or session_id is required"})
	}

	// Validate that only one parameter is provided
	if scheduleID != "" && sessionID != "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot provide both schedule_id and session_id"})
	}

	// Validate that the referenced schedule or session exists
	if scheduleID != "" {
		var schedule models.Schedules
		if err := database.DB.First(&schedule, scheduleID).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schedule not found"})
		}
	}

	if sessionID != "" {
		var session models.Schedule_Sessions
		if err := database.DB.First(&session, sessionID).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Session not found"})
		}
	}

	query := database.DB.Preload("User").Preload("User.Branch").Preload("Schedule").Preload("Session").Order("created_at DESC")

	if scheduleID != "" {
		query = query.Where("schedule_id = ?", scheduleID)
	}
	if sessionID != "" {
		query = query.Where("session_id = ?", sessionID)
	}

	var comments []models.Schedules_or_Sessions_Comment
	if err := query.Find(&comments).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch comments"})
	}

	// Normalize data - ensure all comments have proper user information
	for i := range comments {
		// Comments should always have user data due to preload
		// If user data is missing, it indicates a data integrity issue
		if comments[i].User.ID == 0 {
			// Log warning about missing user data
			fmt.Printf("Warning: Comment ID %d has missing user data\n", comments[i].ID)
		}
	}

	return c.JSON(fiber.Map{
		"comments": comments,
		"count":    len(comments),
	})
}

// CreateMakeupSession - สร้าง makeup session
func (sc *ScheduleController) CreateMakeupSession(c *fiber.Ctx) error {
	userRole := c.Locals("role")
	if userRole != "admin" && userRole != "owner" && userRole != "teacher" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admin, owner, and teacher can create makeup sessions"})
	}

	var req CreateMakeupSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Find original session
	var originalSession models.Schedule_Sessions
	if err := database.DB.First(&originalSession, req.OriginalSessionID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Original session not found"})
	}

	// Parse new start time
	newStartTime, err := time.Parse("15:04", req.NewStartTime)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid time format, use HH:MM"})
	}

	// Create new session date with the time
	newStartDateTime := time.Date(
		req.NewSessionDate.Year(), req.NewSessionDate.Month(), req.NewSessionDate.Day(),
		newStartTime.Hour(), newStartTime.Minute(), 0, 0, req.NewSessionDate.Location(),
	)
	// Calculate duration safely: ensure original start/end times are non-nil
	var newEndDateTime time.Time
	if originalSession.Start_time != nil && originalSession.End_time != nil {
		duration := originalSession.End_time.Sub(*originalSession.Start_time)
		newEndDateTime = newStartDateTime.Add(duration)
	} else {
		// Fallback: assume 1 hour session if original times missing
		newEndDateTime = newStartDateTime.Add(time.Hour)
	}

	tx := database.DB.Begin()

	// Update original session status
	originalSession.Status = req.NewSessionStatus
	originalSession.Cancelling_Reason = req.CancellingReason
	if err := tx.Save(&originalSession).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update original session"})
	}

	// Create makeup session
	// Use pointer assignments for time fields
	nd := req.NewSessionDate
	st := newStartDateTime
	et := newEndDateTime
	makeupSession := models.Schedule_Sessions{
		ScheduleID:            originalSession.ScheduleID,
		Session_date:          &nd,
		Start_time:            &st,
		End_time:              &et,
		Session_number:        originalSession.Session_number,
		Week_number:           originalSession.Week_number,
		Status:                "scheduled",
		Is_makeup:             true,
		Makeup_for_session_id: &originalSession.ID,
		AssignedTeacherID:     originalSession.AssignedTeacherID,
		RoomID:                originalSession.RoomID,
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

// ConfirmSchedule - ยืนยัน schedule (เฉพาะคนที่ถูก assign หรือ admin/owner)
func (sc *ScheduleController) ConfirmSchedule(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	currentUserID := userID.(uint)
	userRole := c.Locals("role").(string)

	scheduleID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid schedule ID"})
	}

	var schedule models.Schedules
	if err := database.DB.Preload("Group").First(&schedule, scheduleID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
	}

	// ตรวจสอบสิทธิ์ในการยืนยัน schedule
	canConfirm := false

	// Admin/Owner สามารถยืนยันให้ใครก็ได้
	if userRole == "admin" || userRole == "owner" {
		canConfirm = true
	} else {
		// สำหรับ class schedules - ตรวจสอบว่าเป็น default teacher
		if schedule.ScheduleType == "class" && schedule.DefaultTeacherID != nil && *schedule.DefaultTeacherID == currentUserID {
			canConfirm = true
		} else if schedule.ScheduleType != "class" {
			// สำหรับ event/appointment - ตรวจสอบว่าเป็น participant
			var participant models.ScheduleParticipant
			if err := database.DB.Where("schedule_id = ? AND user_id = ?", scheduleID, currentUserID).First(&participant).Error; err == nil {
				canConfirm = true
			}
		}
	}

	if !canConfirm {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You are not authorized to confirm this schedule"})
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

	// ส่ง notification ให้ admin และ owner
	go services.NotifyStudentsScheduleConfirmed(uint(scheduleID))

	return c.JSON(fiber.Map{
		"message": "Schedule confirmed successfully",
	})
}

// GetSession - ดูรายละเอียดของ session แบบละเอียด
func (sc *ScheduleController) GetSession(c *fiber.Ctx) error {
	idParam := c.Params("id")
	if idParam == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session id is required"})
	}

	// parse id
	sid, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session id"})
	}

	var session models.Schedule_Sessions
	// Preload related entities: Schedule (with default teacher/room), AssignedTeacher (User), Room, ConfirmedBy (User), and comments
	if err := database.DB.Preload("Schedule").Preload("Schedule.DefaultTeacher").Preload("Schedule.DefaultRoom").Preload("AssignedTeacher").Preload("AssignedTeacher.Branch").Preload("Room").Preload("ConfirmedBy").First(&session, uint(sid)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "session not found"})
	}

	// Load comments related to this session
	var comments []models.Schedules_or_Sessions_Comment
	if err := database.DB.Preload("User").Preload("User.Branch").Where("session_id = ?", session.ID).Order("created_at DESC").Find(&comments).Error; err != nil {
		// non-fatal: include empty comments and log
		fmt.Printf("warning: failed to load comments for session %d: %v\n", session.ID, err)
		comments = []models.Schedules_or_Sessions_Comment{}
	}

	// Normalize fields to avoid zero-value surprises in JSON
	if session.AssignedTeacher == nil && session.Schedule != nil && session.Schedule.DefaultTeacher != nil {
		// If no teacher assigned at session level, use schedule's default teacher if available
		session.AssignedTeacher = session.Schedule.DefaultTeacher
	}

	if session.Room == nil && session.Schedule != nil && session.Schedule.DefaultRoom != nil && session.Schedule.DefaultRoom.ID != 0 {
		// Load default room if present on schedule
		var room models.Room
		if err := database.DB.First(&room, session.Schedule.DefaultRoom.ID).Error; err == nil {
			session.Room = &room
		}
	}

	// Ensure nested users have branch populated where possible
	if session.AssignedTeacher != nil && session.AssignedTeacher.Branch.ID == 0 {
		// try to fill branch from teacher record if exists
		var teacher models.Teacher
		if err := database.DB.Where("user_id = ?", session.AssignedTeacher.ID).Preload("Branch").First(&teacher).Error; err == nil {
			session.AssignedTeacher.Branch = teacher.Branch
		}
	}

	// Prepare response
	resp := fiber.Map{
		"session":  session,
		"comments": comments,
	}

	return c.JSON(resp)
}
