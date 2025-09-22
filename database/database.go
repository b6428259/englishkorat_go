package database

import (
	"context"
	"englishkorat_go/config"
	"englishkorat_go/models"
	"fmt"
	"log"
	"strings"
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

	// Auto migrate (can be skipped via config.SkipMigrate)
	skip := false
	if config.AppConfig != nil {
		skip = config.AppConfig.SkipMigrate
	}
	log.Println("Running automatic migrations... SkipMigrate=", skip)
	if skip {
		log.Println("SkipMigrate=true; skipping automatic migrations")
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

	modelsList := []interface{}{
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
		&models.LineGroup{},
		&models.Book{},
		&models.ClassProgress{},
		&models.Bill{},
	}

	// Pre-sanitize: fix invalid JSON in notifications.channels before altering column type
	_ = sanitizeNotificationChannels(DB)

	err := DB.AutoMigrate(modelsList...)

	if err != nil {
		log.Fatal("Auto migration failed:", err)
	}

	log.Println("Database migration completed successfully")

	// Post-migration: ensure users.email is NULLable (not empty string) to avoid unique '' collisions
	// Some older schemas might have `email` as NOT NULL with default '' which breaks uniqueness when blank.
	// Try to alter column to be NULLABLE with DEFAULT NULL in a best-effort manner.
	if err := ensureUsersEmailNullable(DB); err != nil {
		log.Printf("Warning: could not ensure users.email is nullable: %v", err)
	}

	// Optional: prune extra columns not defined in models (dangerous - gated by env)
	if config.AppConfig != nil && config.AppConfig.PruneColumns {
		log.Println("PRUNE_COLUMNS=true; starting schema prune for extra columns")
		for _, m := range modelsList {
			if err := pruneExtraColumns(DB, m); err != nil {
				log.Printf("Schema prune warning for %T: %v", m, err)
			}
		}
		log.Println("Schema prune completed")
	}
}

// sanitizeNotificationChannels ensures notifications.channels contain valid JSON values
// Sets empty/invalid/NULL to ["normal"]. Best-effort and non-fatal.
func sanitizeNotificationChannels(db *gorm.DB) error {
	// Set NULL to default
	if err := db.Exec(`UPDATE notifications SET channels='["normal"]' WHERE channels IS NULL`).Error; err != nil {
		log.Printf("sanitize channels (NULL) warning: %v", err)
	}

	// Set empty string to default
	if err := db.Exec(`UPDATE notifications SET channels='["normal"]' WHERE channels=''`).Error; err != nil {
		log.Printf("sanitize channels (empty) warning: %v", err)
	}

	// If stored as unquoted 'normal' or other invalid JSON, coerce to default using try/catch pattern
	// MySQL doesn't have try-parse easily; target common bad cases: 'normal', 'popup', 'line'
	if err := db.Exec(`UPDATE notifications SET channels='["normal"]' WHERE channels IN ('normal','popup','line')`).Error; err != nil {
		log.Printf("sanitize channels (bare values) warning: %v", err)
	}
	return nil
}

// ensureUsersEmailNullable makes sure the users.email column is nullable with default NULL.
// This avoids MySQL unique index collisions on empty strings when email is not provided.
func ensureUsersEmailNullable(db *gorm.DB) error {
	// Inspect column definition; if NOT NULL or default '' then alter.
	type colInfo struct {
		IS_NULLABLE    string
		COLUMN_DEFAULT *string
	}
	var info colInfo
	// Query information_schema for current column definition
	// Using DATABASE() for current schema
	row := db.Raw(`SELECT IS_NULLABLE, COLUMN_DEFAULT FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'email'`).Row()
	if err := row.Scan(&info.IS_NULLABLE, &info.COLUMN_DEFAULT); err != nil {
		return err
	}
	// If already NULLable and default is NULL, nothing to do
	if strings.EqualFold(info.IS_NULLABLE, "YES") && (info.COLUMN_DEFAULT == nil) {
		return nil
	}
	// Attempt to alter column
	return db.Exec("ALTER TABLE `users` MODIFY COLUMN `email` varchar(255) NULL DEFAULT NULL").Error
}

// pruneExtraColumns drops columns that exist in the DB table but not in the model definition.
// It uses gorm schema parser to get model fields, then compares with DB column list.
// WARNING: This is destructive. Gate with PRUNE_COLUMNS=true in env and use with backups.
func pruneExtraColumns(db *gorm.DB, model interface{}) error {
	// Resolve model's table name and field names
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return fmt.Errorf("parse model: %w", err)
	}
	tableName := stmt.Schema.Table

	// Columns in model
	modelCols := map[string]struct{}{}
	for _, f := range stmt.Schema.DBNames {
		modelCols[f] = struct{}{}
	}

	// Columns in database
	cols, err := db.Migrator().ColumnTypes(model)
	if err != nil {
		return fmt.Errorf("list columns: %w", err)
	}

	// Verbose logging for debug: list model columns and DB columns
	log.Printf("Prune debug for table %s: model columns=%v", tableName, keysFromMap(modelCols))
	dbColNames := []string{}

	// Find extras
	for _, c := range cols {
		name := c.Name()
		dbColNames = append(dbColNames, name)
		if _, ok := modelCols[name]; !ok {
			// Skip GORM's soft delete column if model embeds gorm.DeletedAt (DB name may vary)
			// We already included DeletedAt via BaseModel. If not present in DBNames, this means model truly lacks it.
			log.Printf("Pruning extra column %s.%s", tableName, name)

			// Before dropping the column, drop any foreign key constraints that reference it.
			// MySQL lists constraints in information_schema.KEY_COLUMN_USAGE where REFERENCED_TABLE_NAME is not null.
			rows, rerr := db.Raw("SELECT CONSTRAINT_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ? AND REFERENCED_TABLE_NAME IS NOT NULL", tableName, name).Rows()
			if rerr != nil {
				log.Printf("Failed to query foreign keys for %s.%s: %v", tableName, name, rerr)
			} else {
				defer rows.Close()
				for rows.Next() {
					var constraintName string
					if serr := rows.Scan(&constraintName); serr != nil {
						log.Printf("Failed to scan constraint name for %s.%s: %v", tableName, name, serr)
						continue
					}
					log.Printf("Found FK constraint %s on %s.%s, attempting to drop it", constraintName, tableName, name)
					if execErr := db.Exec(fmt.Sprintf("ALTER TABLE `%s` DROP FOREIGN KEY `%s`", tableName, constraintName)).Error; execErr != nil {
						log.Printf("Failed to drop FK %s on %s: %v", constraintName, tableName, execErr)
					} else {
						log.Printf("Dropped FK %s on %s successfully", constraintName, tableName)
					}
				}
			}

			if err := db.Migrator().DropColumn(model, name); err != nil {
				log.Printf("Failed to drop column %s.%s: %v", tableName, name, err)
			} else {
				log.Printf("Dropped column %s.%s successfully", tableName, name)
			}
		}
	}
	log.Printf("Prune debug for table %s: db columns=%v", tableName, dbColNames)
	return nil
}

// keysFromMap returns sorted keys of a string->struct{} map (unsorted is fine here)
func keysFromMap(m map[string]struct{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
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
