package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// Redis
	RedisHost     string
	RedisPort     string
	RedisPassword string

	// JWT
	JWTSecret    string
	JWTExpiresIn time.Duration

	// AWS S3
	AWSRegion          string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	S3BucketName       string

	// Server
	Port   string
	AppEnv string

	// File Upload
	MaxFileSize        int64
	AllowedExtensions  string

	// Logging
	LogLevel string
	LogFile  string
}

var AppConfig *Config

func LoadConfig() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Parse JWT expires duration
	jwtExpiresStr := getEnv("JWT_EXPIRES_IN", "24h")
	jwtExpires, err := time.ParseDuration(jwtExpiresStr)
	if err != nil {
		log.Fatal("Invalid JWT_EXPIRES_IN format:", err)
	}

	// Parse max file size
	maxFileSizeStr := getEnv("MAX_FILE_SIZE", "10485760") // 10MB default
	maxFileSize, err := strconv.ParseInt(maxFileSizeStr, 10, 64)
	if err != nil {
		log.Fatal("Invalid MAX_FILE_SIZE format:", err)
	}

	AppConfig = &Config{
		// Database
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "englishkorat_go"),

		// Redis
		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		// JWT
		JWTSecret:    getEnv("JWT_SECRET", "your_super_secret_jwt_key"),
		JWTExpiresIn: jwtExpires,

		// AWS S3
		AWSRegion:          getEnv("AWS_REGION", "ap-southeast-1"),
		AWSAccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", ""),
		AWSSecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
		S3BucketName:       getEnv("S3_BUCKET_NAME", "englishkorat-storage"),

		// Server
		Port:   getEnv("PORT", "3000"),
		AppEnv: getEnv("APP_ENV", "development"),

		// File Upload
		MaxFileSize:       maxFileSize,
		AllowedExtensions: getEnv("ALLOWED_EXTENSIONS", "jpg,jpeg,png,webp,gif"),

		// Logging
		LogLevel: getEnv("LOG_LEVEL", "info"),
		LogFile:  getEnv("LOG_FILE", "logs/app.log"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetDSN returns database connection string
func (c *Config) GetDSN() string {
	return c.DBUser + ":" + c.DBPassword + "@tcp(" + c.DBHost + ":" + c.DBPort + ")/" + c.DBName + "?charset=utf8mb4&parseTime=True&loc=Local"
}

// GetRedisAddr returns Redis connection string
func (c *Config) GetRedisAddr() string {
	return c.RedisHost + ":" + c.RedisPort
}