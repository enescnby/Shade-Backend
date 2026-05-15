package config

import (
	"os"

	"core-backend/pkg/logger"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

type Config struct {
	AppPort    string
	DBHost     string
	DBUser     string
	DBPassword string
	DBName     string
	DBPort     string
	DBSSLMode  string
	DBTimeZone string
	JWTSecret  string

	RabbitMQHost     string
	RabbitMQPort     string
	RabbitMQUser     string
	RabbitMQPassword string
	RabbitMQVHost    string

	R2AccountID    string
	R2AccessKeyID  string
	R2AccessSecret string
	R2BucketName   string
}

var AppConfig Config

func LoadConfig() {
	if err := godotenv.Load(); err != nil {
		logger.Log.Warn(".env file not found — reading from environment variables")
	}

	AppConfig = Config{
		AppPort:    getEnv("APP_PORT", ":8080"),
		DBHost:     getEnv("DB_HOST", ""),
		DBUser:     getEnv("DB_USER", ""),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", ""),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),
		DBTimeZone: getEnv("DB_TIMEZONE", "Europe/Istanbul"),
		JWTSecret:  getEnv("JWT_SECRET", ""),

		RabbitMQHost:     getEnv("RABBITMQ_HOST", "localhost"),
		RabbitMQPort:     getEnv("RABBITMQ_PORT", "5672"),
		RabbitMQUser:     getEnv("RABBITMQ_USER", "guest"),
		RabbitMQPassword: getEnv("RABBITMQ_PASSWORD", "guest"),
		RabbitMQVHost:    getEnv("RABBITMQ_VHOST", "/"),

		R2AccountID:    getEnv("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:  getEnv("R2_ACCESS_KEY_ID", ""),
		R2AccessSecret: getEnv("R2_ACCESS_SECRET", ""),
		R2BucketName:   getEnv("R2_BUCKET_NAME", ""),
	}

	validateConfig()
	logger.Log.Info("Configuration loaded successfully")
}

// validateConfig — kritik değerler eksikse başlatma sırasında hata ver
func validateConfig() {
	if AppConfig.JWTSecret == "" {
		logger.Log.Fatal("JWT_SECRET must be set in .env file",
			zap.String("fix", "run: openssl rand -hex 32"),
		)
	}

	if len(AppConfig.JWTSecret) < 32 {
		logger.Log.Fatal("JWT_SECRET is too short — minimum 32 characters required",
			zap.Int("current_length", len(AppConfig.JWTSecret)),
		)
	}

	if AppConfig.DBHost == "" || AppConfig.DBUser == "" || AppConfig.DBName == "" {
		logger.Log.Fatal("Database configuration is incomplete — check DB_HOST, DB_USER, DB_NAME in .env")
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
