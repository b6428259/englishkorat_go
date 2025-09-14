package utils

import (
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
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	UserID    uint        `json:"user_id"`
	Title     string      `json:"title"`
	TitleTh   string      `json:"title_th,omitempty"`
	Message   string      `json:"message"`
	MessageTh string      `json:"message_th,omitempty"`
	Type      string      `json:"type"`
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
		if name == "" && n.User.Email != "" {
			parts := strings.Split(n.User.Email, "@")
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
		Read:      n.Read,
		ReadAt:    n.ReadAt,
		User:      us,
		Branch:    bs,
		Sender:    sender,
		Recipient: recipient,
	}
}
