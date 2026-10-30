package config

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDatabaseURL = "postgresql://inventory:inventory@localhost:5433/inventory_desktop"
	defaultAccessMin   = 30
	defaultRefreshDays = 7
	defaultBcryptCost  = 12
)

type Config struct {
	DBMode             string
	DatabaseURL        string
	SQLitePath         string
	SecretKey          string
	AccessTokenMinutes int
	RefreshTokenDays   int
	BcryptRounds       int
	AutoMigrate        bool
	DevMode            bool
	LogLevel           slog.Level
}

func (c Config) AccessTokenTTL() time.Duration {
	return time.Duration(c.AccessTokenMinutes) * time.Minute
}

func (c Config) RefreshTokenTTL() time.Duration {
	return time.Duration(c.RefreshTokenDays) * 24 * time.Hour
}

func (c Config) Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: c.LogLevel}))
}

func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	fileValues := map[string]string{}
	if err := readConfigFile(path, fileValues); err != nil {
		return nil, err
	}

	lookup := func(keys ...string) string {
		for _, key := range keys {
			if value, ok := os.LookupEnv(key); ok && value != "" {
				return value
			}
		}
		for _, key := range keys {
			if value := fileValues[key]; value != "" {
				return value
			}
		}
		return ""
	}

	secret, provided := lookupRaw(fileValues, "SECRET_KEY")
	if !provided {
		secret, err = generateSecret()
		if err != nil {
			return nil, fmt.Errorf("generate secret key: %w", err)
		}
		if err := persistSecret(path, secret); err != nil {
			return nil, fmt.Errorf("persist secret key: %w", err)
		}
	} else {
		secret = strings.TrimSpace(secret)
		if err := validateSecret(secret); err != nil {
			return nil, err
		}
	}

	accessMinutes, err := intValue(lookup("ACCESS_TOKEN_MINUTES", "ACCESS_TOKEN_EXPIRE_MINUTES"), defaultAccessMin)
	if err != nil {
		return nil, fmt.Errorf("ACCESS_TOKEN_MINUTES: %w", err)
	}
	refreshDays, err := intValue(lookup("REFRESH_TOKEN_DAYS", "REFRESH_TOKEN_EXPIRE_DAYS"), defaultRefreshDays)
	if err != nil {
		return nil, fmt.Errorf("REFRESH_TOKEN_DAYS: %w", err)
	}
	bcryptRounds, err := intValue(lookup("BCRYPT_ROUNDS"), defaultBcryptCost)
	if err != nil {
		return nil, fmt.Errorf("BCRYPT_ROUNDS: %w", err)
	}
	if accessMinutes <= 0 {
		return nil, errors.New("ACCESS_TOKEN_MINUTES must be positive")
	}
	if refreshDays <= 0 {
		return nil, errors.New("REFRESH_TOKEN_DAYS must be positive")
	}
	if bcryptRounds < 4 || bcryptRounds > 31 {
		return nil, errors.New("BCRYPT_ROUNDS must be between 4 and 31")
	}

	autoMigrate, err := boolValue(lookup("AUTO_MIGRATE"), true)
	if err != nil {
		return nil, fmt.Errorf("AUTO_MIGRATE: %w", err)
	}

	allowDevOrigins, err := boolValue(lookup("ALLOW_DEV_ORIGINS"), false)
	if err != nil {
		return nil, fmt.Errorf("ALLOW_DEV_ORIGINS: %w", err)
	}
	devMode := allowDevOrigins || strings.EqualFold(strings.TrimSpace(lookup("APP_ENV")), "development")

	level := slog.LevelInfo
	if raw := lookup("LOG_LEVEL"); raw != "" {
		if err := level.UnmarshalText([]byte(raw)); err != nil {
			return nil, fmt.Errorf("LOG_LEVEL: %w", err)
		}
	}

	databaseURL := lookup("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = defaultDatabaseURL
	}

	dbMode := strings.ToLower(strings.TrimSpace(lookup("DB_MODE")))
	if dbMode == "" {
		dbMode = "auto"
	}
	switch dbMode {
	case "auto", "postgres", "sqlite":
	default:
		return nil, fmt.Errorf("DB_MODE must be auto, postgres, or sqlite, got %q", dbMode)
	}

	sqlitePath := lookup("SQLITE_PATH")
	if sqlitePath == "" {
		sqlitePath, err = defaultSQLitePath()
		if err != nil {
			return nil, err
		}
	}

	return &Config{
		DBMode:             dbMode,
		DatabaseURL:        databaseURL,
		SQLitePath:         sqlitePath,
		SecretKey:          secret,
		AccessTokenMinutes: accessMinutes,
		RefreshTokenDays:   refreshDays,
		BcryptRounds:       bcryptRounds,
		AutoMigrate:        autoMigrate,
		DevMode:            devMode,
		LogLevel:           level,
	}, nil
}

func lookupRaw(values map[string]string, key string) (string, bool) {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value, true
	}
	if value := strings.TrimSpace(values[key]); value != "" {
		return value, true
	}
	return "", false
}

func validateSecret(secret string) error {
	if secret == "" {
		return errors.New("SECRET_KEY must not be empty")
	}
	switch strings.ToLower(secret) {
	case "replace-with-a-random-32-byte-secret", "change-me", "changeme", "your-secret-key", "secret":
		return errors.New("SECRET_KEY is a known placeholder; generate a random secret")
	}
	if len(secret) < 32 {
		return errors.New("SECRET_KEY must be at least 32 bytes")
	}
	return nil
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(dir, "inventory-desktop", "config.env"), nil
}

func defaultSQLitePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(dir, "inventory-desktop", "data", "inventory.sqlite"), nil
}

func readConfigFile(path string, values map[string]string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect config file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("config file must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return errors.New("config file must be a regular file")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if dirInfo, err := os.Stat(dir); err == nil && dirInfo.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("secure config directory: %w", err)
		}
	}
	if info.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(path, 0o600)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open config file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	return nil
}

func persistSecret(path, secret string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(file, "SECRET_KEY=%s\n", secret); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func generateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func intValue(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func boolValue(raw string, fallback bool) (bool, error) {
	if raw == "" {
		return fallback, nil
	}
	return strconv.ParseBool(raw)
}
