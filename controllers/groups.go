package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type GroupController struct{}

type CreateGroupRequest struct {
	GroupName     string `json:"group_name" validate:"required"`
	CourseID      uint   `json:"course_id" validate:"required"`
	Level         string `json:"level"`
	MaxStudents   int    `json:"max_students" validate:"required,min=1"`
	PaymentStatus string `json:"payment_status" validate:"oneof=pending deposit_paid fully_paid"`
	Description   string `json:"description"`
}

type AddMemberToGroupRequest struct {
	StudentID     uint   `json:"student_id" validate:"required"`
	PaymentStatus string `json:"payment_status" validate:"oneof=pending deposit_paid fully_paid"`
}

// CreateGroup creates a new learning group
func (gc *GroupController) CreateGroup(c *fiber.Ctx) error {
	var req CreateGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate that the course exists
	var course models.Course
	if err := database.DB.First(&course, req.CourseID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Course not found",
		})
	}

	// Create the group
	group := models.Group{
		GroupName:     req.GroupName,
		CourseID:      req.CourseID,
		Level:         req.Level,
		MaxStudents:   req.MaxStudents,
		Status:        "active",
		PaymentStatus: req.PaymentStatus,
		Description:   req.Description,
	}

	if req.PaymentStatus == "" {
		group.PaymentStatus = "pending"
	}

	if err := database.DB.Create(&group).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create group",
		})
	}

	// Preload course for response
	if err := database.DB.Preload("Course").First(&group, group.ID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to load group details",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Group created successfully",
		"group":   group,
	})
}

// GetGroups retrieves all groups with optional filters
func (gc *GroupController) GetGroups(c *fiber.Ctx) error {
	courseID := c.Query("course_id")
	status := c.Query("status")
	paymentStatus := c.Query("payment_status")

	query := database.DB.Preload("Course").Preload("Members.Student")

	if courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if paymentStatus != "" {
		query = query.Where("payment_status = ?", paymentStatus)
	}

	var groups []models.Group
	if err := query.Find(&groups).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch groups",
		})
	}

	return c.JSON(fiber.Map{
		"groups": groups,
	})
}

// GetGroup retrieves a specific group by ID
func (gc *GroupController) GetGroup(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid group ID",
		})
	}

	var group models.Group
	if err := database.DB.Preload("Course").Preload("Members.Student").First(&group, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Group not found",
		})
	}

	return c.JSON(fiber.Map{
		"group": group,
	})
}

// AddMemberToGroup adds a student to a group
func (gc *GroupController) AddMemberToGroup(c *fiber.Ctx) error {
	groupID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid group ID",
		})
	}

	var req AddMemberToGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Check if group exists
	var group models.Group
	if err := database.DB.First(&group, groupID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Group not found",
		})
	}

	// Check if student exists
	var student models.Student
	if err := database.DB.First(&student, req.StudentID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Student not found",
		})
	}

	// Check if student is already in the group
	var existingMember models.GroupMember
	if err := database.DB.Where("group_id = ? AND student_id = ?", groupID, req.StudentID).First(&existingMember).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Student is already a member of this group",
		})
	}

	// Check if group has space
	var memberCount int64
	database.DB.Model(&models.GroupMember{}).Where("group_id = ? AND status = ?", groupID, "active").Count(&memberCount)
	if int(memberCount) >= group.MaxStudents {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Group is full",
		})
	}

	// Add member to group
	member := models.GroupMember{
		GroupID:       uint(groupID),
		StudentID:     req.StudentID,
		PaymentStatus: req.PaymentStatus,
		Status:        "active",
	}

	if req.PaymentStatus == "" {
		member.PaymentStatus = "pending"
	}

	if err := database.DB.Create(&member).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to add member to group",
		})
	}

	// Update group status if needed
	database.DB.Model(&models.GroupMember{}).Where("group_id = ? AND status = ?", groupID, "active").Count(&memberCount)
	if int(memberCount) >= group.MaxStudents {
		database.DB.Model(&group).Update("status", "full")
	} else if memberCount > 0 && group.Status == "empty" {
		database.DB.Model(&group).Update("status", "active")
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Member added to group successfully",
		"member":  member,
	})
}

// RemoveMemberFromGroup removes a student from a group
func (gc *GroupController) RemoveMemberFromGroup(c *fiber.Ctx) error {
	groupID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid group ID",
		})
	}

	studentID, err := strconv.Atoi(c.Params("student_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid student ID",
		})
	}

	// Find and update member status
	var member models.GroupMember
	if err := database.DB.Where("group_id = ? AND student_id = ?", groupID, studentID).First(&member).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Student is not a member of this group",
		})
	}

	// Update member status to inactive instead of deleting
	member.Status = "inactive"
	if err := database.DB.Save(&member).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to remove member from group",
		})
	}

	// Update group status if needed
	var memberCount int64
	database.DB.Model(&models.GroupMember{}).Where("group_id = ? AND status = ?", groupID, "active").Count(&memberCount)
	if memberCount == 0 {
		database.DB.Model(&models.Group{}).Where("id = ?", groupID).Update("status", "empty")
	} else {
		database.DB.Model(&models.Group{}).Where("id = ?", groupID).Update("status", "active")
	}

	return c.JSON(fiber.Map{
		"message": "Member removed from group successfully",
	})
}

// UpdateGroupPaymentStatus updates the payment status of a group or member
func (gc *GroupController) UpdateGroupPaymentStatus(c *fiber.Ctx) error {
	groupID, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid group ID",
		})
	}

	var req struct {
		PaymentStatus string `json:"payment_status" validate:"required,oneof=pending deposit_paid fully_paid"`
		StudentID     *uint  `json:"student_id,omitempty"` // If provided, update member status, otherwise group status
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.StudentID != nil {
		// Update member payment status
		var member models.GroupMember
		if err := database.DB.Where("group_id = ? AND student_id = ?", groupID, *req.StudentID).First(&member).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Student is not a member of this group",
			})
		}

		member.PaymentStatus = req.PaymentStatus
		if err := database.DB.Save(&member).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update member payment status",
			})
		}

		return c.JSON(fiber.Map{
			"message": "Member payment status updated successfully",
		})
	} else {
		// Update group payment status
		var group models.Group
		if err := database.DB.First(&group, groupID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Group not found",
			})
		}

		group.PaymentStatus = req.PaymentStatus
		if err := database.DB.Save(&group).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update group payment status",
			})
		}

		return c.JSON(fiber.Map{
			"message": "Group payment status updated successfully",
		})
	}
}