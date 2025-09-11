package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type UserInCourseController struct{}

type assignUserRequest struct {
	UserID uint   `json:"user_id"`
	Role   string `json:"role"`   // optional: instructor/assistant/observer/student/teacher
	Status string `json:"status"` // optional: active/inactive/enrolled/completed/dropped
}

// AssignUserToCourse assigns a user to a course (admin/owner only)
func (uic *UserInCourseController) AssignUserToCourse(c *fiber.Ctx) error {
	// must be protected and role-checked by route middleware
	// parse course id
	courseID64, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || courseID64 == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid course ID"})
	}
	courseID := uint(courseID64)

	var req assignUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if req.UserID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id is required"})
	}

	// validate course exists
	var course models.Course
	if err := database.DB.First(&course, courseID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Course not found"})
	}

	// validate user exists
	var user models.User
	if err := database.DB.First(&user, req.UserID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	// derive role if not provided
	role := req.Role
	if role == "" {
		if user.Role == "teacher" || user.Role == "admin" || user.Role == "owner" {
			role = "teacher"
		} else {
			role = "student"
		}
	}

	// default status
	status := req.Status
	if status == "" {
		status = "enrolled"
	}

	// check if already assigned
	var existing models.User_inCourse
	if err := database.DB.Where("user_id = ? AND course_id = ?", req.UserID, courseID).First(&existing).Error; err == nil {
		// update status/role if changed, then return
		updates := map[string]interface{}{}
		if existing.Role != role {
			updates["role"] = role
		}
		if existing.Status != status {
			updates["status"] = status
		}
		if len(updates) > 0 {
			if err := database.DB.Model(&existing).Updates(updates).Error; err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update assignment"})
			}
		}
		middleware.LogActivity(c, "UPDATE", "user_in_courses", existing.ID, existing)
		return c.JSON(fiber.Map{
			"message":           "User already assigned; updated role/status if needed",
			"user_in_course_id": existing.ID,
			"assignment":        existing,
		})
	}

	// create new assignment
	assignment := models.User_inCourse{
		UserID:   req.UserID,
		CourseID: courseID,
		Role:     role,
		Status:   status,
	}
	if err := database.DB.Create(&assignment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to assign user to course"})
	}

	middleware.LogActivity(c, "CREATE", "user_in_courses", assignment.ID, assignment)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":           "User assigned to course",
		"user_in_course_id": assignment.ID,
		"assignment":        assignment,
	})
}

// AssignUsersToCourseBulk assigns multiple users to a course (admin/owner only)
func (uic *UserInCourseController) AssignUsersToCourseBulk(c *fiber.Ctx) error {
	courseID64, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || courseID64 == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid course ID"})
	}
	courseID := uint(courseID64)

	// validate course exists once
	var course models.Course
	if err := database.DB.First(&course, courseID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Course not found"})
	}

	var req []assignUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if len(req) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "assignments array is required"})
	}

	type result struct {
		UserID       uint   `json:"user_id"`
		Status       string `json:"status"` // created/updated/unchanged/error
		Role         string `json:"role,omitempty"`
		AssignmentID uint   `json:"user_in_course_id,omitempty"`
		Error        string `json:"error,omitempty"`
	}

	results := make([]result, 0, len(req))
	var created, updated, unchanged, failed int

	for _, item := range req {
		if item.UserID == 0 {
			failed++
			results = append(results, result{UserID: 0, Status: "error", Error: "user_id is required"})
			continue
		}

		// validate user exists
		var user models.User
		if err := database.DB.First(&user, item.UserID).Error; err != nil {
			failed++
			results = append(results, result{UserID: item.UserID, Status: "error", Error: "User not found"})
			continue
		}

		// role derive
		role := item.Role
		if role == "" {
			if user.Role == "teacher" || user.Role == "admin" || user.Role == "owner" {
				role = "teacher"
			} else {
				role = "student"
			}
		}

		// status default
		status := item.Status
		if status == "" {
			status = "enrolled"
		}

		// upsert behavior
		var existing models.User_inCourse
		if err := database.DB.Where("user_id = ? AND course_id = ?", item.UserID, courseID).First(&existing).Error; err == nil {
			// exists: compare
			updates := map[string]interface{}{}
			if existing.Role != role {
				updates["role"] = role
			}
			if existing.Status != status {
				updates["status"] = status
			}
			if len(updates) > 0 {
				if err := database.DB.Model(&existing).Updates(updates).Error; err != nil {
					failed++
					results = append(results, result{UserID: item.UserID, Status: "error", Error: "Failed to update assignment"})
					continue
				}
				updated++
				middleware.LogActivity(c, "UPDATE", "user_in_courses", existing.ID, existing)
				results = append(results, result{UserID: item.UserID, Status: "updated", Role: role, AssignmentID: existing.ID})
			} else {
				unchanged++
				results = append(results, result{UserID: item.UserID, Status: "unchanged", Role: role, AssignmentID: existing.ID})
			}
			continue
		}

		// create new
		assignment := models.User_inCourse{UserID: item.UserID, CourseID: courseID, Role: role, Status: status}
		if err := database.DB.Create(&assignment).Error; err != nil {
			failed++
			results = append(results, result{UserID: item.UserID, Status: "error", Error: "Failed to assign user to course"})
			continue
		}
		created++
		middleware.LogActivity(c, "CREATE", "user_in_courses", assignment.ID, assignment)
		results = append(results, result{UserID: item.UserID, Status: "created", Role: role, AssignmentID: assignment.ID})
	}

	return c.JSON(fiber.Map{
		"message":   "Bulk assignment processed",
		"course_id": courseID,
		"processed": len(req),
		"created":   created,
		"updated":   updated,
		"unchanged": unchanged,
		"failed":    failed,
		"results":   results,
	})
}
