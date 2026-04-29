package main

import (
	"core-backend/internal/rabbitmq"
	"core-backend/pkg/firebase"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

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

	rabbitClient, err := rabbitmq.NewClient(config.AppConfig)
	if err != nil {
		log.Fatalf("failed to connect rabbitmq: %v", err)
	}
	defer rabbitClient.Close()

	if err := rabbitmq.DeclareTopology(rabbitClient); err != nil {
		log.Fatalf("failed to declare rabbitmq topology: %v", err)
	}

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins:     "http://localhost:5173",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS",
		AllowCredentials: true,
	}))

	firebaseApp := firebase.InitFirebase()
	firebaseService := services.NewFCMService(firebaseApp)

	keyRepo := repositories.NewKeyRepository(database.DB)
	userRepo := repositories.NewUserRepository(database.DB)
	auditRepo := repositories.NewAuditRepository(database.DB)
	msgRepo := repositories.NewMessageRepository(database.DB)

	authService := services.NewAuthService(userRepo, auditRepo)
	keyService := services.NewKeyService(keyRepo)
	userService := services.NewUserService(userRepo)
	msgService := services.NewMessageService(rabbitClient)

	cm := websocket.NewConnectionManager(msgRepo, userRepo, firebaseService, rabbitClient)
	wsHandler := handlers.NewWebSocketHandler(cm)
	msgHandler := handlers.NewMessageHandler(msgService)

	authHandler := handlers.NewAuthHandler(authService)
	keyHandler := handlers.NewKeyHandler(keyService)
	userHandler := handlers.NewUserHandler(userService)

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

	messages := v1.Group("/messages", middleware.Protected())
	messages.Get("/inbox", msgHandler.GetInbox)

	v1.Get("/ws", wsHandler.UpgradeAndServe)

	logger.Log.Info("CoreGuard API is starting...")
	log.Fatal(app.Listen(config.AppConfig.AppPort))
}
