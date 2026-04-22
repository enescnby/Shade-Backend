package main

import (
	"core-backend/pkg/firebase"
	"core-backend/pkg/storage"
	"log"

	"github.com/gofiber/fiber/v2"

	"core-backend/internal/config"
	"core-backend/internal/database"
	"core-backend/internal/handlers"
	"core-backend/internal/middleware"
	"core-backend/internal/repositories"
	"core-backend/internal/services"
	"core-backend/internal/websocket"
	"core-backend/pkg/logger"
)

func main() {
	logger.InitLogger()
	defer logger.Log.Sync()

	config.LoadConfig()

	database.Connect()
	defer database.Close()

	database.Migrate()

	app := fiber.New()

	firebaseApp := firebase.InitFirebase()
	firebaseService := services.NewFCMService(firebaseApp)

	r2Client := storage.NewR2Client(&config.AppConfig)

	keyRepo := repositories.NewKeyRepository(database.DB)
	userRepo := repositories.NewUserRepository(database.DB)
	auditRepo := repositories.NewAuditRepository(database.DB)
	msgRepo := repositories.NewMessageRepository(database.DB)
	mediaRepo := repositories.NewMediaRepository(database.DB, r2Client, config.AppConfig.R2BucketName)

	authService := services.NewAuthService(userRepo, auditRepo)
	keyService := services.NewKeyService(keyRepo)
	userService := services.NewUserService(userRepo)
	mediaService := services.NewMediaService(mediaRepo, 10*1024*1024)
	messageService := services.NewMessageService(msgRepo, userRepo)

	cm := websocket.NewConnectionManager(msgRepo, userRepo, firebaseService)
	wsHandler := handlers.NewWebSocketHandler(cm)

	authHandler := handlers.NewAuthHandler(authService)
	keyHandler := handlers.NewKeyHandler(keyService)
	userHandler := handlers.NewUserHandler(userService)
	mediaHandler := handlers.NewMediaHandler(mediaService)
	messageHandler := handlers.NewMessageHandler(messageService, cm)

	api := app.Group("/api")
	v1 := api.Group("/v1")

	auth := v1.Group("/auth")
	auth.Post("/register", authHandler.Register)
	auth.Post("/login/init", authHandler.LoginInit)
	auth.Post("/login/verify", authHandler.LoginVerify)

	keys := v1.Group("/keys", middleware.Protected())
	keys.Get("/:id", keyHandler.GetPublicKey)

	user := v1.Group("/user", middleware.Protected())
	user.Get("/lookup/:shadeId", userHandler.GetUserForLookup)

	media := v1.Group("/media", middleware.Protected())
	media.Post("/upload", mediaHandler.Upload)
	media.Get("/:imageId", mediaHandler.Download)

	messages := v1.Group("/messages", middleware.Protected())
	messages.Get("/undelivered", messageHandler.GetUndelivered)
	messages.Post("/receipts", messageHandler.PostReceipts)
	messages.Get("/receipts/pending", messageHandler.GetPendingReceipts)

	v1.Get("/ws", wsHandler.UpgradeAndServe)

	logger.Log.Info("CoreGuard API is starting...")
	log.Fatal(app.Listen(config.AppConfig.AppPort))
}
