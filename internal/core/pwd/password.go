package pwd

import (
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

const MinPasswordLength = 5

func ValidatePassword(s string) error {
	var (
		hasMinLen  = false
		hasUpper   = false
		hasLower   = false
		hasNumber  = false
		hasSpecial = false
	)
	if len(s) >= MinPasswordLength {
		hasMinLen = true
	}
	for _, char := range s {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}
	if !hasMinLen {
		return passwordError(ErrPasswordTooShort)
	}
	if !hasLower {
		return passwordError(ErrPasswordNoLower)
	}
	if !hasUpper {
		return passwordError(ErrPasswordNoUpper)
	}
	if !hasNumber {
		return passwordError(ErrPasswordNoNumber)
	}
	if !hasSpecial {
		return passwordError(ErrPasswordNoSpecial)
	}
	return nil
}

func ValidateAndHashPassword(password string) (string, error) {
	err := ValidatePassword(password)
	if err != nil {
		return "", err
	}
	return HashPassword(password)
}
