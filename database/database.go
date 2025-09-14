package database

import (
	"context"
	"englishkorat_go/config"
	"englishkorat_go/models"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB
var RedisClient *redis.Client

// Connect initializes the database and Redis connections
func Connect() {
	connectDatabase()
	connectRedis()
}

// connectDatabase initializes the database connection
func connectDatabase() {
	var err error
	dsn := config.AppConfig.GetDSN()

	// Configure GORM logger based on environment
	var gormLogger logger.Interface
	if config.AppConfig.AppEnv == "development" {
		gormLogger = logger.Default.LogMode(logger.Info)
	} else {
		gormLogger = logger.Default.LogMode(logger.Silent)
	}

	// Retry logic for transient tunnel issues
	var lastErr error
	for attempt := 1; attempt <= 8; attempt++ { // 8 attempts ~ exponential backoff up to ~30s total
		DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger:                                   gormLogger,
			DisableForeignKeyConstraintWhenMigrating: false,
		})
		if err == nil {
			break
		}
		lastErr = err
		log.Printf("Database connect attempt %d failed: %v", attempt, err)
		time.Sleep(time.Duration(attempt*attempt) * 300 * time.Millisecond)
	}
	if lastErr != nil && DB == nil {
		log.Fatal("Failed to connect to database after retries:", lastErr)
	}

	log.Println("Database connected successfully")

	// Configure connection pool
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatal("Failed to get database instance:", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetConnMaxLifetime(55 * time.Minute)

	// Auto migrate (can be skipped with SKIP_MIGRATE=true)
	if skip := os.Getenv("SKIP_MIGRATE"); skip == "true" {
		log.Println("SKIP_MIGRATE=true; skipping automatic migrations")
	} else {
		AutoMigrate()
	}
}

// AutoMigrate performs automatic database migration
func AutoMigrate() {
	if DB == nil {
		log.Println("AutoMigrate skipped: DB is nil")
		return
	}

	// Recover from potential panics in underlying drivers to provide clearer logs
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic during AutoMigrate: %v", r)
		}
	}()

	err := DB.AutoMigrate(
		&models.Branch{},
		&models.User{},
		&models.Student{},
		&models.Teacher{},
		&models.Room{},
		&models.Course{},
		&models.CourseCategory{},
		&models.ActivityLog{},
		&models.Notification{},
		&models.LogArchive{},
		&models.Student_Group{}, // Legacy model for backward compatibility
		&models.Group{},         // New Group model
		&models.GroupMember{},   // New GroupMember model
		&models.User_inCourse{},
		&models.Schedules{},
		&models.Schedule_Sessions{},
		&models.Schedules_or_Sessions_Comment{},
		&models.ScheduleParticipant{},
		&models.SessionConfirmation{},
		&models.NotificationPreference{},
	)

	if err != nil {
		log.Fatal("Auto migration failed:", err)
	}

	log.Println("Database migration completed successfully")
}

// connectRedis initializes Redis connection
func connectRedis() {
	addr := fmt.Sprintf("%s:%s", config.AppConfig.RedisHost, config.AppConfig.RedisPort)
	// Log which Redis instance we're attempting to use (do NOT log passwords)
	log.Printf("Attempting to connect to Redis at %s", addr)

	// Retry logic: try multiple times in case tunnel/container is not ready yet
	var lastErr error
	maxAttempts := 8
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		rc := redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: config.AppConfig.RedisPassword,
			DB:       0,
		})

		ctx := context.Background()
		_, err := rc.Ping(ctx).Result()
		if err == nil {
			RedisClient = rc
			log.Printf("Redis connected successfully (%s) on attempt %d", addr, attempt)
			return
		}

		// close client and record error then backoff
		_ = rc.Close()
		lastErr = err
		log.Printf("Redis connect attempt %d failed: %v", attempt, err)
		time.Sleep(time.Duration(attempt*attempt) * 300 * time.Millisecond)
	}

	log.Printf("Redis connection failed to %s after %d attempts: %v", addr, maxAttempts, lastErr)
	log.Println("Continuing without Redis - logs will be saved directly to database")
	RedisClient = nil
}

// GetRedisClient returns the Redis client instance
func GetRedisClient() *redis.Client {
	return RedisClient
}

// DropAndRecreateTable drops and recreates a table (for development)
func DropAndRecreateTable(model interface{}) error {
	if config.AppConfig.AppEnv != "development" {
		return fmt.Errorf("this operation is only allowed in development environment")
	}

	err := DB.Migrator().DropTable(model)
	if err != nil {
		return fmt.Errorf("failed to drop table: %v", err)
	}

	err = DB.AutoMigrate(model)
	if err != nil {
		return fmt.Errorf("failed to recreate table: %v", err)
	}

	return nil
}

// GetDB returns the database instance
func GetDB() *gorm.DB {
	return DB
}

// Close closes the database connection
func Close() {
	sqlDB, err := DB.DB()
	if err != nil {
		log.Println("Error getting database instance:", err)
		return
	}

	err = sqlDB.Close()
	if err != nil {
		log.Println("Error closing database connection:", err)
		return
	}

	log.Println("Database connection closed")
}
