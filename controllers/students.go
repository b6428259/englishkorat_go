package controllers

import (
	"encoding/json"
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"englishkorat_go/services/notifications"
	"englishkorat_go/utils"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type StudentController struct{}

// New DTOs for the structured registration flow
type BasicInformation struct {
	FirstName   string `json:"first_name" validate:"required"`
	LastName    string `json:"last_name" validate:"required"`
	NicknameTh  string `json:"nickname_th" validate:"required"`
	NicknameEn  string `json:"nickname_en" validate:"required"`
	DateOfBirth string `json:"date_of_birth" validate:"required"` // YYYY-MM-DD
	Gender      string `json:"gender" validate:"required,oneof=male female other"`
	Age         int    `json:"age,omitempty"` // Read-only, auto-calculated
}

type ContactInformation struct {
	Phone   string  `json:"phone" validate:"required"`
	Email   *string `json:"email,omitempty"`
	LineID  string  `json:"line_id" validate:"required"`
	Address *string `json:"address,omitempty"`
}

type FullInformation struct {
	CitizenID            string     `json:"citizen_id" validate:"required,len=13"`
	FirstNameEn          *string    `json:"first_name_en,omitempty"`
	LastNameEn           *string    `json:"last_name_en,omitempty"`
	CurrentEducation     string     `json:"current_education" validate:"required"`
	PreferredBranch      uint       `json:"preferred_branch" validate:"required"`
	PreferredLanguage    string     `json:"preferred_language" validate:"required,oneof=english chinese"`
	LanguageLevel        string     `json:"language_level" validate:"required"`
	LearningStyle        string     `json:"learning_style" validate:"required,oneof=private pair group"`
	RecentCEFR           string     `json:"recent_cefr" validate:"required"`
	SelectedCourses      []uint     `json:"selected_courses,omitempty"`
	LearningGoals        *string    `json:"learning_goals,omitempty"`
	TeacherType          string     `json:"teacher_type" validate:"required"`
	PreferredTimeSlots   []TimeSlot `json:"preferred_time_slots,omitempty"`
	UnavailableTimeSlots []TimeSlot `json:"unavailable_time_slots,omitempty"`
	EmergencyContact     *string    `json:"emergency_contact,omitempty"`
	EmergencyPhone       *string    `json:"emergency_phone,omitempty"`
}

type NewStudentRegistrationRequest struct {
	RegistrationType   string             `json:"registration_type" validate:"required,oneof=quick full"`
	BasicInformation   BasicInformation   `json:"basic_information" validate:"required"`
	ContactInformation ContactInformation `json:"contact_information" validate:"required"`
	FullInformation    *FullInformation   `json:"full_information,omitempty"` // Required only for full registration
	// Frontend can send preferred_branch directly; we'll use this for both Student.PreferredBranchID and User.BranchID
	PreferredBranch *uint `json:"preferred_branch,omitempty"`
	// Backward-compatibility: still accept branch_id if sent; lower priority than preferred_branch
	BranchID *uint `json:"branch_id,omitempty"`
}

// Update Student Info DTO for admin completion
type UpdateStudentInfoRequest struct {
	FirstNameEn          *string    `json:"first_name_en,omitempty"`
	LastNameEn           *string    `json:"last_name_en,omitempty"`
	CitizenID            *string    `json:"citizen_id,omitempty"`
	CurrentEducation     *string    `json:"current_education,omitempty"`
	PreferredBranch      *uint      `json:"preferred_branch,omitempty"`
	PreferredLanguage    *string    `json:"preferred_language,omitempty"`
	LanguageLevel        *string    `json:"language_level,omitempty"`
	LearningStyle        *string    `json:"learning_style,omitempty"`
	RecentCEFR           *string    `json:"recent_cefr,omitempty"`
	SelectedCourses      []uint     `json:"selected_courses,omitempty"`
	LearningGoals        *string    `json:"learning_goals,omitempty"`
	TeacherType          *string    `json:"teacher_type,omitempty"`
	PreferredTimeSlots   []TimeSlot `json:"preferred_time_slots,omitempty"`
	UnavailableTimeSlots []TimeSlot `json:"unavailable_time_slots,omitempty"`
	EmergencyContact     *string    `json:"emergency_contact,omitempty"`
	EmergencyPhone       *string    `json:"emergency_phone,omitempty"`
	RegistrationStatus   *string    `json:"registration_status,omitempty"`
}

// Exam Scores DTO
type ExamScoresRequest struct {
	GrammarScore   int `json:"grammar_score" validate:"required,min=0,max=100"`
	SpeakingScore  int `json:"speaking_score" validate:"required,min=0,max=100"`
	ListeningScore int `json:"listening_score" validate:"required,min=0,max=100"`
	ReadingScore   int `json:"reading_score" validate:"required,min=0,max=100"`
	WritingScore   int `json:"writing_score" validate:"required,min=0,max=100"`
}

// Legacy DTOs for backward compatibility - DO NOT REMOVE, NEEDED BY EXISTING ENDPOINTS
type TimeSlot struct {
	ID        string `json:"id"`
	Day       string `json:"day"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type AvailabilityDaySlot struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type StudentRegistrationRequest struct {
	RegistrationType     string                           `json:"registration_type"`
	RegistrationStatus   *string                          `json:"registration_status"`
	FirstName            string                           `json:"first_name"`
	FirstNameEn          string                           `json:"first_name_en"`
	LastName             string                           `json:"last_name"`
	LastNameEn           string                           `json:"last_name_en"`
	NicknameTh           string                           `json:"nickname_th"`
	NicknameEn           string                           `json:"nickname_en"`
	DateOfBirth          string                           `json:"date_of_birth"`
	Gender               string                           `json:"gender"`
	Age                  int                              `json:"age"`
	Address              string                           `json:"address"`
	ParentName           string                           `json:"parent_name"`
	ParentPhone          string                           `json:"parent_phone"`
	EmergencyContact     string                           `json:"emergency_contact"`
	EmergencyPhone       string                           `json:"emergency_phone"`
	ContactSource        string                           `json:"contact_source"`
	CitizenID            *string                          `json:"citizen_id"`
	CurrentEducation     *string                          `json:"current_education"`
	CEFRLevel            *string                          `json:"cefr_level"`
	PreferredLanguage    *string                          `json:"preferred_language"`
	LanguageLevel        *string                          `json:"language_level"`
	RecentCEFR           *string                          `json:"recent_cefr"`
	LearningStyle        *string                          `json:"learning_style"`
	LearningGoals        *string                          `json:"learning_goals"`
	GradeLevel           *string                          `json:"grade_level"`
	AgeGroup             *string                          `json:"age_group"`
	SelectedCourses      []uint                           `json:"selected_courses"`
	PreferredTeacherType *string                          `json:"preferred_teacher_type"`
	PreferredTimeSlots   []TimeSlot                       `json:"preferred_time_slots"`
	UnavailableTimeSlots []TimeSlot                       `json:"unavailable_time_slots"`
	AvailabilitySchedule map[string][]AvailabilityDaySlot `json:"availability_schedule"`
	UnavailableTimes     []TimeSlot                       `json:"unavailable_times"`
}

// Helpers
func calculateAge(dob time.Time) int {
	now := time.Now()
	age := now.Year() - dob.Year()
	// if birthday hasn't occurred yet this year
	if now.Month() < dob.Month() || (now.Month() == dob.Month() && now.Day() < dob.Day()) {
		age--
	}
	return age
}

func isValidGender(g string) bool {
	switch strings.ToLower(g) {
	case "male", "female", "other":
		return true
	default:
		return false
	}
}

func normalizeAgeGroupByAge(age int) string {
	if age < 13 {
		return "kids"
	}
	if age <= 17 {
		return "teens"
	}
	return "adults"
}

func normalizeAgeGroup(input *string, age int) string {
	if input == nil || *input == "" {
		return normalizeAgeGroupByAge(age)
	}
	v := strings.ToLower(strings.TrimSpace(*input))
	switch v {
	case "kids", "kid", "children", "child":
		return "kids"
	case "teens", "teen", "teenager", "teenagers":
		return "teens"
	case "adults", "adult":
		return "adults"
	default:
		return normalizeAgeGroupByAge(age)
	}
}

var phoneRegex = regexp.MustCompile(`^(0\d{2}-?\d{3}-?\d{4}|0\d{9})$`)

func isValidPhone(p string) bool { return phoneRegex.MatchString(strings.TrimSpace(p)) }

func isValidCitizenID(id string) bool {
	// Only check for 13 digits per requirement
	if len(id) != 13 {
		return false
	}
	for i := 0; i < 13; i++ {
		if id[i] < '0' || id[i] > '9' {
			return false
		}
	}
	return true
}

// GetStudents returns all students with pagination
func (sc *StudentController) GetStudents(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	var students []models.Student
	var total int64

	query := database.DB.Model(&models.Student{})

	// Optional search by name/nickname
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		like := "%%" + search + "%%"
		query = query.Where(
			database.DB.Where("first_name LIKE ?", like).
				Or("last_name LIKE ?", like).
				Or("first_name_en LIKE ?", like).
				Or("last_name_en LIKE ?", like).
				Or("nickname_th LIKE ?", like).
				Or("nickname_en LIKE ?", like),
		)
	}

	// Filter by age group if specified
	if ageGroup := c.Query("age_group"); ageGroup != "" {
		query = query.Where("age_group = ?", ageGroup)
	}

	// Filter by CEFR level if specified
	if cefrLevel := c.Query("cefr_level"); cefrLevel != "" {
		query = query.Where("cefr_level = ?", cefrLevel)
	}

	// Filter by registration status if specified
	if status := c.Query("registration_status"); status != "" {
		query = query.Where("registration_status = ?", status)
	}

	// Filter by user status if specified (active/inactive etc.)
	if ustatus := c.Query("status"); ustatus != "" {
		query = query.Joins("JOIN users ON students.user_id = users.id").Where("users.status = ?", ustatus)
	}

	// Get total count
	query.Count(&total)

	// Get students with relationships
	if err := query.Preload("User").Preload("User.Branch").
		Offset(offset).Limit(limit).Find(&students).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch students",
		})
	}

	totalPages := (int(total) + limit - 1) / limit
	return c.JSON(fiber.Map{
		"students":    students,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}

// GetStudent returns a specific student by ID
func (sc *StudentController) GetStudent(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid student ID",
		})
	}

	var student models.Student
	if err := database.DB.Preload("User").Preload("User.Branch").
		First(&student, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Student not found",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    fiber.Map{"student": student},
	})
}

// CreateStudent creates a new student profile
func (sc *StudentController) CreateStudent(c *fiber.Ctx) error {
	var req StudentRegistrationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// For admin creation, require core fields
	errs := validateRegistrationPayload(req, true)
	if len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
			"errors":  errs,
		})
	}

	// Parse DOB
	dob, _ := time.Parse("2006-01-02", strings.TrimSpace(req.DateOfBirth))
	age := calculateAge(dob)
	if age < 3 || age > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
			"errors":  []string{"อายุต้องอยู่ระหว่าง 3-100 ปี"},
		})
	}

	// JSON marshal arrays/objects
	preferredSlotsJSON, _ := json.Marshal(req.PreferredTimeSlots)
	unavailableSlotsJSON, _ := json.Marshal(req.UnavailableTimeSlots)
	selectedCoursesJSON, _ := json.Marshal(req.SelectedCourses)
	availabilityJSON, _ := json.Marshal(req.AvailabilitySchedule)
	unavailableTimesJSON, _ := json.Marshal(req.UnavailableTimes)

	// Determine registration status (admin can set)
	regStatus := "approved"
	if req.RegistrationStatus != nil && *req.RegistrationStatus != "" {
		rs := strings.ToLower(strings.TrimSpace(*req.RegistrationStatus))
		if rs != "pending" && rs != "approved" && rs != "rejected" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"message": "ข้อมูลไม่ถูกต้อง",
				"errors":  []string{"registration_status ไม่ถูกต้อง"},
			})
		}
		regStatus = rs
	}

	student := models.Student{
		UserID:               nil,
		FirstName:            strings.TrimSpace(req.FirstName),
		FirstNameEn:          strings.TrimSpace(req.FirstNameEn),
		LastName:             strings.TrimSpace(req.LastName),
		LastNameEn:           strings.TrimSpace(req.LastNameEn),
		NicknameTh:           strings.TrimSpace(req.NicknameTh),
		NicknameEn:           strings.TrimSpace(req.NicknameEn),
		DateOfBirth:          &dob,
		Gender:               strings.ToLower(strings.TrimSpace(req.Gender)),
		Address:              strings.TrimSpace(req.Address),
		CitizenID:            getStringOrEmpty(req.CitizenID),
		Age:                  age,
		AgeGroup:             normalizeAgeGroup(req.AgeGroup, age),
		GradeLevel:           getStringOrEmpty(req.GradeLevel),
		CurrentEducation:     getStringOrEmpty(req.CurrentEducation),
		CEFRLevel:            getStringOrEmpty(req.CEFRLevel),
		PreferredLanguage:    getStringOrEmpty(req.PreferredLanguage),
		LanguageLevel:        getStringOrEmpty(req.LanguageLevel),
		RecentCEFR:           getStringOrEmpty(req.RecentCEFR),
		LearningStyle:        getStringOrEmpty(req.LearningStyle),
		LearningGoals:        getStringOrEmpty(req.LearningGoals),
		ParentName:           strings.TrimSpace(req.ParentName),
		ParentPhone:          strings.TrimSpace(req.ParentPhone),
		EmergencyContact:     strings.TrimSpace(req.EmergencyContact),
		EmergencyPhone:       strings.TrimSpace(req.EmergencyPhone),
		PreferredTimeSlots:   preferredSlotsJSON,
		UnavailableTimeSlots: unavailableSlotsJSON,
		SelectedCourses:      selectedCoursesJSON,
		AvailabilitySchedule: availabilityJSON,
		UnavailableTimes:     unavailableTimesJSON,
		PreferredTeacherType: getStringOrEmpty(req.PreferredTeacherType),
		ContactSource:        strings.TrimSpace(req.ContactSource),
		RegistrationStatus:   regStatus, // Admin can set or default to approved
		PaymentStatus:        "pending",
		AdminContact:         false,
		DaysWaiting:          0,
	}

	if err := database.DB.Create(&student).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create student profile"})
	}

	// Load relationships
	database.DB.Preload("User").Preload("User.Branch").First(&student, student.ID)

	// Log activity
	middleware.LogActivity(c, "CREATE", "students", student.ID, student)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Student profile created successfully",
		"student": student,
	})
}

// UpdateStudent updates an existing student profile
func (sc *StudentController) UpdateStudent(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid student ID",
		})
	}

	var student models.Student
	if err := database.DB.First(&student, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Student not found",
		})
	}

	// Accept partial updates via map to handle JSON fields
	var payload map[string]interface{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Normalize and validate specific fields
	if v, ok := payload["registration_status"].(string); ok && v != "" {
		vv := strings.ToLower(v)
		if vv != "pending" && vv != "approved" && vv != "rejected" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid registration_status"})
		}
		payload["registration_status"] = vv
		now := time.Now()
		payload["last_status_update"] = &now
	}
	if v, ok := payload["payment_status"].(string); ok && v != "" {
		vv := strings.ToLower(v)
		if vv != "pending" && vv != "paid" && vv != "partial" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid payment_status"})
		}
		payload["payment_status"] = vv
	}
	if v, ok := payload["gender"].(string); ok && v != "" {
		if !isValidGender(v) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid gender"})
		}
		payload["gender"] = strings.ToLower(v)
	}
	// If date_of_birth changed, recompute age
	if v, ok := payload["date_of_birth"].(string); ok && v != "" {
		if dob, err := time.Parse("2006-01-02", v); err == nil {
			age := calculateAge(dob)
			if age < 3 || age > 100 {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "อายุต้องอยู่ระหว่าง 3-100 ปี"})
			}
			payload["age"] = age
			ag := normalizeAgeGroupByAge(age)
			payload["age_group"] = ag
		} else {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid date_of_birth format"})
		}
	}
	// JSON fields: if provided, ensure they are JSON serializable
	for _, jf := range []string{"preferred_time_slots", "unavailable_time_slots", "selected_courses", "availability_schedule", "unavailable_times"} {
		if v, ok := payload[jf]; ok {
			b, err := json.Marshal(v)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("Invalid %s format", jf)})
			}
			payload[jf] = models.JSON(b)
		}
	}
	// Coerce admin_contact to bool if provided
	if v, ok := payload["admin_contact"]; ok {
		switch t := v.(type) {
		case bool:
			// ok
		case float64:
			payload["admin_contact"] = t != 0
		case string:
			lv := strings.ToLower(strings.TrimSpace(t))
			payload["admin_contact"] = (lv == "true" || lv == "1" || lv == "yes")
		}
	}

	if err := database.DB.Model(&student).Updates(payload).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update student profile",
		})
	}

	// Load relationships
	database.DB.Preload("User").Preload("User.Branch").First(&student, student.ID)

	// Log activity
	middleware.LogActivity(c, "UPDATE", "students", student.ID, payload)

	return c.JSON(fiber.Map{
		"message": "Student profile updated successfully",
		"student": student,
	})
}

// DeleteStudent deletes a student profile
func (sc *StudentController) DeleteStudent(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid student ID",
		})
	}

	var student models.Student
	if err := database.DB.First(&student, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Student not found",
		})
	}

	if err := database.DB.Delete(&student).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete student profile",
		})
	}

	// Log activity
	middleware.LogActivity(c, "DELETE", "students", student.ID, student)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "ลบข้อมูลนักเรียนสำเร็จ",
	})
}

// GetStudentsByBranch returns students for a specific branch
func (sc *StudentController) GetStudentsByBranch(c *fiber.Ctx) error {
	branchID, err := strconv.ParseUint(c.Params("branch_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid branch ID",
		})
	}

	var students []models.Student
	if err := database.DB.Joins("JOIN users ON students.user_id = users.id").
		Where("users.branch_id = ?", uint(branchID)).
		Preload("User").Preload("User.Branch").
		Find(&students).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch students",
		})
	}

	return c.JSON(fiber.Map{
		"students": students,
		"total":    len(students),
	})
}

// PublicRegisterStudent handles public student registration (no auth)
func (sc *StudentController) PublicRegisterStudent(c *fiber.Ctx) error {
	var req StudentRegistrationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
			"errors":  []string{"รูปแบบข้อมูลไม่ถูกต้อง"},
		})
	}

	// Validate (public: quick/full)
	errs := validateRegistrationPayload(req, false)
	if len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
			"errors":  errs,
		})
	}

	dob, _ := time.Parse("2006-01-02", strings.TrimSpace(req.DateOfBirth))
	age := calculateAge(dob)
	if age < 3 || age > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
			"errors":  []string{"อายุต้องอยู่ระหว่าง 3-100 ปี"},
		})
	}

	// Prepare JSON fields
	preferredSlotsJSON, _ := json.Marshal(req.PreferredTimeSlots)
	unavailableSlotsJSON, _ := json.Marshal(req.UnavailableTimeSlots)
	selectedCoursesJSON, _ := json.Marshal(req.SelectedCourses)
	availabilityJSON, _ := json.Marshal(req.AvailabilitySchedule)
	unavailableTimesJSON, _ := json.Marshal(req.UnavailableTimes)

	student := models.Student{
		UserID:               nil,
		FirstName:            strings.TrimSpace(req.FirstName),
		FirstNameEn:          strings.TrimSpace(req.FirstNameEn),
		LastName:             strings.TrimSpace(req.LastName),
		LastNameEn:           strings.TrimSpace(req.LastNameEn),
		NicknameTh:           strings.TrimSpace(req.NicknameTh),
		NicknameEn:           strings.TrimSpace(req.NicknameEn),
		DateOfBirth:          &dob,
		Gender:               strings.ToLower(strings.TrimSpace(req.Gender)),
		Address:              strings.TrimSpace(req.Address),
		CitizenID:            getStringOrEmpty(req.CitizenID),
		Age:                  age,
		AgeGroup:             normalizeAgeGroup(req.AgeGroup, age),
		GradeLevel:           getStringOrEmpty(req.GradeLevel),
		CurrentEducation:     getStringOrEmpty(req.CurrentEducation),
		CEFRLevel:            getStringOrEmpty(req.CEFRLevel),
		PreferredLanguage:    getStringOrEmpty(req.PreferredLanguage),
		LanguageLevel:        getStringOrEmpty(req.LanguageLevel),
		RecentCEFR:           getStringOrEmpty(req.RecentCEFR),
		LearningStyle:        getStringOrEmpty(req.LearningStyle),
		LearningGoals:        getStringOrEmpty(req.LearningGoals),
		ParentName:           strings.TrimSpace(req.ParentName),
		ParentPhone:          strings.TrimSpace(req.ParentPhone),
		EmergencyContact:     strings.TrimSpace(req.EmergencyContact),
		EmergencyPhone:       strings.TrimSpace(req.EmergencyPhone),
		PreferredTimeSlots:   preferredSlotsJSON,
		UnavailableTimeSlots: unavailableSlotsJSON,
		SelectedCourses:      selectedCoursesJSON,
		AvailabilitySchedule: availabilityJSON,
		UnavailableTimes:     unavailableTimesJSON,
		PreferredTeacherType: getStringOrEmpty(req.PreferredTeacherType),
		ContactSource:        strings.TrimSpace(req.ContactSource),
		RegistrationStatus:   "pending",
		PaymentStatus:        "pending",
		AdminContact:         false,
		DaysWaiting:          0,
	}

	if err := database.DB.Create(&student).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "เกิดข้อผิดพลาดในการบันทึกข้อมูล",
		})
	}

	// Create registration id
	createdAt := student.CreatedAt
	regID := fmt.Sprintf("REG-%d-%06d", createdAt.Year(), student.ID)

	respStudent := fiber.Map{
		"id":                  student.ID,
		"first_name":          student.FirstName,
		"first_name_en":       student.FirstNameEn,
		"registration_status": student.RegistrationStatus,
		"created_at":          student.CreatedAt,
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "ลงทะเบียนสำเร็จ",
		"data": fiber.Map{
			"student":         respStudent,
			"registration_id": regID,
		},
	})
}

func getStringOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// validateRegistrationPayload validates fields; if isAdmin true, treat as admin creation (citizen_id required)
func validateRegistrationPayload(req StudentRegistrationRequest, isAdmin bool) []string {
	var errs []string
	rt := strings.ToLower(strings.TrimSpace(req.RegistrationType))
	if !isAdmin {
		if rt != "quick" && rt != "full" {
			errs = append(errs, "รูปแบบการลงทะเบียนไม่ถูกต้อง (quick หรือ full)")
		}
	}
	// Required for both
	if strings.TrimSpace(req.FirstName) == "" {
		errs = append(errs, "ชื่อ (ไทย) จำเป็นต้องกรอก")
	}
	if strings.TrimSpace(req.FirstNameEn) == "" {
		errs = append(errs, "ชื่อ (อังกฤษ) จำเป็นต้องกรอก")
	}
	if strings.TrimSpace(req.LastName) == "" {
		errs = append(errs, "นามสกุล (ไทย) จำเป็นต้องกรอก")
	}
	if strings.TrimSpace(req.LastNameEn) == "" {
		errs = append(errs, "นามสกุล (อังกฤษ) จำเป็นต้องกรอก")
	}
	if strings.TrimSpace(req.NicknameTh) == "" {
		errs = append(errs, "ชื่อเล่น (ไทย) จำเป็นต้องกรอก")
	}
	if strings.TrimSpace(req.NicknameEn) == "" {
		errs = append(errs, "ชื่อเล่น (อังกฤษ) จำเป็นต้องกรอก")
	}
	if strings.TrimSpace(req.DateOfBirth) == "" {
		errs = append(errs, "วันเกิดจำเป็นต้องกรอก")
	} else {
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(req.DateOfBirth)); err != nil {
			errs = append(errs, "รูปแบบวันเกิดไม่ถูกต้อง (YYYY-MM-DD)")
		}
	}
	if !isValidGender(req.Gender) {
		errs = append(errs, "เพศต้องเป็น male/female/other")
	}

	// Registration type specific
	if !isAdmin {
		if rt == "full" {
			if req.CitizenID == nil || !isValidCitizenID(strings.TrimSpace(*req.CitizenID)) {
				errs = append(errs, "เลขบัตรประชาชนต้องเป็นตัวเลข 13 หลัก")
			}
		}
	} else {
		// admin creation requires citizen id
		if req.CitizenID == nil || !isValidCitizenID(strings.TrimSpace(*req.CitizenID)) {
			errs = append(errs, "เลขบัตรประชาชนต้องเป็นตัวเลข 13 หลัก")
		}
	}

	return errs
}

// NewPublicRegisterStudent handles the redesigned student registration workflow with structured payload
func (sc *StudentController) NewPublicRegisterStudent(c *fiber.Ctx) error {
	var req NewStudentRegistrationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
			"errors":  []string{"รูปแบบข้อมูลไม่ถูกต้อง"},
		})
	}

	// Validate basic information
	errs := validateNewRegistrationPayload(req)
	if len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
			"errors":  errs,
		})
	}

	// Parse date of birth and calculate age
	dob, err := time.Parse("2006-01-02", strings.TrimSpace(req.BasicInformation.DateOfBirth))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
			"errors":  []string{"รูปแบบวันเกิดไม่ถูกต้อง (YYYY-MM-DD)"},
		})
	}

	age := calculateAge(dob)
	if age < 3 || age > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
			"errors":  []string{"อายุต้องอยู่ระหว่าง 3-100 ปี"},
		})
	}

	// Initialize student model with basic information
	student := models.Student{
		UserID:             nil,
		FirstName:          strings.TrimSpace(req.BasicInformation.FirstName),
		LastName:           strings.TrimSpace(req.BasicInformation.LastName),
		NicknameTh:         strings.TrimSpace(req.BasicInformation.NicknameTh),
		NicknameEn:         strings.TrimSpace(req.BasicInformation.NicknameEn),
		DateOfBirth:        &dob,
		Gender:             strings.ToLower(strings.TrimSpace(req.BasicInformation.Gender)),
		Age:                age,
		AgeGroup:           normalizeAgeGroupByAge(age),
		RegistrationType:   req.RegistrationType,
		RegistrationStatus: "pending_review",
		PaymentStatus:      "pending",
		AdminContact:       false,
		DaysWaiting:        0,
	}

	// Decide preferred/branch to use across User and Student
	var selectedBranchID uint = 3 // default Online branch (per seeders)
	if req.PreferredBranch != nil && *req.PreferredBranch > 0 {
		selectedBranchID = *req.PreferredBranch
	} else if req.BranchID != nil && *req.BranchID > 0 {
		selectedBranchID = *req.BranchID
	} else if req.RegistrationType == "full" && req.FullInformation != nil && req.FullInformation.PreferredBranch > 0 {
		selectedBranchID = req.FullInformation.PreferredBranch
	}
	// Set student's preferred branch immediately
	if selectedBranchID > 0 {
		student.PreferredBranchID = &selectedBranchID
	}

	// Add contact information
	student.Phone = strings.TrimSpace(req.ContactInformation.Phone)
	student.LineID = strings.TrimSpace(req.ContactInformation.LineID)
	if req.ContactInformation.Email != nil {
		student.Email = strings.TrimSpace(*req.ContactInformation.Email)
	}
	if req.ContactInformation.Address != nil {
		student.Address = strings.TrimSpace(*req.ContactInformation.Address)
	}

	// Add full information if this is a full registration
	if req.RegistrationType == "full" && req.FullInformation != nil {
		full := req.FullInformation
		student.CitizenID = strings.TrimSpace(full.CitizenID)
		if full.FirstNameEn != nil {
			student.FirstNameEn = strings.TrimSpace(*full.FirstNameEn)
		}
		if full.LastNameEn != nil {
			student.LastNameEn = strings.TrimSpace(*full.LastNameEn)
		}
		student.CurrentEducation = strings.TrimSpace(full.CurrentEducation)
		// If top-level preferred_branch selected earlier, keep it; otherwise use full info
		if student.PreferredBranchID == nil && full.PreferredBranch > 0 {
			student.PreferredBranchID = &full.PreferredBranch
		}
		student.PreferredLanguage = strings.TrimSpace(full.PreferredLanguage)
		student.LanguageLevel = strings.TrimSpace(full.LanguageLevel)
		student.LearningStyle = strings.TrimSpace(full.LearningStyle)
		student.RecentCEFR = strings.TrimSpace(full.RecentCEFR)
		student.TeacherType = strings.TrimSpace(full.TeacherType)

		if full.LearningGoals != nil {
			student.LearningGoals = strings.TrimSpace(*full.LearningGoals)
		}
		if full.EmergencyContact != nil {
			student.EmergencyContact = strings.TrimSpace(*full.EmergencyContact)
		}
		if full.EmergencyPhone != nil {
			student.EmergencyPhone = strings.TrimSpace(*full.EmergencyPhone)
		}

		// Convert arrays to JSON
		if len(full.SelectedCourses) > 0 {
			selectedCoursesJSON, _ := json.Marshal(full.SelectedCourses)
			student.SelectedCourses = selectedCoursesJSON
		}
		if len(full.PreferredTimeSlots) > 0 {
			preferredSlotsJSON, _ := json.Marshal(full.PreferredTimeSlots)
			student.PreferredTimeSlots = preferredSlotsJSON
		}
		if len(full.UnavailableTimeSlots) > 0 {
			unavailableSlotsJSON, _ := json.Marshal(full.UnavailableTimeSlots)
			student.UnavailableTimeSlots = unavailableSlotsJSON
		}
	}

	// Create or find a user with username = LineID and password = phone (hashed), then link to student
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		username := strings.TrimSpace(req.ContactInformation.LineID)
		phoneRaw := strings.TrimSpace(req.ContactInformation.Phone)
		phoneClean := normalizePhoneNumber(phoneRaw)

		// Check if user already exists
		var user models.User
		if err := tx.Where("username = ?", username).First(&user).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Use previously selectedBranchID for user's branch
				branchID := selectedBranchID

				// Hash password (use cleaned phone number)
				hashedPassword, herr := utils.HashPassword(phoneClean)
				if herr != nil {
					return herr
				}

				user = models.User{
					Username: username,
					Password: hashedPassword,
					Email:    "",
					Phone:    phoneRaw,
					LineID:   username,
					Role:     "student",
					BranchID: branchID,
					Status:   "active",
				}
				if req.ContactInformation.Email != nil {
					user.Email = strings.TrimSpace(*req.ContactInformation.Email)
				}

				if err := tx.Create(&user).Error; err != nil {
					return err
				}
				// Log user creation
				middleware.LogActivity(c, "CREATE", "users", user.ID, fiber.Map{
					"username": user.Username,
					"role":     user.Role,
				})
			} else {
				// Unexpected DB error
				return err
			}
		}

		// Link student to this user
		student.UserID = &user.ID
		// Ensure student's preferred branch is set (fallback to selectedBranchID)
		if student.PreferredBranchID == nil && selectedBranchID > 0 {
			student.PreferredBranchID = &selectedBranchID
		}

		// Save student
		if err := tx.Create(&student).Error; err != nil {
			return err
		}

		// Log student creation
		middleware.LogActivity(c, "CREATE", "students", student.ID, fiber.Map{
			"user_id":      student.UserID,
			"first_name":   student.FirstName,
			"registration": student.RegistrationType,
		})

		return nil
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "เกิดข้อผิดพลาดในการบันทึกข้อมูล",
		})
	}

	// Create registration ID
	createdAt := student.CreatedAt
	regID := fmt.Sprintf("REG-%d-%06d", createdAt.Year(), student.ID)

	// Create admin notification for new student registration
	go func() {
		// Find all admin and owner users
		var admins []models.User
		if err := database.DB.Where("role IN ?", []string{"admin", "owner"}).Find(&admins).Error; err != nil {
			return // Log the error in production
		}

		if len(admins) > 0 {
			// Prepare user IDs for notification
			adminIDs := make([]uint, 0, len(admins))
			for _, admin := range admins {
				adminIDs = append(adminIDs, admin.ID)
			}

			// Create notification message
			title := "New Student Registration"
			titleTh := "การลงทะเบียนนักเรียนใหม่"
			message := fmt.Sprintf("New student %s %s has registered (%s registration). Please review and process.",
				student.FirstName, student.LastName, student.RegistrationType)
			messageTh := fmt.Sprintf("นักเรียนใหม่ %s %s ได้ลงทะเบียนแล้ว (แบบ%s) กรุณาตรวจสอบและดำเนินการ",
				student.FirstName, student.LastName, getRegistrationTypeInThai(student.RegistrationType))

			// Send notification using the notification service
			notificationService := notifications.NewService()
			queuedNotif := notifications.QueuedForController(title, titleTh, message, messageTh, "info")
			if err := notificationService.EnqueueOrCreate(adminIDs, queuedNotif); err != nil {
				// Log error in production
				fmt.Printf("Error creating admin notification for student registration: %v\n", err)
			}
		}
	}()

	// Reload relationships for response
	if err := database.DB.Preload("User").Preload("User.Branch").Preload("PreferredBranch").First(&student, student.ID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "เกิดข้อผิดพลาดในการโหลดข้อมูล",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "ลงทะเบียนสำเร็จ! ทางทีมงานจะติดต่อกลับภายใน 24 ชั่วโมง",
		"data": fiber.Map{
			"student":         student,
			"registration_id": regID,
		},
	})
}

// normalizePhoneNumber removes common separators to keep digits only
func normalizePhoneNumber(p string) string {
	p = strings.ReplaceAll(p, "-", "")
	p = strings.ReplaceAll(p, " ", "")
	return p
}

// validateNewRegistrationPayload validates the structured registration payload
func validateNewRegistrationPayload(req NewStudentRegistrationRequest) []string {
	var errs []string

	// Validate registration type
	if req.RegistrationType != "quick" && req.RegistrationType != "full" {
		errs = append(errs, "รูปแบบการลงทะเบียนไม่ถูกต้อง (quick หรือ full)")
	}

	// Validate basic information
	basic := req.BasicInformation
	if strings.TrimSpace(basic.FirstName) == "" {
		errs = append(errs, "ชื่อ (ไทย) จำเป็นต้องกรอก")
	}
	if strings.TrimSpace(basic.LastName) == "" {
		errs = append(errs, "นามสกุล (ไทย) จำเป็นต้องกรอก")
	}
	if strings.TrimSpace(basic.NicknameTh) == "" {
		errs = append(errs, "ชื่อเล่น (ไทย) จำเป็นต้องกรอก")
	}
	if strings.TrimSpace(basic.NicknameEn) == "" {
		errs = append(errs, "ชื่อเล่น (อังกฤษ) จำเป็นต้องกรอก")
	}
	if strings.TrimSpace(basic.DateOfBirth) == "" {
		errs = append(errs, "วันเกิดจำเป็นต้องกรอก")
	}
	if !isValidGender(basic.Gender) {
		errs = append(errs, "เพศต้องเป็น male/female/other")
	}

	// Validate contact information
	contact := req.ContactInformation
	if !isValidPhone(contact.Phone) {
		errs = append(errs, "เบอร์โทรศัพท์ไม่ถูกต้อง")
	}
	if strings.TrimSpace(contact.LineID) == "" {
		errs = append(errs, "Line ID จำเป็นต้องกรอก")
	}

	// Validate full information if required
	if req.RegistrationType == "full" {
		if req.FullInformation == nil {
			errs = append(errs, "ข้อมูลเพิ่มเติมจำเป็นสำหรับการลงทะเบียนแบบเต็ม")
		} else {
			full := req.FullInformation
			if !isValidCitizenID(strings.TrimSpace(full.CitizenID)) {
				errs = append(errs, "เลขบัตรประชาชนต้องเป็นตัวเลข 13 หลัก")
			}
			if strings.TrimSpace(full.CurrentEducation) == "" {
				errs = append(errs, "ระดับการศึกษาจำเป็นต้องกรอก")
			}
			if full.PreferredBranch == 0 {
				errs = append(errs, "สาขาที่ต้องการจำเป็นต้องเลือก")
			}
			if full.PreferredLanguage != "english" && full.PreferredLanguage != "chinese" {
				errs = append(errs, "ภาษาที่ต้องการต้องเป็น english หรือ chinese")
			}
			if strings.TrimSpace(full.LanguageLevel) == "" {
				errs = append(errs, "ระดับภาษาจำเป็นต้องกรอก")
			}
			if full.LearningStyle != "private" && full.LearningStyle != "pair" && full.LearningStyle != "group" {
				errs = append(errs, "รูปแบบการเรียนต้องเป็น private, pair, หรือ group")
			}
			if strings.TrimSpace(full.RecentCEFR) == "" {
				errs = append(errs, "ระดับ CEFR ล่าสุดจำเป็นต้องกรอก")
			}
			if strings.TrimSpace(full.TeacherType) == "" {
				errs = append(errs, "ประเภทครูที่ต้องการจำเป็นต้องกรอก")
			}
		}
	}

	return errs
}

// Admin endpoints for completing student information

// UpdateStudentInfo allows admins to complete/update student information
func (sc *StudentController) UpdateStudentInfo(c *fiber.Ctx) error {
	// Get student ID from params
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid student ID",
		})
	}

	// Parse request body
	var req UpdateStudentInfoRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Find student
	var student models.Student
	if err := database.DB.First(&student, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Student not found",
		})
	}

	// Update fields that are provided
	if req.FirstNameEn != nil {
		student.FirstNameEn = strings.TrimSpace(*req.FirstNameEn)
	}
	if req.LastNameEn != nil {
		student.LastNameEn = strings.TrimSpace(*req.LastNameEn)
	}
	if req.CitizenID != nil {
		if !isValidCitizenID(strings.TrimSpace(*req.CitizenID)) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "เลขบัตรประชาชนไม่ถูกต้อง",
			})
		}
		student.CitizenID = strings.TrimSpace(*req.CitizenID)
	}
	if req.CurrentEducation != nil {
		student.CurrentEducation = strings.TrimSpace(*req.CurrentEducation)
	}
	if req.PreferredBranch != nil {
		student.PreferredBranchID = req.PreferredBranch
	}
	if req.PreferredLanguage != nil {
		student.PreferredLanguage = strings.TrimSpace(*req.PreferredLanguage)
	}
	if req.LanguageLevel != nil {
		student.LanguageLevel = strings.TrimSpace(*req.LanguageLevel)
	}
	if req.LearningStyle != nil {
		student.LearningStyle = strings.TrimSpace(*req.LearningStyle)
	}
	if req.RecentCEFR != nil {
		student.RecentCEFR = strings.TrimSpace(*req.RecentCEFR)
	}
	if req.LearningGoals != nil {
		student.LearningGoals = strings.TrimSpace(*req.LearningGoals)
	}
	if req.TeacherType != nil {
		student.TeacherType = strings.TrimSpace(*req.TeacherType)
	}
	if req.EmergencyContact != nil {
		student.EmergencyContact = strings.TrimSpace(*req.EmergencyContact)
	}
	if req.EmergencyPhone != nil {
		student.EmergencyPhone = strings.TrimSpace(*req.EmergencyPhone)
	}
	if req.RegistrationStatus != nil {
		student.RegistrationStatus = strings.TrimSpace(*req.RegistrationStatus)
	}

	// Handle JSON fields
	if len(req.SelectedCourses) > 0 {
		selectedCoursesJSON, _ := json.Marshal(req.SelectedCourses)
		student.SelectedCourses = selectedCoursesJSON
	}
	if len(req.PreferredTimeSlots) > 0 {
		preferredSlotsJSON, _ := json.Marshal(req.PreferredTimeSlots)
		student.PreferredTimeSlots = preferredSlotsJSON
	}
	if len(req.UnavailableTimeSlots) > 0 {
		unavailableSlotsJSON, _ := json.Marshal(req.UnavailableTimeSlots)
		student.UnavailableTimeSlots = unavailableSlotsJSON
	}

	// Save changes
	if err := database.DB.Save(&student).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update student information",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "อัปเดตข้อมูลสำเร็จ",
		"data":    fiber.Map{"student": student},
	})
}

// GetStudentsByStatus filters students by registration status
func (sc *StudentController) GetStudentsByStatus(c *fiber.Ctx) error {
	status := c.Params("status")

	// Validate status
	validStatuses := []string{"pending_review", "schedule_exam", "waiting_for_group", "active"}
	isValid := false
	for _, validStatus := range validStatuses {
		if status == validStatus {
			isValid = true
			break
		}
	}

	if !isValid {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid status. Valid values: pending_review, schedule_exam, waiting_for_group, active",
		})
	}

	// Get pagination parameters
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	var students []models.Student
	var total int64

	// Query students by status
	query := database.DB.Model(&models.Student{}).Where("registration_status = ?", status)

	// Get total count
	query.Count(&total)

	// Get students with relationships
	if err := query.Preload("User").Preload("User.Branch").Preload("PreferredBranch").
		Offset(offset).Limit(limit).Find(&students).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch students",
		})
	}

	totalPages := (int(total) + limit - 1) / limit
	return c.JSON(fiber.Map{
		"students":    students,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
		"status":      status,
	})
}

// SetExamScores allows admins to record exam scores for students
func (sc *StudentController) SetExamScores(c *fiber.Ctx) error {
	// Get student ID from params
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid student ID",
		})
	}

	// Parse request body
	var req ExamScoresRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate scores
	if req.GrammarScore < 0 || req.GrammarScore > 100 ||
		req.SpeakingScore < 0 || req.SpeakingScore > 100 ||
		req.ListeningScore < 0 || req.ListeningScore > 100 ||
		req.ReadingScore < 0 || req.ReadingScore > 100 ||
		req.WritingScore < 0 || req.WritingScore > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "คะแนนต้องอยู่ระหว่าง 0-100",
		})
	}

	// Find student
	var student models.Student
	if err := database.DB.First(&student, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Student not found",
		})
	}

	// Update exam scores
	student.GrammarScore = &req.GrammarScore
	student.SpeakingScore = &req.SpeakingScore
	student.ListeningScore = &req.ListeningScore
	student.ReadingScore = &req.ReadingScore
	student.WritingScore = &req.WritingScore

	// Update registration status to Waiting for Group
	waitingStatus := "Waiting for Group"
	student.RegistrationStatus = waitingStatus

	// Save changes
	if err := database.DB.Save(&student).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save exam scores",
		})
	}

	// Calculate average score for response
	avgScore := (req.GrammarScore + req.SpeakingScore + req.ListeningScore + req.ReadingScore + req.WritingScore) / 5

	return c.JSON(fiber.Map{
		"success": true,
		"message": "บันทึกคะแนนสอบสำเร็จ",
		"data": fiber.Map{
			"student_id":      student.ID,
			"grammar_score":   student.GrammarScore,
			"speaking_score":  student.SpeakingScore,
			"listening_score": student.ListeningScore,
			"reading_score":   student.ReadingScore,
			"writing_score":   student.WritingScore,
			"average_score":   avgScore,
		},
	})
}

// getRegistrationTypeInThai converts registration type to Thai
func getRegistrationTypeInThai(regType string) string {
	switch strings.ToLower(regType) {
	case "quick":
		return "รวดเร็ว"
	case "full":
		return "เต็มรูปแบบ"
	default:
		return regType
	}
}
