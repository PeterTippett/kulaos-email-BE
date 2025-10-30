package validation

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrInvalidEmail     = fmt.Errorf("invalid email address")
	ErrInvalidUUID      = fmt.Errorf("invalid UUID format")
	ErrInvalidPhoneE164 = fmt.Errorf("invalid E.164 phone number")
)

// ValidateEmail validates email address format using net/mail and additional checks
func ValidateEmail(email string) error {
	// Use net/mail for RFC 5322 compliance
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return ErrInvalidEmail
	}

	// Additional checks for common issues
	emailAddr := addr.Address

	// Check for consecutive dots
	if strings.Contains(emailAddr, "..") {
		return ErrInvalidEmail
	}

	// Split into local and domain parts
	parts := strings.Split(emailAddr, "@")
	if len(parts) != 2 {
		return ErrInvalidEmail
	}

	localPart, domain := parts[0], parts[1]

	// RFC 5321 length limits
	if len(localPart) > 64 || len(domain) > 255 || len(emailAddr) > 320 {
		return ErrInvalidEmail
	}

	// Local part shouldn't start or end with dot
	if strings.HasPrefix(localPart, ".") || strings.HasSuffix(localPart, ".") {
		return ErrInvalidEmail
	}

	// Domain shouldn't start or end with hyphen
	if strings.HasPrefix(domain, "-") || strings.HasSuffix(domain, "-") {
		return ErrInvalidEmail
	}

	// Domain must have at least one dot
	if !strings.Contains(domain, ".") {
		return ErrInvalidEmail
	}

	return nil
}

// ValidateUUID validates UUID format
func ValidateUUID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return ErrInvalidUUID
	}
	return nil
}

// ValidateE164Phone validates E.164 phone number format
// E.164 format: +[country code][subscriber number]
// Example: +14155552671
func ValidateE164Phone(phone string) error {
	// E.164 regex: + followed by 1-3 digit country code and up to 15 total digits
	e164Regex := regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
	if !e164Regex.MatchString(phone) {
		return ErrInvalidPhoneE164
	}
	return nil
}

// ValidateAccountID validates account ID format (UUID)
func ValidateAccountID(accountID string) error {
	return ValidateUUID(accountID)
}

// ValidatePagination validates pagination parameters
func ValidatePagination(page, perPage, maxPerPage, maxPage int) (int, int, error) {
	// Validate and normalize page
	if page < 1 {
		page = 1
	}
	if page > maxPage {
		return 0, 0, fmt.Errorf("page number exceeds maximum allowed (%d)", maxPage)
	}

	// Validate and normalize perPage
	if perPage < 1 {
		perPage = 20 // default
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	return page, perPage, nil
}
