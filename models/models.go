package models

import (
	"database/sql/driver"
	"time"

	"gorm.io/gorm"
)

// Base model with common fields
type BaseModel struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	CreatedAt *time.Time     `json:"created_at,omitempty"`
	UpdatedAt *time.Time     `json:"updated_at,omitempty"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

// JSON field type for GORM
type JSON []byte

func (j JSON) Value() (driver.Value, error) {
	if j.IsNull() {
		return nil, nil
	}
	return string(j), nil
}

func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	s, ok := value.([]byte)
	if !ok {
		return nil
	}
	*j = append((*j)[0:0], s...)
	return nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return nil
	}
	*j = append((*j)[0:0], data...)
	return nil
}

func (j JSON) IsNull() bool {
	return len(j) == 0 || string(j) == "null"
}

// Branch model
type Branch struct {
	BaseModel
	NameEn  string `json:"name_en" gorm:"size:255;not null"`
	NameTh  string `json:"name_th" gorm:"size:255;not null"`
	Code    string `json:"code" gorm:"size:50;not null;uniqueIndex"`
	Address string `json:"address" gorm:"size:500"`
	Phone   string `json:"phone" gorm:"size:20"`
	Type    string `json:"type" gorm:"size:50;not null;default:'offline';type:enum('offline','online')"` // offline, online
	Active  bool   `json:"active" gorm:"default:true"`

	// Relationships
	Users    []User    `json:"users,omitempty" gorm:"foreignKey:BranchID"`
	Rooms    []Room    `json:"rooms,omitempty" gorm:"foreignKey:BranchID"`
	Courses  []Course  `json:"courses,omitempty" gorm:"foreignKey:BranchID"`
	Teachers []Teacher `json:"teachers,omitempty" gorm:"foreignKey:BranchID"`
}

// User model
type User struct {
	BaseModel
	Username             string     `json:"username" gorm:"size:100;not null;uniqueIndex"`
	Password             string     `json:"-" gorm:"size:255;not null"`
	Email                *string    `json:"email" gorm:"size:255;uniqueIndex;default:null"`
	Phone                string     `json:"phone" gorm:"size:20"`
	LineID               string     `json:"line_id" gorm:"size:100"`
	Role                 string     `json:"role" gorm:"size:50;not null;default:'student';type:enum('owner','admin','teacher','student')"` // owner, admin, teacher, student
	BranchID             uint       `json:"branch_id" gorm:"not null"`
	Status               string     `json:"status" gorm:"size:50;not null;default:'active';type:enum('active','inactive','suspended')"` // active, inactive, suspended
	Avatar               string     `json:"avatar" gorm:"size:500"`
	PasswordResetToken   string     `json:"-" gorm:"size:255"`      // Token for password reset
	PasswordResetExpires *time.Time `json:"-"`                      // Token expiration time
	PasswordResetByAdmin bool       `json:"-" gorm:"default:false"` // Flag if password was reset by admin

	// Relationships
	Branch  Branch   `json:"branch,omitempty" gorm:"foreignKey:BranchID"`
	Student *Student `json:"student,omitempty" gorm:"foreignKey:UserID"`
	Teacher *Teacher `json:"teacher,omitempty" gorm:"foreignKey:UserID"`
}

// Student model
type Student struct {
	BaseModel
	// Make UserID nullable to allow public student registrations without linked user
	UserID      *uint      `json:"user_id" gorm:"uniqueIndex;default:null"` // <-- allow null
	FirstName   string     `json:"first_name" gorm:"size:100;not null"`
	FirstNameEn string     `json:"first_name_en" gorm:"size:100"`
	LastName    string     `json:"last_name" gorm:"size:100;not null"`
	LastNameEn  string     `json:"last_name_en" gorm:"size:100"`
	NicknameEn  string     `json:"nickname_en" gorm:"size:100;not null"`
	NicknameTh  string     `json:"nickname_th" gorm:"size:100;not null"`
	DateOfBirth *time.Time `json:"date_of_birth"`
	Gender      string     `json:"gender" gorm:"size:20;type:enum('male','female','other')"`
	Address     string     `json:"address" gorm:"size:500"`
	CitizenID   string     `json:"citizen_id" gorm:"size:13"`                                   // 13 digits for Thai ID
	Age         int        `json:"age"`                                                         // Auto-calculated from date_of_birth
	AgeGroup    string     `json:"age_group" gorm:"size:50;type:enum('kids','teens','adults')"` // kids, teens, adults

	// Contact Information
	Phone  string `json:"phone" gorm:"size:20"`
	Email  string `json:"email" gorm:"size:255"`
	LineID string `json:"line_id" gorm:"size:100"`

	// Education & Learning
	GradeLevel        string `json:"grade_level" gorm:"size:50"`
	CurrentEducation  string `json:"current_education" gorm:"size:100"`
	CEFRLevel         string `json:"cefr_level" gorm:"size:10"`
	PreferredLanguage string `json:"preferred_language" gorm:"size:50;type:enum('english','chinese')"`
	LanguageLevel     string `json:"language_level" gorm:"size:50"`
	RecentCEFR        string `json:"recent_cefr" gorm:"size:10"`
	LearningStyle     string `json:"learning_style" gorm:"size:50;type:enum('private','pair','group')"`
	LearningGoals     string `json:"learning_goals" gorm:"type:text"`
	PreferredBranchID *uint  `json:"preferred_branch_id" gorm:"index"`
	TeacherType       string `json:"teacher_type" gorm:"size:50"`

	// Emergency Contacts
	ParentName       string `json:"parent_name" gorm:"size:200"`
	ParentPhone      string `json:"parent_phone" gorm:"size:20"`
	EmergencyContact string `json:"emergency_contact" gorm:"size:200"`
	EmergencyPhone   string `json:"emergency_phone" gorm:"size:20"`

	// JSON Fields for complex data
	PreferredTimeSlots   JSON `json:"preferred_time_slots" gorm:"type:json"`
	UnavailableTimeSlots JSON `json:"unavailable_time_slots" gorm:"type:json"`
	SelectedCourses      JSON `json:"selected_courses" gorm:"type:json"`
	AvailabilitySchedule JSON `json:"availability_schedule" gorm:"type:json"`
	UnavailableTimes     JSON `json:"unavailable_times" gorm:"type:json"`

	// Test Scores (nullable until exam completed)
	GrammarScore   *int `json:"grammar_score"`
	SpeakingScore  *int `json:"speaking_score"`
	ListeningScore *int `json:"listening_score"`
	ReadingScore   *int `json:"reading_score"`
	WritingScore   *int `json:"writing_score"`

	// Registration & Status Management
	RegistrationStatus string `json:"registration_status" gorm:"size:50;type:enum('pending_review','schedule_exam','waiting_for_group','active');default:'pending_review'"`
	RegistrationType   string `json:"registration_type" gorm:"size:20;type:enum('quick','full');default:'full'"`

	// Legacy fields
	LearningPreferences  string     `json:"learning_preferences" gorm:"type:text"`
	PreferredTeacherType string     `json:"preferred_teacher_type" gorm:"size:50"`
	ContactSource        string     `json:"contact_source" gorm:"size:100"`
	DepositAmount        float64    `json:"deposit_amount" gorm:"type:decimal(10,2)"`
	PaymentStatus        string     `json:"payment_status" gorm:"size:50;type:enum('pending','paid','partial');default:'pending'"`
	LastStatusUpdate     *time.Time `json:"last_status_update"`
	DaysWaiting          int        `json:"days_waiting" gorm:"default:0"`
	AdminContact         bool       `json:"admin_contact" gorm:"default:false"`

	// Relationships
	User            User   `json:"user,omitempty" gorm:"foreignKey:UserID"`
	PreferredBranch Branch `json:"preferred_branch,omitempty" gorm:"foreignKey:PreferredBranchID"`
}

// Group model - represents a learning group with students, course, and payment status
type Group struct {
	BaseModel
	GroupName     string `json:"group_name" gorm:"size:100;not null;uniqueIndex"`
	CourseID      uint   `json:"course_id" gorm:"not null"`
	Level         string `json:"level" gorm:"size:50"`
	MaxStudents   int    `json:"max_students" gorm:"default:10"`
	Status        string `json:"status" gorm:"size:50;default:'active';type:enum('active','inactive','suspended','full','need-feeling','empty')"`
	PaymentStatus string `json:"payment_status" gorm:"size:50;default:'pending';type:enum('pending','deposit_paid','fully_paid')"`
	Description   string `json:"description" gorm:"type:text"`

	// Relationships
	Course  Course        `json:"course,omitempty" gorm:"foreignKey:CourseID"`
	Members []GroupMember `json:"members,omitempty" gorm:"foreignKey:GroupID"`
}

// GroupMember model - represents students in a group with individual payment status
type GroupMember struct {
	BaseModel
	GroupID       uint       `json:"group_id" gorm:"not null"`
	StudentID     uint       `json:"student_id" gorm:"not null"`
	PaymentStatus string     `json:"payment_status" gorm:"size:50;default:'pending';type:enum('pending','deposit_paid','fully_paid')"`
	JoinedAt      *time.Time `json:"joined_at"`
	Status        string     `json:"status" gorm:"size:50;default:'active';type:enum('active','inactive','suspended')"`

	// Relationships
	Group   Group   `json:"group,omitempty" gorm:"foreignKey:GroupID"`
	Student Student `json:"student,omitempty" gorm:"foreignKey:StudentID"`
}

// Keep Student_Group for backward compatibility, but mark as deprecated
type Student_Group struct {
	BaseModel
	StudentID uint   `json:"student_id" gorm:"not null"`
	GroupName string `json:"group_name" gorm:"size:100;not null"`
	Level     string `json:"level" gorm:"size:50"`
	Status    string `json:"status" gorm:"size:50;default:'active';type:enum('active','inactive','suspended','full','need-feeling','empty')"` // active, inactive
	CourseID  uint   `json:"course_id" gorm:"not null"`

	// Relationships
	Student Student `json:"student,omitempty" gorm:"foreignKey:StudentID"`
	Course  Course  `json:"course,omitempty" gorm:"foreignKey:CourseID"`
}

type LineGroup struct {
	BaseModel
	GroupName      string     `json:"group_name" gorm:"unique;not null"`
	GroupID        string     `json:"group_id" gorm:"unique;not null"`
	LastJoinedAt   time.Time  `json:"last_joined_at" gorm:"not null"`
	LastLeftAt     *time.Time `json:"last_left_at"` // null ได้ถ้า OA ยังอยู่ในกลุ่ม
	IsActive       bool       `json:"is_active" gorm:"default:true"`
	MatchedGroupID *uint      `json:"matched_group_id" gorm:"index"`
}

type User_inCourse struct {
	BaseModel
	UserID   uint `json:"user_id" gorm:"not null"`
	CourseID uint `json:"course_id" gorm:"not null"`

	Role   string `json:"role" gorm:"size:50;not null;type:enum('instructor','assistant','observer','student','teacher')"`        // instructor, assistant, observer, student, teacher
	Status string `json:"status" gorm:"size:50;default:'active';type:enum('active','inactive','enrolled','completed','dropped')"` // active, inactive, enrolled, completed, dropped
}

// Teacher model
type Teacher struct {
	BaseModel
	UserID          uint   `json:"user_id" gorm:"uniqueIndex;not null"`
	FirstNameEn     string `json:"first_name_en" gorm:"size:100"`
	FirstNameTh     string `json:"first_name_th" gorm:"size:100"`
	LastNameEn      string `json:"last_name_en" gorm:"size:100"`
	LastNameTh      string `json:"last_name_th" gorm:"size:100"`
	NicknameEn      string `json:"nickname_en" gorm:"size:100"`
	NicknameTh      string `json:"nickname_th" gorm:"size:100"`
	Nationality     string `json:"nationality" gorm:"size:100"`
	TeacherType     string `json:"teacher_type" gorm:"size:50;type:enum('Both','Adults','Kid','Admin Team')"` // Both, Adults, Kid, Admin Team
	HourlyRate      int    `json:"hourly_rate"`
	Specializations string `json:"specializations" gorm:"type:text"`
	Certifications  string `json:"certifications" gorm:"type:text"`
	Active          bool   `json:"active" gorm:"default:true"`
	BranchID        uint   `json:"branch_id"`

	// Relationships
	User   User   `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Branch Branch `json:"branch,omitempty" gorm:"foreignKey:BranchID"`
}

// Room model
type Room struct {
	BaseModel
	BranchID  uint   `json:"branch_id" gorm:"not null"`
	RoomName  string `json:"room_name" gorm:"size:100;not null"`
	Capacity  int    `json:"capacity" gorm:"not null"`
	Equipment JSON   `json:"equipment" gorm:"type:json"`
	Status    string `json:"status" gorm:"size:50;not null;default:'available';type:enum('available','occupied','maintenance')"` // available, occupied, maintenance

	// Relationships
	Branch Branch `json:"branch,omitempty" gorm:"foreignKey:BranchID"`
}

// Course model
type Course struct {
	BaseModel
	Name        string `json:"name" gorm:"size:255;not null"`
	Code        string `json:"code" gorm:"size:100;uniqueIndex"`
	CourseType  string `json:"course_type" gorm:"size:100"`
	BranchID    uint   `json:"branch_id"`
	Description string `json:"description" gorm:"type:text"`
	Status      string `json:"status" gorm:"size:50;default:'active';type:enum('active','inactive')"` // active, inactive
	CategoryID  *uint  `json:"category_id" gorm:"default:null"`
	DurationID  *uint  `json:"duration_id" gorm:"default:null"`
	Level       string `json:"level" gorm:"size:100"`

	// Relationships
	Branch   Branch         `json:"branch,omitempty" gorm:"foreignKey:BranchID"`
	Category CourseCategory `json:"category,omitempty" gorm:"foreignKey:CategoryID"`
}

type CourseCategory struct {
	BaseModel
	Name          string `json:"name" gorm:"size:100;not null;uniqueIndex"`
	NameEn        string `json:"name_en" gorm:"size:100;not null;uniqueIndex"`
	Description   string `json:"description" gorm:"type:text"`
	DescriptionEn string `json:"description_en" gorm:"type:text"`
	Type          string `json:"type" gorm:"size:50;type:enum('skills','business','test-prep','conversation','kids','language','other')"`
	Level         string `json:"level" gorm:"type:enum('A1','A2','B1','B2','C1','C2','HSK1','HSK2','HSK3','HSK4','HSK5','HSK6','HSK7','HSK8','HSK9');size:50"`
	SortOrder     int    `json:"sort_order" gorm:"default:1"`
	Active        bool   `json:"active" gorm:"default:true"`
}

// Log model for activity tracking
type ActivityLog struct {
	BaseModel
	UserID     uint   `json:"user_id"`
	Action     string `json:"action" gorm:"size:100;not null"`
	Resource   string `json:"resource" gorm:"size:100;not null"`
	ResourceID uint   `json:"resource_id"`
	Details    JSON   `json:"details" gorm:"type:json"`
	IPAddress  string `json:"ip_address" gorm:"size:45"`
	UserAgent  string `json:"user_agent" gorm:"size:500"`

	// Relationships
	User User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// Notification model
type Notification struct {
	BaseModel
	UserID    uint   `json:"user_id" gorm:"not null"`
	Title     string `json:"title" gorm:"size:255;not null"`
	TitleTh   string `json:"title_th" gorm:"size:255"`
	Message   string `json:"message" gorm:"type:text;not null"`
	MessageTh string `json:"message_th" gorm:"type:text"`
	Type      string `json:"type" gorm:"size:50;not null;type:enum('info','warning','error','success')"` // info, warning, error, success
	// Channels defines how to deliver/display this notification on the client side
	// Allowed values: "normal", "popup", "line". Can contain multiple.
	// Note: MySQL JSON columns cannot have a DB-level DEFAULT. We set the default at insert time in code.
	Channels JSON `json:"channels" gorm:"type:json"`
	// Data contains optional structured payload for deep-links or actions (e.g., links to sessions)
	Data   JSON       `json:"data,omitempty" gorm:"type:json"`
	Read   bool       `json:"read" gorm:"default:false"`
	ReadAt *time.Time `json:"read_at"`

	// Relationships
	User User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// LogArchive model for tracking archived logs
type LogArchive struct {
	BaseModel
	FileName    string    `json:"file_name" gorm:"size:255;not null"`
	S3Key       string    `json:"s3_key" gorm:"size:500;not null"`
	StartDate   time.Time `json:"start_date" gorm:"not null"`
	EndDate     time.Time `json:"end_date" gorm:"not null"`
	RecordCount int       `json:"record_count" gorm:"not null"`
	FileSize    int64     `json:"file_size" gorm:"not null"`
	Status      string    `json:"status" gorm:"size:50;not null;default:'pending';type:enum('pending','completed','failed')"` // pending, completed, failed
	Error       string    `json:"error" gorm:"type:text"`
}

type Schedule_Sessions struct {
	BaseModel
	ScheduleID            uint       `json:"schedule_id" gorm:"not null"`
	Session_date          *time.Time `json:"session_date,omitempty" gorm:"not null"`
	Start_time            *time.Time `json:"start_time,omitempty" gorm:"not null"`
	End_time              *time.Time `json:"end_time,omitempty" gorm:"not null"`
	Session_number        int        `json:"session_number" gorm:"not null"`
	Week_number           int        `json:"week_number" gorm:"not null"`
	Status                string     `json:"status" gorm:"size:50;default:'scheduled';type:enum('scheduled','confirmed','pending','completed','cancelled','rescheduled','no-show')"` // scheduled, confirmed, pending, completed, cancelled, rescheduled, no-show
	Cancelling_Reason     string     `json:"cancelling_reason" gorm:"type:text"`
	Is_makeup             bool       `json:"is_makeup" gorm:"default:false"`            //เป็นชดเชยไหม
	Makeup_for_session_id *uint      `json:"makeup_for_session_id" gorm:"default:null"` // ชดเชยให้กับ Session ID ไหน
	Notes                 string     `json:"notes" gorm:"type:text"`

	// New fields for enhanced session management
	AssignedTeacherID *uint      `json:"assigned_teacher_id" gorm:"default:null"` // Teacher can be different per session
	RoomID            *uint      `json:"room_id" gorm:"default:null"`             // Room can be different per session
	ConfirmedAt       *time.Time `json:"confirmed_at"`
	ConfirmedByUserID *uint      `json:"confirmed_by_user_id"`

	// Relationships
	Schedule        *Schedules `json:"schedule,omitempty" gorm:"foreignKey:ScheduleID"`
	AssignedTeacher *User      `json:"assigned_teacher,omitempty" gorm:"foreignKey:AssignedTeacherID"`
	Room            *Room      `json:"room,omitempty" gorm:"foreignKey:RoomID"`
	ConfirmedBy     *User      `json:"confirmed_by,omitempty" gorm:"foreignKey:ConfirmedByUserID"`
}

type Schedules struct {
	BaseModel
	// Core schedule information
	ScheduleName string `json:"schedule_name" gorm:"size:100;not null"`
	ScheduleType string `json:"schedule_type" gorm:"size:50;type:enum('class','meeting','event','holiday','appointment')"` // class, meeting, event, holiday, appointment

	// For class schedules - link to group
	GroupID *uint `json:"group_id" gorm:"default:null"` // For class schedules - links to learning group

	// For event/appointment schedules - creator and participants
	CreatedByUserID *uint `json:"created_by_user_id" gorm:"default:null"` // Who created this schedule

	// Schedule timing and recurrence
	Recurring_pattern  string     `json:"recurring_pattern" gorm:"size:100;type:enum('daily','weekly','bi-weekly','monthly','yearly','custom')"` // daily, weekly, bi-weekly, monthly, yearly, custom
	Total_hours        int        `json:"total_hours"`
	Hours_per_session  int        `json:"hours_per_session"`
	Session_per_week   int        `json:"session_per_week"`
	Start_date         time.Time  `json:"start_date"`
	Estimated_end_date time.Time  `json:"estimated_end_date"`
	Actual_end_date    *time.Time `json:"actual_end_date"`

	// Default assignments (can be overridden per session)
	DefaultTeacherID *uint `json:"default_teacher_id" gorm:"default:null"` // Default teacher for sessions
	DefaultRoomID    *uint `json:"default_room_id" gorm:"default:null"`    // Default room for sessions

	// Schedule management
	Status                  string `json:"status" gorm:"size:50;default:'scheduled';type:enum('scheduled','paused','completed','cancelled','assigned')"` // scheduled, active, paused, completed, cancelled
	Auto_Reschedule_holiday bool   `json:"auto_reschedule" gorm:"default:true"`
	Notes                   string `json:"notes" gorm:"type:text"`
	Admin_assigned          string `json:"admin_assigned" gorm:"size:200"`

	// Relationships
	Group          *Group              `json:"group,omitempty" gorm:"foreignKey:GroupID"`
	CreatedBy      *User               `json:"created_by,omitempty" gorm:"foreignKey:CreatedByUserID"`
	DefaultTeacher *User               `json:"default_teacher,omitempty" gorm:"foreignKey:DefaultTeacherID"`
	DefaultRoom    *Room               `json:"default_room,omitempty" gorm:"foreignKey:DefaultRoomID"`
	Sessions       []Schedule_Sessions `json:"sessions,omitempty" gorm:"foreignKey:ScheduleID"`
}

type ScheduleImport struct {
	BaseModel
	Source          string    `json:"source" gorm:"size:50;default:'schedulista'"`
	Fingerprint     string    `json:"fingerprint" gorm:"size:64;uniqueIndex"`
	AppointmentTime time.Time `json:"appointment_time" gorm:"not null"`
	ProviderName    string    `json:"provider_name" gorm:"size:255"`
	ServiceName     string    `json:"service_name" gorm:"size:255"`
	ClientName      string    `json:"client_name" gorm:"size:255"`
	BranchRaw       string    `json:"branch_raw" gorm:"size:255"`
	CourseName      string    `json:"course_name" gorm:"size:255"`
	ScheduleID      *uint     `json:"schedule_id" gorm:"index"`
	SessionID       *uint     `json:"session_id" gorm:"index"`
}

type Schedules_or_Sessions_Comment struct {
	BaseModel
	ScheduleID *uint  `json:"schedule_id" gorm:"null:true"`
	SessionID  *uint  `json:"session_id" gorm:"null:true"`
	UserID     uint   `json:"user_id" gorm:"not null"`
	Comment    string `json:"comment" gorm:"type:text;not null"`

	// Relationships
	Schedule *Schedules         `json:"schedule" gorm:"foreignKey:ScheduleID"`
	Session  *Schedule_Sessions `json:"session" gorm:"foreignKey:SessionID"`
	User     User               `json:"user" gorm:"foreignKey:UserID"`
}

// ScheduleParticipant model - for event/appointment participants
type ScheduleParticipant struct {
	BaseModel
	ScheduleID uint   `json:"schedule_id" gorm:"not null"`
	UserID     uint   `json:"user_id" gorm:"not null"`
	Role       string `json:"role" gorm:"size:50;default:'participant';type:enum('organizer','participant','observer')"`
	Status     string `json:"status" gorm:"size:50;default:'invited';type:enum('invited','confirmed','declined','tentative')"`

	// Relationships
	Schedule Schedules `json:"schedule" gorm:"foreignKey:ScheduleID"`
	User     User      `json:"user" gorm:"foreignKey:UserID"`
}

// SessionConfirmation model - tracks session confirmations
type SessionConfirmation struct {
	BaseModel
	SessionID   uint       `json:"session_id" gorm:"not null"`
	UserID      uint       `json:"user_id" gorm:"not null"`
	Status      string     `json:"status" gorm:"size:50;default:'pending';type:enum('pending','confirmed','declined','no_show')"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
	DeclinedAt  *time.Time `json:"declined_at"`
	Reason      string     `json:"reason" gorm:"type:text"` // Reason for decline or no-show

	// Relationships
	Session Schedule_Sessions `json:"session" gorm:"foreignKey:SessionID"`
	User    User              `json:"user" gorm:"foreignKey:UserID"`
}

// NotificationPreference model - configurable notification settings
type NotificationPreference struct {
	BaseModel
	UserID                  uint `json:"user_id" gorm:"not null;uniqueIndex"`
	EnableScheduleReminders bool `json:"enable_schedule_reminders" gorm:"default:true"`

	// Reminder timings (in minutes before session)
	FirstReminderMinutes  *int `json:"first_reminder_minutes" gorm:"default:1440"` // 24 hours = 1440 minutes
	SecondReminderMinutes *int `json:"second_reminder_minutes" gorm:"default:60"`  // 1 hour
	ThirdReminderMinutes  *int `json:"third_reminder_minutes" gorm:"default:15"`   // 15 minutes

	// Number of reminders to send (1-3)
	ReminderCount int `json:"reminder_count" gorm:"default:2;check:reminder_count >= 1 AND reminder_count <= 3"`

	// Relationship
	User User `json:"user" gorm:"foreignKey:UserID"`
}

// Book model - stores books used in courses/sessions
type Book struct {
	BaseModel
	Name        string `json:"name" gorm:"size:255;not null;uniqueIndex"`
	Author      string `json:"author" gorm:"size:255"`
	Edition     string `json:"edition" gorm:"size:100"`
	Language    string `json:"language" gorm:"size:50"`
	Description string `json:"description" gorm:"type:text"`
}

// ClassProgress model - imported per-session progress logs for a group/course
type ClassProgress struct {
	BaseModel
	// Provenance
	FileName       string `json:"file_name" gorm:"size:255"`
	FileID         string `json:"file_id" gorm:"size:255"`
	SpreadsheetURL string `json:"spreadsheet_url" gorm:"size:500"`
	SheetTab       string `json:"sheet_tab" gorm:"size:100"`

	// Relationships
	GroupID   *uint `json:"group_id" gorm:"index;default:null"`
	CourseID  *uint `json:"course_id" gorm:"index;default:null"`
	TeacherID *uint `json:"teacher_id" gorm:"index;default:null"`
	BookID    *uint `json:"book_id" gorm:"index;default:null"`

	// Session/progress info
	Number        *int       `json:"number" gorm:"default:null"` // Session number (No)
	LessonPlan    string     `json:"lesson_plan" gorm:"type:text"`
	Date          *time.Time `json:"date" gorm:"default:null"`
	Hour          *int       `json:"hour" gorm:"default:null"` // cumulative learned hours reached
	WarmUp        string     `json:"warm_up" gorm:"type:text"`
	Topic         string     `json:"topic" gorm:"type:text"`
	LastPage      string     `json:"last_page" gorm:"size:100"`
	ProgressCheck string     `json:"progress_check" gorm:"type:text"`
	Comment       string     `json:"comment" gorm:"type:text"`
	GoalInfo      string     `json:"goal_info" gorm:"type:text"`    // Goal + Information
	BookNameRaw   string     `json:"book_name_raw" gorm:"size:255"` // original text from import for trace

	// Group/course meta captured from import header (not normalized elsewhere)
	Level        string `json:"level" gorm:"size:100"`
	CoursePath   string `json:"course_path" gorm:"size:255"`
	TargetHours  *int   `json:"target_hours" gorm:"default:null"`
	SpecialHours *int   `json:"special_hours" gorm:"default:null"`
	TotalHours   *int   `json:"total_hours" gorm:"default:null"`
	BranchRaw    string `json:"branch_raw" gorm:"size:100"` // e.g., "1,3"
}

type Bill struct {
	BaseModel
	// Provenance
	Source string `json:"source" gorm:"size:50;default:'wave'"`
	// TransactionID is an application-generated deterministic ID that groups multiple lines of the same bill
	// Lines with the same invoice number will share the same TransactionID
	TransactionID string `json:"transaction_id" gorm:"size:100;index"`
	// SourceTransactionID stores the original ID from Wave export (normalized from scientific notation)
	SourceTransactionID string `json:"source_transaction_id" gorm:"size:100;index"`
	// RowUID is a deterministic unique identifier for a row constructed from multiple fields for deduplication
	RowUID          string     `json:"row_uid" gorm:"size:255;uniqueIndex"`
	TransactionDate *time.Time `json:"transaction_date"`

	// High-level bill semantics for easy understanding and filtering
	// BillType: normal (default), deposit, installment, payment, adjustment
	BillType          string `json:"bill_type" gorm:"size:20;default:'normal';type:enum('normal','deposit','installment','payment','adjustment')"`
	InstallmentNo     *int   `json:"installment_no"`
	TotalInstallments *int   `json:"total_installments"`

	// Wave export core columns
	AccountName                string `json:"account_name" gorm:"size:255"`
	TransactionDescription     string `json:"transaction_description" gorm:"size:500"`
	TransactionLineDescription string `json:"transaction_line_description" gorm:"size:500"`

	Amount       *float64 `json:"amount" gorm:"type:decimal(18,2)"`
	DebitAmount  *float64 `json:"debit_amount" gorm:"type:decimal(18,2)"`
	CreditAmount *float64 `json:"credit_amount" gorm:"type:decimal(18,2)"`

	OtherAccount  string `json:"other_account" gorm:"size:255"`
	Customer      string `json:"customer" gorm:"size:255"`
	InvoiceNumber string `json:"invoice_number" gorm:"size:100"`
	NotesMemo     string `json:"notes_memo" gorm:"type:text"`

	AmountBeforeSalesTax *float64 `json:"amount_before_sales_tax" gorm:"type:decimal(18,2)"`
	SalesTaxAmount       *float64 `json:"sales_tax_amount" gorm:"type:decimal(18,2)"`
	SalesTaxName         string   `json:"sales_tax_name" gorm:"size:255"`

	TransactionDateAdded        *time.Time `json:"transaction_date_added"`
	TransactionDateLastModified *time.Time `json:"transaction_date_last_modified"`

	AccountGroup string `json:"account_group" gorm:"size:100"`
	AccountType  string `json:"account_type" gorm:"size:100"`
	AccountID    string `json:"account_id" gorm:"size:100"`

	// Derived metadata
	PaymentMethod string `json:"payment_method" gorm:"size:20;type:enum('cash','debit_card','credit_card','transfer','other','unknown');default:'unknown'"`
	Currency      string `json:"currency" gorm:"size:10"`
	// Status represents the reconciliation/payment state of the bill.
	// Allowed values: 'Paid', 'Unpaid', 'Overdue', 'Partially Paid'
	Status string `json:"status" gorm:"size:50;default:'Unpaid';type:enum('Paid','Unpaid','Overdue','Partially Paid')"`

	// Optional due/paid dates (if applicable)
	DueDate  *time.Time `json:"due_date"`
	PaidDate *time.Time `json:"paid_date"`

	// Raw preserve
	Raw JSON `json:"raw" gorm:"type:json"`
}

// ประวัติการลา
type Absence struct {
    BaseModel
    GroupID   uint       `json:"group_id" gorm:"not null"`
    SessionID uint       `json:"session_id" gorm:"not null"`
    Reason    string     `json:"reason" gorm:"type:text"`
    Status    string     `json:"status" gorm:"size:50;default:'approved';type:enum('approved','rejected','pending')"`
    Note      string     `json:"note" gorm:"type:text"`
	CreatedBy   uint      `json:"created_by" gorm:"not null"` // user_id ของนักเรียนที่ส่งเรื่อง
	ApprovedBy  *uint     `json:"approved_by" gorm:"default:null"` // user_id ของแอดมินที่กดอนุมัติ
    ApprovedAt  *time.Time `json:"approved_at"`
	
	// Relationship
	Group   Group             `json:"group" gorm:"foreignKey:GroupID"`
	Session Schedule_Sessions `json:"session" gorm:"foreignKey:SessionID"`
}

// จำนวนสิทธิ์การลาคงเหลือ
type GroupLeaveQuota struct {
	BaseModel
    GroupID      uint      `gorm:"uniqueIndex;not null"` // ผูกกับกลุ่มเดียว
    TotalQuota   int       `gorm:"not null"`             // สิทธิ์ลา รวม (เช่น 2)
    UsedQuota    int       `gorm:"default:0"`            // สิทธิ์ที่ใช้ไปแล้ว
    LastUsedAt *time.Time  `json:"last_used_at"`
}


