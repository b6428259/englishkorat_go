package services

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"englishkorat_go/database"
	"englishkorat_go/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// LogArchiveService handles flushing cached logs and archiving old logs to S3
type LogArchiveService struct {
	redisClient *redis.Client
	awsConfig   aws.Config
}

// ArchivedLog is the exported representation stored inside archives
type ArchivedLog struct {
	ID         uint           `json:"id"`
	UserID     uint           `json:"user_id"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	ResourceID uint           `json:"resource_id"`
	Details    map[string]any `json:"details"`
	IPAddress  string         `json:"ip_address"`
	UserAgent  string         `json:"user_agent"`
	CreatedAt  *time.Time     `json:"created_at"`
	Username   string         `json:"username,omitempty"`
	UserRole   string         `json:"user_role,omitempty"`
}

// NewLogArchiveService creates a new service instance
func NewLogArchiveService() *LogArchiveService {
	cfg, err := awscfg.LoadDefaultConfig(context.Background(), awscfg.WithRegion(os.Getenv("AWS_REGION")))
	if err != nil {
		logrus.WithError(err).Warn("Failed to load AWS config; S3 operations will fail until configured")
	}

	return &LogArchiveService{
		redisClient: database.GetRedisClient(),
		awsConfig:   cfg,
	}
}

// FlushCachedLogsToDatabase moves logs from Redis cache to database
func (las *LogArchiveService) FlushCachedLogsToDatabase() error {
	if las.redisClient == nil {
		return fmt.Errorf("redis client not available")
	}

	ctx := context.Background()
	cutoffTime := time.Now().Add(-24 * time.Hour)

	// Get all expired logs from the sorted set
	expiredLogs, err := las.redisClient.ZRangeByScore(ctx, "logs:queue", &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", cutoffTime.Unix()),
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to get expired logs: %v", err)
	}

	logrus.Infof("Processing %d expired cached logs", len(expiredLogs))

	var processedCount int
	var errorCount int

	for _, logKey := range expiredLogs {
		// Get log data from cache
		logData, err := las.redisClient.Get(ctx, logKey).Result()
		if err != nil {
			if err != redis.Nil {
				logrus.WithError(err).Errorf("Failed to get log data for key: %s", logKey)
				errorCount++
			}
			continue
		}

		// Parse log data
		var activityLog models.ActivityLog
		if err := json.Unmarshal([]byte(logData), &activityLog); err != nil {
			logrus.WithError(err).Errorf("Failed to unmarshal log data for key: %s", logKey)
			errorCount++
			continue
		}

		// Save to database
		if err := database.DB.Create(&activityLog).Error; err != nil {
			logrus.WithError(err).Errorf("Failed to save log to database: %v", activityLog)
			errorCount++
			continue
		}

		// Remove from cache and queue
		pipeline := las.redisClient.Pipeline()
		pipeline.Del(ctx, logKey)
		pipeline.ZRem(ctx, "logs:queue", logKey)
		_, err = pipeline.Exec(ctx)

		if err != nil {
			logrus.WithError(err).Errorf("Failed to remove log from cache: %s", logKey)
		}

		processedCount++
	}

	logrus.Infof("Flushed %d logs to database, %d errors", processedCount, errorCount)
	return nil
}

// ArchiveOldLogs archives logs older than specified days to S3 and removes from database
func (las *LogArchiveService) ArchiveOldLogs(daysOld int) error {
	if daysOld < 7 {
		return fmt.Errorf("minimum archive age is 7 days for safety")
	}

	cutoffDate := time.Now().AddDate(0, 0, -daysOld)

	// Get logs to archive in batches
	batchSize := 1000
	var allLogs []ArchivedLog

	for offset := 0; ; offset += batchSize {
		var logs []models.ActivityLog

		err := database.DB.
			Preload("User").
			Where("created_at < ?", cutoffDate).
			Limit(batchSize).
			Offset(offset).
			Find(&logs).Error

		if err != nil {
			return fmt.Errorf("failed to fetch logs for archiving: %v", err)
		}

		if len(logs) == 0 {
			break
		}

		// Convert to archived format
		for _, log := range logs {
			archivedLog := ArchivedLog{
				ID:         log.ID,
				UserID:     log.UserID,
				Action:     log.Action,
				Resource:   log.Resource,
				ResourceID: log.ResourceID,
				IPAddress:  log.IPAddress,
				UserAgent:  log.UserAgent,
				CreatedAt:  log.CreatedAt,
			}

			// Parse details
			if log.Details != nil && len(log.Details) > 0 {
				var details map[string]any
				if err := json.Unmarshal(log.Details, &details); err == nil {
					archivedLog.Details = details
				} else {
					var detailsLegacy map[string]interface{}
					if err2 := json.Unmarshal(log.Details, &detailsLegacy); err2 == nil {
						m := make(map[string]any, len(detailsLegacy))
						for k, v := range detailsLegacy {
							m[k] = v
						}
						archivedLog.Details = m
					}
				}
			}

			if log.User.ID > 0 {
				archivedLog.Username = log.User.Username
				archivedLog.UserRole = log.User.Role
			}

			allLogs = append(allLogs, archivedLog)
		}
	}

	if len(allLogs) == 0 {
		logrus.Info("No logs to archive")
		return nil
	}
	logrus.Infof("Archiving %d logs older than %s", len(allLogs), cutoffDate.Format("2006-01-02"))

	// Create ZIP archive
	archiveFileName := fmt.Sprintf("activity_logs_%s.zip", cutoffDate.Format("2006-01-02"))
	zipBuffer, err := las.createZipArchive(allLogs, archiveFileName)
	if err != nil {
		return fmt.Errorf("failed to create ZIP archive: %v", err)
	}

	// Upload to S3
	s3Key := fmt.Sprintf("logs/archived/%d/%02d/%s",
		cutoffDate.Year(),
		cutoffDate.Month(),
		archiveFileName)

	if err := las.uploadToS3(s3Key, zipBuffer); err != nil {
		return fmt.Errorf("failed to upload archive to S3: %v", err)
	}

	logrus.Infof("Successfully uploaded archive to S3: %s", s3Key)

	// Delete archived logs from database
	result := database.DB.Where("created_at < ?", cutoffDate).Delete(&models.ActivityLog{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete archived logs from database: %v", result.Error)
	}

	logrus.Infof("Deleted %d archived logs from database", result.RowsAffected)

	// Create archive metadata record
	archiveMetadata := models.LogArchive{
		FileName:    archiveFileName,
		S3Key:       s3Key,
		StartDate:   time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), // Earliest possible date
		EndDate:     cutoffDate,
		RecordCount: len(allLogs),
		FileSize:    int64(zipBuffer.Len()),
		Status:      "completed",
	}

	if err := database.DB.Create(&archiveMetadata).Error; err != nil {
		logrus.WithError(err).Error("Failed to save archive metadata")
	}

	return nil
}

// createZipArchive creates a ZIP file containing the logs as JSON
func (las *LogArchiveService) createZipArchive(logs []ArchivedLog, fileName string) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Create main logs JSON file
	logsFile, err := zipWriter.Create("activity_logs.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create logs file in ZIP: %v", err)
	}

	// Write logs as JSON
	encoder := json.NewEncoder(logsFile)
	encoder.SetIndent("", "  ")

	logData := map[string]any{
		"export_date":    time.Now().UTC(),
		"record_count":   len(logs),
		"format_version": "1.0",
		"logs":           logs,
	}
	if err := encoder.Encode(logData); err != nil {
		return nil, fmt.Errorf("failed to encode logs to JSON: %v", err)
	}

	// Create metadata file
	metadataFile, err := zipWriter.Create("metadata.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata file in ZIP: %v", err)
	}

	metadata := map[string]any{
		"file_name":    fileName,
		"created_at":   time.Now().UTC(),
		"record_count": len(logs),
		"date_range": map[string]any{
			"start": logs[0].CreatedAt,
			"end":   logs[len(logs)-1].CreatedAt,
		},
		"schema_version": "1.0",
		"description":    "English Korat Activity Logs Archive",
	}
	metadataEncoder := json.NewEncoder(metadataFile)
	if err := metadataEncoder.Encode(metadata); err != nil {
		return nil, fmt.Errorf("failed to encode metadata to JSON: %v", err)
	}

	// Create CSV file
	csvFile, err := zipWriter.Create("activity_logs.csv")
	if err != nil {
		return nil, fmt.Errorf("failed to create CSV file in ZIP: %v", err)
	}

	// Write CSV header
	csvHeader := "ID,User ID,Username,Role,Action,Resource,Resource ID,IP Address,User Agent,Created At,Details\n"
	csvFile.Write([]byte(csvHeader))

	// Write CSV data
	for _, log := range logs {
		details := ""
		if log.Details != nil {
			if detailsBytes, err := json.Marshal(log.Details); err == nil {
				details = strings.ReplaceAll(string(detailsBytes), "\"", "\"\"") // Escape quotes
			}
		}

		csvLine := fmt.Sprintf("%d,%d,%s,%s,%s,%s,%d,%s,%s,%s,\"%s\"\n",
			log.ID,
			log.UserID,
			log.Username,
			log.UserRole,
			log.Action,
			log.Resource,
			log.ResourceID,
			log.IPAddress,
			log.UserAgent,
			log.CreatedAt.Format("2006-01-02 15:04:05"),
			details,
		)
		csvFile.Write([]byte(csvLine))
	}

	// Close ZIP writer
	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close ZIP writer: %v", err)
	}

	return buf, nil
}

// uploadToS3 uploads data to S3 bucket
func (las *LogArchiveService) uploadToS3(key string, data *bytes.Buffer) error {
	if las.awsConfig.Region == "" {
		return fmt.Errorf("AWS not configured")
	}

	s3Client := s3.NewFromConfig(las.awsConfig)
	bucketName := os.Getenv("S3_BUCKET_NAME")

	_, err := s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &bucketName,
		Key:         &key,
		Body:        bytes.NewReader(data.Bytes()),
		ContentType: aws.String("application/zip"),
	})

	return err
}

// downloadFromS3 downloads a key from S3
func (las *LogArchiveService) downloadFromS3(key string) (io.ReadCloser, error) {
	if las.awsConfig.Region == "" {
		return nil, fmt.Errorf("AWS not configured")
	}

	s3Client := s3.NewFromConfig(las.awsConfig)
	bucketName := os.Getenv("S3_BUCKET_NAME")

	result, err := s3Client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &key,
	})

	if err != nil {
		return nil, err
	}

	return result.Body, nil
}

// GetArchivedLogs retrieves list of archived log files
func (las *LogArchiveService) GetArchivedLogs() ([]models.LogArchive, error) {
	var archives []models.LogArchive

	err := database.DB.
		Order("created_at DESC").
		Find(&archives).Error

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve archived logs: %v", err)
	}

	return archives, nil
}

// DownloadArchivedLogs downloads a specific archive from S3
func (las *LogArchiveService) DownloadArchivedLogs(archiveID uint) (io.ReadCloser, string, error) {
	var archive models.LogArchive

	err := database.DB.First(&archive, archiveID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, "", fmt.Errorf("archive not found")
		}
		return nil, "", fmt.Errorf("failed to retrieve archive: %v", err)
	}

	// Download from S3
	reader, err := las.downloadFromS3(archive.S3Key)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download archive from S3: %v", err)
	}

	return reader, archive.FileName, nil
}

// StartLogMaintenanceScheduler starts background goroutine to flush and archive logs periodically
func (las *LogArchiveService) StartLogMaintenanceScheduler() {
	go func() {
		// Run immediately once
		if err := las.FlushCachedLogsToDatabase(); err != nil {
			logrus.WithError(err).Warn("initial FlushCachedLogsToDatabase failed")
		}
		if err := las.ArchiveOldLogs(30); err != nil {
			logrus.WithError(err).Warn("initial ArchiveOldLogs failed")
		}

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := las.FlushCachedLogsToDatabase(); err != nil {
				logrus.WithError(err).Warn("periodic FlushCachedLogsToDatabase failed")
			}
			if err := las.ArchiveOldLogs(30); err != nil {
				logrus.WithError(err).Warn("periodic ArchiveOldLogs failed")
			}
		}
	}()
}
