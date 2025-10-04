package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime/multipart"
	"path/filepath"
	"strings"

	"englishkorat_go/database"
	"englishkorat_go/models"
	notifsvc "englishkorat_go/services/notifications"
	"englishkorat_go/storage"

	"gorm.io/gorm"
)

const (
	soundIDCustom                    = "custom"
	preferenceKeyCustomSoundURL      = "custom_sound_url"
	preferenceKeyCustomSoundFilename = "custom_sound_filename"
	preferenceKeyCustomSoundKey      = "custom_sound_s3_key"
	maxCustomSoundSizeBytes          = 5 * 1024 * 1024 // 5 MB
)

var (
	allowedLanguages = map[string]struct{}{
		"th":   {},
		"en":   {},
		"auto": {},
	}

	allowedCustomSoundExtensions = map[string]struct{}{
		"mp3": {},
		"wav": {},
	}

	builtInSoundLookup = func() map[string]models.NotificationSoundOption {
		lookup := make(map[string]models.NotificationSoundOption, len(models.BuiltInNotificationSoundOptions))
		for _, opt := range models.BuiltInNotificationSoundOptions {
			lookup[opt.ID] = opt
		}
		return lookup
	}()

	// ErrSettingsValidation indicates a user-facing validation error while updating settings
	ErrSettingsValidation = errors.New("settings validation error")
)

// SettingsInternalError wraps non-user (server-side) failures with a short machine code
// so the controller layer can surface a stable error code while hiding internals.
type SettingsInternalError struct {
	Code string
	Err  error
}

func (e *SettingsInternalError) Error() string { return e.Err.Error() }
func (e *SettingsInternalError) Unwrap() error { return e.Err }

// SettingsService manages persistence for user settings/preferences
type SettingsService struct{}

func init() {
	notifsvc.SetSettingsProvider(func(userID uint) (*notifsvc.SettingsSnapshot, error) {
		svc := NewSettingsService()
		settings, err := svc.GetOrCreate(userID)
		if err != nil {
			return nil, err
		}
		response := svc.BuildSettingsResponse(settings)
		return &notifsvc.SettingsSnapshot{
			Settings:        response.Settings,
			AvailableSounds: response.AvailableSounds,
			Metadata:        response.Metadata,
		}, nil
	})
	// Ensure latest schema (adds explicit custom sound columns if they don't exist)
	if database.DB != nil {
		if err := database.DB.AutoMigrate(&models.UserSettings{}); err != nil {
			log.Printf("warning: AutoMigrate UserSettings failed: %v", err)
		}
	}
}

// UpdateUserSettingsInput describes the fields that can be updated for a user's settings
type UpdateUserSettingsInput struct {
	Language                 *string                `json:"language"`
	EnableNotificationSound  *bool                  `json:"enable_notification_sound"`
	NotificationSound        *string                `json:"notification_sound"`
	EnableEmailNotifications *bool                  `json:"enable_email_notifications"`
	EnablePhoneNotifications *bool                  `json:"enable_phone_notifications"`
	EnableInAppNotifications *bool                  `json:"enable_in_app_notifications"`
	AdditionalPreferences    map[string]interface{} `json:"additional_preferences"`
}

// SettingsDTO is a structured representation of user settings enriched with sound metadata.
type SettingsDTO struct {
	UserID                   uint   `json:"user_id"`
	Language                 string `json:"language"`
	EnableNotificationSound  bool   `json:"enable_notification_sound"`
	NotificationSound        string `json:"notification_sound"`
	NotificationSoundFile    string `json:"notification_sound_file,omitempty"`
	EnableEmailNotifications bool   `json:"enable_email_notifications"`
	EnablePhoneNotifications bool   `json:"enable_phone_notifications"`
	EnableInAppNotifications bool   `json:"enable_in_app_notifications"`
	CustomSoundURL           string `json:"custom_sound_url,omitempty"`
	CustomSoundFilename      string `json:"custom_sound_filename,omitempty"`
}

// SettingsResponse bundles settings alongside available sound options for clients.
type SettingsResponse struct {
	Settings        SettingsDTO                      `json:"settings"`
	AvailableSounds []models.NotificationSoundOption `json:"available_sounds"`
	Metadata        map[string]interface{}           `json:"metadata,omitempty"`
}

// NewSettingsService creates a new service instance
func NewSettingsService() *SettingsService {
	return &SettingsService{}
}

// GetOrCreate fetches user settings, creating a default record if necessary
func (s *SettingsService) GetOrCreate(userID uint) (*models.UserSettings, error) {
	settings := &models.UserSettings{}
	if err := database.DB.Where("user_id = ?", userID).First(settings).Error; err != nil {
		// If no record found, create defaults. If the table itself doesn't exist
		// (for example when automatic migrations were skipped), attempt to
		// AutoMigrate the `user_settings` table then create the defaults.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			defaults := models.UserSettings{
				UserID:                   userID,
				Language:                 "th",
				EnableNotificationSound:  true,
				NotificationSound:        "default",
				EnableEmailNotifications: false,
				EnablePhoneNotifications: false,
				EnableInAppNotifications: true,
				AdditionalPreferences:    models.JSON([]byte("{}")),
			}
			if createErr := database.DB.Create(&defaults).Error; createErr != nil {
				return nil, createErr
			}
			s.ensurePreferencesInitialized(&defaults)
			return &defaults, nil
		}

		// Detect MySQL "table doesn't exist" error (1146) or generic missing table
		// message and try to create the table then create defaults.
		if strings.Contains(err.Error(), "1146") || strings.Contains(strings.ToLower(err.Error()), "doesn't exist") {
			if migrateErr := database.DB.AutoMigrate(&models.UserSettings{}); migrateErr != nil {
				return nil, migrateErr
			}

			// After creating the table, insert default settings for the user.
			defaults := models.UserSettings{
				UserID:                   userID,
				Language:                 "th",
				EnableNotificationSound:  true,
				NotificationSound:        "default",
				EnableEmailNotifications: false,
				EnablePhoneNotifications: false,
				EnableInAppNotifications: true,
				AdditionalPreferences:    models.JSON([]byte("{}")),
			}
			if createErr := database.DB.Create(&defaults).Error; createErr != nil {
				return nil, createErr
			}
			s.ensurePreferencesInitialized(&defaults)
			return &defaults, nil
		}

		return nil, err
	}
	s.ensurePreferencesInitialized(settings)
	return settings, nil
}

func (s *SettingsService) ensurePreferencesInitialized(settings *models.UserSettings) {
	if settings == nil {
		return
	}
	if settings.AdditionalPreferences.IsNull() {
		settings.AdditionalPreferences = models.JSON([]byte("{}"))
	}
}

// Update applies the requested changes to the user's settings, enforcing validation rules
func (s *SettingsService) Update(user *models.User, input UpdateUserSettingsInput) (*models.UserSettings, error) { //nolint:gocognit,gocyclo
	if user == nil {
		return nil, validationError("user is required")
	}

	settings, err := s.GetOrCreate(user.ID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}

	if err := applyLanguageUpdate(updates, input.Language); err != nil {
		return nil, err
	}

	if err := s.applyNotificationSoundSelection(updates, settings, input.AdditionalPreferences, input.NotificationSound); err != nil {
		return nil, err
	}

	if val, ok := extractBool(input.EnableNotificationSound); ok {
		updates["enable_notification_sound"] = val
	}

	if err := applyValidatedBool(updates, "enable_email_notifications", input.EnableEmailNotifications, func(value bool) error {
		if !value {
			return nil
		}
		if user.Email == nil || strings.TrimSpace(*user.Email) == "" {
			return validationError("cannot enable email notifications without a user email")
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if err := applyValidatedBool(updates, "enable_phone_notifications", input.EnablePhoneNotifications, func(value bool) error {
		if !value {
			return nil
		}
		if strings.TrimSpace(user.Phone) == "" {
			return validationError("cannot enable phone notifications without a user phone number")
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if val, ok := extractBool(input.EnableInAppNotifications); ok {
		updates["enable_in_app_notifications"] = val
	}

	if err := applyAdditionalPreferences(updates, input.AdditionalPreferences); err != nil {
		return nil, err
	}

	if len(updates) == 0 {
		s.ensurePreferencesInitialized(settings)
		return settings, nil
	}

	if err := database.DB.Model(settings).Updates(updates).Error; err != nil {
		return nil, err
	}

	if err := database.DB.First(settings, settings.ID).Error; err != nil {
		return nil, err
	}

	s.ensurePreferencesInitialized(settings)
	return settings, nil
}

func applyLanguageUpdate(updates map[string]interface{}, value *string) error {
	if value == nil {
		return nil
	}

	lang := strings.ToLower(strings.TrimSpace(*value))
	if lang == "" {
		return validationError("language cannot be empty")
	}
	if _, ok := allowedLanguages[lang]; !ok {
		return validationError(fmt.Sprintf("unsupported language '%s'", lang))
	}
	updates["language"] = lang
	return nil
}

func applyValidatedBool(updates map[string]interface{}, key string, value *bool, validator func(bool) error) error {
	if value == nil {
		return nil
	}
	if validator != nil {
		if err := validator(*value); err != nil {
			return err
		}
	}
	updates[key] = *value
	return nil
}

func applyAdditionalPreferences(updates map[string]interface{}, prefs map[string]interface{}) error {
	if prefs == nil {
		return nil
	}
	buffer, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("failed to encode additional_preferences: %w", err)
	}
	updates["additional_preferences"] = models.JSON(buffer)
	return nil
}

func (s *SettingsService) applyNotificationSoundSelection(updates map[string]interface{}, settings *models.UserSettings, incomingPrefs map[string]interface{}, value *string) error {
	if value == nil {
		return nil
	}
	if settings == nil {
		return validationError("settings record not found")
	}

	sound := strings.ToLower(strings.TrimSpace(*value))
	if sound == "" {
		return validationError("notification_sound cannot be empty")
	}

	if sound == soundIDCustom {
		url, _, _ := s.resolveCustomSoundMetadata(settings, incomingPrefs)
		if url == "" {
			return validationError("custom notification sound not uploaded yet")
		}
		updates["notification_sound"] = soundIDCustom
		return nil
	}

	if _, ok := builtInSoundLookup[sound]; !ok {
		return validationError(fmt.Sprintf("unsupported notification_sound '%s'", sound))
	}

	updates["notification_sound"] = sound
	return nil
}

func (s *SettingsService) resolveCustomSoundMetadata(settings *models.UserSettings, incoming map[string]interface{}) (url, filename, key string) {
	// Prefer explicit columns (added Oct 2025) falling back to legacy JSON preferences
	if settings != nil {
		url = strings.TrimSpace(settings.CustomSoundURL)
		filename = strings.TrimSpace(settings.CustomSoundFilename)
		key = strings.TrimSpace(settings.CustomSoundS3Key)
	}
	if url != "" || filename != "" || key != "" { // already resolved via new columns
		return
	}
	combined := decodeAdditionalPreferences(settings.AdditionalPreferences)
	if incoming != nil {
		combined = mergePreferenceMaps(combined, incoming)
	}
	url = getStringPreference(combined, preferenceKeyCustomSoundURL)
	filename = getStringPreference(combined, preferenceKeyCustomSoundFilename)
	key = getStringPreference(combined, preferenceKeyCustomSoundKey)
	return
}

func decodeAdditionalPreferences(data models.JSON) map[string]interface{} {
	if data.IsNull() {
		return map[string]interface{}{}
	}
	var prefs map[string]interface{}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return map[string]interface{}{}
	}
	if prefs == nil {
		return map[string]interface{}{}
	}
	return prefs
}

func mergePreferenceMaps(base map[string]interface{}, overrides map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = map[string]interface{}{}
	}
	if overrides == nil {
		return base
	}
	for key, value := range overrides {
		base[key] = value
	}
	return base
}

func getStringPreference(prefs map[string]interface{}, key string) string {
	if prefs == nil {
		return ""
	}
	value, ok := prefs[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func cloneSoundOptions() []models.NotificationSoundOption {
	options := make([]models.NotificationSoundOption, len(models.BuiltInNotificationSoundOptions))
	copy(options, models.BuiltInNotificationSoundOptions)
	return options
}

func extractS3KeyFromURL(url string) string {
	parts := strings.Split(url, ".amazonaws.com/")
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func (s *SettingsService) notificationSoundFile(settings *models.UserSettings) string {
	if settings == nil {
		return ""
	}
	if settings.NotificationSound == soundIDCustom {
		url, _, _ := s.resolveCustomSoundMetadata(settings, nil)
		return url
	}
	if option, ok := builtInSoundLookup[settings.NotificationSound]; ok {
		return option.File
	}
	return ""
}

func (s *SettingsService) customSoundInfo(settings *models.UserSettings) (string, string) {
	url, filename, _ := s.resolveCustomSoundMetadata(settings, nil)
	if filename == "" && url != "" {
		filename = filepath.Base(url)
	}
	return url, filename
}

func (s *SettingsService) AvailableSoundOptions() []models.NotificationSoundOption {
	return cloneSoundOptions()
}

func (s *SettingsService) BuildSettingsResponse(settings *models.UserSettings) SettingsResponse {
	if settings == nil {
		return SettingsResponse{
			Settings:        SettingsDTO{},
			AvailableSounds: cloneSoundOptions(),
			Metadata: map[string]interface{}{
				"supports_custom_sound":           true,
				"max_custom_sound_size_bytes":     maxCustomSoundSizeBytes,
				"allowed_custom_sound_extensions": []string{"mp3", "wav"},
			},
		}
	}

	s.ensurePreferencesInitialized(settings)

	notificationSoundFile := s.notificationSoundFile(settings)
	customURL, customFilename := s.customSoundInfo(settings)
	// If explicit columns present, override legacy extracted values
	if strings.TrimSpace(settings.CustomSoundURL) != "" {
		customURL = settings.CustomSoundURL
	}
	if strings.TrimSpace(settings.CustomSoundFilename) != "" {
		customFilename = settings.CustomSoundFilename
	}

	dto := SettingsDTO{
		UserID:                   settings.UserID,
		Language:                 settings.Language,
		EnableNotificationSound:  settings.EnableNotificationSound,
		NotificationSound:        settings.NotificationSound,
		NotificationSoundFile:    notificationSoundFile,
		EnableEmailNotifications: settings.EnableEmailNotifications,
		EnablePhoneNotifications: settings.EnablePhoneNotifications,
		EnableInAppNotifications: settings.EnableInAppNotifications,
		CustomSoundURL:           customURL,
		CustomSoundFilename:      customFilename,
	}

	if dto.NotificationSound == soundIDCustom && dto.NotificationSoundFile == "" {
		dto.NotificationSoundFile = customURL
	}

	metadata := map[string]interface{}{
		"supports_custom_sound":           true,
		"max_custom_sound_size_bytes":     maxCustomSoundSizeBytes,
		"allowed_custom_sound_extensions": []string{"mp3", "wav"},
	}

	return SettingsResponse{
		Settings:        dto,
		AvailableSounds: cloneSoundOptions(),
		Metadata:        metadata,
	}
}

func (s *SettingsService) UploadCustomSound(user *models.User, fileHeader *multipart.FileHeader) (*models.UserSettings, error) {
	if user == nil {
		return nil, validationError("user is required")
	}
	if fileHeader == nil {
		return nil, validationError("sound file is required")
	}
	if fileHeader.Size == 0 {
		return nil, validationError("sound file cannot be empty")
	}
	if fileHeader.Size > maxCustomSoundSizeBytes {
		return nil, validationError(fmt.Sprintf("sound file is too large (max %d bytes)", maxCustomSoundSizeBytes))
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileHeader.Filename)), ".")
	if _, ok := allowedCustomSoundExtensions[ext]; !ok {
		return nil, validationError("unsupported sound file type; allowed: mp3, wav")
	}

	settings, err := s.GetOrCreate(user.ID)
	if err != nil {
		return nil, err
	}

	storageService, err := storage.NewStorageService()
	if err != nil {
		return nil, &SettingsInternalError{Code: "S3_INIT_FAILED", Err: fmt.Errorf("failed to initialize storage service: %w", err)}
	}

	oldURL, _, _ := s.resolveCustomSoundMetadata(settings, nil)

	uploadedURL, err := storageService.UploadFile(fileHeader, "custom-notification-sounds", user.ID)
	if err != nil {
		return nil, &SettingsInternalError{Code: "S3_UPLOAD_FAILED", Err: fmt.Errorf("failed to upload custom sound: %w", err)}
	}

	// Update both new explicit columns and legacy JSON preferences for backward compatibility
	newFilename := filepath.Base(fileHeader.Filename)
	newKey := extractS3KeyFromURL(uploadedURL)
	prefs := decodeAdditionalPreferences(settings.AdditionalPreferences)
	prefs[preferenceKeyCustomSoundURL] = uploadedURL
	prefs[preferenceKeyCustomSoundFilename] = newFilename
	prefs[preferenceKeyCustomSoundKey] = newKey
	buffer, err := json.Marshal(prefs)
	if err != nil {
		return nil, &SettingsInternalError{Code: "PREFS_ENCODE_FAILED", Err: fmt.Errorf("failed to encode custom sound preferences: %w", err)}
	}
	updates := map[string]interface{}{
		"additional_preferences": models.JSON(buffer),
		"notification_sound":     soundIDCustom,
		"custom_sound_url":       uploadedURL,
		"custom_sound_filename":  newFilename,
		"custom_sound_s3_key":    newKey,
	}

	if err := database.DB.Model(settings).Updates(updates).Error; err != nil {
		return nil, &SettingsInternalError{Code: "DB_UPDATE_FAILED", Err: err}
	}

	if err := database.DB.First(settings, settings.ID).Error; err != nil {
		return nil, &SettingsInternalError{Code: "DB_RELOAD_FAILED", Err: err}
	}
	s.ensurePreferencesInitialized(settings)

	if oldURL != "" && oldURL != uploadedURL {
		if err := storageService.DeleteFile(oldURL); err != nil {
			log.Printf("warning: failed to delete old custom sound for user %d: %v", user.ID, err)
		}
	}

	return settings, nil
}

func validationError(message string) error {
	return fmt.Errorf("%w: %s", ErrSettingsValidation, message)
}

func extractBool(value *bool) (bool, bool) {
	if value == nil {
		return false, false
	}
	return *value, true
}
