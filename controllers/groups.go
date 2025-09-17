package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/models"
	"englishkorat_go/utils"
	"strconv"
	"time"

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

	// Preload course and its relations for response
	if err := database.DB.Preload("Course").Preload("Course.Branch").Preload("Course.Category").First(&group, group.ID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to load group details",
		})
	}

	dto := utils.ToGroupDTO(group)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Group created successfully",
		"group":   dto,
	})
}

// GetGroups retrieves all groups with optional filters
func (gc *GroupController) GetGroups(c *fiber.Ctx) error {
	courseID := c.Query("course_id")
	branchID := c.Query("branch_id")
	status := c.Query("status")
	paymentStatus := c.Query("payment_status")

	query := database.DB.Model(&models.Group{}).
		Preload("Course").
		Preload("Course.Branch").
		Preload("Course.Category").
		Preload("Members.Student")

	if courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}
	// Filter by branch via the joined course
	if branchID != "" {
		// join courses to filter by branch
		query = query.Joins("JOIN courses ON courses.id = groups.course_id").Where("courses.branch_id = ?", branchID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if paymentStatus != "" {
		query = query.Where("payment_status = ?", paymentStatus)
	}

	// Pagination
	page := 1
	perPage := 20
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := c.Query("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 {
			perPage = v
		}
	}
	if perPage > 100 {
		perPage = 100
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to count groups"})
	}

	offset := (page - 1) * perPage
	var groups []models.Group
	if err := query.Limit(perPage).Offset(offset).Find(&groups).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch groups",
		})
	}
	// Map to DTOs
	dtos := utils.ToGroupDTOs(groups)
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}
	return c.JSON(fiber.Map{
		"groups":      dtos,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
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
	if err := database.DB.
		Preload("Course").
		Preload("Course.Branch").
		Preload("Course.Category").
		Preload("Members.Student").
		First(&group, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Group not found",
		})
	}
	dto := utils.ToGroupDTO(group)
	return c.JSON(fiber.Map{
		"group": dto,
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
	now := time.Now()
	member := models.GroupMember{
		GroupID:       uint(groupID),
		StudentID:     req.StudentID,
		PaymentStatus: req.PaymentStatus,
		JoinedAt:      &now,
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

	// Reload member with student for DTO
	if err := database.DB.Preload("Student").First(&member, member.ID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to load member details",
		})
	}
	memberDTO := utils.ToGroupMemberDTO(member)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Member added to group successfully",
		"member":  memberDTO,
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
