package service

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// StandaloneAccountImportConfig is the runtime configuration for the
// password-protected account import entry that intentionally does not use admin JWT.
type StandaloneAccountImportConfig struct {
	Enabled            bool
	PasswordHash       string
	PasswordConfigured bool
}

// GetStandaloneAccountImportConfig returns the import entry toggle and password hash.
func (s *SettingService) GetStandaloneAccountImportConfig(ctx context.Context) (*StandaloneAccountImportConfig, error) {
	values, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyStandaloneAccountImportEnabled,
		SettingKeyStandaloneAccountImportPasswordHash,
	})
	if err != nil {
		return nil, fmt.Errorf("get standalone account import config: %w", err)
	}

	passwordHash := strings.TrimSpace(values[SettingKeyStandaloneAccountImportPasswordHash])
	return &StandaloneAccountImportConfig{
		Enabled:            values[SettingKeyStandaloneAccountImportEnabled] == "true",
		PasswordHash:       passwordHash,
		PasswordConfigured: passwordHash != "",
	}, nil
}

// HashStandaloneAccountImportPassword hashes the standalone import password before persistence.
func HashStandaloneAccountImportPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", nil
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash standalone account import password: %w", err)
	}
	return string(hashed), nil
}

// CheckStandaloneAccountImportPassword verifies a plaintext password against the configured hash.
func CheckStandaloneAccountImportPassword(passwordHash, password string) bool {
	passwordHash = strings.TrimSpace(passwordHash)
	password = strings.TrimSpace(password)
	if passwordHash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil
}
