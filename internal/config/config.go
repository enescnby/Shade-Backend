package config

import (
	"os"

	"core-backend/pkg/logger"

	"github.com/joho/godotenv"
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
	err := godotenv.Load()
	if err != nil {
		logger.Log.Warn(".env file can not be found")
	}

	AppConfig = Config{
		AppPort:    getEnv("APP_PORT", ""),
		DBHost:     getEnv("DB_HOST", ""),
		DBUser:     getEnv("DB_USER", ""),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", ""),
		DBPort:     getEnv("DB_PORT", ""),
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

	logger.Log.Info("Configuration successfully imported!")
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
