package models

import (
	"database/sql/driver"
	"time"

	"gorm.io/gorm"
)

// Base model with common fields
type BaseModel struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
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
	Type    string `json:"type" gorm:"size:50;not null;default:'offline'"` // offline, online
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
	Username string `json:"username" gorm:"size:100;not null;uniqueIndex"`
	Password string `json:"-" gorm:"size:255;not null"`
	Email    string `json:"email" gorm:"size:255;uniqueIndex"`
	Phone    string `json:"phone" gorm:"size:20"`
	LineID   string `json:"line_id" gorm:"size:100"`
	Role     string `json:"role" gorm:"size:50;not null;default:'student'"` // owner, admin, teacher, student
	BranchID uint   `json:"branch_id" gorm:"not null"`
	Status   string `json:"status" gorm:"size:50;not null;default:'active'"` // active, inactive, suspended
	Avatar   string `json:"avatar" gorm:"size:500"`

	// Relationships
	Branch  Branch   `json:"branch,omitempty" gorm:"foreignKey:BranchID"`
	Student *Student `json:"student,omitempty" gorm:"foreignKey:UserID"`
	Teacher *Teacher `json:"teacher,omitempty" gorm:"foreignKey:UserID"`
}

// Student model
type Student struct {
	BaseModel
	UserID               uint       `json:"user_id" gorm:"uniqueIndex;not null"`
	FirstName            string     `json:"first_name" gorm:"size:100"`
	FirstNameEn          string     `json:"first_name_en" gorm:"size:100"`
	LastName             string     `json:"last_name" gorm:"size:100"`
	LastNameEn           string     `json:"last_name_en" gorm:"size:100"`
	NicknameEn           string     `json:"nickname_en" gorm:"size:100"`
	NicknameTh           string     `json:"nickname_th" gorm:"size:100"`
	DateOfBirth          *time.Time `json:"date_of_birth"`
	Gender               string     `json:"gender" gorm:"size:20"`
	Address              string     `json:"address" gorm:"size:500"`
	CitizenID            string     `json:"citizen_id" gorm:"size:255"` // encrypted
	Age                  int        `json:"age"`
	AgeGroup             string     `json:"age_group" gorm:"size:50"` // kids, teens, adults
	GradeLevel           string     `json:"grade_level" gorm:"size:50"`
	CurrentEducation     string     `json:"current_education" gorm:"size:100"`
	CEFRLevel            string     `json:"cefr_level" gorm:"size:10"`
	PreferredLanguage    string     `json:"preferred_language" gorm:"size:50"`
	LanguageLevel        string     `json:"language_level" gorm:"size:50"`
	RecentCEFR           string     `json:"recent_cefr" gorm:"size:10"`
	LearningStyle        string     `json:"learning_style" gorm:"size:50"`
	LearningGoals        string     `json:"learning_goals" gorm:"type:text"`
	ParentName           string     `json:"parent_name" gorm:"size:200"`
	ParentPhone          string     `json:"parent_phone" gorm:"size:20"`
	EmergencyContact     string     `json:"emergency_contact" gorm:"size:200"`
	EmergencyPhone       string     `json:"emergency_phone" gorm:"size:20"`
	PreferredTimeSlots   JSON       `json:"preferred_time_slots" gorm:"type:json"`
	UnavailableTimeSlots JSON       `json:"unavailable_time_slots" gorm:"type:json"`
	SelectedCourses      string     `json:"selected_courses" gorm:"size:500"`
	GrammarScore         int        `json:"grammar_score"`
	SpeakingScore        int        `json:"speaking_score"`
	ListeningScore       int        `json:"listening_score"`
	ReadingScore         int        `json:"reading_score"`
	WritingScore         int        `json:"writing_score"`
	LearningPreferences  string     `json:"learning_preferences" gorm:"type:text"`
	AvailabilitySchedule JSON       `json:"availability_schedule" gorm:"type:json"`
	UnavailableTimes     JSON       `json:"unavailable_times" gorm:"type:json"`
	PreferredTeacherType string     `json:"preferred_teacher_type" gorm:"size:50"`
	ContactSource        string     `json:"contact_source" gorm:"size:100"`
	RegistrationStatus   string     `json:"registration_status" gorm:"size:50"`
	DepositAmount        int        `json:"deposit_amount"`
	PaymentStatus        string     `json:"payment_status" gorm:"size:50"`
	LastStatusUpdate     *time.Time `json:"last_status_update"`
	DaysWaiting          int        `json:"days_waiting"`
	AdminContact         string     `json:"admin_contact" gorm:"size:200"`

	// Relationships
	User User `json:"user,omitempty" gorm:"foreignKey:UserID"`
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
	TeacherType     string `json:"teacher_type" gorm:"size:50"` // Both, Adults, Kid, Admin Team
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
	Status    string `json:"status" gorm:"size:50;not null;default:'available'"` // available, occupied, maintenance

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
	Status      string `json:"status" gorm:"size:50;default:'active'"` // active, inactive
	CategoryID  uint   `json:"category_id"`
	DurationID  uint   `json:"duration_id"`
	Level       string `json:"level" gorm:"size:100"`

	// Relationships
	Branch Branch `json:"branch,omitempty" gorm:"foreignKey:BranchID"`
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
	UserID    uint       `json:"user_id" gorm:"not null"`
	Title     string     `json:"title" gorm:"size:255;not null"`
	TitleTh   string     `json:"title_th" gorm:"size:255"`
	Message   string     `json:"message" gorm:"type:text;not null"`
	MessageTh string     `json:"message_th" gorm:"type:text"`
	Type      string     `json:"type" gorm:"size:50;not null"` // info, warning, error, success
	Read      bool       `json:"read" gorm:"default:false"`
	ReadAt    *time.Time `json:"read_at"`

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
	Status      string    `json:"status" gorm:"size:50;not null;default:'pending'"` // pending, completed, failed
	Error       string    `json:"error" gorm:"type:text"`
}
