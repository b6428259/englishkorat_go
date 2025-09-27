package controllers

import (
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"englishkorat_go/services"

	"github.com/gofiber/fiber/v2"
)

type AbsenceController struct{}

func (ac *AbsenceController) CreateAbsence(c *fiber.Ctx) error {
	var req struct {
		GroupID   uint   `json:"group_id"`
		SessionID uint   `json:"session_id"`
		Reason    string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	claims, err := middleware.GetCurrentClaims(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	userID := claims.UserID

	absence, err := services.CreateAbsence(req.GroupID, req.SessionID, userID, req.Reason)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(absence)
}

func (ac *AbsenceController) ApproveAbsence(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid absence ID"})
	}
	var req struct {
		Approve bool `json:"approve"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	claims, err := middleware.GetCurrentClaims(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	adminID := claims.UserID

	if err := services.ApproveAbsence(uint(id), adminID, req.Approve); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "success"})
}

func (ac *AbsenceController) GetAbsences(c *fiber.Ctx) error {

	var absences []models.Absence
	if err := database.DB.
		Preload("Session").
		Preload("Group").
		Order("created_at DESC").
		Find(&absences).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "cannot fetch absences"})
	}

	return c.JSON(absences)
}


func (ac *AbsenceController) GetAbsencesByGroup(c *fiber.Ctx) error {
	groupID, err := c.ParamsInt("group_id")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid group ID"})
	}

	var absences []models.Absence
	if err := database.DB.
		Preload("Session").
		Preload("Group").
        Where("group_id = ?", groupID).
		Order("created_at DESC").
		Find(&absences).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "cannot fetch absences"})
	}

	return c.JSON(absences)
}
