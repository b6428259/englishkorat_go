# User Settings Module

## Overview

The user settings module stores per-user preferences for language and notification delivery. Each user has a single record in the `user_settings` table. Records are created on demand the first time a user (or an administrator) loads the settings endpoint.

### Default values

| Field                         | Default | Notes |
| ----------------------------- | ------- | ----- |
| `language`                    | `th`    | Allowed values: `th`, `en`, `auto` |
| `enable_notification_sound`   | `true`  | Controls whether the client should play sounds when in-app notifications arrive |
| `notification_sound`          | `default` | Lower-case identifier of the desired sound profile |
| `enable_email_notifications`  | `false` | Can be enabled only if the user has a non-empty email address |
| `enable_phone_notifications`  | `false` | Can be enabled only if the user has a non-empty phone number |
| `enable_in_app_notifications` | `true`  | Allows the UI to suppress in-app popups if disabled |
| `additional_preferences`      | `{}`    | JSON blob for future expansion (store custom sound metadata under `custom_sound_url`, `custom_sound_filename`, `custom_sound_s3_key`) |

## Database

The new `models.UserSettings` struct is migrated automatically via `database.AutoMigrate()`. The table enforces a unique `user_id` per record.

```text
user_settings
├── id (PK)
├── user_id (unique, FK → users.id)
├── language (varchar 20)
├── enable_notification_sound (bool)
├── notification_sound (varchar 100)
├── enable_email_notifications (bool)
├── enable_phone_notifications (bool)
├── enable_in_app_notifications (bool)
├── additional_preferences (JSON nullable)
├── created_at / updated_at / deleted_at
```

`POST /api/settings/me/custom-sound`

| Field | Type | Notes |
| ----- | ---- | ----- |
| `sound` | file | Required MP3 or WAV file (≤ 5 MB). |

Response mirrors the structure above (`settings`, `available_sounds`, `metadata`).

## Service contract

`services.SettingsService` exposes:

- `GetOrCreate(userID uint)` – returns existing settings or inserts defaults.
- `Update(user *models.User, input UpdateUserSettingsInput)` – validates and persists updates. Validation errors return `ErrSettingsValidation` (mapped to HTTP 400).
- `UploadCustomSound(user *models.User, file *multipart.FileHeader)` – uploads/replaces a per-user notification sound in S3, updates settings, and purges the previous file.
- `BuildSettingsResponse(settings *models.UserSettings)` – returns a `SettingsResponse` containing normalized data plus available sound metadata for API responses.
- `AvailableSoundOptions()` – exposes the built-in sound catalog for display.

Validation rules enforced by the service:

- `language` must be one of `th`, `en`, `auto`.
- `notification_sound` must be one of the published built-in identifiers or the special value `custom` (only allowed when a custom sound has been uploaded).
- Email notifications cannot be enabled unless the user has an email address.
- Phone notifications cannot be enabled unless the user has a phone number.

### Notification sounds

Built-in sounds are described by `models.BuiltInNotificationSoundOptions` and surfaced via `SettingsService.AvailableSoundOptions()`. Each option includes:

| Field | Description |
| ----- | ----------- |
| `id` | Lower-case identifier sent in `notification_sound`. |
| `label` | Friendly name for UI display. |
| `description` | Short usage hint. |
| `file` | Relative asset path (clients bundle or fetch as appropriate). |

To provide a per-user sound, clients can upload an MP3 or WAV file (max 5 MB). The backend stores the public S3 URL and ensures only one active custom sound per user (older uploads are deleted automatically).

## HTTP API

All routes are registered under `/api` and require authentication unless noted.

### Current user routes

| Method | Path                           | Description |
| ------ | ------------------------------ | ----------- |
| `GET`  | `/api/settings/me`             | Fetch the authenticated user's settings (creates defaults if missing). |
| `PUT`  | `/api/settings/me`             | Update the authenticated user's settings. |
| `POST` | `/api/settings/me/custom-sound`| Upload or replace the user's custom notification sound (`multipart/form-data` with `sound` field). |

#### Request body (`PUT /api/settings/me`)

All fields are optional; only provided values are updated.

```json
{
	"language": "en",
	"enable_notification_sound": false,
	"notification_sound": "chime",
	"enable_email_notifications": true,
	"enable_phone_notifications": false,
	"enable_in_app_notifications": true,
	"additional_preferences": {
		"daily_summary_time": "08:00"
	}
}
```

#### Responses

`200 OK`

```json
{
	"message": "Settings updated",
	"settings": {
		"user_id": 42,
		"language": "en",
		"enable_notification_sound": true,
		"notification_sound": "custom",
		"notification_sound_file": "https://cdn.example.com/sounds/chime.mp3",
		"enable_email_notifications": true,
		"enable_phone_notifications": false,
		"enable_in_app_notifications": true,
		"custom_sound_url": "https://s3.ap-southeast-1.amazonaws.com/bucket/custom-notification-sounds/42/.../sound.mp3",
		"custom_sound_filename": "sound.mp3"
	},
	"available_sounds": [
		{
			"id": "default",
			"label": "Default",
			"description": "Classic alert chime",
			"file": "/sounds/default.mp3"
		},
		{
			"id": "soft",
			"label": "Soft Bell",
			"description": "Gentle bell ideal for focus mode",
			"file": "/sounds/soft.mp3"
		}
	],
	"metadata": {
		"supports_custom_sound": true,
		"max_custom_sound_size_bytes": 5242880,
		"allowed_custom_sound_extensions": ["mp3", "wav"]
	}
}
```

`400 Bad Request` is returned for validation failures (e.g., enabling email notifications without an email on file). `500 Internal Server Error` is returned for unexpected database issues.

### Administrative routes

Available to owners and admins only:

| Method | Path                         | Description |
| ------ | ---------------------------- | ----------- |
| `GET`  | `/api/users/:id/settings`    | Fetch settings for a specified user. |
| `PUT`  | `/api/users/:id/settings`    | Update settings for a specified user. |
| `POST` | `/api/users/:id/settings/custom-sound` | Upload or replace a user's custom notification sound (owners/admins only). |

Body format and validation are identical to the current-user endpoint. Successful updates are logged via `LogActivity` under the `user_settings` resource.

## Client integration notes

- Clients should respect boolean toggles and handle missing optional fields gracefully.
- When `enable_notification_sound` is `false`, `notification_sound` remains stored for future reuse when the toggle is re-enabled.
- New sound identifiers can be introduced without schema changes; keep them lower-case alphanumeric (plus `-` / `_`).
- `additional_preferences` is intended for forward-compatible key/value additions and is not interpreted by the backend.
- WebSocket notifications and REST notification responses now embed the current settings snapshot (`settings`, `available_sounds`, `metadata`) so the UI can decide whether to play sounds immediately.
