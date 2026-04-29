package database

import (
	"core-backend/internal/config"
	"core-backend/internal/models"
	"core-backend/pkg/logger"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	cfg := config.AppConfig
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s Timezone=%s",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort, cfg.DBSSLMode, cfg.DBTimeZone,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})

	if err != nil {
		logger.Log.Fatal("Database connection failed!", zap.Error(err))
	}

	sqlDB, err := db.DB()
	if err != nil {
		logger.Log.Fatal("SQL DB object error!", zap.Error(err))
	}

	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(3)
	sqlDB.SetConnMaxLifetime(time.Hour)

	DB = db

	logger.Log.Info("Connected to the database successfully!")
}

func Migrate() {
	if DB == nil {
		logger.Log.Fatal("Connect the database before migration!")
	}

	logger.Log.Info("Database tables are updating (Migration)...")

	err := DB.AutoMigrate(
		&models.User{},
		&models.UserKey{},
		&models.UserDevice{},
		&models.EncryptedMessages{},
		&models.DeliveryStatus{},
		&models.PendingReceipt{},
		&models.SecurityAuditLog{},
		&models.ImageMetaData{},
		&models.WebSession{},
	)

	if err != nil {
		logger.Log.Fatal("AutoMigrate failed!", zap.Error(err))
	}

	logger.Log.Info("All tables are created/updated successfully!")
}

func PingDB() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

func Close() {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err == nil {
			sqlDB.Close()
			logger.Log.Info("Database connection closed successfully!")
		}
	}
}
