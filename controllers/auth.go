package controllers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"englishkorat_go/utils"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type AuthController struct{}

// Logout invalidates the current JWT by storing it in Redis blacklist for 24 hours
func (ac *AuthController) Logout(c *fiber.Ctx) error {
	// Extract token from Authorization header
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Missing authorization header"})
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid authorization header format"})
	}

	// Store token in Redis blacklist with 24 hour TTL if Redis is available
	rc := database.GetRedisClient()
	if rc != nil {
		ctx := context.Background()
		key := "blacklist:jwt:" + tokenString
		// Set with TTL 24 hours
		if err := rc.Set(ctx, key, "1", 24*time.Hour).Err(); err != nil {
			// If Redis fails, log activity but return success (don't block logout)
			middleware.LogActivity(c, "LOGOUT", "auth", 0, fiber.Map{"error": err.Error()})
		}
	}

	// Try to log who logged out (if possible)
	if user, err := middleware.GetCurrentUser(c); err == nil {
		middleware.LogActivity(c, "LOGOUT", "auth", user.ID, fiber.Map{"username": user.Username})
	} else {
		middleware.LogActivity(c, "LOGOUT", "auth", 0, fiber.Map{"note": "anonymous or token invalid"})
	}

	return c.JSON(fiber.Map{"message": "Logged out successfully"})
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// RegisterRequest represents the registration request body
type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required,min=6"`
	Email    string `json:"email" validate:"email"`
	Phone    string `json:"phone"`
	LineID   string `json:"line_id"`
	Role     string `json:"role" validate:"required"`
	BranchID uint   `json:"branch_id" validate:"required"`
}

// Login authenticates a user and returns a JWT token
func (ac *AuthController) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Find user by username
	var user models.User
	if err := database.DB.Where("username = ? AND status = ?", req.Username, "active").First(&user).Error; err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid credentials",
		})
	}

	// Check password
	if err := utils.CheckPassword(req.Password, user.Password); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid credentials",
		})
	}

	// Generate JWT token
	token, err := middleware.GenerateToken(&user)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate token",
		})
	}

	// Load user relationships
	database.DB.Preload("Branch").First(&user, user.ID)

	// Log the login activity
	middleware.LogActivity(c, "LOGIN", "auth", user.ID, fiber.Map{
		"username": user.Username,
		"role":     user.Role,
	})

	return c.JSON(fiber.Map{
		"message": "Login successful",
		"token":   token,
		"user": fiber.Map{
			"id":        user.ID,
			"username":  user.Username,
			"email":     user.Email,
			"role":      user.Role,
			"branch_id": user.BranchID,
			"branch":    user.Branch,
			"avatar":    user.Avatar,
		},
	})
}

// Register creates a new user account (admin only)
func (ac *AuthController) Register(c *fiber.Ctx) error {
	var req RegisterRequest
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

	// Check if email already exists (if provided)
	if req.Email != "" {
		if err := database.DB.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Email already exists",
			})
		}
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
		Email:    req.Email,
		Phone:    req.Phone,
		LineID:   req.LineID,
		Role:     req.Role,
		BranchID: req.BranchID,
		Status:   "active",
	}

	if err := database.DB.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create user",
		})
	}

	// Load user relationships
	database.DB.Preload("Branch").First(&user, user.ID)

	// Log the registration activity
	middleware.LogActivity(c, "CREATE", "users", user.ID, fiber.Map{
		"username": user.Username,
		"role":     user.Role,
	})

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User created successfully",
		"user": fiber.Map{
			"id":        user.ID,
			"username":  user.Username,
			"email":     user.Email,
			"role":      user.Role,
			"branch_id": user.BranchID,
			"branch":    user.Branch,
		},
	})
}

// GetProfile returns the current user's profile
func (ac *AuthController) GetProfile(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Load user relationships
	database.DB.Preload("Branch").First(user, user.ID)

	return c.JSON(fiber.Map{
		"user": fiber.Map{
			"id":        user.ID,
			"username":  user.Username,
			"email":     user.Email,
			"phone":     user.Phone,
			"line_id":   user.LineID,
			"role":      user.Role,
			"branch_id": user.BranchID,
			"branch":    user.Branch,
			"status":    user.Status,
			"avatar":    user.Avatar,
		},
	})
}

// ChangePassword allows users to change their password
func (ac *AuthController) ChangePassword(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var req struct {
		CurrentPassword string `json:"current_password" validate:"required"`
		NewPassword     string `json:"new_password" validate:"required,min=6"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Check current password
	if err := utils.CheckPassword(req.CurrentPassword, user.Password); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Current password is incorrect",
		})
	}

	// Hash new password
	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to hash password",
		})
	}

	// Update password
	if err := database.DB.Model(user).Update("password", hashedPassword).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update password",
		})
	}

	// Log the password change activity
	middleware.LogActivity(c, "UPDATE", "users", user.ID, fiber.Map{
		"action": "password_change",
	})

	return c.JSON(fiber.Map{
		"message": "Password changed successfully",
	})
}

// GeneratePasswordResetToken generates a password reset token for a user
func (ac *AuthController) GeneratePasswordResetToken(c *fiber.Ctx) error {
	// Get current user (admin/owner only)
	currentUser, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Check role permissions (admin and owner only)
	if currentUser.Role != "admin" && currentUser.Role != "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Only admin and owner can generate password reset tokens",
		})
	}

	var req struct {
		UserID uint `json:"user_id" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Find target user
	var targetUser models.User
	if err := database.DB.First(&targetUser, req.UserID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Admin cannot reset owner password
	if currentUser.Role == "admin" && targetUser.Role == "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Admin cannot reset owner password",
		})
	}

	// Generate secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate reset token",
		})
	}
	token := hex.EncodeToString(tokenBytes)

	// Set token expiration (1 hour from now)
	expiresAt := time.Now().Add(1 * time.Hour)

	// Update user with reset token
	if err := database.DB.Model(&targetUser).Updates(map[string]interface{}{
		"password_reset_token":   token,
		"password_reset_expires": expiresAt,
	}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save reset token",
		})
	}

	// Log the activity
	middleware.LogActivity(c, "CREATE", "password_reset_token", targetUser.ID, fiber.Map{
		"target_user":  targetUser.Username,
		"generated_by": currentUser.Username,
		"expires_at":   expiresAt,
	})

	return c.JSON(fiber.Map{
		"message":    "Password reset token generated successfully",
		"token":      token,
		"expires_at": expiresAt,
		"user": fiber.Map{
			"id":       targetUser.ID,
			"username": targetUser.Username,
			"email":    targetUser.Email,
		},
	})
}

// ResetPasswordByAdmin allows admin/owner to reset user password directly
func (ac *AuthController) ResetPasswordByAdmin(c *fiber.Ctx) error {
	// Get current user (admin/owner only)
	currentUser, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Check role permissions (admin and owner only)
	if currentUser.Role != "admin" && currentUser.Role != "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Only admin and owner can reset passwords",
		})
	}

	var req struct {
		UserID                uint   `json:"user_id" validate:"required"`
		NewPassword           string `json:"new_password" validate:"required,min=6"`
		RequirePasswordChange bool   `json:"require_password_change" validate:"omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Find target user
	var targetUser models.User
	if err := database.DB.First(&targetUser, req.UserID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Admin cannot reset owner password
	if currentUser.Role == "admin" && targetUser.Role == "owner" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Admin cannot reset owner password",
		})
	}

	// Hash new password
	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to hash password",
		})
	}

	// Update password and mark as reset by admin
	updates := map[string]interface{}{
		"password":                hashedPassword,
		"password_reset_by_admin": true,
		"password_reset_token":    nil, // Clear any existing reset token
		"password_reset_expires":  nil,
	}

	// If require password change is set, we might want to add a flag for that
	// For now, we'll use the password_reset_by_admin flag to indicate they should change it

	if err := database.DB.Model(&targetUser).Updates(updates).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to reset password",
		})
	}

	// Log the activity
	middleware.LogActivity(c, "UPDATE", "password_reset_admin", targetUser.ID, fiber.Map{
		"target_user":             targetUser.Username,
		"reset_by":                currentUser.Username,
		"require_password_change": req.RequirePasswordChange,
	})

	return c.JSON(fiber.Map{
		"message": "Password reset successfully",
		"user": fiber.Map{
			"id":       targetUser.ID,
			"username": targetUser.Username,
			"email":    targetUser.Email,
		},
		"require_password_change": req.RequirePasswordChange,
	})
}

// ResetPasswordWithToken allows users to reset password using a valid token
func (ac *AuthController) ResetPasswordWithToken(c *fiber.Ctx) error {
	var req struct {
		Token       string `json:"token" validate:"required"`
		NewPassword string `json:"new_password" validate:"required,min=6"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Find user with valid token
	var user models.User
	if err := database.DB.Where("password_reset_token = ? AND password_reset_expires > ?",
		req.Token, time.Now()).First(&user).Error; err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid or expired reset token",
		})
	}

	// Hash new password
	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to hash password",
		})
	}

	// Update password and clear reset token
	if err := database.DB.Model(&user).Updates(map[string]interface{}{
		"password":                hashedPassword,
		"password_reset_token":    nil,
		"password_reset_expires":  nil,
		"password_reset_by_admin": false, // Clear admin reset flag
	}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to reset password",
		})
	}

	// Log the activity
	middleware.LogActivity(c, "UPDATE", "password_reset_token", user.ID, fiber.Map{
		"username": user.Username,
		"method":   "token_reset",
	})

	return c.JSON(fiber.Map{
		"message": "Password reset successfully",
	})
}
