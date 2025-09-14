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
	CreatedAt time.Time `json:"created_at"`
}

const redisListKey = "notifications:queue"

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

// QueuedForController creates minimal queuedNotification (public helper for controller)
func QueuedForController(title, titleTh, message, messageTh, typ string) queuedNotification {
	return queuedNotification{Title: title, TitleTh: titleTh, Message: message, MessageTh: messageTh, Type: typ}
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

// createDirect writes directly to DB (used by worker or fallback)
func (s *Service) createDirect(userIDs []uint, n queuedNotification) error {
	if len(userIDs) == 0 {
		return nil
	}
	notifs := make([]models.Notification, 0, len(userIDs))
	for _, uid := range userIDs {
		notifs = append(notifs, models.Notification{
			UserID:    uid,
			Title:     n.Title,
			TitleTh:   n.TitleTh,
			Message:   n.Message,
			MessageTh: n.MessageTh,
			Type:      n.Type,
			Read:      false,
		})
	}

	// Create notifications in database
	if err := s.db.Create(&notifs).Error; err != nil {
		return err
	}

	// Send WebSocket notifications if hub is available
	if s.wsHub != nil {
		for _, notif := range notifs {
			// Preload user data for WebSocket message
			s.db.Preload("User").Preload("User.Student").Preload("User.Teacher").Preload("User.Branch").First(&notif, notif.ID)

			// Convert to DTO and send via WebSocket
			dto := utils.ToNotificationDTO(notif)
			wsMessage := map[string]interface{}{
				"type": "notification",
				"data": dto,
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

func (s *Service) flushBatch(ctx context.Context, batchSize int) {
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
