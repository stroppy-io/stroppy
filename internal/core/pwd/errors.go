package pwd

import (
	"errors"
)

var (
	ErrPasswordTooShort  = errors.New("password too short")
	ErrPasswordNoLower   = errors.New("password has no lower case letter")
	ErrPasswordNoUpper   = errors.New("password has no upper case letter")
	ErrPasswordNoNumber  = errors.New("password has no number")
	ErrPasswordNoSpecial = errors.New("password has no special character")

	ErrInvalidPassword = errors.New("invalid password")
)

func passwordError(err error) error {
	return errors.Join(ErrInvalidPassword, err)
}
