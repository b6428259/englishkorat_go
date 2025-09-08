package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type StudentController struct{}

// GetStudents returns all students with pagination
func (sc *StudentController) GetStudents(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	var students []models.Student
	var total int64

	query := database.DB.Model(&models.Student{})

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

	// Get total count
	query.Count(&total)

	// Get students with relationships
	if err := query.Preload("User").Preload("User.Branch").
		Offset(offset).Limit(limit).Find(&students).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch students",
		})
	}

	return c.JSON(fiber.Map{
		"students": students,
		"pagination": fiber.Map{
			"page":  page,
			"limit": limit,
			"total": total,
		},
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
		"student": student,
	})
}

// CreateStudent creates a new student profile
func (sc *StudentController) CreateStudent(c *fiber.Ctx) error {
	var student models.Student
	if err := c.BodyParser(&student); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if student.UserID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User ID is required",
		})
	}

	// Check if user exists and is a student
	var user models.User
	if err := database.DB.Where("id = ? AND role = ?", student.UserID, "student").
		First(&user).Error; err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User not found or not a student",
		})
	}

	// Check if student profile already exists
	var existingStudent models.Student
	if err := database.DB.Where("user_id = ?", student.UserID).
		First(&existingStudent).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Student profile already exists for this user",
		})
	}

	if err := database.DB.Create(&student).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create student profile",
		})
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

	var updateData models.Student
	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Don't allow changing UserID
	updateData.UserID = student.UserID

	if err := database.DB.Model(&student).Updates(updateData).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update student profile",
		})
	}

	// Load relationships
	database.DB.Preload("User").Preload("User.Branch").First(&student, student.ID)

	// Log activity
	middleware.LogActivity(c, "UPDATE", "students", student.ID, updateData)

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
		"message": "Student profile deleted successfully",
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