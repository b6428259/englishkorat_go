package storage

import (
	"bytes"
	"englishkorat_go/config"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/chai2010/webp"
	"github.com/google/uuid"
)

type StorageService struct {
	s3Client *s3.S3
	bucket   string
}

// NewStorageService creates a new storage service
func NewStorageService() (*StorageService, error) {
	// Create AWS session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(config.AppConfig.AWSRegion),
		Credentials: credentials.NewStaticCredentials(
			config.AppConfig.AWSAccessKeyID,
			config.AppConfig.AWSSecretAccessKey,
			"",
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}

	return &StorageService{
		s3Client: s3.New(sess),
		bucket:   config.AppConfig.S3BucketName,
	}, nil
}

// UploadFile uploads a file to S3 and converts to WebP if it's an image
func (s *StorageService) UploadFile(file *multipart.FileHeader, folder string, userID uint) (string, error) {
	// Open the uploaded file
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open file: %v", err)
	}
	defer src.Close()

	// Read file content
	fileBytes, err := io.ReadAll(src)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// Check if it's an image file
	isImage := s.isImageFile(file.Filename)
	var finalBytes []byte
	var finalExtension string

	if isImage {
		// Convert to WebP
		webpBytes, err := s.convertToWebP(fileBytes)
		if err != nil {
			return "", fmt.Errorf("failed to convert to WebP: %v", err)
		}
		finalBytes = webpBytes
		finalExtension = "webp"
	} else {
		finalBytes = fileBytes
		finalExtension = s.getFileExtension(file.Filename)
	}

	// Generate unique filename
	now := time.Now()
	randomID := uuid.New().String()[:16]
	filename := fmt.Sprintf("%s/%d/%d/%02d/%02d/%s.%s",
		folder,
		userID,
		now.Year(),
		now.Month(),
		now.Day(),
		randomID,
		finalExtension,
	)

	// Upload to S3
	_, err = s.s3Client.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(filename),
		Body:        bytes.NewReader(finalBytes),
		ContentType: aws.String(s.getContentType(finalExtension)),
		ACL:         aws.String("public-read"),
	})

	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %v", err)
	}

	// Return the public URL
	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s",
		s.bucket,
		config.AppConfig.AWSRegion,
		filename,
	)

	return url, nil
}

// DeleteFile deletes a file from S3
func (s *StorageService) DeleteFile(fileURL string) error {
	// Extract key from URL
	key := s.extractKeyFromURL(fileURL)
	if key == "" {
		return fmt.Errorf("invalid file URL")
	}

	_, err := s.s3Client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	return err
}

// isImageFile checks if the file is an image based on extension
func (s *StorageService) isImageFile(filename string) bool {
	ext := strings.ToLower(s.getFileExtension(filename))
	imageExtensions := []string{"jpg", "jpeg", "png", "gif", "bmp", "tiff"}
	
	for _, imgExt := range imageExtensions {
		if ext == imgExt {
			return true
		}
	}
	return false
}

// getFileExtension extracts file extension from filename
func (s *StorageService) getFileExtension(filename string) string {
	ext := filepath.Ext(filename)
	if len(ext) > 1 {
		return strings.ToLower(ext[1:]) // Remove the dot
	}
	return ""
}

// convertToWebP converts image to WebP format
func (s *StorageService) convertToWebP(imageBytes []byte) ([]byte, error) {
	// Decode the image
	img, format, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	// If already WebP, return as is
	if format == "webp" {
		return imageBytes, nil
	}

	// Convert to WebP
	var buf bytes.Buffer
	err = webp.Encode(&buf, img, &webp.Options{Quality: 80})
	if err != nil {
		return nil, fmt.Errorf("failed to encode to WebP: %v", err)
	}

	return buf.Bytes(), nil
}

// getContentType returns the MIME type for the file extension
func (s *StorageService) getContentType(extension string) string {
	switch strings.ToLower(extension) {
	case "webp":
		return "image/webp"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

// extractKeyFromURL extracts the S3 key from a full URL
func (s *StorageService) extractKeyFromURL(url string) string {
	// Example URL: https://bucket.s3.region.amazonaws.com/path/to/file.ext
	parts := strings.Split(url, ".amazonaws.com/")
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}