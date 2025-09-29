package notifications

import (
	"context"
	"encoding/json"
	"englishkorat_go/config"
	"englishkorat_go/database"
	"englishkorat_go/models"
	"englishkorat_go/utils"
	"errors"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// Queue item structure stored in Redis
// Keep minimal to reduce payload size
// We allow batching many userIDs for same payload
// CIA Goals:
//  - Confidentiality: payload only includes non-sensitive fields
//  - Integrity: we store created_at to detect stale items; DB write is source of truth
//  - Availability: if Redis down -> fallback to direct DB insert

type queuedNotification struct {
	UserIDs   []uint    `json:"user_ids"`
	Title     string    `json:"title"`
	TitleTh   string    `json:"title_th"`
	Message   string    `json:"message"`
	MessageTh string    `json:"message_th"`
	Type      string    `json:"type"`
	Channels  []string  `json:"channels,omitempty"`
	Data      any       `json:"data,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

const redisListKey = "notifications:queue"

// SettingsSnapshot represents current notification-related user preferences delivered with notifications.
type SettingsSnapshot struct {
	Settings        interface{}            `json:"settings"`
	AvailableSounds interface{}            `json:"available_sounds"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// SettingsProviderFunc resolves the settings snapshot for a user.
type SettingsProviderFunc func(userID uint) (*SettingsSnapshot, error)

var settingsProvider SettingsProviderFunc

// SetSettingsProvider configures the package-level settings provider used when broadcasting notifications.
func SetSettingsProvider(provider SettingsProviderFunc) {
	settingsProvider = provider
}

// Service exposes notification creation with optional Redis queue
// If Redis disabled/unavailable, performs direct DB insert.

type Service struct {
	db       *gorm.DB
	redis    *redis.Client
	useRedis bool
	wsHub    WSHub // WebSocket hub interface
}

// WSHub interface for WebSocket broadcasting
type WSHub interface {
	BroadcastToUser(userID uint, message interface{})
}

// defaultHub allows services created in different parts of the app (e.g., schedulers)
// to automatically broadcast over the same WebSocket hub without manually wiring each instance.
var defaultHub WSHub

// SetDefaultWSHub sets the package-level default WebSocket hub used by new Service instances.
func SetDefaultWSHub(h WSHub) {
	defaultHub = h
}

func NewService() *Service {
	return &Service{
		db:       database.GetDB(),
		redis:    database.GetRedisClient(),
		useRedis: config.AppConfig != nil && config.AppConfig.UseRedisNotifications && database.GetRedisClient() != nil,
		wsHub:    defaultHub,
	}
}

// SetWebSocketHub sets the WebSocket hub for real-time notifications
func (s *Service) SetWebSocketHub(hub WSHub) {
	s.wsHub = hub
}

// normalizeChannels keeps only allowed values and ensures default channel
func normalizeChannels(in []string) []string {
	if len(in) == 0 {
		return []string{"normal"}
	}
	allowed := map[string]struct{}{"normal": {}, "popup": {}, "line": {}}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, ch := range in {
		if _, ok := allowed[ch]; ok {
			if _, dup := seen[ch]; !dup {
				out = append(out, ch)
				seen[ch] = struct{}{}
			}
		}
	}
	if len(out) == 0 {
		out = []string{"normal"}
	}
	return out
}

// QueuedForController creates minimal queuedNotification (public helper for controller)
func QueuedForController(title, titleTh, message, messageTh, typ string, channels ...string) queuedNotification {
	ch := normalizeChannels(channels)
	return queuedNotification{Title: title, TitleTh: titleTh, Message: message, MessageTh: messageTh, Type: typ, Channels: ch}
}

// QueuedWithData allows attaching a structured data payload (deep-links/actions)
func QueuedWithData(title, titleTh, message, messageTh, typ string, data any, channels ...string) queuedNotification {
	ch := normalizeChannels(channels)
	return queuedNotification{Title: title, TitleTh: titleTh, Message: message, MessageTh: messageTh, Type: typ, Channels: ch, Data: data}
}

// EnqueueOrCreate stores notifications using Redis queue if enabled, else direct insert.
func (s *Service) EnqueueOrCreate(userIDs []uint, n queuedNotification) error {
	if len(userIDs) == 0 {
		return errors.New("no user ids")
	}
	n.UserIDs = userIDs
	n.CreatedAt = time.Now().UTC()

	if s.useRedis {
		b, err := json.Marshal(n)
		if err != nil {
			return err
		}
		if err = s.redis.RPush(context.Background(), redisListKey, b).Err(); err == nil {
			return nil // queued successfully
		}
		log.Printf("[notif] Redis queue failed, falling back to direct insert: %v", err)
	}

	// fallback: direct db insert
	return s.createDirect(userIDs, n)
}

// createDirect writes directly to DB (used by worker or fallback).
func (s *Service) createDirect(userIDs []uint, n queuedNotification) error { //nolint:gocognit
	if len(userIDs) == 0 {
		return nil
	}
	notifs := make([]models.Notification, 0, len(userIDs))
	// marshal channels to JSON
	// Always set channels JSON, defaulting to ["normal"] to avoid DB default on JSON which MySQL forbids
	var channelsJSON []byte
	var err error
	channelsJSON, err = json.Marshal(normalizeChannels(n.Channels))
	if err != nil {
		channelsJSON = []byte(`["normal"]`)
	}
	// marshal data if provided
	var dataJSON []byte
	if n.Data != nil {
		if b, err2 := json.Marshal(n.Data); err2 == nil {
			dataJSON = b
		}
	}
	for _, uid := range userIDs {
		notifs = append(notifs, models.Notification{
			UserID:    uid,
			Title:     n.Title,
			TitleTh:   n.TitleTh,
			Message:   n.Message,
			MessageTh: n.MessageTh,
			Type:      n.Type,
			Read:      false,
			Channels:  channelsJSON,
			Data:      dataJSON,
		})
	}

	// Create notifications in database
	if err := s.db.Create(&notifs).Error; err != nil {
		return err
	}

	// Send WebSocket notifications if hub is available
	if s.wsHub != nil {
		for _, notif := range notifs {
			var snapshot *SettingsSnapshot
			if settingsProvider != nil {
				if snap, err := settingsProvider(notif.UserID); err == nil {
					snapshot = snap
				}
			}
			// Preload user data for WebSocket message
			s.db.Preload("User").Preload("User.Student").Preload("User.Teacher").Preload("User.Branch").First(&notif, notif.ID)

			// Convert to DTO and send via WebSocket
			dto := utils.ToNotificationDTO(notif)
			wsMessage := map[string]interface{}{
				"type": "notification",
				"data": dto,
			}
			if snapshot != nil {
				wsMessage["settings"] = snapshot.Settings
				wsMessage["available_sounds"] = snapshot.AvailableSounds
				if len(snapshot.Metadata) > 0 {
					wsMessage["settings_metadata"] = snapshot.Metadata
				}
			}
			s.wsHub.BroadcastToUser(notif.UserID, wsMessage)
		}
	}

	return nil
}

// StartWorker starts a background worker polling Redis queue and flushing to DB
func (s *Service) StartWorker(stop <-chan struct{}) {
	if !s.useRedis {
		log.Println("[notif] Redis notifications disabled; worker not started")
		return
	}
	go func() {
		log.Println("[notif] Redis notification worker started")
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		ctx := context.Background()
		batchSize := 200
		for {
			select {
			case <-stop:
				log.Println("[notif] Worker stopping")
				return
			case <-ticker.C:
				s.flushBatch(ctx, batchSize)
			}
		}
	}()
}

// flushBatch polls redis queue and processes notifications in batches.
func (s *Service) flushBatch(ctx context.Context, batchSize int) { //nolint:gocognit
	if s.redis == nil {
		return
	}
	// Use pipeline: LRange + LTrim approach to make it safe for moderate concurrency
	for i := 0; i < 5; i++ { // up to 5 sub-batches per tick
		vals, err := s.redis.LRange(ctx, redisListKey, 0, int64(batchSize-1)).Result()
		if err != nil || len(vals) == 0 {
			return
		}
		// Trim immediately to avoid duplicates (best-effort)
		if err = s.redis.LTrim(ctx, redisListKey, int64(len(vals)), -1).Err(); err != nil {
			log.Printf("[notif] LTrim failed: %v", err)
		}
		for _, raw := range vals {
			var q queuedNotification
			if err := json.Unmarshal([]byte(raw), &q); err != nil {
				continue
			}
			if err := s.createDirect(q.UserIDs, q); err != nil {
				log.Printf("[notif] DB insert failed (retry later?): %v", err)
			}
		}
		if len(vals) < batchSize {
			return
		}
	}
}
