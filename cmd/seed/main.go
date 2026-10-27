package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/mail"
	"os"
	"strings"
	"unicode/utf8"

	"inventory-desktop/internal/auth"
	"inventory-desktop/internal/bootstrap"
	"inventory-desktop/internal/config"
	"inventory-desktop/internal/domain"
)

func main() {
	email := flag.String("email", os.Getenv("SEED_EMAIL"), "user email")
	password := flag.String("password", os.Getenv("SEED_PASSWORD"), "user password")
	name := flag.String("name", os.Getenv("SEED_NAME"), "user full name")
	superuser := flag.Bool("superuser", false, "grant superuser access")
	flag.Parse()

	*email = strings.TrimSpace(*email)
	if *email == "" {
		fail("email is required (use -email or SEED_EMAIL)")
	}
	address, err := mail.ParseAddress(*email)
	if err != nil || address.Address != *email {
		fail("invalid email: " + *email)
	}
	if *password != "" {
		length := utf8.RuneCountInString(*password)
		if length < 8 || length > 128 {
			fail("password must be between 8 and 128 characters")
		}
		if len(*password) > 72 {
			fail("password must not exceed 72 bytes (bcrypt limit)")
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fail("load config: " + err.Error())
	}
	logger := cfg.Logger()

	ctx := context.Background()
	stores, err := bootstrap.Open(ctx, cfg, logger)
	if err != nil {
		logger.Error("database startup failed", "error", err)
		os.Exit(1)
	}
	defer stores.Closer()

	users := stores.Users
	existing, err := users.GetByEmail(ctx, *email)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		logger.Error("look up user failed", "error", err)
		os.Exit(1)
	}

	if errors.Is(err, domain.ErrNotFound) {
		if *password == "" {
			fail("password is required when creating a user (use -password or SEED_PASSWORD)")
		}
		hash, err := auth.HashPassword(*password, cfg.BcryptRounds)
		if err != nil {
			fail("hash password: " + err.Error())
		}
		user := &domain.User{
			FullName:       strings.TrimSpace(*name),
			Email:          *email,
			HashedPassword: hash,
			IsActive:       true,
			IsSuperuser:    *superuser,
		}
		if err := users.Create(ctx, user); err != nil {
			logger.Error("create user failed", "error", err)
			os.Exit(1)
		}
		fmt.Printf("created user %d (%s)\n", user.ID, user.Email)
		return
	}

	if *name != "" {
		existing.FullName = strings.TrimSpace(*name)
	}
	if *password != "" {
		hash, err := auth.HashPassword(*password, cfg.BcryptRounds)
		if err != nil {
			fail("hash password: " + err.Error())
		}
		existing.HashedPassword = hash
	}
	existing.IsActive = true
	superuserSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "superuser" {
			superuserSet = true
		}
	})
	if superuserSet {
		existing.IsSuperuser = *superuser
	}
	if err := users.Update(ctx, existing); err != nil {
		logger.Error("update user failed", "error", err)
		os.Exit(1)
	}
	fmt.Printf("updated user %d (%s)\n", existing.ID, existing.Email)
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
