package validation

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	e164Regex  = regexp.MustCompile(`^\+?[1-9]\d{7,14}$`)
)

// ValidateEmail validates email format
func ValidateEmail(email string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	return emailRegex.MatchString(email)
}

// ValidateE164Mobile validates mobile number in (loosely) E.164 format
func ValidateE164Mobile(mobile string) bool {
	mobile = strings.TrimSpace(mobile)
	return e164Regex.MatchString(mobile)
}

// ValidatePassword validates password strength
func ValidatePassword(password string) bool {
	// Minimum length
	if len(password) < 12 {
		return false
	}

	// No spaces
	if strings.Contains(password, " ") {
		return false
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasNumber = true
		case strings.ContainsRune("@$!%*?&_-#=+^~.", char):
			hasSpecial = true
		}
	}

	return hasUpper && hasLower && hasNumber && hasSpecial
}

// ValidateUsername validates username format
func ValidateUsername(username string) bool {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 30 {
		return false
	}
	// Allow alphanumeric, underscore, and hyphen
	matched, _ := regexp.MatchString("^[a-zA-Z0-9_-]+$", username)
	return matched
}

// SanitizeString removes potentially harmful characters
func SanitizeString(input string) string {
	// Basic sanitization
	input = strings.TrimSpace(input)
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")
	return input
}
