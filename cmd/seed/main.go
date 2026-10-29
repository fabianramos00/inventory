package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"inventory-desktop/internal/adapters/outbound/security"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/bootstrap"
	"inventory-desktop/internal/config"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
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
	superuserSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "superuser" {
			superuserSet = true
		}
	})

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

	passwords := security.NewPasswordHasher(cfg.BcryptRounds)
	users := application.NewUserService(stores.Users, stores.RefreshTokens, passwords)
	existing, err := stores.Users.GetByEmail(ctx, *email)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		logger.Error("look up user failed", "error", err)
		os.Exit(1)
	}

	var superuserValue *bool
	if superuserSet {
		superuserValue = superuser
	}

	if errors.Is(err, domain.ErrNotFound) {
		if *password == "" {
			fail("password is required when creating a user (use -password or SEED_PASSWORD)")
		}
		user, err := users.Create(ctx, inbound.CreateUserInput{
			Email:       *email,
			Password:    *password,
			FullName:    *name,
			IsSuperuser: superuserValue,
		})
		if err != nil {
			logger.Error("create user failed", "error", err)
			os.Exit(1)
		}
		fmt.Printf("created user %d (%s)\n", user.ID, user.Email)
		return
	}

	var nameValue *string
	if *name != "" {
		nameValue = name
	}
	var passwordValue *string
	if *password != "" {
		passwordValue = password
	}
	active := true
	updated, err := users.Update(ctx, existing.ID, inbound.UpdateUserInput{
		FullName:    nameValue,
		Password:    passwordValue,
		IsActive:    &active,
		IsSuperuser: superuserValue,
	})
	if err != nil {
		logger.Error("update user failed", "error", err)
		os.Exit(1)
	}
	fmt.Printf("updated user %d (%s)\n", updated.ID, updated.Email)
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
