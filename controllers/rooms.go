package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type RoomController struct{}

// GetRooms returns all rooms with pagination
func (rc *RoomController) GetRooms(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	offset := (page - 1) * limit

	var rooms []models.Room
	var total int64

	query := database.DB.Model(&models.Room{})

	// Filter by branch if specified
	if branchID := c.Query("branch_id"); branchID != "" {
		query = query.Where("branch_id = ?", branchID)
	}

	// Filter by status if specified
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// Filter by minimum capacity if specified
	if minCapacity := c.Query("min_capacity"); minCapacity != "" {
		query = query.Where("capacity >= ?", minCapacity)
	}

	// Get total count
	query.Count(&total)

	// Get rooms with relationships
	if err := query.Preload("Branch").
		Offset(offset).Limit(limit).Find(&rooms).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch rooms",
		})
	}

	return c.JSON(fiber.Map{
		"rooms": rooms,
		"pagination": fiber.Map{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetRoom returns a specific room by ID
func (rc *RoomController) GetRoom(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid room ID",
		})
	}

	var room models.Room
	if err := database.DB.Preload("Branch").First(&room, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Room not found",
		})
	}

	return c.JSON(fiber.Map{
		"room": room,
	})
}

// CreateRoom creates a new room
func (rc *RoomController) CreateRoom(c *fiber.Ctx) error {
	var room models.Room
	if err := c.BodyParser(&room); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if room.BranchID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Branch ID is required",
		})
	}

	if room.RoomName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Room name is required",
		})
	}

	if room.Capacity <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Capacity must be greater than 0",
		})
	}

	// Check if branch exists
	var branch models.Branch
	if err := database.DB.First(&branch, room.BranchID).Error; err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Branch not found",
		})
	}

	// Check if room name already exists in the same branch
	var existingRoom models.Room
	if err := database.DB.Where("branch_id = ? AND room_name = ?", room.BranchID, room.RoomName).
		First(&existingRoom).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Room with this name already exists in the branch",
		})
	}

	// Set default status
	if room.Status == "" {
		room.Status = "available"
	}

	if err := database.DB.Create(&room).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create room",
		})
	}

	// Load relationships
	database.DB.Preload("Branch").First(&room, room.ID)

	// Log activity
	middleware.LogActivity(c, "CREATE", "rooms", room.ID, room)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Room created successfully",
		"room":    room,
	})
}

// UpdateRoom updates an existing room
func (rc *RoomController) UpdateRoom(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid room ID",
		})
	}

	var room models.Room
	if err := database.DB.First(&room, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Room not found",
		})
	}

	var updateData models.Room
	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate capacity if provided
	if updateData.Capacity != 0 && updateData.Capacity <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Capacity must be greater than 0",
		})
	}

	// Check if room name already exists in the same branch (if changing)
	if updateData.RoomName != "" && updateData.RoomName != room.RoomName {
		var existingRoom models.Room
		branchID := room.BranchID
		if updateData.BranchID != 0 {
			branchID = updateData.BranchID
		}

		if err := database.DB.Where("branch_id = ? AND room_name = ? AND id != ?",
			branchID, updateData.RoomName, room.ID).First(&existingRoom).Error; err == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Room with this name already exists in the branch",
			})
		}
	}

	// Validate status if provided
	if updateData.Status != "" {
		validStatuses := []string{"available", "occupied", "maintenance"}
		isValid := false
		for _, status := range validStatuses {
			if updateData.Status == status {
				isValid = true
				break
			}
		}
		if !isValid {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid status. Must be: available, occupied, or maintenance",
			})
		}
	}

	if err := database.DB.Model(&room).Updates(updateData).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update room",
		})
	}

	// Load relationships
	database.DB.Preload("Branch").First(&room, room.ID)

	// Log activity
	middleware.LogActivity(c, "UPDATE", "rooms", room.ID, updateData)

	return c.JSON(fiber.Map{
		"message": "Room updated successfully",
		"room":    room,
	})
}

// DeleteRoom deletes a room
func (rc *RoomController) DeleteRoom(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid room ID",
		})
	}

	var room models.Room
	if err := database.DB.First(&room, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Room not found",
		})
	}

	if err := database.DB.Delete(&room).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete room",
		})
	}

	// Log activity
	middleware.LogActivity(c, "DELETE", "rooms", room.ID, room)

	return c.JSON(fiber.Map{
		"message": "Room deleted successfully",
	})
}

// GetRoomsByBranch returns rooms for a specific branch
func (rc *RoomController) GetRoomsByBranch(c *fiber.Ctx) error {
	branchID, err := strconv.ParseUint(c.Params("branch_id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid branch ID",
		})
	}

	var rooms []models.Room
	query := database.DB.Where("branch_id = ?", uint(branchID))

	// Filter by status if specified
	if status := c.Query("status", "available"); status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Preload("Branch").Find(&rooms).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch rooms",
		})
	}

	return c.JSON(fiber.Map{
		"rooms": rooms,
		"total": len(rooms),
	})
}

// GetAvailableRooms returns only available rooms
func (rc *RoomController) GetAvailableRooms(c *fiber.Ctx) error {
	var rooms []models.Room
	query := database.DB.Where("status = ?", "available")

	// Filter by branch if specified
	if branchID := c.Query("branch_id"); branchID != "" {
		query = query.Where("branch_id = ?", branchID)
	}

	// Filter by minimum capacity if specified
	if minCapacity := c.Query("min_capacity"); minCapacity != "" {
		query = query.Where("capacity >= ?", minCapacity)
	}

	if err := query.Preload("Branch").Find(&rooms).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch available rooms",
		})
	}

	return c.JSON(fiber.Map{
		"rooms": rooms,
		"total": len(rooms),
	})
}

// UpdateRoomStatus updates only the status of a room
func (rc *RoomController) UpdateRoomStatus(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid room ID",
		})
	}

	var room models.Room
	if err := database.DB.First(&room, uint(id)).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Room not found",
		})
	}

	var req struct {
		Status string `json:"status" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate status
	validStatuses := []string{"available", "occupied", "maintenance"}
	isValid := false
	for _, status := range validStatuses {
		if req.Status == status {
			isValid = true
			break
		}
	}
	if !isValid {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid status. Must be: available, occupied, or maintenance",
		})
	}

	if err := database.DB.Model(&room).Update("status", req.Status).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update room status",
		})
	}

	// Log activity
	middleware.LogActivity(c, "UPDATE", "rooms", room.ID, fiber.Map{
		"action":     "status_change",
		"old_status": room.Status,
		"new_status": req.Status,
	})

	return c.JSON(fiber.Map{
		"message": "Room status updated successfully",
		"status":  req.Status,
	})
}
