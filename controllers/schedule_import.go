package controllers

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"englishkorat_go/database"
	"englishkorat_go/models"
	"englishkorat_go/utils"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ScheduleImportController handles importing schedules from Schedulista exports
type ScheduleImportController struct{}

// Import parses a CSV/XLSX Schedulista export and creates schedules/sessions
func (sic *ScheduleImportController) Import(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is required"})
	}

	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot open file"})
	}
	defer file.Close()

	filename := strings.ToLower(fileHeader.Filename)
	var rows [][]string
	var parseErr error

	if strings.HasSuffix(filename, ".csv") {
		rows, parseErr = readCSV(file)
	} else if strings.HasSuffix(filename, ".xlsx") || strings.HasSuffix(filename, ".xls") {
		tmpDir, _ := os.MkdirTemp("", "ekschedule-")
		tmp := filepath.Join(tmpDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeFilename(fileHeader.Filename)))
		if err := c.SaveFile(fileHeader, tmp); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to buffer upload"})
		}
		rows, parseErr = readXLSX(tmp)
		_ = os.Remove(tmp)
	} else {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unsupported file type (csv, xlsx)"})
	}
	if parseErr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": parseErr.Error()})
	}

	if len(rows) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is empty"})
	}

	header := rows[0]
	colIndex := buildFlexibleColumnIndex(header)
	required := []string{"appointment time", "appointment duration", "provider name", "service name", "branch", "course name", "client name"}
	for _, key := range required {
		if _, ok := colIndex[key]; !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("missing column: %s", key)})
		}
	}

	parsedRows := make([]scheduleImportRow, 0, len(rows)-1)
	var parseErrors []string
	for i := 1; i < len(rows); i++ {
		raw := rows[i]
		if isRowEmpty(raw) {
			continue
		}
		r, err := parseScheduleRow(raw, colIndex, i+1)
		if err != nil {
			parseErrors = append(parseErrors, err.Error())
			continue
		}
		parsedRows = append(parsedRows, r)
	}

	if len(parsedRows) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no valid data rows found", "parse_errors": parseErrors})
	}

	grouped := groupScheduleRows(parsedRows)
	stats := &scheduleImportStats{TotalRows: len(parsedRows)}
	defaultPassword := "1424123"
	hashedDefault, _ := utils.HashPassword(defaultPassword)

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		for _, bucket := range grouped {
			if err := processScheduleBucket(tx, bucket, hashedDefault, stats); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error(), "stats": stats})
	}

	response := fiber.Map{
		"success":            true,
		"file_name":          fileHeader.Filename,
		"total_rows":         stats.TotalRows,
		"schedules_created":  stats.SchedulesCreated,
		"schedules_reused":   stats.SchedulesReused,
		"sessions_created":   stats.SessionsCreated,
		"sessions_skipped":   stats.SessionsSkipped,
		"groups_created":     stats.GroupsCreated,
		"groups_reused":      stats.GroupsReused,
		"students_created":   stats.StudentsCreated,
		"students_reused":    stats.StudentsReused,
		"missing_teachers":   stats.MissingTeachers,
		"unmatched_students": stats.UnmatchedStudents,
		"parse_errors":       parseErrors,
		"notes":              stats.Notes,
	}

	if len(stats.MissingTeachers) > 0 || len(stats.UnmatchedStudents) > 0 {
		response["has_unmatched"] = true
	}

	return c.JSON(response)
}

// --- Parsing helpers ---

type scheduleImportRow struct {
	RowNumber        int
	AppointmentTime  time.Time
	DurationMinutes  int
	ProviderName     string
	ServiceName      string
	BranchRaw        string
	CourseName       string
	ClientName       string
	AppointmentNotes string
	CreatedAt        *time.Time
	Fingerprint      string
}

type scheduleBucket struct {
	Key  scheduleGroupKey
	Rows []scheduleImportRow
	Meta clientMeta
}

type scheduleGroupKey struct {
	ServiceName string
	ClientName  string
	Provider    string
	BranchRaw   string
	CourseName  string
}

type clientMeta struct {
	Names      []string
	Level      string
	TotalHours int
	BranchHint string
}

type scheduleImportStats struct {
	TotalRows         int
	SchedulesCreated  int
	SchedulesReused   int
	SessionsCreated   int
	SessionsSkipped   int
	GroupsCreated     int
	GroupsReused      int
	StudentsCreated   int
	StudentsReused    int
	MissingTeachers   []string
	UnmatchedStudents []string
	Notes             []string
}

func buildFlexibleColumnIndex(header []string) map[string]int {
	col := map[string]int{}
	for idx, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		if key == "" {
			continue
		}
		col[key] = idx
		// allow alternate spellings
		switch key {
		case "appointment time":
			col["appointment_time"] = idx
		case "appointment duration":
			col["appointment_duration"] = idx
		case "provider name":
			col["provider"] = idx
		case "service name":
			col["service"] = idx
		case "course name":
			col["course"] = idx
		case "client name":
			col["client"] = idx
		case "appointment notes":
			col["notes"] = idx
		case "appointment created":
			col["created_at"] = idx
		}
	}
	return col
}

func isRowEmpty(row []string) bool {
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

func getValue(row []string, col map[string]int, key string) string {
	if idx, ok := col[key]; ok && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

func parseScheduleRow(row []string, col map[string]int, rowNum int) (scheduleImportRow, error) {
	rawTime := getValue(row, col, "appointment time")
	if rawTime == "" {
		return scheduleImportRow{}, fmt.Errorf("row %d: missing appointment time", rowNum)
	}
	apptTime, err := parseSchedulistaDateTime(rawTime)
	if err != nil {
		return scheduleImportRow{}, fmt.Errorf("row %d: invalid appointment time: %v", rowNum, err)
	}

	rawDuration := getValue(row, col, "appointment duration")
	if rawDuration == "" {
		return scheduleImportRow{}, fmt.Errorf("row %d: missing appointment duration", rowNum)
	}
	durationMinutes, err := strconv.Atoi(strings.TrimSpace(rawDuration))
	if err != nil {
		return scheduleImportRow{}, fmt.Errorf("row %d: invalid appointment duration: %v", rowNum, err)
	}
	if durationMinutes <= 0 {
		return scheduleImportRow{}, fmt.Errorf("row %d: appointment duration must be positive", rowNum)
	}

	createdAt := parseOptionalDateTime(getValue(row, col, "appointment created"))

	parsed := scheduleImportRow{
		RowNumber:        rowNum,
		AppointmentTime:  apptTime,
		DurationMinutes:  durationMinutes,
		ProviderName:     getValue(row, col, "provider name"),
		ServiceName:      getValue(row, col, "service name"),
		BranchRaw:        getValue(row, col, "branch"),
		CourseName:       getValue(row, col, "course name"),
		ClientName:       getValue(row, col, "client name"),
		AppointmentNotes: getValue(row, col, "appointment notes"),
		CreatedAt:        createdAt,
	}
	parsed.Fingerprint = buildScheduleFingerprint(parsed)
	return parsed, nil
}

func parseSchedulistaDateTime(value string) (time.Time, error) {
	layouts := []string{
		"1/2/06 15:04",
		"01/02/06 15:04",
		"1/2/2006 15:04",
		"01/02/2006 15:04",
		"2006-01-02 15:04",
		time.RFC3339,
	}
	loc, _ := time.LoadLocation("Asia/Bangkok")
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t, nil
		}
	}
	// fallback: try parsing with seconds
	extraLayouts := []string{"1/2/06 15:04:05", "01/02/06 15:04:05", "1/2/2006 15:04:05", "01/02/2006 15:04:05", "2006-01-02 15:04:05"}
	for _, layout := range extraLayouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized datetime format: %s", value)
}

func parseOptionalDateTime(value string) *time.Time {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if t, err := parseSchedulistaDateTime(value); err == nil {
		return &t
	}
	return nil
}

func buildScheduleFingerprint(row scheduleImportRow) string {
	parts := []string{
		row.AppointmentTime.UTC().Format(time.RFC3339),
		strings.ToLower(compactSpaces(normalizeTeacherName(row.ProviderName))),
		strings.ToLower(compactSpaces(row.ServiceName)),
		strings.ToLower(compactSpaces(row.ClientName)),
		strings.ToLower(compactSpaces(row.BranchRaw)),
		strings.ToLower(compactSpaces(row.CourseName)),
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func compactSpaces(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func groupScheduleRows(rows []scheduleImportRow) []*scheduleBucket {
	buckets := map[string]*scheduleBucket{}
	for _, r := range rows {
		key := scheduleGroupKey{
			ServiceName: strings.TrimSpace(r.ServiceName),
			ClientName:  strings.TrimSpace(r.ClientName),
			Provider:    strings.TrimSpace(r.ProviderName),
			BranchRaw:   strings.TrimSpace(r.BranchRaw),
			CourseName:  strings.TrimSpace(r.CourseName),
		}
		mapKey := key.hash()
		bucket, ok := buckets[mapKey]
		if !ok {
			bucket = &scheduleBucket{Key: key}
			bucket.Meta = extractClientMeta(key.ClientName)
			buckets[mapKey] = bucket
		}
		bucket.Rows = append(bucket.Rows, r)
	}

	result := make([]*scheduleBucket, 0, len(buckets))
	for _, b := range buckets {
		sort.Slice(b.Rows, func(i, j int) bool {
			return b.Rows[i].AppointmentTime.Before(b.Rows[j].AppointmentTime)
		})
		result = append(result, b)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Rows[0].AppointmentTime.Equal(result[j].Rows[0].AppointmentTime) {
			return strings.Compare(result[i].Key.ServiceName, result[j].Key.ServiceName) < 0
		}
		return result[i].Rows[0].AppointmentTime.Before(result[j].Rows[0].AppointmentTime)
	})
	return result
}

func (k scheduleGroupKey) hash() string {
	parts := []string{
		strings.ToLower(k.ServiceName),
		strings.ToLower(k.ClientName),
		strings.ToLower(k.Provider),
		strings.ToLower(k.BranchRaw),
		strings.ToLower(k.CourseName),
	}
	return strings.Join(parts, "|")
}

// --- Client metadata parsing ---

var levelKeywords = []string{"a1", "a1+", "a2", "b1", "b2", "c1", "c2", "ielts", "toeic", "toefl", "primary", "elementary", "middle", "kid", "kids", "adults", "conversation", "headway", "phonics", "chinese", "hsk"}

func extractClientMeta(raw string) clientMeta {
	segments := splitClientSegments(raw)
	meta := clientMeta{}
	for _, segment := range segments {
		lower := strings.ToLower(segment)
		switch {
		case strings.Contains(lower, "ชั่วโมง") || strings.Contains(lower, "hour") || strings.Contains(lower, "hrs"):
			if meta.TotalHours == 0 {
				meta.TotalHours = extractDigitsInt(segment)
			}
		case strings.Contains(lower, "สาขา") || strings.Contains(lower, "branch"):
			if meta.BranchHint == "" {
				meta.BranchHint = segment
			}
		case isLevelIndicator(lower):
			if meta.Level == "" {
				meta.Level = segment
			}
		default:
			if strings.TrimSpace(segment) != "" {
				meta.Names = append(meta.Names, segment)
			}
		}
	}
	return meta
}

func splitClientSegments(raw string) []string {
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, "\\", "/")
	parts := strings.Split(raw, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func extractDigitsInt(raw string) int {
	digits := ""
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			digits += string(ch)
		}
	}
	if digits == "" {
		return 0
	}
	n, _ := strconv.Atoi(digits)
	return n
}

func isLevelIndicator(lower string) bool {
	for _, kw := range levelKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// --- Processing ---

func processScheduleBucket(tx *gorm.DB, bucket *scheduleBucket, hashedDefault string, stats *scheduleImportStats) error {
	if len(bucket.Rows) == 0 {
		return nil
	}

	validRows, skippedDup, err := filterNewScheduleRows(tx, bucket.Rows)
	if err != nil {
		return err
	}
	if skippedDup > 0 {
		stats.SessionsSkipped += skippedDup
	}
	if len(validRows) == 0 {
		return nil
	}
	bucket.Rows = validRows

	first := validRows[0]
	branchID := resolveBranchByNumbers(tx, first.BranchRaw, first.ClientName)

	teacherUserID, teacherName, err := resolveTeacherUserID(tx, first.ProviderName, branchID)
	if err != nil {
		stats.MissingTeachers = appendUnique(stats.MissingTeachers, teacherName)
	} else if teacherUserID != nil {
		stats.Notes = append(stats.Notes, fmt.Sprintf("matched teacher '%s' -> user %d", teacherName, *teacherUserID))
	}

	courseName := strings.TrimSpace(first.CourseName)
	if courseName == "" {
		courseName = "Course for " + bucket.Key.ServiceName
	}
	course, err := findOrCreateCourse(tx, courseName, branchID, bucket.Meta.Level)
	if err != nil {
		return fmt.Errorf("group %s: %w", bucket.Key.ClientName, err)
	}

	group, created, err := findOrCreateGroupForSchedule(tx, bucket.Key.ClientName, course, bucket.Meta.Level)
	if err != nil {
		return err
	}
	if created {
		stats.GroupsCreated++
	} else {
		stats.GroupsReused++
	}

	studentsCreated, studentsReused, unmatched := ensureStudentsForGroup(tx, bucket.Meta.Names, branchID, bucket.Meta.Level, hashedDefault, group)
	stats.StudentsCreated += studentsCreated
	stats.StudentsReused += studentsReused
	if len(unmatched) > 0 {
		stats.UnmatchedStudents = append(stats.UnmatchedStudents, unmatched...)
	}

	startDate := validRows[0].AppointmentTime
	endDate := validRows[len(validRows)-1].AppointmentTime
	sessionPerWeek := calculateSessionsPerWeek(validRows)
	durationMinutes := validRows[0].DurationMinutes
	hoursPerSession := int(math.Round(float64(durationMinutes) / 60.0))
	if hoursPerSession < 1 {
		hoursPerSession = 1
	}

	totalDurationMinutes := 0
	for _, r := range validRows {
		totalDurationMinutes += r.DurationMinutes
	}
	totalHours := bucket.Meta.TotalHours
	if totalHours == 0 {
		totalHours = int(math.Round(float64(totalDurationMinutes) / 60.0))
		if totalHours == 0 {
			totalHours = int(hoursPerSession) * len(validRows)
		}
	}

	schedule, createdSchedule, err := findOrCreateSchedule(tx, bucket, group, teacherUserID, startDate, endDate, hoursPerSession, totalHours, sessionPerWeek)
	if err != nil {
		return err
	}
	if createdSchedule {
		stats.SchedulesCreated++
	} else {
		stats.SchedulesReused++
	}

	sessionsCreated, sessionsSkipped, err := ensureSessions(tx, schedule, validRows, teacherUserID)
	if err != nil {
		return err
	}
	stats.SessionsCreated += sessionsCreated
	stats.SessionsSkipped += sessionsSkipped

	return nil
}

func filterNewScheduleRows(tx *gorm.DB, rows []scheduleImportRow) ([]scheduleImportRow, int, error) {
	if len(rows) == 0 {
		return rows, 0, nil
	}
	fingerprints := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].Fingerprint == "" {
			rows[i].Fingerprint = buildScheduleFingerprint(rows[i])
		}
		if rows[i].Fingerprint != "" {
			fingerprints = append(fingerprints, rows[i].Fingerprint)
		}
	}
	if len(fingerprints) == 0 {
		return rows, 0, nil
	}
	var existing []models.ScheduleImport
	if err := tx.Where("fingerprint IN ?", fingerprints).Find(&existing).Error; err != nil {
		return nil, 0, err
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, rec := range existing {
		existingSet[rec.Fingerprint] = struct{}{}
	}
	deduped := make([]scheduleImportRow, 0, len(rows))
	skipped := 0
	for _, r := range rows {
		if r.Fingerprint != "" {
			if _, found := existingSet[r.Fingerprint]; found {
				skipped++
				continue
			}
		}
		deduped = append(deduped, r)
	}
	return deduped, skipped, nil
}

func recordScheduleImport(tx *gorm.DB, scheduleID uint, sessionID uint, row scheduleImportRow) error {
	fingerprint := row.Fingerprint
	if fingerprint == "" {
		fingerprint = buildScheduleFingerprint(row)
	}
	if fingerprint == "" {
		return nil
	}
	scheduleIDCopy := scheduleID
	sessionIDCopy := sessionID
	entry := models.ScheduleImport{
		Source:          "schedulista",
		Fingerprint:     fingerprint,
		AppointmentTime: row.AppointmentTime,
		ProviderName:    compactSpaces(row.ProviderName),
		ServiceName:     compactSpaces(row.ServiceName),
		ClientName:      compactSpaces(row.ClientName),
		BranchRaw:       compactSpaces(row.BranchRaw),
		CourseName:      compactSpaces(row.CourseName),
		ScheduleID:      &scheduleIDCopy,
		SessionID:       &sessionIDCopy,
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "fingerprint"}},
		DoUpdates: clause.AssignmentColumns([]string{"schedule_id", "session_id", "appointment_time", "provider_name", "service_name", "client_name", "branch_raw", "course_name"}),
	}).Create(&entry).Error
}

func appendUnique(slice []string, value string) []string {
	if value == "" {
		return slice
	}
	for _, v := range slice {
		if strings.EqualFold(v, value) {
			return slice
		}
	}
	return append(slice, value)
}

func findOrCreateGroupForSchedule(tx *gorm.DB, groupName string, course *models.Course, level string) (*models.Group, bool, error) {
	if strings.TrimSpace(groupName) == "" {
		groupName = fmt.Sprintf("Group %d", time.Now().UnixNano())
	}
	var group models.Group
	if err := tx.Where("group_name = ?", groupName).First(&group).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			group = models.Group{GroupName: groupName, CourseID: course.ID, Level: level, Status: "active"}
			if err := tx.Create(&group).Error; err != nil {
				return nil, false, err
			}
			return &group, true, nil
		}
		return nil, false, err
	}
	update := map[string]interface{}{}
	if group.CourseID != course.ID {
		update["course_id"] = course.ID
	}
	if level != "" && group.Level == "" {
		update["level"] = level
	}
	if len(update) > 0 {
		if err := tx.Model(&group).Updates(update).Error; err != nil {
			return nil, false, err
		}
	}
	return &group, false, nil
}

func ensureStudentsForGroup(tx *gorm.DB, names []string, branchID uint, level string, hashedDefault string, group *models.Group) (created, reused int, unmatched []string) {
	if len(names) == 0 {
		return 0, 0, nil
	}
	seen := map[string]struct{}{}
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(name)]; ok {
			continue
		}
		seen[strings.ToLower(name)] = struct{}{}

		student, existed, err := findOrCreateStudent(tx, name, branchID, level, hashedDefault)
		if err != nil {
			unmatched = appendUnique(unmatched, name)
			continue
		}
		if existed {
			reused++
		} else {
			created++
		}
		if err := ensureGroupMembership(tx, group.ID, student.ID); err != nil {
			unmatched = appendUnique(unmatched, name)
			continue
		}
	}
	return created, reused, unmatched
}

func findOrCreateStudent(tx *gorm.DB, name string, branchID uint, level string, hashedDefault string) (*models.Student, bool, error) {
	if student := findStudentByNicknames(tx, name, name, branchID); student != nil {
		return student, true, nil
	}

	normalized := normalizeNameToken(name)
	if normalized != name {
		if student := findStudentByNicknames(tx, normalized, normalized, branchID); student != nil {
			return student, true, nil
		}
	}

	username := buildUsername(name, branchID)
	user, err := findOrCreateUserByUsername(tx, username, branchID, hashedDefault)
	if err != nil {
		return nil, false, err
	}

	student := models.Student{
		UserID:           &user.ID,
		FirstName:        name,
		LastName:         name,
		NicknameTh:       name,
		NicknameEn:       normalized,
		LanguageLevel:    level,
		AgeGroup:         "adults",
		RegistrationType: "full",
	}
	if err := tx.Create(&student).Error; err != nil {
		return nil, false, err
	}
	return &student, false, nil
}

func ensureGroupMembership(tx *gorm.DB, groupID, studentID uint) error {
	var gm models.GroupMember
	if err := tx.Where("group_id = ? AND student_id = ?", groupID, studentID).First(&gm).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			gm = models.GroupMember{GroupID: groupID, StudentID: studentID, Status: "active"}
			return tx.Create(&gm).Error
		}
		return err
	}
	return nil
}

func normalizeNameToken(name string) string {
	trimmed := strings.TrimSpace(name)
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	return trimmed
}

func buildUsername(name string, branchID uint) string {
	base := strings.ToLower(normalizeNameToken(name))
	if base == "" {
		base = fmt.Sprintf("student_%d", time.Now().UnixNano())
	}
	base = strings.ReplaceAll(base, "\u200b", "")
	base = strings.TrimSpace(base)
	return fmt.Sprintf("%s_b%d", base, branchID)
}

func calculateSessionsPerWeek(rows []scheduleImportRow) int {
	counts := map[string]int{}
	for _, r := range rows {
		year, week := r.AppointmentTime.ISOWeek()
		key := fmt.Sprintf("%d-%d", year, week)
		counts[key]++
	}
	max := 0
	for _, v := range counts {
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return 1
	}
	return max
}

func findOrCreateSchedule(tx *gorm.DB, bucket *scheduleBucket, group *models.Group, teacherUserID *uint, startDate, endDate time.Time, hoursPerSession, totalHours, sessionsPerWeek int) (*models.Schedules, bool, error) {
	var schedule models.Schedules
	if err := tx.Where("schedule_name = ? AND group_id = ?", bucket.Key.ServiceName, group.ID).First(&schedule).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return nil, false, err
		}
		schedule = models.Schedules{
			ScheduleName:            bucket.Key.ServiceName,
			ScheduleType:            "class",
			GroupID:                 &group.ID,
			Recurring_pattern:       "none",
			Total_hours:             totalHours,
			Hours_per_session:       hoursPerSession,
			Session_per_week:        sessionsPerWeek,
			Start_date:              startDate,
			Estimated_end_date:      endDate,
			Status:                  "scheduled",
			Auto_Reschedule_holiday: false,
			Notes:                   "Imported from Schedulista",
			Admin_assigned:          "import",
		}
		if teacherUserID != nil {
			schedule.DefaultTeacherID = teacherUserID
		}
		if err := tx.Create(&schedule).Error; err != nil {
			return nil, false, err
		}
		return &schedule, true, nil
	}

	updates := map[string]interface{}{}
	if schedule.Status == "assigned" {
		updates["status"] = "scheduled"
	}
	if teacherUserID != nil && (schedule.DefaultTeacherID == nil || *schedule.DefaultTeacherID != *teacherUserID) {
		updates["default_teacher_id"] = *teacherUserID
	}
	if schedule.Notes == "" {
		updates["notes"] = "Imported from Schedulista"
	}
	if schedule.Total_hours == 0 && totalHours > 0 {
		updates["total_hours"] = totalHours
	}
	if schedule.Hours_per_session == 0 && hoursPerSession > 0 {
		updates["hours_per_session"] = hoursPerSession
	}
	if schedule.Session_per_week == 0 && sessionsPerWeek > 0 {
		updates["session_per_week"] = sessionsPerWeek
	}
	if schedule.Start_date.After(startDate) {
		updates["start_date"] = startDate
	}
	if schedule.Estimated_end_date.Before(endDate) {
		updates["estimated_end_date"] = endDate
	}
	if len(updates) > 0 {
		if err := tx.Model(&schedule).Updates(updates).Error; err != nil {
			return nil, false, err
		}
	}
	return &schedule, false, nil
}

func ensureSessions(tx *gorm.DB, schedule *models.Schedules, rows []scheduleImportRow, teacherUserID *uint) (created, skipped int, err error) {
	now := time.Now()
	for idx, r := range rows {
		sessionDate := time.Date(r.AppointmentTime.Year(), r.AppointmentTime.Month(), r.AppointmentTime.Day(), 0, 0, 0, 0, r.AppointmentTime.Location())
		startTime := r.AppointmentTime
		endTime := r.AppointmentTime.Add(time.Duration(r.DurationMinutes) * time.Minute)
		nowInLoc := now.In(startTime.Location())

		var existing models.Schedule_Sessions
		if err := tx.Where("schedule_id = ? AND start_time = ?", schedule.ID, startTime).First(&existing).Error; err == nil {
			if endTime.Before(nowInLoc) && existing.Status != "confirmed" {
				updates := map[string]interface{}{"status": "confirmed"}
				if existing.ConfirmedAt == nil {
					updates["confirmed_at"] = nowInLoc
				}
				if err := tx.Model(&existing).Updates(updates).Error; err != nil {
					return created, skipped, err
				}
			}
			if err := recordScheduleImport(tx, schedule.ID, existing.ID, r); err != nil {
				return created, skipped, err
			}
			skipped++
			continue
		} else if err != gorm.ErrRecordNotFound {
			return created, skipped, err
		}

		weekNumber := calculateWeekNumber(schedule.Start_date, r.AppointmentTime)
		status := "scheduled"
		var confirmedAt *time.Time
		if endTime.Before(nowInLoc) {
			status = "confirmed"
			ts := nowInLoc
			confirmedAt = &ts
		}
		session := models.Schedule_Sessions{
			ScheduleID:     schedule.ID,
			Session_date:   &sessionDate,
			Start_time:     &startTime,
			End_time:       &endTime,
			Session_number: idx + 1,
			Week_number:    weekNumber,
			Status:         status,
			Notes:          r.AppointmentNotes,
		}
		if confirmedAt != nil {
			session.ConfirmedAt = confirmedAt
		}
		if teacherUserID != nil {
			session.AssignedTeacherID = teacherUserID
		}
		if err := tx.Create(&session).Error; err != nil {
			return created, skipped, err
		}
		if err := recordScheduleImport(tx, schedule.ID, session.ID, r); err != nil {
			return created, skipped, err
		}
		created++
	}
	return created, skipped, nil
}

func calculateWeekNumber(startDate, current time.Time) int {
	if current.Before(startDate) {
		return 1
	}
	diff := current.Sub(startDate)
	weeks := int(diff.Hours()/(24*7)) + 1
	if weeks <= 0 {
		weeks = 1
	}
	return weeks
}

// --- Teacher resolution ---

func resolveTeacherUserID(tx *gorm.DB, providerRaw string, branchID uint) (*uint, string, error) {
	candidates := buildTeacherCandidates(providerRaw)
	clean := normalizeTeacherName(providerRaw)
	if clean != "" {
		candidates = append([]string{clean}, candidates...)
	}
	if len(candidates) == 0 {
		return nil, providerRaw, fmt.Errorf("empty provider name")
	}

	if branchID != 0 {
		if teacher, label := findTeacherByCandidates(tx, candidates, branchID, true); teacher != nil {
			return &teacher.UserID, label, nil
		}
	}
	if teacher, label := findTeacherByCandidates(tx, candidates, 0, true); teacher != nil {
		return &teacher.UserID, label, nil
	}
	if branchID != 0 {
		if teacher, label := findTeacherByCandidates(tx, candidates, branchID, false); teacher != nil {
			return &teacher.UserID, label, nil
		}
	}
	if teacher, label := findTeacherByCandidates(tx, candidates, 0, false); teacher != nil {
		return &teacher.UserID, label, nil
	}

	teacher := findTeacherClosest(tx, clean)
	if teacher == nil && branchID != 0 {
		teacher = findTeacherByBranchFallback(tx, clean, branchID)
	}
	if teacher == nil {
		return nil, clean, fmt.Errorf("teacher not found")
	}
	return &teacher.UserID, clean, nil
}

func normalizeTeacherName(name string) string {
	n := compactSpaces(strings.TrimSpace(name))
	if n == "" {
		return ""
	}
	replacer := strings.NewReplacer("(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ", "|", " ", "/", " ", "\\", " ", ",", " ", "-", " ", "_", " ")
	n = replacer.Replace(n)
	n = removeTeacherMarkers(n)
	return compactSpaces(n)
}

func findTeacherByBranchFallback(tx *gorm.DB, name string, branchID uint) *models.Teacher {
	var teacher models.Teacher
	if err := tx.Where("(nickname_en = ? OR nickname_th = ?) AND branch_id = ?", name, name, branchID).First(&teacher).Error; err == nil {
		return &teacher
	}
	if err := tx.Where("(nickname_en LIKE ? OR nickname_th LIKE ?) AND branch_id = ?", "%"+name+"%", "%"+name+"%", branchID).First(&teacher).Error; err == nil {
		return &teacher
	}
	return nil
}

func findTeacherByCandidates(tx *gorm.DB, candidates []string, branchID uint, exact bool) (*models.Teacher, string) {
	for _, cand := range candidates {
		trimmed := strings.TrimSpace(cand)
		if trimmed == "" {
			continue
		}
		query := tx.Session(&gorm.Session{NewDB: true}).Model(&models.Teacher{})
		if branchID != 0 {
			query = query.Where("branch_id = ?", branchID)
		}
		var teacher models.Teacher
		if exact {
			if err := query.Where("(nickname_en = ? OR nickname_th = ? OR first_name_en = ? OR first_name_th = ? OR last_name_en = ? OR last_name_th = ?)", trimmed, trimmed, trimmed, trimmed, trimmed, trimmed).First(&teacher).Error; err == nil {
				return &teacher, trimmed
			}
			compact := strings.ReplaceAll(trimmed, " ", "")
			if compact != trimmed {
				if err := query.Where("(nickname_en = ? OR nickname_th = ?)", compact, compact).First(&teacher).Error; err == nil {
					return &teacher, trimmed
				}
			}
		} else {
			pattern := "%" + trimmed + "%"
			if err := query.Where("(nickname_en LIKE ? OR nickname_th LIKE ? OR first_name_en LIKE ? OR first_name_th LIKE ? OR last_name_en LIKE ? OR last_name_th LIKE ?)", pattern, pattern, pattern, pattern, pattern, pattern).First(&teacher).Error; err == nil {
				return &teacher, trimmed
			}
			compact := strings.ReplaceAll(trimmed, " ", "")
			if compact != trimmed {
				pattern = "%" + compact + "%"
				if err := query.Where("(nickname_en LIKE ? OR nickname_th LIKE ?)", pattern, pattern).First(&teacher).Error; err == nil {
					return &teacher, trimmed
				}
			}
		}
	}
	return nil, ""
}

func buildTeacherCandidates(raw string) []string {
	base := compactSpaces(strings.TrimSpace(raw))
	if base == "" {
		return nil
	}
	replacer := strings.NewReplacer("(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ", "|", " ", "/", " ", "\\", " ", ",", " ", "-", " ", "_", " ")
	normalized := replacer.Replace(base)
	cleaned := removeTeacherMarkers(normalized)
	seen := map[string]struct{}{}
	result := []string{}
	addCandidate := func(val string) {
		val = compactSpaces(val)
		if val == "" {
			return
		}
		key := strings.ToLower(val)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		result = append(result, val)
	}
	addCandidate(base)
	addCandidate(cleaned)
	tokens := strings.Fields(normalized)
	if len(tokens) > 0 {
		addCandidate(strings.Join(tokens, " "))
		addCandidate(strings.Join(tokens, ""))
		if len(tokens) > 1 {
			reversed := make([]string, len(tokens))
			for i := range tokens {
				reversed[i] = tokens[len(tokens)-1-i]
			}
			addCandidate(strings.Join(reversed, " "))
		}
		for _, token := range tokens {
			addCandidate(removeTeacherMarkers(token))
		}
	}
	cleanedTokens := strings.Fields(cleaned)
	if len(cleanedTokens) > 0 {
		addCandidate(strings.Join(cleanedTokens, " "))
		addCandidate(strings.Join(cleanedTokens, ""))
		if len(cleanedTokens) > 1 {
			reversed := make([]string, len(cleanedTokens))
			for i := range cleanedTokens {
				reversed[i] = cleanedTokens[len(cleanedTokens)-1-i]
			}
			addCandidate(strings.Join(reversed, " "))
		}
	}
	return result
}

func removeTeacherMarkers(value string) string {
	if value == "" {
		return ""
	}
	replacements := []string{"Teacher", "teacher", "TEACHER", "ครู", "คุณครู", "อาจารย์", "Ajarn", "ajarn", "Kru", "kru", "Khru", "khru", "T.", "t.", "T-", "t-", "T_", "t_", "T:", "t:", "T/", "t/", "T|", "t|"}
	cleaned := value
	for _, marker := range replacements {
		cleaned = strings.ReplaceAll(cleaned, marker, " ")
	}
	return compactSpaces(cleaned)
}
