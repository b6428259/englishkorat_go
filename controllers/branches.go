package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type BranchController struct{}

// GetBranches returns all branches
func (bc *BranchController) GetBranches(c *fiber.Ctx) error {
	var branches []models.Branch
	
	query := database.DB.Model(&models.Branch{})
	
	// Filter by active status if specified
	if active := c.Query("active"); active != "" {
		if active == "true" {
			query = query.Where("active = ?", true)
		} else if active == "false" {
			query = query.Where("active = ?", false)
		}
	}

	// Filter by type if specified
	if branchType := c.Query("type"); branchType != "" {
		query = query.Where("type = ?", branchType)
	}

	if err := query.Find(&branches).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch branches",
		})
	}

	return c.JSON(fiber.Map{
		"branches": branches,
		"total":    len(branches),
	})
}

// GetBranch returns a specific branch by ID
func (bc *BranchController) GetBranch(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid branch ID",
		})
	}

	var branch models.Branch
	if err := database.DB.First(&branch, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Branch not found",
		})
	}

	return c.JSON(fiber.Map{
		"branch": branch,
	})
}

// CreateBranch creates a new branch
func (bc *BranchController) CreateBranch(c *fiber.Ctx) error {
	var branch models.Branch
	if err := c.BodyParser(&branch); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if branch.NameEn == "" || branch.NameTh == "" || branch.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Name (EN), Name (TH), and Code are required",
		})
	}

	// Check if code already exists
	var existingBranch models.Branch
	if err := database.DB.Where("code = ?", branch.Code).First(&existingBranch).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Branch code already exists",
		})
	}

	// Set default values
	if branch.Type == "" {
		branch.Type = "offline"
	}
	branch.Active = true

	if err := database.DB.Create(&branch).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create branch",
		})
	}

	// Log activity
	middleware.LogActivity(c, "CREATE", "branches", branch.ID, branch)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Branch created successfully",
		"branch":  branch,
	})
}

// UpdateBranch updates an existing branch
func (bc *BranchController) UpdateBranch(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid branch ID",
		})
	}

	var branch models.Branch
	if err := database.DB.First(&branch, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Branch not found",
		})
	}

	var updateData models.Branch
	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Check if code already exists (if changing)
	if updateData.Code != "" && updateData.Code != branch.Code {
		var existingBranch models.Branch
		if err := database.DB.Where("code = ? AND id != ?", updateData.Code, branch.ID).First(&existingBranch).Error; err == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Branch code already exists",
			})
		}
	}

	if err := database.DB.Model(&branch).Updates(updateData).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update branch",
		})
	}

	// Log activity
	middleware.LogActivity(c, "UPDATE", "branches", branch.ID, updateData)

	return c.JSON(fiber.Map{
		"message": "Branch updated successfully",
		"branch":  branch,
	})
}

// DeleteBranch deletes a branch
func (bc *BranchController) DeleteBranch(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid branch ID",
		})
	}

	var branch models.Branch
	if err := database.DB.First(&branch, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Branch not found",
		})
	}

	// Check if branch has associated users
	var userCount int64
	database.DB.Model(&models.User{}).Where("branch_id = ?", branch.ID).Count(&userCount)
	if userCount > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Cannot delete branch with associated users",
		})
	}

	if err := database.DB.Delete(&branch).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete branch",
		})
	}

	// Log activity
	middleware.LogActivity(c, "DELETE", "branches", branch.ID, branch)

	return c.JSON(fiber.Map{
		"message": "Branch deleted successfully",
	})
}