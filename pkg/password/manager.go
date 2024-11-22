package password

import (
	"errors"
	"github.com/cybericebox/lib/pkg/appError"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type (
	Manager struct {
		cost                     int
		passwordComplexityConfig PasswordComplexityConfig
	}

	PasswordComplexityConfig struct {
		MinLength            int
		MaxLength            int
		MinCapitalLetters    int
		MinSmallLetters      int
		MinDigits            int
		MinSpecialCharacters int
	}

	Dependencies struct {
		Cost               int
		PasswordComplexity PasswordComplexityConfig
	}
)

func NewHashManager(deps Dependencies) *Manager {
	return &Manager{cost: deps.Cost, passwordComplexityConfig: deps.PasswordComplexity}
}

func (m *Manager) Hash(plaintextPassword string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), m.cost)
	if err != nil {
		return "", appError.ErrPassword.WithError(err).WithMessage("Failed to hash password").Err()
	}

	return string(hashedPassword), nil
}

func (m *Manager) Matches(plaintextPassword, hashedPassword string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plaintextPassword))
	if err != nil {
		switch {
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			return false, nil
		case strings.Contains(err.Error(), "bcrypt:"):
			return false, appError.ErrPasswordInvalidHashPassword.WithError(err).Err()
		default:
			return false, appError.ErrPassword.WithError(err).WithMessage("Failed to compare password").Err()
		}
	}

	return true, nil
}

func (m *Manager) CheckPasswordComplexity(password string) error {
	if len(password) < m.passwordComplexityConfig.MinLength {
		return appError.ErrorInvalidPasswordComplexity.WithDetail("minLength", m.passwordComplexityConfig.MinLength).Err()
	}

	if len(password) > m.passwordComplexityConfig.MaxLength {
		return appError.ErrorInvalidPasswordComplexity.WithDetail("maxLength", m.passwordComplexityConfig.MaxLength).Err()
	}

	if len(strings.FieldsFunc(password, func(r rune) bool {
		return r >= '0' && r <= '9'
	})) < m.passwordComplexityConfig.MinDigits {
		return appError.ErrorInvalidPasswordComplexity.WithDetail("minDigits", m.passwordComplexityConfig.MinDigits).Err()
	}

	if len(strings.FieldsFunc(password, func(r rune) bool {
		return r >= 'A' && r <= 'Z'
	})) < m.passwordComplexityConfig.MinCapitalLetters {
		return appError.ErrorInvalidPasswordComplexity.WithDetail("minCapitalLetters", m.passwordComplexityConfig.MinCapitalLetters).Err()
	}

	if len(strings.FieldsFunc(password, func(r rune) bool {
		return r >= 'a' && r <= 'z'
	})) < m.passwordComplexityConfig.MinSmallLetters {
		return appError.ErrorInvalidPasswordComplexity.WithDetail("minSmallLetters", m.passwordComplexityConfig.MinSmallLetters).Err()
	}

	if len(strings.FieldsFunc(password, func(r rune) bool {
		return r >= 33 && r <= 47
	})) < m.passwordComplexityConfig.MinSpecialCharacters {
		return appError.ErrorInvalidPasswordComplexity.WithDetail("minSpecialCharacters", m.passwordComplexityConfig.MinSpecialCharacters).Err()
	}

	return nil
}
