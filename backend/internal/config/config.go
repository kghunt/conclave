package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL         string
	RedisURL            string
	GoogleClientID      string
	GoogleClientSecret  string
	JWTSecret           string
	BaseURL             string
	FrontendURL         string
	Port                string
	AvatarDir           string
	StaticDir           string
	InstanceAdminEmail  string
	VAPIDPublicKey      string
	VAPIDPrivateKey     string
	VAPIDEmail          string
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:        env("DATABASE_URL", ""),
		RedisURL:           env("REDIS_URL", "redis://localhost:6379"),
		GoogleClientID:     env("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: env("GOOGLE_CLIENT_SECRET", ""),
		JWTSecret:          env("JWT_SECRET", ""),
		BaseURL:            env("BASE_URL", "http://localhost:8080"),
		FrontendURL:        env("FRONTEND_URL", env("BASE_URL", "http://localhost:5173")),
		Port:               env("PORT", "8080"),
		AvatarDir:          env("AVATAR_DIR", "./data/avatars"),
		StaticDir:          env("STATIC_DIR", "./public"),
		InstanceAdminEmail: env("INSTANCE_ADMIN_EMAIL", ""),
		VAPIDPublicKey:     env("VAPID_PUBLIC_KEY", ""),
		VAPIDPrivateKey:    env("VAPID_PRIVATE_KEY", ""),
		VAPIDEmail:         env("VAPID_EMAIL", "admin@localhost"),
	}

	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if c.GoogleClientID == "" {
		return nil, fmt.Errorf("GOOGLE_CLIENT_ID is required")
	}
	if c.GoogleClientSecret == "" {
		return nil, fmt.Errorf("GOOGLE_CLIENT_SECRET is required")
	}
	if c.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return c, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
