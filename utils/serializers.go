package utils

import (
	"encoding/json"
	"strings"
	"time"

	"englishkorat_go/models"
)

// Compact representations used across APIs
type UserShort struct {
	ID          uint   `json:"id"`
	FirstNameEn string `json:"first_name_en,omitempty"`
	FirstNameTh string `json:"first_name_th,omitempty"`
	LastNameEn  string `json:"last_name_en,omitempty"`
	LastNameTh  string `json:"last_name_th,omitempty"`
}

type BranchShort struct {
	ID     uint   `json:"id"`
	NameEn string `json:"name_en,omitempty"`
	NameTh string `json:"name_th,omitempty"`
}

type CategoryShort struct {
	ID     uint   `json:"id"`
	Name   string `json:"name,omitempty"`
	NameEn string `json:"name_en,omitempty"`
}

// Student minimal representation
type StudentShort struct {
	ID          uint   `json:"id"`
	FirstNameEn string `json:"first_name_en,omitempty"`
	FirstNameTh string `json:"first_name_th,omitempty"`
	LastNameEn  string `json:"last_name_en,omitempty"`
	LastNameTh  string `json:"last_name_th,omitempty"`
	NicknameEn  string `json:"nickname_en,omitempty"`
	NicknameTh  string `json:"nickname_th,omitempty"`
}

// Course DTO - compact normalized representation returned by public endpoints
type CourseDTO struct {
	ID          uint          `json:"id"`
	CreatedAt   *time.Time    `json:"created_at"`
	UpdatedAt   *time.Time    `json:"updated_at"`
	Name        string        `json:"name"`
	Code        string        `json:"code,omitempty"`
	CourseType  string        `json:"course_type,omitempty"`
	BranchID    uint          `json:"branch_id,omitempty"`
	Description string        `json:"description,omitempty"`
	Status      string        `json:"status,omitempty"`
	CategoryID  *uint         `json:"category_id,omitempty"`
	DurationID  *uint         `json:"duration_id,omitempty"`
	Level       string        `json:"level,omitempty"`
	Branch      BranchShort   `json:"branch,omitempty"`
	Category    CategoryShort `json:"category,omitempty"`
}

type Sender struct {
	Type string `json:"type"` // "system" or "user"
	ID   *uint  `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type Recipient struct {
	Type string `json:"type"` // "user", "role", etc.
	ID   uint   `json:"id"`
}

type NotificationDTO struct {
	ID        uint        `json:"id"`
	CreatedAt *time.Time  `json:"created_at"`
	UpdatedAt *time.Time  `json:"updated_at"`
	UserID    uint        `json:"user_id"`
	Title     string      `json:"title"`
	TitleTh   string      `json:"title_th,omitempty"`
	Message   string      `json:"message"`
	MessageTh string      `json:"message_th,omitempty"`
	Type      string      `json:"type"`
	Channels  []string    `json:"channels,omitempty"`
	Read      bool        `json:"read"`
	ReadAt    *time.Time  `json:"read_at,omitempty"`
	User      UserShort   `json:"user"`
	Branch    BranchShort `json:"branch"`
	Sender    Sender      `json:"sender"`
	Recipient Recipient   `json:"recipient"`
}

// ToNotificationDTO maps a models.Notification to the compact DTO.
// Assumptions: caller has preloaded User, User.Student, and User.Branch when possible.
func ToNotificationDTO(n models.Notification) NotificationDTO {
	var us UserShort
	var bs BranchShort
	// Parse channels JSON -> []string, default to ["normal"] if none
	channels := []string{}
	if !n.Channels.IsNull() {
		// best-effort parse; ignore errors and leave empty to fall back
		_ = json.Unmarshal(n.Channels, &channels)
	}
	if len(channels) == 0 {
		channels = []string{"normal"}
	}

	// User name from Student profile if available
	if n.User.Student != nil {
		us = UserShort{
			ID:          n.User.ID,
			FirstNameEn: n.User.Student.FirstNameEn,
			FirstNameTh: n.User.Student.FirstName,
			LastNameEn:  n.User.Student.LastNameEn,
			LastNameTh:  n.User.Student.LastName,
		}
	} else if n.User.Teacher != nil {
		us = UserShort{
			ID:          n.User.ID,
			FirstNameEn: n.User.Teacher.FirstNameEn,
			FirstNameTh: n.User.Teacher.FirstNameTh,
			LastNameEn:  n.User.Teacher.LastNameEn,
			LastNameTh:  n.User.Teacher.LastNameTh,
		}
	} else {
		// Fallback: use username or email local-part if no profile exists
		name := n.User.Username
		if name == "" && n.User.Email != nil && *n.User.Email != "" {
			parts := strings.Split(*n.User.Email, "@")
			name = parts[0]
		}
		// split into first/last if possible
		parts := strings.Fields(name)
		fnameEn := ""
		lnameEn := ""
		if len(parts) > 0 {
			fnameEn = parts[0]
		}
		if len(parts) > 1 {
			lnameEn = strings.Join(parts[1:], " ")
		}
		us = UserShort{ID: n.User.ID, FirstNameEn: fnameEn, LastNameEn: lnameEn, FirstNameTh: fnameEn, LastNameTh: lnameEn}
	}

	// Ensure both language fields are present: if one side missing, copy from the other
	if us.FirstNameEn == "" && us.FirstNameTh != "" {
		us.FirstNameEn = us.FirstNameTh
	}
	if us.FirstNameTh == "" && us.FirstNameEn != "" {
		us.FirstNameTh = us.FirstNameEn
	}
	if us.LastNameEn == "" && us.LastNameTh != "" {
		us.LastNameEn = us.LastNameTh
	}
	if us.LastNameTh == "" && us.LastNameEn != "" {
		us.LastNameTh = us.LastNameEn
	}

	// Branch short
	if n.User.Branch.ID != 0 {
		bs = BranchShort{ID: n.User.Branch.ID, NameEn: n.User.Branch.NameEn, NameTh: n.User.Branch.NameTh}
	}

	// Sender: models don't track created_by; default to system. If later we add created_by, update mapping.
	sender := Sender{Type: "system", Name: "Notification Service"}

	recipient := Recipient{Type: "user", ID: n.UserID}

	return NotificationDTO{
		ID:        n.ID,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
		UserID:    n.UserID,
		Title:     n.Title,
		TitleTh:   n.TitleTh,
		Message:   n.Message,
		MessageTh: n.MessageTh,
		Type:      n.Type,
		Channels:  channels,
		Read:      n.Read,
		ReadAt:    n.ReadAt,
		User:      us,
		Branch:    bs,
		Sender:    sender,
		Recipient: recipient,
	}
}

// ToCourseDTO maps a models.Course to the compact CourseDTO
func ToCourseDTO(c models.Course) CourseDTO {
	var bs BranchShort
	var cs CategoryShort

	if c.Branch.ID != 0 {
		bs = BranchShort{ID: c.Branch.ID, NameEn: c.Branch.NameEn, NameTh: c.Branch.NameTh}
	}
	if c.Category.ID != 0 {
		cs = CategoryShort{ID: c.Category.ID, Name: c.Category.Name, NameEn: c.Category.NameEn}
	}

	return CourseDTO{
		ID:          c.ID,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
		Name:        c.Name,
		Code:        c.Code,
		CourseType:  c.CourseType,
		BranchID:    c.BranchID,
		Description: c.Description,
		Status:      c.Status,
		CategoryID:  c.CategoryID,
		DurationID:  c.DurationID,
		Level:       c.Level,
		Branch:      bs,
		Category:    cs,
	}
}

// ToCourseDTOs maps a slice of models.Course to slice of CourseDTO
func ToCourseDTOs(src []models.Course) []CourseDTO {
	out := make([]CourseDTO, 0, len(src))
	for _, c := range src {
		out = append(out, ToCourseDTO(c))
	}
	return out
}

// Group and GroupMember DTOs
type GroupMemberDTO struct {
	ID            uint         `json:"id"`
	CreatedAt     *time.Time   `json:"created_at,omitempty"`
	UpdatedAt     *time.Time   `json:"updated_at,omitempty"`
	GroupID       uint         `json:"group_id"`
	StudentID     uint         `json:"student_id"`
	PaymentStatus string       `json:"payment_status"`
	JoinedAt      *time.Time   `json:"joined_at,omitempty"`
	Status        string       `json:"status"`
	Student       StudentShort `json:"student"`
}

type GroupDTO struct {
	ID            uint             `json:"id"`
	CreatedAt     *time.Time       `json:"created_at"`
	UpdatedAt     *time.Time       `json:"updated_at"`
	GroupName     string           `json:"group_name"`
	CourseID      uint             `json:"course_id,omitempty"`
	Level         string           `json:"level,omitempty"`
	MaxStudents   int              `json:"max_students"`
	Status        string           `json:"status"`
	PaymentStatus string           `json:"payment_status"`
	Description   string           `json:"description,omitempty"`
	Course        CourseDTO        `json:"course"`
	Members       []GroupMemberDTO `json:"members"`
}

// Map GroupMember to DTO
func ToGroupMemberDTO(m models.GroupMember) GroupMemberDTO {
	// Build student short
	ss := StudentShort{}
	if m.Student.ID != 0 {
		ss = StudentShort{
			ID:          m.Student.ID,
			FirstNameEn: m.Student.FirstNameEn,
			FirstNameTh: m.Student.FirstName,
			LastNameEn:  m.Student.LastNameEn,
			LastNameTh:  m.Student.LastName,
			NicknameEn:  m.Student.NicknameEn,
			NicknameTh:  m.Student.NicknameTh,
		}
		// Ensure both languages have values when one side exists
		if ss.FirstNameEn == "" && ss.FirstNameTh != "" {
			ss.FirstNameEn = ss.FirstNameTh
		}
		if ss.FirstNameTh == "" && ss.FirstNameEn != "" {
			ss.FirstNameTh = ss.FirstNameEn
		}
		if ss.LastNameEn == "" && ss.LastNameTh != "" {
			ss.LastNameEn = ss.LastNameTh
		}
		if ss.LastNameTh == "" && ss.LastNameEn != "" {
			ss.LastNameTh = ss.LastNameEn
		}
	}

	return GroupMemberDTO{
		ID:            m.ID,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
		GroupID:       m.GroupID,
		StudentID:     m.StudentID,
		PaymentStatus: m.PaymentStatus,
		JoinedAt:      m.JoinedAt,
		Status:        m.Status,
		Student:       ss,
	}
}

// Map Group to DTO
func ToGroupDTO(g models.Group) GroupDTO {
	// Course compact
	cdto := ToCourseDTO(g.Course)

	// Members slice - ensure empty array (not null)
	members := make([]GroupMemberDTO, 0, len(g.Members))
	for _, m := range g.Members {
		members = append(members, ToGroupMemberDTO(m))
	}

	return GroupDTO{
		ID:            g.ID,
		CreatedAt:     g.CreatedAt,
		UpdatedAt:     g.UpdatedAt,
		GroupName:     g.GroupName,
		CourseID:      g.CourseID,
		Level:         g.Level,
		MaxStudents:   g.MaxStudents,
		Status:        g.Status,
		PaymentStatus: g.PaymentStatus,
		Description:   g.Description,
		Course:        cdto,
		Members:       members,
	}
}

func ToGroupDTOs(src []models.Group) []GroupDTO {
	out := make([]GroupDTO, 0, len(src))
	for _, g := range src {
		out = append(out, ToGroupDTO(g))
	}
	return out
}
