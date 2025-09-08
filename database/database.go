package database

import (
	"englishkorat_go/config"
	"englishkorat_go/models"
	"fmt"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Connect initializes the database connection
func Connect() {
	var err error
	dsn := config.AppConfig.GetDSN()

	// Configure GORM logger based on environment
	var gormLogger logger.Interface
	if config.AppConfig.AppEnv == "development" {
		gormLogger = logger.Default.LogMode(logger.Info)
	} else {
		gormLogger = logger.Default.LogMode(logger.Silent)
	}

	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormLogger,
		// Enable safe delete (require WHERE clause)
		DisableForeignKeyConstraintWhenMigrating: false,
	})

	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	log.Println("Database connected successfully")

	// Configure connection pool
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatal("Failed to get database instance:", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	// Auto migrate
	AutoMigrate()
}

// AutoMigrate performs automatic database migration
func AutoMigrate() {
	err := DB.AutoMigrate(
		&models.Branch{},
		&models.User{},
		&models.Student{},
		&models.Teacher{},
		&models.Room{},
		&models.Course{},
		&models.ActivityLog{},
		&models.Notification{},
	)

	if err != nil {
		log.Fatal("Auto migration failed:", err)
	}

	log.Println("Database migration completed successfully")
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