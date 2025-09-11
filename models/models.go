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
	Email                string     `json:"email" gorm:"size:255;uniqueIndex"`
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
	AgeGroup             string     `json:"age_group" gorm:"size:50;type:enum('kids','teens','adults')"` // kids, teens, adults
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
	CategoryID  uint   `json:"category_id"`
	DurationID  uint   `json:"duration_id"`
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
	UserID    uint       `json:"user_id" gorm:"not null"`
	Title     string     `json:"title" gorm:"size:255;not null"`
	TitleTh   string     `json:"title_th" gorm:"size:255"`
	Message   string     `json:"message" gorm:"type:text;not null"`
	MessageTh string     `json:"message_th" gorm:"type:text"`
	Type      string     `json:"type" gorm:"size:50;not null;type:enum('info','warning','error','success')"` // info, warning, error, success
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
	Status      string    `json:"status" gorm:"size:50;not null;default:'pending';type:enum('pending','completed','failed')"` // pending, completed, failed
	Error       string    `json:"error" gorm:"type:text"`
}

type Schedule_Sessions struct {
	ScheduleID        uint      `json:"schedule_id" gorm:"not null"`
	Session_date      time.Time `json:"session_date" gorm:"not null"`
	Start_time        time.Time `json:"start_time" gorm:"not null"`
	End_time          time.Time `json:"end_time" gorm:"not null"`
	Session_number    int       `json:"session_number" gorm:"not null"`
	Week_number       int       `json:"week_number" gorm:"not null"`
	Status            string    `json:"status" gorm:"size:50;default:'scheduled';type:enum('scheduled','confirmed','pending','completed','cancelled','rescheduled','no-show')"` // scheduled, confirmed, pending, completed, cancelled, rescheduled, no-show
	Cencelling_Reason string    `json:"cencelling_reason" gorm:"type:text"`
	Is_makeup         bool      `json:"is_makeup" gorm:"default:false"` //เป็นชดเชยไหม
	// Use a nullable foreign key so normal sessions insert NULL instead of 0
	Makeup_for_session_id *uint  `json:"makeup_for_session_id" gorm:"default:null"` // ชดเชยให้กับ Session ID ไหน
	Notes                 string `json:"notes" gorm:"type:text"`
	BaseModel

	// Relationships
	Schedule Schedules `json:"schedule" gorm:"foreignKey:ScheduleID"`
}

type Schedules struct {
	BaseModel
	CourseID                uint       `json:"course_id"`
	User_inCourseID         uint       `json:"user_in_course_id" gorm:"not null"`
	AssignedToUserID        uint       `json:"assigned_to_teacher_id"`
	RoomID                  uint       `json:"room_id"`
	ScheduleName            string     `json:"schedule_name" gorm:"size:100;not null"`
	ScheduleType            string     `json:"schedule_type" gorm:"size:50;type:enum('class','meeting','event','holiday','appointment')"`             // class, meeting, event, holiday, appointment
	Recurring_pattern       string     `json:"recurring_pattern" gorm:"size:100;type:enum('daily','weekly','bi-weekly','monthly','yearly','custom')"` // daily, weekly, bi-weekly, monthly, yearly, custom
	Total_hours             int        `json:"total_hours"`
	Hours_per_session       int        `json:"hours_per_session"`
	Session_per_week        int        `json:"session_per_week"`
	Max_students            int        `json:"max_students"`
	Current_students        int        `json:"current_students"`
	Start_date              time.Time  `json:"start_date"`
	Estimated_end_date      time.Time  `json:"estimated_end_date"`
	Actual_end_date         *time.Time `json:"actual_end_date"`
	Status                  string     `json:"status" gorm:"size:50;default:'scheduled';type:enum('scheduled','paused','completed','cancelled','assigned')"` // scheduled, active, paused, completed, cancelled
	Auto_Reschedule_holiday bool       `json:"auto_reschedule" gorm:"default:true"`
	Notes                   string     `json:"notes" gorm:"type:text"`
	Admin_assigned          string     `json:"admin_assigned" gorm:"size:200"`

	// Relationships
	Course        Course        `json:"course" gorm:"foreignKey:CourseID"`
	User_inCourse User_inCourse `json:"user_in_course" gorm:"foreignKey:User_inCourseID"`
	AssignedTo    User          `json:"assigned_to" gorm:"foreignKey:AssignedToUserID"`
	Room          Room          `json:"room" gorm:"foreignKey:RoomID"`
}

type Schedules_or_Sessions_Comment struct {
	BaseModel
	ScheduleID uint   `json:"schedule_id" gorm:"null:true"`
	SessionID  uint   `json:"session_id" gorm:"null:true"`
	UserID     uint   `json:"user_id" gorm:"not null"`
	Comment    string `json:"comment" gorm:"type:text;not null"`

	// Relationships
	Schedule Schedules         `json:"schedule" gorm:"foreignKey:ScheduleID"`
	Session  Schedule_Sessions `json:"session" gorm:"foreignKey:SessionID"`
	User     User              `json:"user" gorm:"foreignKey:UserID"`
}
