package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"englishkorat_go/storage"
	"englishkorat_go/utils"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type UserController struct{}

// GetUsers returns all users with pagination
func (uc *UserController) GetUsers(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	var users []models.User
	var total int64

	// Build query
	query := database.DB.Model(&models.User{})

	// Filter by role if specified
	if role := c.Query("role"); role != "" {
		query = query.Where("role = ?", role)
	}

	// Filter by branch if specified
	if branchID := c.Query("branch_id"); branchID != "" {
		query = query.Where("branch_id = ?", branchID)
	}

	// Filter by status
	status := c.Query("status", "active")
	query = query.Where("status = ?", status)

	// Get total count
	query.Count(&total)

	// Get users with pagination
	if err := query.Preload("Branch").Preload("Student").Preload("Teacher").
		Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch users",
		})
	}

	return c.JSON(fiber.Map{
		"users": users,
		"pagination": fiber.Map{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetUser returns a specific user by ID
func (uc *UserController) GetUser(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID",
		})
	}

	var user models.User
	if err := database.DB.Preload("Branch").Preload("Student").Preload("Teacher").
		First(&user, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(fiber.Map{
		"user": user,
	})
}

// CreateUser creates a new user
func (uc *UserController) CreateUser(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username" validate:"required,min=3,max=50"`
		Password string `json:"password" validate:"required,min=6"`
		Email    string `json:"email" validate:"email"`
		Phone    string `json:"phone"`
		LineID   string `json:"line_id"`
		Role     string `json:"role" validate:"required"`
		BranchID uint   `json:"branch_id" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate role
	if !utils.IsValidRole(req.Role) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid role",
		})
	}

	// Check if username already exists
	var existingUser models.User
	if err := database.DB.Where("username = ?", req.Username).First(&existingUser).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Username already exists",
		})
	}

	// Hash password
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to hash password",
		})
	}

	// Create user
	user := models.User{
		Username: req.Username,
		Password: hashedPassword,
		Phone:    req.Phone,
		LineID:   req.LineID,
		Role:     req.Role,
		BranchID: req.BranchID,
		Status:   "active",
	}
	if strings.TrimSpace(req.Email) != "" {
		email := strings.TrimSpace(req.Email)
		user.Email = &email
	}

	if err := database.DB.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create user",
		})
	}

	// Load relationships
	database.DB.Preload("Branch").First(&user, user.ID)

	// Log activity
	middleware.LogActivity(c, "CREATE", "users", user.ID, user)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User created successfully",
		"user":    user,
	})
}

// UpdateUser updates an existing user
func (uc *UserController) UpdateUser(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID",
		})
	}

	var user models.User
	if err := database.DB.First(&user, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var updateData struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		LineID   string `json:"line_id"`
		Role     string `json:"role"`
		BranchID uint   `json:"branch_id"`
		Status   string `json:"status"`
	}

	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate role if provided
	if updateData.Role != "" && !utils.IsValidRole(updateData.Role) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid role",
		})
	}

	// Validate status if provided
	if updateData.Status != "" && !utils.IsValidStatus(updateData.Status) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid status",
		})
	}

	// Update user
	if err := database.DB.Model(&user).Updates(updateData).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update user",
		})
	}

	// Load relationships
	database.DB.Preload("Branch").First(&user, user.ID)

	// Log activity
	middleware.LogActivity(c, "UPDATE", "users", user.ID, updateData)

	return c.JSON(fiber.Map{
		"message": "User updated successfully",
		"user":    user,
	})
}

// DeleteUser deletes a user
func (uc *UserController) DeleteUser(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID",
		})
	}

	var user models.User
	if err := database.DB.First(&user, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Soft delete
	if err := database.DB.Delete(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete user",
		})
	}

	// Log activity
	middleware.LogActivity(c, "DELETE", "users", user.ID, user)

	return c.JSON(fiber.Map{
		"message": "User deleted successfully",
	})
}

// UploadAvatar uploads an avatar for a user
func (uc *UserController) UploadAvatar(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID",
		})
	}

	var user models.User
	if err := database.DB.First(&user, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Get uploaded file
	file, err := c.FormFile("avatar")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No file uploaded",
		})
	}

	// Initialize storage service
	storageService, err := storage.NewStorageService()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Storage service initialization failed",
		})
	}

	// Upload file
	avatarURL, err := storageService.UploadFile(file, "avatars", user.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to upload avatar",
		})
	}

	// Delete old avatar if exists
	if user.Avatar != "" {
		go storageService.DeleteFile(user.Avatar)
	}

	// Update user avatar
	if err := database.DB.Model(&user).Update("avatar", avatarURL).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update user avatar",
		})
	}

	// Log activity
	middleware.LogActivity(c, "UPDATE", "users", user.ID, fiber.Map{
		"action": "avatar_upload",
		"avatar": avatarURL,
	})

	return c.JSON(fiber.Map{
		"message": "Avatar uploaded successfully",
		"avatar":  avatarURL,
	})
}
