package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type TeacherController struct{}

// GetTeachers returns all teachers with pagination
func (tc *TeacherController) GetTeachers(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	var teachers []models.Teacher
	var total int64

	query := database.DB.Model(&models.Teacher{})

	// Filter by teacher type if specified
	if teacherType := c.Query("teacher_type"); teacherType != "" {
		query = query.Where("teacher_type = ?", teacherType)
	}

	// Filter by active status
	if active := c.Query("active"); active == "false" {
		query = query.Where("active = ?", false)
	} else {
		query = query.Where("active = ?", true)
	}

	// Filter by branch if specified
	if branchID := c.Query("branch_id"); branchID != "" {
		query = query.Where("branch_id = ?", branchID)
	}

	// Get total count
	query.Count(&total)

	// Get teachers with relationships
	if err := query.Preload("User").Preload("User.Branch").Preload("Branch").
		Offset(offset).Limit(limit).Find(&teachers).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch teachers",
		})
	}

	return c.JSON(fiber.Map{
		"teachers": teachers,
		"pagination": fiber.Map{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetTeacher returns a specific teacher by ID
func (tc *TeacherController) GetTeacher(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid teacher ID",
		})
	}

	var teacher models.Teacher
	if err := database.DB.Preload("User").Preload("User.Branch").Preload("Branch").
		First(&teacher, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Teacher not found",
		})
	}

	return c.JSON(fiber.Map{
		"teacher": teacher,
	})
}

// CreateTeacher creates a new teacher profile
func (tc *TeacherController) CreateTeacher(c *fiber.Ctx) error {
	var teacher models.Teacher
	if err := c.BodyParser(&teacher); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if teacher.UserID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User ID is required",
		})
	}

	// Check if user exists and is a teacher
	var user models.User
	if err := database.DB.Where("id = ? AND role = ?", teacher.UserID, "teacher").
		First(&user).Error; err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User not found or not a teacher",
		})
	}

	// Check if teacher profile already exists
	var existingTeacher models.Teacher
	if err := database.DB.Where("user_id = ?", teacher.UserID).
		First(&existingTeacher).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Teacher profile already exists for this user",
		})
	}

	// Set default values
	if teacher.TeacherType == "" {
		teacher.TeacherType = "Both"
	}
	teacher.Active = true

	// Set branch ID from user if not provided
	if teacher.BranchID == 0 {
		teacher.BranchID = user.BranchID
	}

	if err := database.DB.Create(&teacher).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create teacher profile",
		})
	}

	// Load relationships
	database.DB.Preload("User").Preload("User.Branch").Preload("Branch").
		First(&teacher, teacher.ID)

	// Log activity
	middleware.LogActivity(c, "CREATE", "teachers", teacher.ID, teacher)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Teacher profile created successfully",
		"teacher": teacher,
	})
}

// UpdateTeacher updates an existing teacher profile
func (tc *TeacherController) UpdateTeacher(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid teacher ID",
		})
	}

	var teacher models.Teacher
	if err := database.DB.First(&teacher, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Teacher not found",
		})
	}

	var updateData models.Teacher
	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Don't allow changing UserID
	updateData.UserID = teacher.UserID

	if err := database.DB.Model(&teacher).Updates(updateData).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update teacher profile",
		})
	}

	// Load relationships
	database.DB.Preload("User").Preload("User.Branch").Preload("Branch").
		First(&teacher, teacher.ID)

	// Log activity
	middleware.LogActivity(c, "UPDATE", "teachers", teacher.ID, updateData)

	return c.JSON(fiber.Map{
		"message": "Teacher profile updated successfully",
		"teacher": teacher,
	})
}

// DeleteTeacher deletes a teacher profile
func (tc *TeacherController) DeleteTeacher(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid teacher ID",
		})
	}

	var teacher models.Teacher
	if err := database.DB.First(&teacher, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Teacher not found",
		})
	}

	if err := database.DB.Delete(&teacher).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete teacher profile",
		})
	}

	// Log activity
	middleware.LogActivity(c, "DELETE", "teachers", teacher.ID, teacher)

	return c.JSON(fiber.Map{
		"message": "Teacher profile deleted successfully",
	})
}

// GetTeachersByBranch returns teachers for a specific branch
func (tc *TeacherController) GetTeachersByBranch(c *fiber.Ctx) error {
	branchID, err := strconv.ParseUint(c.Params("branch_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid branch ID",
		})
	}

	var teachers []models.Teacher
	if err := database.DB.Where("branch_id = ? AND active = ?", uint(branchID), true).
		Preload("User").Preload("User.Branch").Preload("Branch").
		Find(&teachers).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch teachers",
		})
	}

	return c.JSON(fiber.Map{
		"teachers": teachers,
		"total":    len(teachers),
	})
}

// GetTeacherSpecializations returns available teacher specializations
func (tc *TeacherController) GetTeacherSpecializations(c *fiber.Ctx) error {
	specializations := []string{
		"Kid Jolly Phonics",
		"Kid Conversation",
		"Adult Conversation",
		"Adult Test preparation",
		"IELTS",
		"TOEIC",
		"TOEFL",
		"Business English",
		"Chinese Class",
		"Admin",
	}

	return c.JSON(fiber.Map{
		"specializations": specializations,
	})
}

// GetTeacherTypes returns available teacher types
func (tc *TeacherController) GetTeacherTypes(c *fiber.Ctx) error {
	teacherTypes := []string{
		"Both",
		"Adults",
		"Kid",
		"Admin Team",
	}

	return c.JSON(fiber.Map{
		"teacher_types": teacherTypes,
	})
}