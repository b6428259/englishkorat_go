package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/models"
	"englishkorat_go/services"
	notifsvc "englishkorat_go/services/notifications"
	"englishkorat_go/utils"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type SessionTimeSlot struct {
	Weekday   int    `json:"weekday"`
	StartTime string `json:"start_time"`
}

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
	AutoRescheduleHoliday bool `json:"auto_reschedule"`
	// Alias to accept client field auto_reschedule_holidays as well
	AutoRescheduleHolidaysAlias bool              `json:"auto_reschedule_holidays"`
	Notes                       string            `json:"notes"`
	SessionStartTime            string            `json:"session_start_time" validate:"required"` // เวลาเริ่มต้นของแต่ละ session เช่น "09:00"
	CustomRecurringDays         []int             `json:"custom_recurring_days,omitempty"`        // สำหรับ custom pattern [0=วันอาทิตย์, 1=วันจันทร์, ...]
	SessionTimes                []SessionTimeSlot `json:"session_times,omitempty"`
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

type CheckRoomConflictRequest struct {
	RoomID              uint              `json:"room_id"`
	RoomIDs             []uint            `json:"room_ids,omitempty"`
	BranchID            *uint             `json:"branch_id,omitempty"`
	RecurringPattern    string            `json:"recurring_pattern"`
	TotalHours          int               `json:"total_hours"`
	HoursPerSession     int               `json:"hours_per_session"`
	SessionPerWeek      int               `json:"session_per_week"`
	StartDate           time.Time         `json:"start_date"`
	EstimatedEndDate    time.Time         `json:"estimated_end_date"`
	SessionStartTime    string            `json:"session_start_time"`
	CustomRecurringDays []int             `json:"custom_recurring_days,omitempty"`
	SessionTimes        []SessionTimeSlot `json:"session_times,omitempty"`
	ExcludeScheduleID   *uint             `json:"exclude_schedule_id,omitempty"`
}

type RoomConflictDetail struct {
	RoomID       uint   `json:"room_id"`
	ExistingRoom *uint  `json:"existing_room_id,omitempty"`
	SessionID    uint   `json:"session_id"`
	ScheduleID   uint   `json:"schedule_id"`
	ScheduleName string `json:"schedule_name"`
	SessionDate  string `json:"session_date"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
}

type RoomConflictSummary struct {
	RoomID    uint                 `json:"room_id"`
	Conflicts []RoomConflictDetail `json:"conflicts"`
}

type GroupConflictInfo struct {
	ScheduleID       uint       `json:"schedule_id"`
	ScheduleName     string     `json:"schedule_name"`
	Status           string     `json:"status"`
	StartDate        *time.Time `json:"start_date,omitempty"`
	EstimatedEndDate *time.Time `json:"estimated_end_date,omitempty"`
}

type ScheduleConflictSlot struct {
	ScheduleID   uint   `json:"schedule_id"`
	ScheduleName string `json:"schedule_name"`
	SessionID    uint   `json:"session_id"`
	SessionDate  string `json:"session_date"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
}

type TeacherConflictDetail struct {
	TeacherID   uint                   `json:"teacher_id"`
	TeacherName string                 `json:"teacher_name"`
	Conflicts   []ScheduleConflictSlot `json:"conflicts"`
}

type ParticipantConflictDetail struct {
	UserID    uint                   `json:"user_id"`
	Username  string                 `json:"username"`
	Conflicts []ScheduleConflictSlot `json:"conflicts"`
}

type StudentConflictDetail struct {
	StudentID   uint                   `json:"student_id"`
	StudentName string                 `json:"student_name"`
	UserID      *uint                  `json:"user_id,omitempty"`
	Conflicts   []ScheduleConflictSlot `json:"conflicts"`
}

type SchedulePreviewIssue struct {
	Severity string      `json:"severity"`
	Code     string      `json:"code"`
	Message  string      `json:"message"`
	Details  interface{} `json:"details,omitempty"`
}

type SessionPreview struct {
	SessionNumber int    `json:"session_number"`
	WeekNumber    int    `json:"week_number"`
	Date          string `json:"date"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time"`
	Notes         string `json:"notes,omitempty"`
}

type HolidayImpact struct {
	SessionNumber  int    `json:"session_number"`
	Date           string `json:"date"`
	HolidayName    string `json:"holiday_name,omitempty"`
	ShiftedTo      string `json:"shifted_to,omitempty"`
	WasRescheduled bool   `json:"was_rescheduled"`
}

type GroupPaymentMember struct {
	MemberID      uint   `json:"member_id"`
	StudentID     uint   `json:"student_id"`
	StudentName   string `json:"student_name"`
	PaymentStatus string `json:"payment_status"`
}

type GroupPaymentSummary struct {
	GroupID            uint                 `json:"group_id"`
	GroupName          string               `json:"group_name"`
	GroupPaymentStatus string               `json:"group_payment_status"`
	EligibleMembers    int                  `json:"eligible_members"`
	IneligibleMembers  int                  `json:"ineligible_members"`
	MemberTotals       map[string]int       `json:"member_totals"`
	RequireDeposit     bool                 `json:"require_deposit"`
	Members            []GroupPaymentMember `json:"members"`
}

type ScheduleConflictReport struct {
	GroupConflict        *GroupConflictInfo          `json:"group_conflict,omitempty"`
	RoomConflicts        []RoomConflictSummary       `json:"room_conflicts,omitempty"`
	TeacherConflicts     []TeacherConflictDetail     `json:"teacher_conflicts,omitempty"`
	ParticipantConflicts []ParticipantConflictDetail `json:"participant_conflicts,omitempty"`
	StudentConflicts     []StudentConflictDetail     `json:"student_conflicts,omitempty"`
}

type conflictReportOptions struct {
	IncludeStudentConflicts bool
}

type ScheduleController struct{}

func getSessionDateRange(sessions []models.Schedule_Sessions) (time.Time, time.Time) {
	var minDate, maxDate time.Time
	for _, session := range sessions {
		if session.Session_date == nil {
			continue
		}
		date := *session.Session_date
		if minDate.IsZero() || date.Before(minDate) {
			minDate = date
		}
		if maxDate.IsZero() || date.After(maxDate) {
			maxDate = date
		}
	}
	return minDate, maxDate
}

func sessionsOverlapOnSameDay(a, b models.Schedule_Sessions) bool {
	if a.Start_time == nil || a.End_time == nil || b.Start_time == nil || b.End_time == nil {
		return false
	}

	aDay := time.Date(a.Start_time.Year(), a.Start_time.Month(), a.Start_time.Day(), 0, 0, 0, 0, a.Start_time.Location())
	bDay := time.Date(b.Start_time.Year(), b.Start_time.Month(), b.Start_time.Day(), 0, 0, 0, 0, b.Start_time.Location())
	if !aDay.Equal(bDay) {
		return false
	}

	return a.Start_time.Before(*b.End_time) && b.Start_time.Before(*a.End_time)
}

func parseHourMinute(value string) (int, int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, fmt.Errorf("time value cannot be empty")
	}

	layout := "15:04"
	if colonCount := strings.Count(value, ":"); colonCount >= 2 {
		layout = "15:04:05"
	}

	if t, err := time.Parse(layout, value); err == nil {
		return t.Hour(), t.Minute(), nil
	} else {
		fallbackLayouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04",
			"2006-01-02T15:04",
		}

		for _, layout := range fallbackLayouts {
			if parsed, altErr := time.Parse(layout, value); altErr == nil {
				return parsed.Hour(), parsed.Minute(), nil
			}
		}

		timePattern := regexp.MustCompile(`\d{1,2}:\d{2}(?::\d{2})?`)
		if match := timePattern.FindString(value); match != "" && match != value {
			return parseHourMinute(match)
		}

		return 0, 0, fmt.Errorf("invalid time format %q: %w", value, err)
	}
}

func resolveBranchHours(branch *models.Branch) (int, int, error) {
	const (
		defaultOpenMinutes  = 8 * 60
		defaultCloseMinutes = 21 * 60
	)

	if branch == nil {
		return defaultOpenMinutes, defaultCloseMinutes, nil
	}

	// Convert branch time.Time fields to strings if present, otherwise treat as empty
	openStr := ""
	if !branch.OpenTime.IsZero() {
		openStr = branch.OpenTime.Format("15:04")
	}

	closeStr := ""
	if !branch.CloseTime.IsZero() {
		closeStr = branch.CloseTime.Format("15:04")
	}

	openStr = strings.TrimSpace(openStr)
	closeStr = strings.TrimSpace(closeStr)

	openMinutes := defaultOpenMinutes
	if openStr != "" {
		hour, minute, err := parseHourMinute(openStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid branch open_time: %w", err)
		}
		openMinutes = hour*60 + minute
	}

	closeMinutes := defaultCloseMinutes
	if closeStr != "" {
		hour, minute, err := parseHourMinute(closeStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid branch close_time: %w", err)
		}
		closeMinutes = hour*60 + minute
	}

	if closeMinutes <= openMinutes {
		return 0, 0, fmt.Errorf("branch closing time must be after opening time")
	}

	return openMinutes, closeMinutes, nil
}

func formatSessionTime(t *time.Time) string {
	if t == nil {
		return "unknown"
	}
	return t.Format("15:04")
}

func formatSessionDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

type roomConflictRecord struct {
	RoomID       *uint      `gorm:"column:room_id"`
	SessionID    uint       `gorm:"column:session_id"`
	ScheduleID   uint       `gorm:"column:schedule_id"`
	ScheduleName string     `gorm:"column:schedule_name"`
	SessionDate  *time.Time `gorm:"column:session_date"`
	StartTime    *time.Time `gorm:"column:start_time"`
	EndTime      *time.Time `gorm:"column:end_time"`
}

func findRoomConflicts(roomID uint, candidateSessions []models.Schedule_Sessions, excludeScheduleID *uint) ([]roomConflictRecord, error) {
	if roomID == 0 || len(candidateSessions) == 0 {
		return nil, nil
	}

	minDate, maxDate := getSessionDateRange(candidateSessions)
	if minDate.IsZero() || maxDate.IsZero() {
		return nil, nil
	}

	var existingSessions []roomConflictRecord
	query := database.DB.Table("schedule_sessions").
		Select("schedule_sessions.room_id, schedule_sessions.id AS session_id, schedule_sessions.schedule_id, schedules.schedule_name, schedule_sessions.session_date, schedule_sessions.start_time, schedule_sessions.end_time").
		Joins("JOIN schedules ON schedules.id = schedule_sessions.schedule_id").
		Where("(schedule_sessions.room_id = ? OR (schedule_sessions.room_id IS NULL AND schedules.default_room_id = ?))", roomID, roomID).
		Where("schedule_sessions.session_date BETWEEN ? AND ?", minDate, maxDate).
		Where("schedule_sessions.status NOT IN ?", []string{"cancelled", "no-show"}).
		Where("schedules.status IN ?", []string{"assigned", "scheduled"})

	if excludeScheduleID != nil {
		query = query.Where("schedule_sessions.schedule_id <> ?", *excludeScheduleID)
	}

	if err := query.Find(&existingSessions).Error; err != nil {
		return nil, err
	}

	conflicts := make([]roomConflictRecord, 0, len(existingSessions))
	for _, row := range existingSessions {
		existing := models.Schedule_Sessions{
			Session_date: row.SessionDate,
			Start_time:   row.StartTime,
			End_time:     row.EndTime,
		}

		for _, candidate := range candidateSessions {
			if sessionsOverlapOnSameDay(existing, candidate) {
				conflicts = append(conflicts, row)
				break
			}
		}
	}

	return conflicts, nil
}

type scheduleConflictRow struct {
	SessionID    uint       `gorm:"column:session_id"`
	ScheduleID   uint       `gorm:"column:schedule_id"`
	ScheduleName string     `gorm:"column:schedule_name"`
	SessionDate  *time.Time `gorm:"column:session_date"`
	StartTime    *time.Time `gorm:"column:start_time"`
	EndTime      *time.Time `gorm:"column:end_time"`
}

type participantConflictRow struct {
	scheduleConflictRow
	UserID uint `gorm:"column:user_id"`
}

type studentConflictRow struct {
	scheduleConflictRow
	StudentID uint `gorm:"column:student_id"`
}

func copySessions(src []models.Schedule_Sessions) []models.Schedule_Sessions {
	cloned := make([]models.Schedule_Sessions, len(src))
	for i, session := range src {
		cloned[i] = session
		if session.Session_date != nil {
			dateCopy := *session.Session_date
			cloned[i].Session_date = &dateCopy
		}
		if session.Start_time != nil {
			startCopy := *session.Start_time
			cloned[i].Start_time = &startCopy
		}
		if session.End_time != nil {
			endCopy := *session.End_time
			cloned[i].End_time = &endCopy
		}
	}
	return cloned
}

func sessionsToPreview(sessions []models.Schedule_Sessions) []SessionPreview {
	previews := make([]SessionPreview, 0, len(sessions))
	for _, session := range sessions {
		preview := SessionPreview{
			SessionNumber: session.Session_number,
			WeekNumber:    session.Week_number,
			Date:          formatSessionDate(session.Session_date),
			StartTime:     formatSessionTime(session.Start_time),
			EndTime:       formatSessionTime(session.End_time),
			Notes:         strings.TrimSpace(session.Notes),
		}
		if preview.Notes == "" {
			preview.Notes = ""
		}
		previews = append(previews, preview)
	}
	return previews
}

func buildGroupPaymentSummary(group *models.Group) GroupPaymentSummary {
	summary := GroupPaymentSummary{
		GroupID:            group.ID,
		GroupName:          group.GroupName,
		GroupPaymentStatus: group.PaymentStatus,
		MemberTotals:       map[string]int{"pending": 0, "deposit_paid": 0, "fully_paid": 0},
		RequireDeposit:     true,
	}

	for _, member := range group.Members {
		status := strings.ToLower(strings.TrimSpace(member.PaymentStatus))
		if status == "" {
			status = "pending"
		}
		if _, ok := summary.MemberTotals[status]; !ok {
			summary.MemberTotals[status] = 0
		}
		summary.MemberTotals[status]++
		if status == "deposit_paid" || status == "fully_paid" {
			summary.EligibleMembers++
		} else {
			summary.IneligibleMembers++
		}

		studentName := ""
		if member.Student.ID != 0 {
			studentName = strings.TrimSpace(member.Student.NicknameEn)
			if studentName == "" {
				fullEn := strings.TrimSpace(strings.TrimSpace(member.Student.FirstNameEn + " " + member.Student.LastNameEn))
				if fullEn != "" {
					studentName = fullEn
				} else {
					fullTh := strings.TrimSpace(strings.TrimSpace(member.Student.FirstName + " " + member.Student.LastName))
					if fullTh != "" {
						studentName = fullTh
					}
				}
			}
			if studentName == "" {
				studentName = fmt.Sprintf("Student #%d", member.Student.ID)
			}
		}

		summary.Members = append(summary.Members, GroupPaymentMember{
			MemberID:      member.ID,
			StudentID:     member.StudentID,
			StudentName:   studentName,
			PaymentStatus: status,
		})
	}

	return summary
}

func makeConflictSlot(row scheduleConflictRow) ScheduleConflictSlot {
	return ScheduleConflictSlot{
		ScheduleID:   row.ScheduleID,
		ScheduleName: row.ScheduleName,
		SessionID:    row.SessionID,
		SessionDate:  formatSessionDate(row.SessionDate),
		StartTime:    formatSessionTime(row.StartTime),
		EndTime:      formatSessionTime(row.EndTime),
	}
}

func collectGroupActiveSchedule(groupID uint) (*GroupConflictInfo, error) {
	if groupID == 0 {
		return nil, nil
	}

	var existing models.Schedules
	result := database.DB.Where("group_id = ? AND status IN ? AND schedule_type = ?", groupID, []string{"assigned", "scheduled"}, "class").Order("updated_at DESC").First(&existing)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	var startDatePtr *time.Time
	if !existing.Start_date.IsZero() {
		startCopy := existing.Start_date
		startDatePtr = &startCopy
	}

	var estimatedEndPtr *time.Time
	if !existing.Estimated_end_date.IsZero() {
		endCopy := existing.Estimated_end_date
		estimatedEndPtr = &endCopy
	}

	return &GroupConflictInfo{
		ScheduleID:       existing.ID,
		ScheduleName:     existing.ScheduleName,
		Status:           existing.Status,
		StartDate:        startDatePtr,
		EstimatedEndDate: estimatedEndPtr,
	}, nil
}

func collectTeacherConflictDetail(teacherID *uint, candidateSessions []models.Schedule_Sessions, minDate, maxDate time.Time, excludeScheduleID *uint) (*TeacherConflictDetail, error) {
	if teacherID == nil || *teacherID == 0 || len(candidateSessions) == 0 || minDate.IsZero() || maxDate.IsZero() {
		return nil, nil
	}

	var rows []scheduleConflictRow
	query := database.DB.Table("schedule_sessions").
		Select("schedule_sessions.id AS session_id, schedule_sessions.schedule_id, schedules.schedule_name, schedule_sessions.session_date, schedule_sessions.start_time, schedule_sessions.end_time").
		Joins("JOIN schedules ON schedules.id = schedule_sessions.schedule_id").
		Where("(schedule_sessions.assigned_teacher_id = ? OR schedules.default_teacher_id = ?)", *teacherID, *teacherID).
		Where("schedule_sessions.status NOT IN ?", []string{"cancelled", "no-show"}).
		Where("schedules.status IN ?", []string{"assigned", "scheduled"}).
		Where("schedule_sessions.session_date BETWEEN ? AND ?", minDate, maxDate)
	if excludeScheduleID != nil {
		query = query.Where("schedule_sessions.schedule_id <> ?", *excludeScheduleID)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}

	slots := make([]ScheduleConflictSlot, 0)
	for _, row := range rows {
		existing := models.Schedule_Sessions{
			Session_date: row.SessionDate,
			Start_time:   row.StartTime,
			End_time:     row.EndTime,
		}
		for _, candidate := range candidateSessions {
			if sessionsOverlapOnSameDay(existing, candidate) {
				slots = append(slots, makeConflictSlot(row))
				break
			}
		}
	}

	if len(slots) == 0 {
		return nil, nil
	}

	var teacher models.User
	if err := database.DB.Select("id, username").First(&teacher, *teacherID).Error; err != nil {
		teacher = models.User{BaseModel: models.BaseModel{ID: *teacherID}, Username: fmt.Sprintf("Teacher #%d", *teacherID)}
	}

	return &TeacherConflictDetail{
		TeacherID:   *teacherID,
		TeacherName: teacher.Username,
		Conflicts:   slots,
	}, nil
}

func collectParticipantConflictDetails(userIDs []uint, candidateSessions []models.Schedule_Sessions, minDate, maxDate time.Time, excludeScheduleID *uint) ([]ParticipantConflictDetail, error) {
	if len(userIDs) == 0 || len(candidateSessions) == 0 || minDate.IsZero() || maxDate.IsZero() {
		return nil, nil
	}

	var rows []participantConflictRow
	query := database.DB.Table("schedule_sessions").
		Select("schedule_participants.user_id AS user_id, schedule_sessions.id AS session_id, schedule_sessions.schedule_id, schedules.schedule_name, schedule_sessions.session_date, schedule_sessions.start_time, schedule_sessions.end_time").
		Joins("JOIN schedules ON schedules.id = schedule_sessions.schedule_id").
		Joins("JOIN schedule_participants ON schedule_participants.schedule_id = schedules.id").
		Where("schedule_participants.user_id IN ?", userIDs).
		Where("schedule_sessions.status NOT IN ?", []string{"cancelled", "no-show"}).
		Where("schedules.status IN ?", []string{"assigned", "scheduled"}).
		Where("schedule_sessions.session_date BETWEEN ? AND ?", minDate, maxDate)
	if excludeScheduleID != nil {
		query = query.Where("schedule_sessions.schedule_id <> ?", *excludeScheduleID)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}

	conflictsByUser := make(map[uint][]ScheduleConflictSlot)
	for _, row := range rows {
		existing := models.Schedule_Sessions{Session_date: row.SessionDate, Start_time: row.StartTime, End_time: row.EndTime}
		for _, candidate := range candidateSessions {
			if sessionsOverlapOnSameDay(existing, candidate) {
				conflictsByUser[row.UserID] = append(conflictsByUser[row.UserID], makeConflictSlot(row.scheduleConflictRow))
				break
			}
		}
	}

	if len(conflictsByUser) == 0 {
		return nil, nil
	}

	userIDsUnique := make([]uint, 0, len(conflictsByUser))
	for id := range conflictsByUser {
		userIDsUnique = append(userIDsUnique, id)
	}
	sort.Slice(userIDsUnique, func(i, j int) bool { return userIDsUnique[i] < userIDsUnique[j] })

	var users []models.User
	if err := database.DB.Select("id, username").Where("id IN ?", userIDsUnique).Find(&users).Error; err != nil {
		return nil, err
	}
	usernames := make(map[uint]string, len(users))
	for _, u := range users {
		usernames[u.ID] = u.Username
	}

	details := make([]ParticipantConflictDetail, 0, len(conflictsByUser))
	for _, id := range userIDsUnique {
		conflicts := conflictsByUser[id]
		if len(conflicts) == 0 {
			continue
		}
		details = append(details, ParticipantConflictDetail{
			UserID:    id,
			Username:  usernames[id],
			Conflicts: conflicts,
		})
	}

	return details, nil
}

func collectStudentConflictDetails(group *models.Group, candidateSessions []models.Schedule_Sessions, minDate, maxDate time.Time, excludeScheduleID *uint) ([]StudentConflictDetail, error) {
	if group == nil || len(group.Members) == 0 || len(candidateSessions) == 0 || minDate.IsZero() || maxDate.IsZero() {
		return nil, nil
	}

	studentIDs := make([]uint, 0, len(group.Members))
	studentsMap := make(map[uint]models.GroupMember, len(group.Members))
	for _, member := range group.Members {
		studentIDs = append(studentIDs, member.StudentID)
		studentsMap[member.StudentID] = member
	}

	var rows []studentConflictRow
	query := database.DB.Table("schedule_sessions").
		Select("group_members.student_id AS student_id, schedule_sessions.id AS session_id, schedule_sessions.schedule_id, schedules.schedule_name, schedule_sessions.session_date, schedule_sessions.start_time, schedule_sessions.end_time").
		Joins("JOIN schedules ON schedules.id = schedule_sessions.schedule_id").
		Joins("JOIN `groups` ON schedules.group_id = `groups`.id").
		Joins("JOIN group_members ON group_members.group_id = `groups`.id").
		Where("group_members.student_id IN ?", studentIDs).
		Where("schedule_sessions.status NOT IN ?", []string{"cancelled", "no-show"}).
		Where("schedules.status IN ?", []string{"assigned", "scheduled"}).
		Where("schedule_sessions.session_date BETWEEN ? AND ?", minDate, maxDate)
	if excludeScheduleID != nil {
		query = query.Where("schedule_sessions.schedule_id <> ?", *excludeScheduleID)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}

	conflictsByStudent := make(map[uint][]ScheduleConflictSlot)
	for _, row := range rows {
		existing := models.Schedule_Sessions{Session_date: row.SessionDate, Start_time: row.StartTime, End_time: row.EndTime}
		for _, candidate := range candidateSessions {
			if sessionsOverlapOnSameDay(existing, candidate) {
				conflictsByStudent[row.StudentID] = append(conflictsByStudent[row.StudentID], makeConflictSlot(row.scheduleConflictRow))
				break
			}
		}
	}

	if len(conflictsByStudent) == 0 {
		return nil, nil
	}

	studentIDsUnique := make([]uint, 0, len(conflictsByStudent))
	for id := range conflictsByStudent {
		studentIDsUnique = append(studentIDsUnique, id)
	}
	sort.Slice(studentIDsUnique, func(i, j int) bool { return studentIDsUnique[i] < studentIDsUnique[j] })

	details := make([]StudentConflictDetail, 0, len(conflictsByStudent))
	for _, sid := range studentIDsUnique {
		conflicts := conflictsByStudent[sid]
		member := studentsMap[sid]

		studentName := ""
		if member.Student.ID != 0 {
			studentName = strings.TrimSpace(member.Student.NicknameEn)
			if studentName == "" {
				fullEn := strings.TrimSpace(strings.TrimSpace(member.Student.FirstNameEn + " " + member.Student.LastNameEn))
				if fullEn != "" {
					studentName = fullEn
				} else {
					fullTh := strings.TrimSpace(strings.TrimSpace(member.Student.FirstName + " " + member.Student.LastName))
					if fullTh != "" {
						studentName = fullTh
					}
				}
			}
		}
		var userIDPtr *uint
		if member.Student.UserID != nil {
			userID := *member.Student.UserID
			userIDPtr = &userID
		}

		details = append(details, StudentConflictDetail{
			StudentID:   sid,
			StudentName: studentName,
			UserID:      userIDPtr,
			Conflicts:   conflicts,
		})
	}

	return details, nil
}

func generateScheduleConflictReport(req CreateScheduleRequest, group *models.Group, candidateSessions []models.Schedule_Sessions, minDate, maxDate time.Time, excludeScheduleID *uint, opts conflictReportOptions) (*ScheduleConflictReport, error) {
	report := &ScheduleConflictReport{}

	scheduleType := strings.ToLower(strings.TrimSpace(req.ScheduleType))
	if scheduleType == "class" && req.GroupID != nil {
		groupConflict, err := collectGroupActiveSchedule(*req.GroupID)
		if err != nil {
			return nil, err
		}
		report.GroupConflict = groupConflict
	}

	if req.DefaultRoomID != nil && *req.DefaultRoomID != 0 {
		roomConflicts, err := findRoomConflicts(*req.DefaultRoomID, candidateSessions, excludeScheduleID)
		if err != nil {
			return nil, err
		}
		if len(roomConflicts) > 0 {
			details := make([]RoomConflictDetail, 0, len(roomConflicts))
			for _, conflict := range roomConflicts {
				details = append(details, RoomConflictDetail{
					RoomID:       *req.DefaultRoomID,
					ExistingRoom: conflict.RoomID,
					SessionID:    conflict.SessionID,
					ScheduleID:   conflict.ScheduleID,
					ScheduleName: conflict.ScheduleName,
					SessionDate:  formatSessionDate(conflict.SessionDate),
					StartTime:    formatSessionTime(conflict.StartTime),
					EndTime:      formatSessionTime(conflict.EndTime),
				})
			}
			report.RoomConflicts = append(report.RoomConflicts, RoomConflictSummary{RoomID: *req.DefaultRoomID, Conflicts: details})
		}
	}

	teacherDetail, err := collectTeacherConflictDetail(req.DefaultTeacherID, candidateSessions, minDate, maxDate, excludeScheduleID)
	if err != nil {
		return nil, err
	}
	if teacherDetail != nil {
		report.TeacherConflicts = append(report.TeacherConflicts, *teacherDetail)
	}

	participantDetails, err := collectParticipantConflictDetails(req.ParticipantUserIDs, candidateSessions, minDate, maxDate, excludeScheduleID)
	if err != nil {
		return nil, err
	}
	if len(participantDetails) > 0 {
		report.ParticipantConflicts = participantDetails
	}

	if opts.IncludeStudentConflicts {
		studentDetails, err := collectStudentConflictDetails(group, candidateSessions, minDate, maxDate, excludeScheduleID)
		if err != nil {
			return nil, err
		}
		if len(studentDetails) > 0 {
			report.StudentConflicts = studentDetails
		}
	}

	return report, nil
}

// CreateSchedule - สร้าง schedule ใหม่ (เฉพาะ admin และ owner)
func (sc *ScheduleController) CreateSchedule(c *fiber.Ctx) error {
	userRole := c.Locals("role")
	if userRole != "admin" && userRole != "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admin and owner can create schedules"})
	}

	userID := c.Locals("user_id").(uint)

	var req CreateScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if req.GroupID != nil && *req.GroupID == 0 {
		req.GroupID = nil
	}
	if req.DefaultTeacherID != nil && *req.DefaultTeacherID == 0 {
		req.DefaultTeacherID = nil
	}
	if req.DefaultRoomID != nil && *req.DefaultRoomID == 0 {
		req.DefaultRoomID = nil
	}

	scheduleType := strings.ToLower(req.ScheduleType)
	if scheduleType != "class" {
		req.GroupID = nil
	}

	autoReschedule := req.AutoRescheduleHoliday || req.AutoRescheduleHolidaysAlias

	if req.HoursPerSession <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "hours_per_session must be greater than zero"})
	}
	if req.TotalHours <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "total_hours must be greater than zero"})
	}
	if req.TotalHours%req.HoursPerSession != 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "total_hours must be divisible by hours_per_session"})
	}
	totalSessions := req.TotalHours / req.HoursPerSession

	var branch *models.Branch
	branchHours := services.BranchHours{OpenMinutes: 8 * 60, CloseMinutes: 21 * 60}
	sessionSlots := make([]services.SessionSlot, 0, len(req.SessionTimes))

	if scheduleType == "class" {
		if req.GroupID == nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "group_id is required for class schedules"})
		}

		var group models.Group
		if err := database.DB.Preload("Members").Preload("Course.Branch").First(&group, *req.GroupID).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Group not found"})
		}

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

		if group.CourseID != 0 && group.Course.ID == 0 {
			if err := database.DB.Preload("Branch").First(&group.Course, group.CourseID).Error; err == nil {
				branch = &group.Course.Branch
			}
		} else if group.Course.ID != 0 {
			branch = &group.Course.Branch
		}

		openMinutes, closeMinutes, err := resolveBranchHours(branch)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		branchHours = services.BranchHours{OpenMinutes: openMinutes, CloseMinutes: closeMinutes}

		if len(req.SessionTimes) > 0 {
			if req.SessionPerWeek != len(req.SessionTimes) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_per_week must match the number of session_times provided"})
			}

			seenWeekdays := make(map[int]struct{}, len(req.SessionTimes))
			for _, slot := range req.SessionTimes {
				if slot.Weekday < 0 || slot.Weekday > 6 {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_times.weekday must be between 0 (Sunday) and 6 (Saturday)"})
				}
				if _, exists := seenWeekdays[slot.Weekday]; exists {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_times contains duplicate weekdays"})
				}

				hour, minute, err := parseHourMinute(slot.StartTime)
				if err != nil {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("invalid start_time in session_times: %v", err)})
				}

				startMinutes := hour*60 + minute
				endMinutes := startMinutes + req.HoursPerSession*60
				if startMinutes < branchHours.OpenMinutes || endMinutes > branchHours.CloseMinutes {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("session on weekday %d (%02d:%02d) is outside branch operating hours", slot.Weekday, hour, minute)})
				}

				seenWeekdays[slot.Weekday] = struct{}{}
				sessionSlots = append(sessionSlots, services.SessionSlot{
					Weekday:     time.Weekday(slot.Weekday),
					StartHour:   hour,
					StartMinute: minute,
				})
			}
		}
	} else {
		if len(req.ParticipantUserIDs) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "participant_user_ids is required for event/appointment schedules"})
		}
	}

	if req.DefaultTeacherID != nil {
		var teacher models.User
		if err := database.DB.Where("id = ? AND role IN ?", *req.DefaultTeacherID, []string{"teacher", "admin", "owner"}).First(&teacher).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Default teacher not found or not authorized to teach"})
		}
	}

	if len(sessionSlots) == 0 && strings.TrimSpace(req.SessionStartTime) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_start_time is required when session_times are not provided"})
	}

	if scheduleType == "class" && len(sessionSlots) == 0 {
		hour, minute, err := parseHourMinute(req.SessionStartTime)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("invalid session_start_time: %v", err)})
		}
		startMinutes := hour*60 + minute
		endMinutes := startMinutes + req.HoursPerSession*60
		if startMinutes < branchHours.OpenMinutes || endMinutes > branchHours.CloseMinutes {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_start_time is outside branch operating hours"})
		}
	}

	pattern := req.RecurringPattern
	if len(sessionSlots) > 0 && strings.TrimSpace(pattern) == "" {
		pattern = "custom"
	}

	var assignedUser models.User
	if err := database.DB.First(&assignedUser, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get user info"})
	}

	schedule := models.Schedules{
		ScheduleName:            req.ScheduleName,
		ScheduleType:            req.ScheduleType,
		GroupID:                 req.GroupID,
		CreatedByUserID:         &userID,
		Recurring_pattern:       pattern,
		Total_hours:             req.TotalHours,
		Hours_per_session:       req.HoursPerSession,
		Session_per_week:        req.SessionPerWeek,
		Start_date:              req.StartDate,
		Estimated_end_date:      req.EstimatedEndDate,
		DefaultTeacherID:        req.DefaultTeacherID,
		DefaultRoomID:           req.DefaultRoomID,
		Status:                  "assigned",
		Auto_Reschedule_holiday: autoReschedule,
		Notes:                   req.Notes,
		Admin_assigned:          assignedUser.Username,
	}

	var (
		sessions []models.Schedule_Sessions
		err      error
	)
	if len(sessionSlots) > 0 {
		sessions, err = services.GenerateScheduleSessionsWithSlots(schedule, sessionSlots, totalSessions, req.HoursPerSession, branchHours)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
	} else {
		sessions, err = services.GenerateScheduleSessions(schedule, req.SessionStartTime, req.CustomRecurringDays)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to generate sessions: " + err.Error()})
		}
	}

	if len(sessions) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no sessions generated for the provided configuration"})
	}

	minDate, maxDate := getSessionDateRange(sessions)
	startYear := schedule.Start_date.Year()
	if !minDate.IsZero() && minDate.Year() < startYear {
		startYear = minDate.Year()
	}
	endYear := schedule.Estimated_end_date.Year()
	if endYear < startYear {
		endYear = startYear
	}
	if !maxDate.IsZero() && maxDate.Year() > endYear {
		endYear = maxDate.Year()
	}

	// Apply holiday rescheduling if enabled
	if autoReschedule {
		var holidayDates []time.Time
		if _, err := services.GetThaiHolidaysWithNames(startYear, endYear); err == nil {
			// Fetch dates for rescheduling
			if holidays, err := services.GetThaiHolidays(startYear, endYear); err == nil {
				holidayDates = holidays
			}
		}

		if len(holidayDates) > 0 {
			sessions = services.RescheduleSessions(sessions, holidayDates)
		}
	}

	services.ReindexSessions(sessions, schedule.Start_date)
	minDate, maxDate = getSessionDateRange(sessions)
	if !maxDate.IsZero() {
		schedule.Estimated_end_date = maxDate
	}

	if err := checkScheduleConflicts(req, userID, sessions); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	tx := database.DB.Begin()
	if err := tx.Create(&schedule).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create schedule"})
	}

	if scheduleType != "class" && len(req.ParticipantUserIDs) > 0 {
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
			if participantID == userID {
				participant.Role = "organizer"
			}

			if err := tx.Create(&participant).Error; err != nil {
				tx.Rollback()
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to add participants to schedule"})
			}
		}
	}

	for i := range sessions {
		sessions[i].ScheduleID = schedule.ID
		sessions[i].AssignedTeacherID = req.DefaultTeacherID
		sessions[i].RoomID = req.DefaultRoomID

		if scheduleType == "class" {
			sessions[i].Status = "assigned"
		} else if strings.TrimSpace(sessions[i].Status) == "" {
			sessions[i].Status = "scheduled"
		}

		if err := tx.Create(&sessions[i]).Error; err != nil {
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create sessions"})
		}

		if scheduleType == "class" {
			services.ScheduleTeacherConfirmReminders(sessions[i], schedule)
		}
	}

	var notifyDefaultTeacherIDs []uint
	var notifyParticipantIDs []uint
	if scheduleType == "class" && req.DefaultTeacherID != nil {
		notifyDefaultTeacherIDs = append(notifyDefaultTeacherIDs, *req.DefaultTeacherID)
	} else if scheduleType != "class" {
		notifyParticipantIDs = append(notifyParticipantIDs, req.ParticipantUserIDs...)
	}

	tx.Commit()

	notifService := notifsvc.NewService()
	if len(notifyDefaultTeacherIDs) > 0 {
		data := fiber.Map{
			"link": fiber.Map{
				"href":   fmt.Sprintf("/api/schedules/%d/sessions", schedule.ID),
				"method": "GET",
			},
			"action":      "review-schedule",
			"schedule_id": schedule.ID,
		}
		payload := notifsvc.QueuedWithData(
			"New Schedule Assignment",
			"การมอบหมายตารางใหม่",
			fmt.Sprintf("You have been assigned to schedule: %s. Please review your sessions.", req.ScheduleName),
			fmt.Sprintf("คุณได้รับมอบหมายตาราง: %s กรุณาตรวจสอบคาบเรียนของคุณ", req.ScheduleName),
			"info", data,
			"normal",
		)
		_ = notifService.EnqueueOrCreate(notifyDefaultTeacherIDs, payload)
	}
	if len(notifyParticipantIDs) > 0 {
		data := fiber.Map{
			"link": fiber.Map{
				"href":   fmt.Sprintf("/api/schedules/%d", schedule.ID),
				"method": "GET",
			},
			"action":      "confirm-participation",
			"schedule_id": schedule.ID,
		}
		payload := notifsvc.QueuedWithData(
			"Schedule invitation",
			"คำเชิญเข้าร่วมตาราง",
			fmt.Sprintf("You were invited to schedule: %s.", req.ScheduleName),
			fmt.Sprintf("คุณได้รับคำเชิญให้เข้าร่วมตาราง: %s", req.ScheduleName),
			"info", data,
			"popup", "normal",
		)
		_ = notifService.EnqueueOrCreate(notifyParticipantIDs, payload)
	}

	database.DB.Preload("Group.Course").Preload("DefaultTeacher").Preload("DefaultRoom").Preload("CreatedBy").First(&schedule, schedule.ID)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":  "Schedule created successfully",
		"schedule": schedule,
	})
}

// CheckRoomConflicts pre-validates if a room has scheduling conflicts for a proposed session plan.
func (sc *ScheduleController) CheckRoomConflicts(c *fiber.Ctx) error {
	userRole := c.Locals("role")
	if userRole != "admin" && userRole != "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admin and owner can check room conflicts"})
	}

	var req CheckRoomConflictRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	roomIDSet := make(map[uint]struct{})
	if req.RoomID != 0 {
		roomIDSet[req.RoomID] = struct{}{}
	}
	for _, id := range req.RoomIDs {
		if id != 0 {
			roomIDSet[id] = struct{}{}
		}
	}

	if len(roomIDSet) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "at least one room_id must be provided"})
	}

	roomIDs := make([]uint, 0, len(roomIDSet))
	for id := range roomIDSet {
		roomIDs = append(roomIDs, id)
	}
	sort.Slice(roomIDs, func(i, j int) bool { return roomIDs[i] < roomIDs[j] })

	var rooms []models.Room
	if err := database.DB.Where("id IN ?", roomIDs).Find(&rooms).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("failed to fetch rooms: %v", err)})
	}
	roomsByID := make(map[uint]models.Room, len(rooms))
	for _, room := range rooms {
		roomsByID[room.ID] = room
	}
	if len(roomsByID) != len(roomIDs) {
		missing := make([]string, 0)
		for _, id := range roomIDs {
			if _, ok := roomsByID[id]; !ok {
				missing = append(missing, fmt.Sprintf("%d", id))
			}
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("room_id(s) not found: %s", strings.Join(missing, ", "))})
	}

	if req.HoursPerSession <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "hours_per_session must be greater than zero"})
	}
	if req.TotalHours <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "total_hours must be greater than zero"})
	}
	if req.TotalHours%req.HoursPerSession != 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "total_hours must be divisible by hours_per_session"})
	}
	if req.SessionPerWeek <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_per_week must be greater than zero"})
	}

	pattern := strings.TrimSpace(req.RecurringPattern)
	if pattern == "" {
		pattern = "weekly"
	}

	branchHours := services.BranchHours{OpenMinutes: 8 * 60, CloseMinutes: 21 * 60}
	if req.BranchID != nil {
		var branch models.Branch
		if err := database.DB.First(&branch, *req.BranchID).Error; err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Branch not found"})
		}
		openMinutes, closeMinutes, err := resolveBranchHours(&branch)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		branchHours = services.BranchHours{OpenMinutes: openMinutes, CloseMinutes: closeMinutes}
	} else if primaryRoom, ok := roomsByID[roomIDs[0]]; ok && primaryRoom.BranchID != 0 {
		var branch models.Branch
		if err := database.DB.First(&branch, primaryRoom.BranchID).Error; err == nil {
			if openMinutes, closeMinutes, err := resolveBranchHours(&branch); err == nil {
				branchHours = services.BranchHours{OpenMinutes: openMinutes, CloseMinutes: closeMinutes}
			}
		}
	}

	sessionSlots := make([]services.SessionSlot, 0, len(req.SessionTimes))
	if len(req.SessionTimes) > 0 {
		if req.SessionPerWeek != len(req.SessionTimes) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_per_week must match the number of session_times provided"})
		}

		seenWeekdays := make(map[int]struct{}, len(req.SessionTimes))
		for _, slot := range req.SessionTimes {
			if slot.Weekday < 0 || slot.Weekday > 6 {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_times.weekday must be between 0 (Sunday) and 6 (Saturday)"})
			}
			if _, exists := seenWeekdays[slot.Weekday]; exists {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_times contains duplicate weekdays"})
			}

			hour, minute, err := parseHourMinute(slot.StartTime)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("invalid start_time in session_times: %v", err)})
			}

			startMinutes := hour*60 + minute
			endMinutes := startMinutes + req.HoursPerSession*60
			if startMinutes < branchHours.OpenMinutes || endMinutes > branchHours.CloseMinutes {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("session on weekday %d (%02d:%02d) is outside branch operating hours", slot.Weekday, hour, minute)})
			}

			seenWeekdays[slot.Weekday] = struct{}{}
			sessionSlots = append(sessionSlots, services.SessionSlot{
				Weekday:     time.Weekday(slot.Weekday),
				StartHour:   hour,
				StartMinute: minute,
			})
		}
		pattern = "custom"
	}

	if len(sessionSlots) == 0 && strings.TrimSpace(req.SessionStartTime) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_start_time is required when session_times are not provided"})
	}

	if len(sessionSlots) == 0 {
		hour, minute, err := parseHourMinute(req.SessionStartTime)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("invalid session_start_time: %v", err)})
		}
		startMinutes := hour*60 + minute
		endMinutes := startMinutes + req.HoursPerSession*60
		if startMinutes < branchHours.OpenMinutes || endMinutes > branchHours.CloseMinutes {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_start_time is outside branch operating hours"})
		}
	}

	totalSessions := req.TotalHours / req.HoursPerSession
	schedule := models.Schedules{
		Recurring_pattern:  pattern,
		Total_hours:        req.TotalHours,
		Hours_per_session:  req.HoursPerSession,
		Session_per_week:   req.SessionPerWeek,
		Start_date:         req.StartDate,
		Estimated_end_date: req.EstimatedEndDate,
	}

	var (
		candidateSessions []models.Schedule_Sessions
		err               error
	)
	if len(sessionSlots) > 0 {
		candidateSessions, err = services.GenerateScheduleSessionsWithSlots(schedule, sessionSlots, totalSessions, req.HoursPerSession, branchHours)
	} else {
		candidateSessions, err = services.GenerateScheduleSessions(schedule, req.SessionStartTime, req.CustomRecurringDays)
	}
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	allDetails := make([]RoomConflictDetail, 0)
	perRoom := make([]RoomConflictSummary, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		conflicts, err := findRoomConflicts(roomID, candidateSessions, req.ExcludeScheduleID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("failed to check room conflicts for room %d: %v", roomID, err)})
		}

		roomDetails := make([]RoomConflictDetail, 0, len(conflicts))
		for _, conflict := range conflicts {
			roomDetails = append(roomDetails, RoomConflictDetail{
				RoomID:       roomID,
				ExistingRoom: conflict.RoomID,
				SessionID:    conflict.SessionID,
				ScheduleID:   conflict.ScheduleID,
				ScheduleName: conflict.ScheduleName,
				SessionDate:  formatSessionDate(conflict.SessionDate),
				StartTime:    formatSessionTime(conflict.StartTime),
				EndTime:      formatSessionTime(conflict.EndTime),
			})
		}

		perRoom = append(perRoom, RoomConflictSummary{
			RoomID:    roomID,
			Conflicts: roomDetails,
		})
		allDetails = append(allDetails, roomDetails...)
	}

	return c.JSON(fiber.Map{
		"has_conflict":     len(allDetails) > 0,
		"conflicts":        allDetails,
		"rooms":            perRoom,
		"checked_room_ids": roomIDs,
	})
}

// PreviewSchedule performs a dry-run validation of a schedule configuration and reports potential issues.
func (sc *ScheduleController) PreviewSchedule(c *fiber.Ctx) error {
	userRole := c.Locals("role")
	if userRole != "admin" && userRole != "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Only admin and owner can preview schedules"})
	}

	var req CreateScheduleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if req.GroupID != nil && *req.GroupID == 0 {
		req.GroupID = nil
	}
	if req.DefaultTeacherID != nil && *req.DefaultTeacherID == 0 {
		req.DefaultTeacherID = nil
	}
	if req.DefaultRoomID != nil && *req.DefaultRoomID == 0 {
		req.DefaultRoomID = nil
	}

	autoReschedule := req.AutoRescheduleHoliday || req.AutoRescheduleHolidaysAlias
	issues := make([]SchedulePreviewIssue, 0)
	blocking := false
	canProceed := true

	addIssue := func(severity, code, message string, details interface{}, fatal bool) {
		issues = append(issues, SchedulePreviewIssue{
			Severity: severity,
			Code:     code,
			Message:  message,
			Details:  details,
		})
		if severity == "error" {
			blocking = true
		}
		if fatal {
			canProceed = false
		}
	}

	scheduleType := strings.ToLower(strings.TrimSpace(req.ScheduleType))
	if scheduleType == "" {
		addIssue("error", "missing_schedule_type", "schedule_type is required", nil, true)
	} else {
		supportedTypes := map[string]bool{"class": true, "meeting": true, "event": true, "holiday": true, "appointment": true}
		if !supportedTypes[scheduleType] {
			addIssue("error", "invalid_schedule_type", fmt.Sprintf("unsupported schedule_type: %s", req.ScheduleType), nil, true)
		}
	}

	if strings.TrimSpace(req.ScheduleName) == "" {
		addIssue("error", "missing_schedule_name", "schedule_name is required", nil, false)
	}

	if req.TotalHours <= 0 {
		addIssue("error", "invalid_total_hours", "total_hours must be greater than zero", nil, true)
	}
	if req.HoursPerSession <= 0 {
		addIssue("error", "invalid_hours_per_session", "hours_per_session must be greater than zero", nil, true)
	}
	if req.SessionPerWeek <= 0 {
		addIssue("error", "invalid_session_per_week", "session_per_week must be greater than zero", nil, true)
	}
	if req.TotalHours > 0 && req.HoursPerSession > 0 && req.TotalHours%req.HoursPerSession != 0 {
		addIssue("error", "total_hours_not_divisible", "total_hours must be divisible by hours_per_session", nil, true)
	}

	if req.StartDate.IsZero() || req.EstimatedEndDate.IsZero() {
		addIssue("error", "missing_dates", "start_date and estimated_end_date are required", nil, false)
	}

	branchHours := services.BranchHours{OpenMinutes: 8 * 60, CloseMinutes: 21 * 60}
	var branch *models.Branch
	var group *models.Group
	var paymentSummary *GroupPaymentSummary

	if scheduleType == "class" {
		if req.GroupID == nil {
			addIssue("error", "missing_group_id", "group_id is required for class schedules", nil, true)
		} else {
			var g models.Group
			if err := database.DB.Preload("Members.Student.User").Preload("Course.Branch").First(&g, *req.GroupID).Error; err != nil {
				addIssue("error", "group_not_found", fmt.Sprintf("group %d not found", *req.GroupID), nil, true)
			} else {
				group = &g
				summary := buildGroupPaymentSummary(group)
				paymentSummary = &summary
				if summary.EligibleMembers == 0 {
					addIssue("error", "insufficient_payment", "group must have at least one member with deposit paid or fully paid status", summary, false)
				} else if summary.MemberTotals["pending"] > 0 {
					addIssue("warning", "group_payment_pending", "some group members still have pending payment status", summary, false)
				}
				if group.Course.Branch.ID != 0 {
					branch = &group.Course.Branch
				}
			}
		}
	} else {
		if len(req.ParticipantUserIDs) == 0 {
			addIssue("warning", "missing_participants", "participant_user_ids were not provided; they are required when the schedule is created", nil, false)
		}
	}

	if branch != nil {
		if open, close, err := resolveBranchHours(branch); err != nil {
			addIssue("error", "invalid_branch_hours", err.Error(), nil, true)
		} else {
			branchHours = services.BranchHours{OpenMinutes: open, CloseMinutes: close}
		}
	}

	if req.DefaultTeacherID != nil {
		var teacher models.User
		if err := database.DB.Where("id = ? AND role IN ?", *req.DefaultTeacherID, []string{"teacher", "admin", "owner"}).First(&teacher).Error; err != nil {
			addIssue("error", "invalid_default_teacher", "default teacher not found or not authorized", nil, false)
		}
	}

	totalSessions := 0
	if req.TotalHours > 0 && req.HoursPerSession > 0 {
		totalSessions = req.TotalHours / req.HoursPerSession
	}
	if totalSessions <= 0 {
		addIssue("error", "no_sessions", "unable to derive session count from provided hours", nil, true)
	}

	pattern := strings.TrimSpace(req.RecurringPattern)

	sessionSlots := make([]services.SessionSlot, 0, len(req.SessionTimes))
	if len(req.SessionTimes) > 0 {
		if req.SessionPerWeek != len(req.SessionTimes) {
			addIssue("error", "session_times_mismatch", "session_per_week must match the number of session_times provided", nil, true)
		}

		seenWeekdays := make(map[int]struct{}, len(req.SessionTimes))
		for _, slot := range req.SessionTimes {
			if slot.Weekday < 0 || slot.Weekday > 6 {
				addIssue("error", "invalid_weekday", "session_times.weekday must be between 0 (Sunday) and 6 (Saturday)", slot, true)
				continue
			}
			if _, exists := seenWeekdays[slot.Weekday]; exists {
				addIssue("error", "duplicate_weekday", fmt.Sprintf("session_times contains duplicate weekday %d", slot.Weekday), slot, true)
				continue
			}

			hour, minute, err := parseHourMinute(slot.StartTime)
			if err != nil {
				addIssue("error", "invalid_session_time", fmt.Sprintf("invalid start_time in session_times: %v", err), slot, true)
				continue
			}

			startMinutes := hour*60 + minute
			endMinutes := startMinutes + req.HoursPerSession*60
			if startMinutes < branchHours.OpenMinutes || endMinutes > branchHours.CloseMinutes {
				addIssue("error", "session_outside_branch", fmt.Sprintf("session on weekday %d (%02d:%02d) is outside branch operating hours", slot.Weekday, hour, minute), slot, true)
				continue
			}

			seenWeekdays[slot.Weekday] = struct{}{}
			sessionSlots = append(sessionSlots, services.SessionSlot{
				Weekday:     time.Weekday(slot.Weekday),
				StartHour:   hour,
				StartMinute: minute,
			})
		}
		pattern = "custom"
	}

	if len(sessionSlots) == 0 {
		if strings.TrimSpace(req.SessionStartTime) == "" {
			addIssue("error", "missing_session_start_time", "session_start_time is required when session_times are not provided", nil, true)
		}
		if strings.TrimSpace(req.SessionStartTime) != "" {
			hour, minute, err := parseHourMinute(req.SessionStartTime)
			if err != nil {
				addIssue("error", "invalid_session_start_time", fmt.Sprintf("invalid session_start_time: %v", err), nil, true)
			} else {
				startMinutes := hour*60 + minute
				endMinutes := startMinutes + req.HoursPerSession*60
				if startMinutes < branchHours.OpenMinutes || endMinutes > branchHours.CloseMinutes {
					addIssue("error", "session_start_outside_branch", "session_start_time is outside branch operating hours", nil, true)
				}
			}
		}
		if len(sessionSlots) == 0 && strings.TrimSpace(pattern) == "" {
			pattern = "weekly"
		}
	}

	schedule := models.Schedules{
		ScheduleName:            req.ScheduleName,
		ScheduleType:            req.ScheduleType,
		GroupID:                 req.GroupID,
		Recurring_pattern:       pattern,
		Total_hours:             req.TotalHours,
		Hours_per_session:       req.HoursPerSession,
		Session_per_week:        req.SessionPerWeek,
		Start_date:              req.StartDate,
		Estimated_end_date:      req.EstimatedEndDate,
		DefaultTeacherID:        req.DefaultTeacherID,
		DefaultRoomID:           req.DefaultRoomID,
		Auto_Reschedule_holiday: autoReschedule,
	}

	var generatedSessions []models.Schedule_Sessions
	originalSessions := make([]models.Schedule_Sessions, 0)
	holidayImpacts := make([]HolidayImpact, 0)
	conflictReport := &ScheduleConflictReport{}
	sessionsGenerated := false
	minDate := req.StartDate
	maxDate := req.EstimatedEndDate
	holidayDates := make([]time.Time, 0)

	if canProceed && totalSessions > 0 {
		var err error
		if len(sessionSlots) > 0 {
			generatedSessions, err = services.GenerateScheduleSessionsWithSlots(schedule, sessionSlots, totalSessions, req.HoursPerSession, branchHours)
		} else {
			generatedSessions, err = services.GenerateScheduleSessions(schedule, req.SessionStartTime, req.CustomRecurringDays)
		}
		if err != nil {
			addIssue("error", "session_generation_failed", err.Error(), nil, true)
		}

		if len(generatedSessions) == 0 {
			addIssue("error", "no_sessions_generated", "no sessions generated for the provided configuration", nil, true)
		} else {
			sessionsGenerated = true
			originalSessions = copySessions(generatedSessions)
			minDate, maxDate = getSessionDateRange(generatedSessions)
			if minDate.IsZero() {
				minDate = req.StartDate
			}
			if maxDate.IsZero() {
				maxDate = req.EstimatedEndDate
			}

			startYear := minDate.Year()
			if startYear > req.StartDate.Year() {
				startYear = req.StartDate.Year()
			}
			endYear := maxDate.Year()
			if endYear < req.EstimatedEndDate.Year() {
				endYear = req.EstimatedEndDate.Year()
			}

			holidayNames := make(map[string]string)
			if names, err := services.GetThaiHolidaysWithNames(startYear, endYear); err == nil {
				holidayNames = names
				for dateStr := range names {
					if date, err := time.Parse("2006-01-02", dateStr); err == nil {
						holidayDates = append(holidayDates, date)
					}
				}
			} else {
				addIssue("warning", "holiday_lookup_failed", fmt.Sprintf("failed to fetch holiday list: %v", err), nil, false)
			}

			// Build mapping from original date to rescheduled date BEFORE reindexing
			originalToRescheduledDate := make(map[string]string)
			if autoReschedule && len(holidayDates) > 0 {
				// Create mapping before reschedule to track which dates moved
				for _, origSession := range originalSessions {
					origDate := formatSessionDate(origSession.Session_date)
					if origDate != "" {
						originalToRescheduledDate[origDate] = origDate // Initially map to itself
					}
				}

				generatedSessions = services.RescheduleSessions(generatedSessions, holidayDates)

				// Update mapping after reschedule to show new dates
				holidayMap := make(map[string]bool)
				for _, h := range holidayDates {
					holidayMap[h.Format("2006-01-02")] = true
				}

				// Map rescheduled sessions by their position
				rescheduledIdx := 0
				for _, origSession := range originalSessions {
					origDate := formatSessionDate(origSession.Session_date)
					if origDate != "" && !holidayMap[origDate] {
						// Non-holiday session keeps its date (but might get new number)
						if rescheduledIdx < len(generatedSessions) {
							newDate := formatSessionDate(generatedSessions[rescheduledIdx].Session_date)
							originalToRescheduledDate[origDate] = newDate
							rescheduledIdx++
						}
					}
				}

				// Holiday sessions are appended at the end
				for _, origSession := range originalSessions {
					origDate := formatSessionDate(origSession.Session_date)
					if origDate != "" && holidayMap[origDate] {
						if rescheduledIdx < len(generatedSessions) {
							newDate := formatSessionDate(generatedSessions[rescheduledIdx].Session_date)
							originalToRescheduledDate[origDate] = newDate
							rescheduledIdx++
						}
					}
				}
			}

			services.ReindexSessions(generatedSessions, schedule.Start_date)
			newMin, newMax := getSessionDateRange(generatedSessions)
			if !newMin.IsZero() {
				minDate = newMin
			}
			if !newMax.IsZero() {
				maxDate = newMax
				schedule.Estimated_end_date = newMax
			}

			for _, session := range originalSessions {
				date := formatSessionDate(session.Session_date)
				if date == "" {
					continue
				}
				if holidayName, ok := holidayNames[date]; ok {
					impact := HolidayImpact{
						SessionNumber:  session.Session_number,
						Date:           date,
						HolidayName:    holidayName,
						WasRescheduled: autoReschedule,
					}
					if autoReschedule {
						if newDate, exists := originalToRescheduledDate[date]; exists && newDate != date {
							impact.ShiftedTo = newDate
						} else if !exists || newDate == date {
							impact.WasRescheduled = false
						}
					}
					holidayImpacts = append(holidayImpacts, impact)
				}
			}

			conflictOpts := conflictReportOptions{IncludeStudentConflicts: scheduleType == "class"}
			if report, err := generateScheduleConflictReport(req, group, generatedSessions, minDate, maxDate, nil, conflictOpts); err != nil {
				addIssue("error", "conflict_check_failed", fmt.Sprintf("failed to evaluate conflicts: %v", err), nil, false)
			} else if report != nil {
				conflictReport = report
			}
		}
	}

	if conflictReport != nil {
		if conflictReport.GroupConflict != nil {
			addIssue("error", "group_active_schedule", "group already has an active class schedule", conflictReport.GroupConflict, false)
		}
		for _, roomSummary := range conflictReport.RoomConflicts {
			if len(roomSummary.Conflicts) > 0 {
				addIssue("error", "room_conflict", fmt.Sprintf("room %d has %d conflicting session(s)", roomSummary.RoomID, len(roomSummary.Conflicts)), roomSummary, false)
			}
		}
		for _, teacherDetail := range conflictReport.TeacherConflicts {
			if len(teacherDetail.Conflicts) > 0 {
				addIssue("error", "teacher_conflict", fmt.Sprintf("teacher %s has %d conflicting session(s)", teacherDetail.TeacherName, len(teacherDetail.Conflicts)), teacherDetail, false)
			}
		}
		for _, participantDetail := range conflictReport.ParticipantConflicts {
			if len(participantDetail.Conflicts) > 0 {
				addIssue("error", "participant_conflict", fmt.Sprintf("participant %d has %d conflicting session(s)", participantDetail.UserID, len(participantDetail.Conflicts)), participantDetail, false)
			}
		}
		for _, studentDetail := range conflictReport.StudentConflicts {
			if len(studentDetail.Conflicts) > 0 {
				addIssue("warning", "student_conflict", fmt.Sprintf("student %d appears in %d conflicting session(s)", studentDetail.StudentID, len(studentDetail.Conflicts)), studentDetail, false)
			}
		}
	}

	if len(holidayImpacts) > 0 && !autoReschedule {
		addIssue("warning", "holiday_overlap", "some sessions fall on Thai public holidays", holidayImpacts, false)
	}

	canCreate := !blocking
	sessionPreview := make([]SessionPreview, 0)
	originalPreview := make([]SessionPreview, 0)
	if sessionsGenerated {
		sessionPreview = sessionsToPreview(generatedSessions)
		originalPreview = sessionsToPreview(originalSessions)
	}

	branchInfo := fiber.Map{
		"open_minutes":  branchHours.OpenMinutes,
		"close_minutes": branchHours.CloseMinutes,
	}
	formatMinutes := func(minutes int) string {
		if minutes < 0 {
			return ""
		}
		hours := minutes / 60
		mins := minutes % 60
		return fmt.Sprintf("%02d:%02d", hours, mins)
	}
	branchInfo["open_time"] = formatMinutes(branchHours.OpenMinutes)
	branchInfo["close_time"] = formatMinutes(branchHours.CloseMinutes)

	conflictsMap := fiber.Map{}
	if conflictReport != nil {
		conflictsMap["group"] = conflictReport.GroupConflict
		conflictsMap["rooms"] = conflictReport.RoomConflicts
		conflictsMap["teachers"] = conflictReport.TeacherConflicts
		conflictsMap["participants"] = conflictReport.ParticipantConflicts
		conflictsMap["students"] = conflictReport.StudentConflicts
	}

	checkedRoomIDs := make([]uint, 0)
	if req.DefaultRoomID != nil && *req.DefaultRoomID != 0 {
		checkedRoomIDs = append(checkedRoomIDs, *req.DefaultRoomID)
	}

	summary := fiber.Map{
		"schedule_name":      req.ScheduleName,
		"schedule_type":      scheduleType,
		"start_date":         formatSessionDate(&schedule.Start_date),
		"estimated_end_date": formatSessionDate(&schedule.Estimated_end_date),
		"total_hours":        req.TotalHours,
		"hours_per_session":  req.HoursPerSession,
		"session_per_week":   req.SessionPerWeek,
		"total_sessions":     len(sessionPreview),
	}

	response := fiber.Map{
		"can_create":        canCreate,
		"issues":            issues,
		"summary":           summary,
		"sessions":          sessionPreview,
		"original_sessions": originalPreview,
		"holiday_impacts":   holidayImpacts,
		"conflicts":         conflictsMap,
		"group_payment":     paymentSummary,
		"auto_reschedule":   autoReschedule,
		"branch_hours":      branchInfo,
		"checked_room_ids":  checkedRoomIDs,
	}

	return c.JSON(response)
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
			Joins("JOIN `groups` ON `groups`.id = schedules.group_id").
			Joins("JOIN group_members ON group_members.group_id = `groups`.id").
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

	// Sanitize potential duplicate/comma-joined query values (e.g., date_filter=day,day)
	sanitize := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return s
		}
		// If multiple values are joined by comma, take the first
		if idx := strings.IndexByte(s, ','); idx >= 0 {
			s = s[:idx]
		}
		return strings.TrimSpace(s)
	}
	dateFilter = strings.ToLower(sanitize(dateFilter))
	dateStr = sanitize(dateStr)
	startDate = sanitize(startDate)
	endDate = sanitize(endDate)
	branchID = sanitize(branchID)

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
			Joins("LEFT JOIN `groups` ON schedules.group_id = `groups`.id").
			Joins("LEFT JOIN courses ON `groups`.course_id = courses.id").
			Where("courses.branch_id = ?", branchID)
	}

	if err := sessionsQuery.Find(&sessions).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch sessions"})
	}

	// Load participants for non-class schedules referenced by these sessions
	participantsBySchedule := make(map[uint][]map[string]interface{})
	{
		// Collect distinct schedule IDs
		idsSet := make(map[uint]struct{})
		for _, s := range sessions {
			if s.ScheduleID != 0 {
				idsSet[s.ScheduleID] = struct{}{}
			}
		}
		if len(idsSet) > 0 {
			ids := make([]uint, 0, len(idsSet))
			for id := range idsSet {
				ids = append(ids, id)
			}
			var parts []models.ScheduleParticipant
			if err := database.DB.Where("schedule_id IN ?", ids).Preload("User").Find(&parts).Error; err == nil {
				for _, p := range parts {
					participantsBySchedule[p.ScheduleID] = append(participantsBySchedule[p.ScheduleID], map[string]interface{}{
						"user_id": p.UserID,
						"role":    p.Role,
						"status":  p.Status,
						"user": map[string]interface{}{
							"id":       p.User.ID,
							"username": p.User.Username,
							"avatar":   p.User.Avatar,
						},
					})
				}
			}
		}
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

			// Attach participants for non-class schedules
			if s.Schedule != nil && s.Schedule.ScheduleType != "class" {
				if plist, ok := participantsBySchedule[s.ScheduleID]; ok {
					sessMap["participants"] = plist
				} else {
					sessMap["participants"] = make([]map[string]interface{}, 0)
				}
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

		// Attach participants for non-class schedules
		if s.Schedule != nil && s.Schedule.ScheduleType != "class" {
			if plist, ok := participantsBySchedule[s.ScheduleID]; ok {
				sessMap["participants"] = plist
			} else {
				sessMap["participants"] = make([]map[string]interface{}, 0)
			}
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
			Joins("JOIN `groups` ON schedules.group_id = `groups`.id").
			Joins("JOIN group_members ON group_members.group_id = `groups`.id").
			Joins("JOIN students ON students.id = group_members.student_id").
			Where("students.user_id = ?", userID)
	}

	// Admin/Owner can filter by branch
	if (userRole == "admin" || userRole == "owner") && branchID != "" {
		query = query.Joins("JOIN schedules ON schedule_sessions.schedule_id = schedules.id").
			Joins("LEFT JOIN `groups` ON schedules.group_id = `groups`.id").
			Joins("LEFT JOIN courses ON `groups`.course_id = courses.id").
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

	// Preload participants for non-class schedules referenced in these sessions
	participantsBySchedule := make(map[uint][]map[string]interface{})
	{
		idsSet := make(map[uint]struct{})
		for _, s := range sessions {
			if s.ScheduleID != 0 {
				idsSet[s.ScheduleID] = struct{}{}
			}
		}
		if len(idsSet) > 0 {
			ids := make([]uint, 0, len(idsSet))
			for id := range idsSet {
				ids = append(ids, id)
			}
			var parts []models.ScheduleParticipant
			if err := database.DB.Where("schedule_id IN ?", ids).Preload("User").Find(&parts).Error; err == nil {
				for _, p := range parts {
					participantsBySchedule[p.ScheduleID] = append(participantsBySchedule[p.ScheduleID], map[string]interface{}{
						"user_id": p.UserID,
						"role":    p.Role,
						"status":  p.Status,
						"user": map[string]interface{}{
							"id":       p.User.ID,
							"username": p.User.Username,
							"avatar":   p.User.Avatar,
						},
					})
				}
			}
		}
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
			// Class schedule - participants are group members
			for _, member := range session.Schedule.Group.Members {
				participants = append(participants, map[string]interface{}{
					"id":             member.Student.ID,
					"name":           fmt.Sprintf("%s %s", member.Student.FirstName, member.Student.LastName),
					"nickname":       member.Student.NicknameEn,
					"payment_status": member.PaymentStatus,
				})
			}
		} else {
			// Non-class schedule - participants from ScheduleParticipant
			if plist, ok := participantsBySchedule[session.ScheduleID]; ok {
				participants = plist
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

		// Set event type for non-class schedules
		if session.Schedule != nil && session.Schedule.Group == nil && session.Schedule.ScheduleType != "" {
			event["type"] = session.Schedule.ScheduleType
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

// GetScheduleDetail - ดูรายละเอียด schedule แบบ normalize + preload ครบ
func (sc *ScheduleController) GetScheduleDetail(c *fiber.Ctx) error {
	idParam := c.Params("id")
	sid, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil || sid == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid schedule ID"})
	}

	// Load schedule with rich preloads
	var s models.Schedules
	if err := database.DB.Preload("Group").
		Preload("Group.Course").
		Preload("Group.Members").
		Preload("Group.Members.Student").
		Preload("CreatedBy").
		Preload("DefaultTeacher").
		Preload("DefaultRoom").
		Preload("Sessions").
		Preload("Sessions.AssignedTeacher").
		Preload("Sessions.Room").
		First(&s, uint(sid)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
	}

	// Load participants for non-class schedule
	participants := make([]models.ScheduleParticipant, 0)
	if strings.ToLower(s.ScheduleType) != "class" {
		if err := database.DB.Where("schedule_id = ?", s.ID).Preload("User").Find(&participants).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load participants"})
		}
	}

	// Build DTO
	var createdBy *utils.UserBasic
	if s.CreatedBy != nil {
		cb := utils.UserBasic{ID: s.CreatedBy.ID, Username: s.CreatedBy.Username, Avatar: s.CreatedBy.Avatar}
		createdBy = &cb
	}
	var defTeacher *utils.UserBasic
	if s.DefaultTeacher != nil {
		dt := utils.UserBasic{ID: s.DefaultTeacher.ID, Username: s.DefaultTeacher.Username, Avatar: s.DefaultTeacher.Avatar}
		defTeacher = &dt
	}
	var defRoom *utils.RoomShort
	if s.DefaultRoom != nil && s.DefaultRoom.ID != 0 {
		id := s.DefaultRoom.ID
		dr := utils.RoomShort{ID: &id, Name: s.DefaultRoom.RoomName}
		defRoom = &dr
	}

	// Group DTO (only for class)
	var gdto *utils.GroupDTO
	if s.Group != nil {
		g := *s.Group
		// ensure Course and Members.Student were preloaded
		g.Course = s.Group.Course
		g.Members = s.Group.Members
		tmp := utils.ToGroupDTO(g)
		gdto = &tmp
	}

	// Sessions DTOs
	sDTOs := make([]utils.SessionDTO, 0, len(s.Sessions))
	for _, sess := range s.Sessions {
		sDTOs = append(sDTOs, utils.ToSessionDTO(sess, s.DefaultTeacher, s.DefaultRoom))
	}

	// Participants DTOs
	pDTOs := make([]utils.ParticipantDTO, 0, len(participants))
	for _, p := range participants {
		pDTOs = append(pDTOs, utils.ToParticipantDTO(p))
	}

	resp := utils.ScheduleDetailDTO{
		ID:               s.ID,
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
		ScheduleName:     s.ScheduleName,
		ScheduleType:     s.ScheduleType,
		Status:           s.Status,
		RecurringPattern: s.Recurring_pattern,
		TotalHours:       s.Total_hours,
		HoursPerSession:  s.Hours_per_session,
		SessionPerWeek:   s.Session_per_week,
		StartDate:        s.Start_date,
		EstimatedEndDate: s.Estimated_end_date,
		Notes:            s.Notes,
		AutoReschedule:   s.Auto_Reschedule_holiday,
		CreatedBy:        createdBy,
		DefaultTeacher:   defTeacher,
		DefaultRoom:      defRoom,
		Group:            gdto,
		Participants:     pDTOs,
		Sessions:         sDTOs,
	}

	return c.JSON(fiber.Map{"success": true, "data": resp})
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
		"status":               "scheduled",
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

// checkScheduleConflicts ตรวจสอบการชนกันของตารางสำหรับการสร้างใหม่
func checkScheduleConflicts(req CreateScheduleRequest, userID uint, candidateSessions []models.Schedule_Sessions) error {
	scheduleType := strings.ToLower(req.ScheduleType)

	minDate, maxDate := getSessionDateRange(candidateSessions)
	if minDate.IsZero() {
		minDate = req.StartDate
	}
	if maxDate.IsZero() {
		maxDate = req.EstimatedEndDate
	}
	if maxDate.Before(minDate) {
		maxDate = minDate
	}

	report, err := generateScheduleConflictReport(req, nil, candidateSessions, minDate, maxDate, nil, conflictReportOptions{})
	if err != nil {
		return fmt.Errorf("failed to evaluate conflicts: %v", err)
	}
	if report == nil {
		return nil
	}

	if scheduleType == "class" && req.GroupID != nil && report.GroupConflict != nil {
		return fmt.Errorf("group already has an active class schedule (ID: %d)", report.GroupConflict.ScheduleID)
	}

	if len(report.ParticipantConflicts) > 0 {
		conflict := report.ParticipantConflicts[0]
		slot := conflict.Conflicts[0]
		return fmt.Errorf("participant %s already has a conflicting session at %s-%s",
			conflict.Username,
			slot.StartTime,
			slot.EndTime)
	}

	if len(report.TeacherConflicts) > 0 {
		conflict := report.TeacherConflicts[0]
		slot := conflict.Conflicts[0]
		name := conflict.TeacherName
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("ID %d", conflict.TeacherID)
		}
		return fmt.Errorf("teacher %s already has a conflicting session at %s-%s",
			name,
			slot.StartTime,
			slot.EndTime)
	}

	if len(report.RoomConflicts) > 0 {
		room := report.RoomConflicts[0]
		if len(room.Conflicts) > 0 {
			slot := room.Conflicts[0]
			return fmt.Errorf("room already booked at %s-%s", slot.StartTime, slot.EndTime)
		}
	}

	return nil
}

// UpdateMyParticipationStatus - participant updates their status for a non-class schedule
func (sc *ScheduleController) UpdateMyParticipationStatus(c *fiber.Ctx) error {
	// Parse params and auth
	sidParam := c.Params("id")
	sid, err := strconv.ParseUint(sidParam, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid schedule ID"})
	}
	userID := c.Locals("user_id").(uint)

	// Load schedule
	var schedule models.Schedules
	if err := database.DB.First(&schedule, uint(sid)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
	}

	// Only non-class schedules support participant status
	if schedule.ScheduleType == "class" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Participation status is not applicable to class schedules"})
	}

	// Parse request body
	var req struct {
		Status string `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	switch req.Status {
	case "confirmed", "declined", "tentative":
		// ok
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid status. Use confirmed, declined, or tentative"})
	}

	// Find participant record
	var participant models.ScheduleParticipant
	if err := database.DB.Where("schedule_id = ? AND user_id = ?", uint(sid), userID).First(&participant).Error; err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You are not a participant of this schedule"})
	}

	// Update status
	if err := database.DB.Model(&participant).Update("status", req.Status).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update participation status"})
	}

	// Reload with user for response
	database.DB.Preload("User").First(&participant, participant.ID)

	return c.JSON(fiber.Map{
		"message":     "Participation status updated",
		"participant": participant,
		"schedule_id": uint(sid),
		"new_status":  req.Status,
	})
}

// AddSessionToSchedule - add a new session into an existing schedule
func (sc *ScheduleController) AddSessionToSchedule(c *fiber.Ctx) error {
	// Auth
	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)

	// Parse schedule id
	sidParam := c.Params("id")
	sid, err := strconv.ParseUint(sidParam, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid schedule ID"})
	}

	// Load schedule
	var schedule models.Schedules
	if err := database.DB.First(&schedule, uint(sid)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Schedule not found"})
	}

	// Authorization: admin/owner or default teacher of the schedule
	if !(role == "admin" || role == "owner" || (schedule.DefaultTeacherID != nil && *schedule.DefaultTeacherID == userID)) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You are not authorized to add a session to this schedule"})
	}

	// Parse request
	var req struct {
		Date              string `json:"date"`       // "2006-01-02"
		StartTime         string `json:"start_time"` // "15:04"
		EndTime           string `json:"end_time"`   // optional, "15:04"
		DurationHours     *int   `json:"hours"`      // optional, fallback to schedule.Hours_per_session
		AssignedTeacherID *uint  `json:"assigned_teacher_id"`
		RoomID            *uint  `json:"room_id"`
		Notes             string `json:"notes"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if strings.TrimSpace(req.Date) == "" || strings.TrimSpace(req.StartTime) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "date and start_time are required"})
	}

	// Timezone Asia/Bangkok
	loc, _ := time.LoadLocation("Asia/Bangkok")

	// Parse date and time
	d, err := time.ParseInLocation("2006-01-02", req.Date, loc)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid date format (use YYYY-MM-DD)"})
	}
	st, err := time.ParseInLocation("15:04", req.StartTime, loc)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid start_time format (use HH:MM)"})
	}
	// Compose full start datetime
	startDT := time.Date(d.Year(), d.Month(), d.Day(), st.Hour(), st.Minute(), 0, 0, loc)

	var endDT time.Time
	if strings.TrimSpace(req.EndTime) != "" {
		et, err := time.ParseInLocation("15:04", req.EndTime, loc)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid end_time format (use HH:MM)"})
		}
		endDT = time.Date(d.Year(), d.Month(), d.Day(), et.Hour(), et.Minute(), 0, 0, loc)
		if !endDT.After(startDT) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "end_time must be after start_time"})
		}
	} else {
		// Use duration hours
		durHrs := schedule.Hours_per_session
		if req.DurationHours != nil && *req.DurationHours > 0 {
			durHrs = *req.DurationHours
		}
		if durHrs <= 0 {
			durHrs = 1
		}
		endDT = startDT.Add(time.Duration(durHrs) * time.Hour)
	}

	// Compute next session number
	var maxNo int
	database.DB.Model(&models.Schedule_Sessions{}).Where("schedule_id = ?", uint(sid)).Select("COALESCE(MAX(session_number),0)").Scan(&maxNo)
	nextNo := maxNo + 1

	// Compute week number relative to schedule start_date (1-based)
	days := int(startDT.Sub(schedule.Start_date).Hours() / 24)
	weekNo := (days / 7) + 1
	if weekNo < 1 {
		weekNo = 1
	}

	// Create the session
	sd := startDT
	stPtr := startDT
	etPtr := endDT
	newSession := models.Schedule_Sessions{
		ScheduleID:     uint(sid),
		Session_date:   &sd,
		Start_time:     &stPtr,
		End_time:       &etPtr,
		Session_number: nextNo,
		Week_number:    weekNo,
		Status: func() string {
			if strings.ToLower(schedule.ScheduleType) == "class" {
				return "assigned"
			}
			return "scheduled"
		}(),
		Notes:             req.Notes,
		RoomID:            req.RoomID,
		AssignedTeacherID: req.AssignedTeacherID,
	}

	if err := database.DB.Create(&newSession).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create session"})
	}

	// After creating the session, send notifications to relevant user(s)
	// Rule: If AssignedTeacherID present -> notify that user (channels: popup, normal)
	// For non-class schedules: also notify participants (channels: popup, normal)
	{
		targetIDs := make([]uint, 0, 4)
		if newSession.AssignedTeacherID != nil {
			targetIDs = append(targetIDs, *newSession.AssignedTeacherID)
		}
		if strings.ToLower(schedule.ScheduleType) != "class" {
			var participantIDs []uint
			database.DB.Model(&models.ScheduleParticipant{}).
				Where("schedule_id = ?", schedule.ID).
				Pluck("user_id", &participantIDs)
			if len(participantIDs) > 0 {
				targetIDs = append(targetIDs, participantIDs...)
			}
		}
		if len(targetIDs) > 0 {
			loc, _ := time.LoadLocation("Asia/Bangkok")
			startStr := startDT.In(loc).Format("2006-01-02 15:04")
			data := fiber.Map{
				"link": fiber.Map{
					"href":   fmt.Sprintf("/api/schedules/sessions/%d", newSession.ID),
					"method": "GET",
				},
				"action":      "confirm-session",
				"session_id":  newSession.ID,
				"schedule_id": schedule.ID,
			}
			// Message depends on schedule type
			titleEn := "New session scheduled"
			titleTh := "มีการสร้างคาบเรียน/นัดหมายใหม่"
			msgEn := fmt.Sprintf("A new session for schedule '%s' is scheduled at %s.", schedule.ScheduleName, startStr)
			msgTh := fmt.Sprintf("มีการสร้างคาบเรียน/นัดหมายสำหรับตาราง '%s' เวลา %s", schedule.ScheduleName, startStr)
			if strings.ToLower(schedule.ScheduleType) == "class" {
				titleEn = "Please confirm your session"
				titleTh = "กรุณายืนยันคาบเรียนของคุณ"
				msgEn = fmt.Sprintf("Please confirm the session for '%s' at %s.", schedule.ScheduleName, startStr)
				msgTh = fmt.Sprintf("กรุณายืนยันคาบเรียนสำหรับ '%s' เวลา %s", schedule.ScheduleName, startStr)
			}
			payload := notifsvc.QueuedWithData(
				titleEn,
				titleTh,
				msgEn,
				msgTh,
				"info", data,
				"popup", "normal",
			)
			_ = notifsvc.NewService().EnqueueOrCreate(targetIDs, payload)
		}
	}

	// For class schedules, also schedule teacher confirmation reminders (T-24h, T-6h)
	if strings.ToLower(schedule.ScheduleType) == "class" {
		services.ScheduleTeacherConfirmReminders(newSession, schedule)
	}

	// Return the created session
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Session added successfully",
		"session": newSession,
	})
}
