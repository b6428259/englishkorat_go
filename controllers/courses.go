package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"englishkorat_go/utils"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type CourseController struct{}

// GetCourses returns all courses (PUBLIC endpoint)
func (cc *CourseController) GetCourses(c *fiber.Ctx) error {
	var courses []models.Course

	// Build query
	query := database.DB.Model(&models.Course{})

	// Filter by branch if specified
	if branchID := c.Query("branch_id"); branchID != "" {
		query = query.Where("branch_id = ?", branchID)
	}

	// Filter by status (default to active)
	status := c.Query("status", "active")
	query = query.Where("status = ?", status)

	// Filter by course type if specified
	if courseType := c.Query("course_type"); courseType != "" {
		query = query.Where("course_type = ?", courseType)
	}

	// Filter by level if specified
	if level := c.Query("level"); level != "" {
		query = query.Where("level = ?", level)
	}

	// Load relationships
	query = query.Preload("Branch").Preload("Category")

	// Pagination: page & per_page
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
	// cap perPage to prevent abuse
	if perPage > 100 {
		perPage = 100
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to count courses"})
	}

	// Apply limit/offset and execute
	offset := (page - 1) * perPage
	if err := query.Limit(perPage).Offset(offset).Find(&courses).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch courses",
		})
	}

	// Normalize response using DTOs
	coursesDTO := utils.ToCourseDTOs(courses)

	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}

	return c.JSON(fiber.Map{
		"courses":     coursesDTO,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}

// GetCourse returns a specific course by ID (PUBLIC endpoint)
func (cc *CourseController) GetCourse(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid course ID",
		})
	}

	var course models.Course
	if err := database.DB.Preload("Branch").Preload("Category").First(&course, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Course not found",
		})
	}

	courseDTO := utils.ToCourseDTO(course)
	return c.JSON(fiber.Map{
		"course": courseDTO,
	})
}

// CreateCourse creates a new course (PROTECTED - admin/owner only)
func (cc *CourseController) CreateCourse(c *fiber.Ctx) error {
	var course models.Course
	if err := c.BodyParser(&course); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if course.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Course name is required",
		})
	}

	// Check if code already exists
	if course.Code != "" {
		var existingCourse models.Course
		if err := database.DB.Where("code = ?", course.Code).First(&existingCourse).Error; err == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Course code already exists",
			})
		}
	}

	// Set default status
	if course.Status == "" {
		course.Status = "active"
	}

	// Create course
	if err := database.DB.Create(&course).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create course",
		})
	}

	// Load relationships
	database.DB.Preload("Branch").Preload("Category").First(&course, course.ID)

	// Log activity
	middleware.LogActivity(c, "CREATE", "courses", course.ID, course)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Course created successfully",
		"course":  course,
	})
}

// UpdateCourse updates an existing course (PROTECTED - admin/owner only)
func (cc *CourseController) UpdateCourse(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid course ID",
		})
	}

	var course models.Course
	if err := database.DB.First(&course, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Course not found",
		})
	}

	// Store original course for logging
	originalCourse := course

	var updateData models.Course
	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Check if code already exists (if changing)
	if updateData.Code != "" && updateData.Code != course.Code {
		var existingCourse models.Course
		if err := database.DB.Where("code = ? AND id != ?", updateData.Code, course.ID).First(&existingCourse).Error; err == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Course code already exists",
			})
		}
	}

	// Update course
	if err := database.DB.Model(&course).Updates(updateData).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update course",
		})
	}

	// Load relationships
	database.DB.Preload("Branch").Preload("Category").First(&course, course.ID)

	// Log activity
	middleware.LogActivity(c, "UPDATE", "courses", course.ID, fiber.Map{
		"original": originalCourse,
		"updated":  course,
	})

	return c.JSON(fiber.Map{
		"message": "Course updated successfully",
		"course":  course,
	})
}

// DeleteCourse deletes a course (PROTECTED - admin/owner only)
func (cc *CourseController) DeleteCourse(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid course ID",
		})
	}

	var course models.Course
	if err := database.DB.First(&course, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Course not found",
		})
	}

	// Soft delete
	if err := database.DB.Delete(&course).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete course",
		})
	}

	// Log activity
	middleware.LogActivity(c, "DELETE", "courses", course.ID, course)

	return c.JSON(fiber.Map{
		"message": "Course deleted successfully",
	})
}

// GetCoursesByBranch returns courses for a specific branch
func (cc *CourseController) GetCoursesByBranch(c *fiber.Ctx) error {
	branchID, err := strconv.ParseUint(c.Params("branch_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid branch ID",
		})
	}

	// Base query for branch
	query := database.DB.Model(&models.Course{}).Where("branch_id = ? AND status = ?", uint(branchID), "active").Preload("Branch").Preload("Category")

	// Pagination: page & per_page
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to count courses"})
	}

	offset := (page - 1) * perPage
	var courses []models.Course
	if err := query.Limit(perPage).Offset(offset).Find(&courses).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch courses",
		})
	}

	coursesDTO := utils.ToCourseDTOs(courses)
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}

	return c.JSON(fiber.Map{
		"courses":     coursesDTO,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}
