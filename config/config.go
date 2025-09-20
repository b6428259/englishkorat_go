package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
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
	MaxFileSize       int64
	AllowedExtensions string

	// Logging
	LogLevel string
	LogFile  string

	// Feature Toggles
	UseRedisNotifications bool
	SkipMigrate          bool
	PruneColumns         bool
}

func (c *Config) GetDSN() string {
	return c.DBUser + ":" + c.DBPassword + "@tcp(" + c.DBHost + ":" + c.DBPort + ")/" + c.DBName + "?charset=utf8mb4&parseTime=True&loc=Local"
}

var AppConfig *Config

func LoadConfig() {
	useSSM := getEnv("USE_SSM", "false") == "true"

	var (
		ssmClient *ssm.SSM
		paramMap  map[string]string
	)

	// Stage & base path for SSM (allows multi-env without code changes)
	basePath := getEnv("SSM_BASE_PATH", "/englishkorat")
	stage := getEnv("STAGE", getEnv("APP_ENV", "production"))
	basePath = strings.TrimRight(basePath, "/")
	prefix := basePath + "/" + stage

	if useSSM {
		sess, err := session.NewSession(&aws.Config{Region: aws.String(getEnv("AWS_REGION", "ap-southeast-1"))})
		if err != nil {
			log.Fatal("Failed to create AWS session:", err)
		}
		ssmClient = ssm.New(sess)
		log.Printf("Using AWS SSM Parameter Store (prefix=%s)", prefix)
		paramMap = fetchSSMParameters(ssmClient, prefix)
	} else {
		if err := godotenv.Load(); err != nil {
			log.Println("Warning: .env file not found, using environment variables")
		}
	}

	// Helper accessor respecting map / env fallback
	getVal := func(key, def string) string {
		if useSSM {
			// map key stored uppercase
			uk := strings.ToUpper(key)
			if v, ok := paramMap[uk]; ok && v != "" {
				return v
			}
		}
		return getEnv(strings.ToUpper(key), def)
	}

	// Parse JWT_EXPIRES_IN with shorthand support
	jwtExpiresStr := getVal("JWT_EXPIRES_IN", "24h")
	jwtExpires, err := time.ParseDuration(jwtExpiresStr)
	if err != nil {
		s := strings.TrimSpace(strings.ToLower(jwtExpiresStr))
		if len(s) > 1 {
			unit := s[len(s)-1]
			numStr := s[:len(s)-1]
			if n, err2 := strconv.Atoi(numStr); err2 == nil {
				switch unit {
				case 'd':
					jwtExpires = time.Duration(n) * 24 * time.Hour
					err = nil
				case 'w':
					jwtExpires = time.Duration(n*7) * 24 * time.Hour
					err = nil
				}
			}
		}
		if err != nil {
			log.Fatal("Invalid JWT_EXPIRES_IN format:", err)
		}
	}

	maxFileSizeStr := getVal("MAX_FILE_SIZE", "10485760")
	maxFileSize, err := strconv.ParseInt(maxFileSizeStr, 10, 64)
	if err != nil {
		log.Fatal("Invalid MAX_FILE_SIZE format:", err)
	}

	AppConfig = &Config{
		DBHost:     getVal("DB_HOST", "localhost"),
		DBPort:     getVal("DB_PORT", "3306"),
		DBUser:     getVal("DB_USER", "root"),
		DBPassword: getVal("DB_PASSWORD", ""),
		DBName:     getVal("DB_NAME", "englishkorat_go"),

		RedisHost:     getVal("REDIS_HOST", "localhost"),
		RedisPort:     getVal("REDIS_PORT", "6379"),
		RedisPassword: getVal("REDIS_PASSWORD", ""),

		JWTSecret:    getVal("JWT_SECRET", "your_super_secret_jwt_key"),
		JWTExpiresIn: jwtExpires,

		AWSRegion:          getVal("AWS_REGION", "ap-southeast-1"),
		AWSAccessKeyID:     getVal("AWS_ACCESS_KEY_ID", ""),
		AWSSecretAccessKey: getVal("AWS_SECRET_ACCESS_KEY", ""),
		S3BucketName:       getVal("S3_BUCKET_NAME", "englishkorat-storage"),

		Port:   getVal("PORT", "3000"),
		AppEnv: getVal("APP_ENV", "development"),

		MaxFileSize:       maxFileSize,
		AllowedExtensions: getVal("ALLOWED_EXTENSIONS", "jpg,jpeg,png,webp,gif"),

		LogLevel: getVal("LOG_LEVEL", "info"),
		LogFile:  getVal("LOG_FILE", "logs/app.log"),

		UseRedisNotifications: strings.ToLower(getVal("USE_REDIS_NOTIFICATIONS", "false")) == "true",
		SkipMigrate:          strings.ToLower(getVal("SKIP_MIGRATE", "false")) == "true",
		PruneColumns:         strings.ToLower(getVal("PRUNE_COLUMNS", "true")) == "true",
	}

	validateConfig(AppConfig, useSSM)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getConfigValue retrieves configuration value from SSM or environment variables
// Backwards compatibility wrapper if other code calls it (none currently)
func getConfigValue(_ *ssm.SSM, _ bool, key, defaultValue string) string { //nolint:revive
	return getEnv(key, defaultValue)
}

// fetchSSMParameters reads all parameters under prefix (non-recursive expected) and returns map with UPPERCASE keys.
func fetchSSMParameters(client *ssm.SSM, prefix string) map[string]string {
	out := make(map[string]string)
	next := aws.String("")
	for {
		in := &ssm.GetParametersByPathInput{
			Path:           aws.String(prefix),
			WithDecryption: aws.Bool(true),
			Recursive:      aws.Bool(true),
		}
		if *next != "" {
			in.NextToken = next
		}
		resp, err := client.GetParametersByPath(in)
		if err != nil {
			log.Printf("Warning: unable to fetch SSM parameters for prefix %s: %v", prefix, err)
			break
		}
		for _, p := range resp.Parameters {
			if p.Name == nil || p.Value == nil {
				continue
			}
			name := *p.Name
			// last segment after '/'
			idx := strings.LastIndex(name, "/")
			key := name
			if idx >= 0 {
				key = name[idx+1:]
			}
			if key == "" {
				continue
			}
			out[strings.ToUpper(key)] = *p.Value
		}
		if resp.NextToken == nil || *resp.NextToken == "" {
			break
		}
		next = resp.NextToken
	}
	return out
}

func validateConfig(c *Config, usedSSM bool) {
	// Only enforce stricter rules in production
	if strings.ToLower(c.AppEnv) != "production" {
		return
	}
	// Required secrets
	required := map[string]string{
		"DB_PASSWORD": c.DBPassword,
		"JWT_SECRET":  c.JWTSecret,
	}
	for k, v := range required {
		if strings.TrimSpace(v) == "" {
			log.Fatalf("Missing required secret %s in production (SSM=%v)", k, usedSSM)
		}
	}
	if len(c.JWTSecret) < 16 {
		log.Fatal("JWT_SECRET too short (min 16 chars)")
	}
}
