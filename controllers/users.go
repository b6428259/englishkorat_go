package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"englishkorat_go/storage"
	"englishkorat_go/utils"

	"strconv"
	"strings"

	"gorm.io/gorm/clause"

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

	// Base query (don't join students yet for performance)
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

	// Apply search across username, email, phone and optionally student names (case-insensitive)
	search := strings.TrimSpace(c.Query("search", ""))
	strict := strings.ToLower(strings.TrimSpace(c.Query("strict", ""))) == "true"
	if search != "" {
		lsearch := strings.ToLower(search)
		s := "%" + lsearch + "%"

		// FIRST: try an exact-match pass (email/phone/username OR student name equality)
		exactQuery := database.DB.Model(&models.User{})
		// apply role/branch/status filters to exactQuery
		if role := c.Query("role"); role != "" {
			exactQuery = exactQuery.Where("role = ?", role)
		}
		if branchID := c.Query("branch_id"); branchID != "" {
			exactQuery = exactQuery.Where("branch_id = ?", branchID)
		}
		exactQuery = exactQuery.Where("status = ?", status)

		// exact-match conditions
		exactConds := []string{
			"LOWER(users.username) = ?",
			"LOWER(COALESCE(users.email, '')) = ?",
			"LOWER(COALESCE(users.phone, '')) = ?",
		}
		exactArgs := []interface{}{lsearch, lsearch, lsearch}

		// include student name exact matches (join students)
		exactQuery = exactQuery.Joins("LEFT JOIN students ON students.user_id = users.id")
		nameExactConds := []string{
			"LOWER(COALESCE(students.first_name,'')) = ?",
			"LOWER(COALESCE(students.last_name,'')) = ?",
			"LOWER(COALESCE(students.nickname_th,'')) = ?",
			"LOWER(COALESCE(students.nickname_en,'')) = ?",
		}
		for _, nc := range nameExactConds {
			exactConds = append(exactConds, nc)
			exactArgs = append(exactArgs, lsearch)
		}
		exactCombined := strings.Join(exactConds, " OR ")
		exactQuery = exactQuery.Where(exactCombined, exactArgs...)

		var exactTotal int64
		exactQuery.Distinct("users.id").Count(&exactTotal)

		if exactTotal > 0 {
			// If we have exact matches, prefer them: use equality conditions for the listing
			query = query.Joins("LEFT JOIN students ON students.user_id = users.id")
			query = query.Where(exactCombined, exactArgs...)
		} else if strict {
			// strict mode: return empty result if no exact matches
			return c.JSON(fiber.Map{
				"users":      []interface{}{},
				"pagination": fiber.Map{"page": page, "limit": limit, "total": 0},
			})
		} else {
			// Fallback to the existing broad LIKE search when exact didn't match and not strict
			// Build list of OR LIKE conditions and args
			conds := []string{
				"LOWER(users.username) LIKE ?",
				"LOWER(COALESCE(users.email, '')) LIKE ?",
				"LOWER(COALESCE(users.phone, '')) LIKE ?",
			}
			args := []interface{}{s, s, s}

			// For name search we need to join students; do this only when search present to avoid extra joins
			query = query.Joins("LEFT JOIN students ON students.user_id = users.id")
			nameConds := []string{
				"LOWER(COALESCE(students.first_name, '')) LIKE ?",
				"LOWER(COALESCE(students.last_name, '')) LIKE ?",
				"LOWER(COALESCE(students.nickname_th, '')) LIKE ?",
				"LOWER(COALESCE(students.nickname_en, '')) LIKE ?",
			}
			for _, nc := range nameConds {
				conds = append(conds, nc)
				args = append(args, s)
			}

			// Combine conditions
			combined := strings.Join(conds, " OR ")
			query = query.Where(combined, args...)

			// Add an ORDER BY that ranks exact email/phone matches highest, then exact username,
			// then name exact matches, then partial username/email/phone, then partial name matches.
			// Lower numeric value => higher priority (sorted ascending).
			orderExpr := `(CASE
				WHEN LOWER(COALESCE(users.email,'')) = ? THEN 0
				WHEN LOWER(COALESCE(users.phone,'')) = ? THEN 0
				WHEN LOWER(users.username) = ? THEN 1
				WHEN LOWER(COALESCE(students.first_name,'')) = ? THEN 2
				WHEN LOWER(COALESCE(students.last_name,'')) = ? THEN 2
				WHEN LOWER(COALESCE(students.nickname_th,'')) = ? THEN 2
				WHEN LOWER(COALESCE(students.nickname_en,'')) = ? THEN 2
				WHEN LOWER(users.username) LIKE ? THEN 3
				WHEN LOWER(COALESCE(users.email,'')) LIKE ? THEN 4
				WHEN LOWER(COALESCE(users.phone,'')) LIKE ? THEN 4
				WHEN LOWER(COALESCE(students.first_name,'')) LIKE ? THEN 5
				WHEN LOWER(COALESCE(students.last_name,'')) LIKE ? THEN 5
				ELSE 6 END) ASC`
			orderArgs := []interface{}{
				lsearch, lsearch, lsearch,
				lsearch, lsearch, lsearch, lsearch,
				s, s, s, s, s,
			}
			query = query.Order(clause.Expr{SQL: orderExpr, Vars: orderArgs})
		}
	}

	// Count with distinct users.id to avoid duplicates from the join
	// Build a fresh count query and apply the same filters to avoid surprises
	countQuery := database.DB.Model(&models.User{})
	// Re-apply role/branch/status filters to the count query
	if role := c.Query("role"); role != "" {
		countQuery = countQuery.Where("role = ?", role)
	}
	if branchID := c.Query("branch_id"); branchID != "" {
		countQuery = countQuery.Where("branch_id = ?", branchID)
	}
	countQuery = countQuery.Where("status = ?", status)

	// If search is present, re-apply the same join + where conditions used for the listing
	if search != "" {
		// we already computed combined and args in the listing flow; rebuild them here
		// Recreate local vars to build the same combined condition
		lsearch := strings.ToLower(search)
		s := "%" + lsearch + "%"
		conds := []string{
			"LOWER(users.username) LIKE ?",
			"LOWER(COALESCE(users.email, '')) LIKE ?",
			"LOWER(COALESCE(users.phone, '')) LIKE ?",
		}
		args := []interface{}{s, s, s}
		// join students and add name conditions
		countQuery = countQuery.Joins("LEFT JOIN students ON students.user_id = users.id")
		nameConds := []string{
			"LOWER(COALESCE(students.first_name, '')) LIKE ?",
			"LOWER(COALESCE(students.last_name, '')) LIKE ?",
			"LOWER(COALESCE(students.nickname_th, '')) LIKE ?",
			"LOWER(COALESCE(students.nickname_en, '')) LIKE ?",
		}
		for _, nc := range nameConds {
			conds = append(conds, nc)
			args = append(args, s)
		}
		combined := strings.Join(conds, " OR ")
		countQuery = countQuery.Where(combined, args...)
	}

	if res := countQuery.Distinct("users.id").Count(&total); res.Error != nil {
		// Return a helpful error for debugging (can be redacted for production)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to count users",
			"details": res.Error.Error(),
		})
	}

	// If a search was provided but there are no matches, return an empty result set
	if search != "" && total == 0 {
		return c.JSON(fiber.Map{
			"users":      []interface{}{},
			"pagination": fiber.Map{"page": page, "limit": limit, "total": total},
		})
	}

	// Get users with pagination and preload relations
	if res := query.Preload("Branch").Preload("Student").Preload("Teacher").
		Offset(offset).Limit(limit).Find(&users); res.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to fetch users",
			"details": res.Error.Error(),
		})
	}

	// Normalize and sanitize response (omit password and sensitive internal fields)
	sanitized := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		userMap := map[string]interface{}{
			"id":        u.ID,
			"username":  u.Username,
			"email":     nil,
			"phone":     u.Phone,
			"role":      u.Role,
			"branch_id": u.BranchID,
			"avatar":    u.Avatar,
			"status":    u.Status,
		}
		if u.Email != nil {
			userMap["email"] = *u.Email
		}
		// include basic student info when available (guard against nil Student)
		if u.Student != nil && u.Student.UserID != nil {
			userMap["student"] = map[string]interface{}{
				"first_name": u.Student.FirstName,
				"last_name":  u.Student.LastName,
				"nickname":   u.Student.NicknameTh,
			}
		}
		sanitized = append(sanitized, userMap)
	}

	return c.JSON(fiber.Map{
		"users": sanitized,
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
