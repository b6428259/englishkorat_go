package controllers

import (
	"errors"
	"strconv"

	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/models"
	"englishkorat_go/services"

	"github.com/gofiber/fiber/v2"
)

const (
	errUserNotFoundMessage      = "User not found"
	errSoundFileRequiredMessage = "Sound file is required"
)

type SettingsController struct {
	service *services.SettingsService
}

type updateSettingsRequest struct {
	Language                 *string                `json:"language"`
	EnableNotificationSound  *bool                  `json:"enable_notification_sound"`
	NotificationSound        *string                `json:"notification_sound"`
	EnableEmailNotifications *bool                  `json:"enable_email_notifications"`
	EnablePhoneNotifications *bool                  `json:"enable_phone_notifications"`
	EnableInAppNotifications *bool                  `json:"enable_in_app_notifications"`
	AdditionalPreferences    map[string]interface{} `json:"additional_preferences"`
}

func NewSettingsController() *SettingsController {
	return &SettingsController{service: services.NewSettingsService()}
}

func (sc *SettingsController) GetMySettings(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": errUserNotFoundMessage})
	}

	settings, err := sc.service.GetOrCreate(user.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load settings"})
	}

	response := sc.service.BuildSettingsResponse(settings)
	return c.JSON(response)
}

func (sc *SettingsController) UpdateMySettings(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": errUserNotFoundMessage})
	}

	var req updateSettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	input := services.UpdateUserSettingsInput{
		Language:                 req.Language,
		EnableNotificationSound:  req.EnableNotificationSound,
		NotificationSound:        req.NotificationSound,
		EnableEmailNotifications: req.EnableEmailNotifications,
		EnablePhoneNotifications: req.EnablePhoneNotifications,
		EnableInAppNotifications: req.EnableInAppNotifications,
		AdditionalPreferences:    req.AdditionalPreferences,
	}

	settings, err := sc.service.Update(user, input)
	if err != nil {
		return handleSettingsError(c, err)
	}

	middleware.LogActivity(c, "UPDATE", "user_settings", settings.ID, fiber.Map{
		"target_user_id": user.ID,
	})

	response := sc.service.BuildSettingsResponse(settings)
	payload := fiber.Map{
		"message":          "Settings updated",
		"settings":         response.Settings,
		"available_sounds": response.AvailableSounds,
	}
	if len(response.Metadata) > 0 {
		payload["metadata"] = response.Metadata
	}
	return c.JSON(payload)
}

func (sc *SettingsController) GetUserSettings(c *fiber.Ctx) error {
	user, err := findUserByParamID(c)
	if err != nil {
		return err
	}

	settings, svcErr := sc.service.GetOrCreate(user.ID)
	if svcErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load settings"})
	}

	response := sc.service.BuildSettingsResponse(settings)
	payload := fiber.Map{
		"user":             fiber.Map{"id": user.ID, "username": user.Username, "email": user.Email, "phone": user.Phone},
		"settings":         response.Settings,
		"available_sounds": response.AvailableSounds,
	}
	if len(response.Metadata) > 0 {
		payload["metadata"] = response.Metadata
	}
	return c.JSON(payload)
}

func (sc *SettingsController) UpdateUserSettings(c *fiber.Ctx) error {
	user, err := findUserByParamID(c)
	if err != nil {
		return err
	}

	var req updateSettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	input := services.UpdateUserSettingsInput{
		Language:                 req.Language,
		EnableNotificationSound:  req.EnableNotificationSound,
		NotificationSound:        req.NotificationSound,
		EnableEmailNotifications: req.EnableEmailNotifications,
		EnablePhoneNotifications: req.EnablePhoneNotifications,
		EnableInAppNotifications: req.EnableInAppNotifications,
		AdditionalPreferences:    req.AdditionalPreferences,
	}

	settings, svcErr := sc.service.Update(user, input)
	if svcErr != nil {
		return handleSettingsError(c, svcErr)
	}

	middleware.LogActivity(c, "UPDATE", "user_settings", settings.ID, fiber.Map{
		"target_user_id": user.ID,
	})

	response := sc.service.BuildSettingsResponse(settings)
	payload := fiber.Map{
		"message":          "Settings updated",
		"user":             fiber.Map{"id": user.ID, "username": user.Username, "email": user.Email, "phone": user.Phone},
		"settings":         response.Settings,
		"available_sounds": response.AvailableSounds,
	}
	if len(response.Metadata) > 0 {
		payload["metadata"] = response.Metadata
	}
	return c.JSON(payload)
}

func (sc *SettingsController) UploadMyCustomSound(c *fiber.Ctx) error {
	user, err := middleware.GetCurrentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": errUserNotFoundMessage})
	}

	file, err := c.FormFile("sound")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": errSoundFileRequiredMessage})
	}

	settings, svcErr := sc.service.UploadCustomSound(user, file)
	if svcErr != nil {
		return handleSettingsError(c, svcErr)
	}

	response := sc.service.BuildSettingsResponse(settings)
	payload := fiber.Map{
		"message":          "Custom notification sound uploaded",
		"settings":         response.Settings,
		"available_sounds": response.AvailableSounds,
	}
	if len(response.Metadata) > 0 {
		payload["metadata"] = response.Metadata
	}
	middleware.LogActivity(c, "UPDATE", "user_settings", settings.ID, fiber.Map{
		"target_user_id": user.ID,
		"action":         "upload_custom_sound",
		"filename":       response.Settings.CustomSoundFilename,
	})
	return c.JSON(payload)
}

func (sc *SettingsController) UploadUserCustomSound(c *fiber.Ctx) error {
	user, err := findUserByParamID(c)
	if err != nil {
		return err
	}

	file, err := c.FormFile("sound")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": errSoundFileRequiredMessage})
	}

	settings, svcErr := sc.service.UploadCustomSound(user, file)
	if svcErr != nil {
		return handleSettingsError(c, svcErr)
	}

	response := sc.service.BuildSettingsResponse(settings)
	payload := fiber.Map{
		"message":          "Custom notification sound uploaded",
		"user":             fiber.Map{"id": user.ID, "username": user.Username, "email": user.Email, "phone": user.Phone},
		"settings":         response.Settings,
		"available_sounds": response.AvailableSounds,
	}
	if len(response.Metadata) > 0 {
		payload["metadata"] = response.Metadata
	}
	middleware.LogActivity(c, "UPDATE", "user_settings", settings.ID, fiber.Map{
		"target_user_id": user.ID,
		"action":         "upload_custom_sound",
		"filename":       response.Settings.CustomSoundFilename,
	})
	return c.JSON(payload)
}

func findUserByParamID(c *fiber.Ctx) (*models.User, error) {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return nil, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	var user models.User
	if dbErr := database.DB.First(&user, uint(id)).Error; dbErr != nil {
		return nil, c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": errUserNotFoundMessage})
	}

	return &user, nil
}

func handleSettingsError(c *fiber.Ctx, err error) error {
	if err == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Unknown settings error"})
	}

	if errors.Is(err, services.ErrSettingsValidation) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update settings"})
}
