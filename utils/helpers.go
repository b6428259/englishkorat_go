package utils

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// CheckPassword compares a password with its hash
func CheckPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// GenerateRandomString generates a random string of specified length
func GenerateRandomString(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// IsValidRole checks if a role is valid
func IsValidRole(role string) bool {
	validRoles := []string{"owner", "admin", "teacher", "student"}
	for _, validRole := range validRoles {
		if role == validRole {
			return true
		}
	}
	return false
}

// IsValidStatus checks if a status is valid
func IsValidStatus(status string) bool {
	validStatuses := []string{"active", "inactive", "suspended"}
	for _, validStatus := range validStatuses {
		if status == validStatus {
			return true
		}
	}
	return false
}

// IsValidFileExtension checks if file extension is allowed
func IsValidFileExtension(filename string, allowedExtensions []string) bool {
	if filename == "" {
		return false
	}

	parts := strings.Split(filename, ".")
	if len(parts) < 2 {
		return false
	}

	ext := strings.ToLower(parts[len(parts)-1])

	for _, allowedExt := range allowedExtensions {
		if ext == strings.ToLower(allowedExt) {
			return true
		}
	}
	return false
}

// SanitizeString removes dangerous characters from string
func SanitizeString(input string) string {
	// Remove null bytes and control characters
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	return input
}
