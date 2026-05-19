package main

import (
	"core-backend/internal/config"
	"core-backend/internal/database"
	"core-backend/internal/handlers"
	"core-backend/internal/middleware"
	"core-backend/internal/rabbitmq"
	"core-backend/internal/repositories"
	"core-backend/internal/services"
	"core-backend/internal/websocket"
	"core-backend/pkg/firebase"
	"core-backend/pkg/logger"
	"core-backend/pkg/storage"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"go.uber.org/zap"
)

func main() {
	// ── Başlatma ────────────────────────────────────────────────────────────
	logger.InitLogger()
	defer logger.Log.Sync() //nolint:errcheck // zap standard pattern

	config.LoadConfig()

	database.Connect()
	defer database.Close()

	database.Migrate()

	rabbitClient, err := rabbitmq.NewClient(config.AppConfig)
	if err != nil {
		log.Fatalf("RabbitMQ connection failed: %v", err)
	}
	defer rabbitClient.Close()

	if err := rabbitmq.DeclareTopology(rabbitClient); err != nil {
		log.Fatalf("RabbitMQ topology declaration failed: %v", err)
	}

	// ── Fiber ────────────────────────────────────────────────────────────────
	app := fiber.New(fiber.Config{
		// Panic'leri yakala — sunucu çökmeden devam etsin
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			logger.Log.Error("unhandled request error",
				zap.Error(err),
				zap.String("path", c.Path()),
				zap.String("method", c.Method()),
			)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		},
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins:     "http://localhost:5173, https://web.shadeapp.tech",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowCredentials: true,
	}))

	// ── Bağımlılıklar ────────────────────────────────────────────────────────
	firebaseApp := firebase.InitFirebase()
	firebaseService := services.NewFCMService(firebaseApp)

	keyRepo := repositories.NewKeyRepository(database.DB)
	userRepo := repositories.NewUserRepository(database.DB)
	auditRepo := repositories.NewAuditRepository(database.DB)
	msgRepo := repositories.NewMessageRepository(database.DB)

	var mediaRepo repositories.MediaRepository
	if config.AppConfig.R2AccountID == "" || config.AppConfig.R2AccessKeyID == "" || config.AppConfig.R2AccessSecret == "" {
		logger.Log.Warn("R2 credentials not configured — falling back to local disk storage")
		mediaRepo = repositories.NewLocalMediaRepository(database.DB, "./uploads")
	} else {
		r2Client := storage.NewR2Client(&config.AppConfig)
		mediaRepo = repositories.NewMediaRepository(database.DB, r2Client, config.AppConfig.R2BucketName)
	}
	webSessionRepo := repositories.NewWebSessionRepository(database.DB)
	groupRepo := repositories.NewGroupRepository(database.DB)
	skdmRepo := repositories.NewSenderKeyDistributionRepository(database.DB)

	authService := services.NewAuthService(userRepo, auditRepo)
	keyService := services.NewKeyService(keyRepo, userRepo)
	userService := services.NewUserService(userRepo)
	mediaService := services.NewMediaService(mediaRepo, 10*1024*1024)
	groupBindingService := services.NewGroupBindingService(rabbitClient, userRepo, groupRepo)
	messageService := services.NewMessageService(rabbitClient, groupBindingService, skdmRepo, msgRepo)
	webSessionService := services.NewSessionService(webSessionRepo, userRepo, groupBindingService)
	groupEventPublisher := services.NewGroupEventPublisher(rabbitClient)
	groupService := services.NewGroupService(groupRepo, userRepo, groupBindingService, groupEventPublisher)

	cm := websocket.NewConnectionManager(msgRepo, userRepo, groupRepo, skdmRepo, firebaseService, rabbitClient, groupBindingService)
	wsHandler := handlers.NewWebSocketHandler(cm)
	syncManager := websocket.NewSyncManager()

	// ── Handler'lar ──────────────────────────────────────────────────────────
	healthHandler := handlers.NewHealthHandler()
	authHandler := handlers.NewAuthHandler(authService)
	keyHandler := handlers.NewKeyHandler(keyService)
	userHandler := handlers.NewUserHandler(userService)
	mediaHandler := handlers.NewMediaHandler(mediaService)
	messageHandler := handlers.NewMessageHandler(messageService)
	webSessionHandler := handlers.NewWebSessionHandler(webSessionService)
	syncHandler := handlers.NewSyncHandler(syncManager, webSessionService)
	groupHandler := handlers.NewGroupHandler(groupService)

	// ── Route'lar ────────────────────────────────────────────────────────────
	// ── Health endpoints — auth ve rate limit YOK ──────────────────────────
	app.Get("/health/live", healthHandler.Live)
	app.Get("/health/ready", healthHandler.Ready)

	api := app.Group("/api")
	v1 := api.Group("/v1")

	// ── Auth — en sıkı limit: dakikada 10 istek / IP ────────────────────────
	auth := v1.Group("/auth", middleware.NewAuthLimiter())
	auth.Post("/register", authHandler.Register)
	auth.Post("/login/init", authHandler.LoginInit)
	auth.Post("/login/verify", authHandler.LoginVerify)

	// Web session — auth grubundan miras alır (aynı limiter)
	webAuth := auth.Group("/web")
	webAuth.Post("/session", webSessionHandler.CreateSession)
	webAuth.Get("/session/:session_id", webSessionHandler.PollSession)
	webAuth.Post("/session/:session_id/authorize", middleware.Protected(), webSessionHandler.AuthorizeSession)

	// ── Authenticated API — dakikada 120 istek / IP ──────────────────────────
	keys := v1.Group("/keys", middleware.Protected(), middleware.NewAPILimiter())
	keys.Get("/:id", keyHandler.GetPublicKey)

	user := v1.Group("/user", middleware.Protected(), middleware.NewAPILimiter())
	user.Get("/lookup/:shadeId", userHandler.GetUserForLookup)
	user.Patch("/displayname", userHandler.UpdateDisplayName)
	user.Patch("/avatar", userHandler.UpdateAvatar)
	user.Delete("/avatar", userHandler.RemoveAvatar)

	// ── Medya — dakikada 20 istek / IP (bant genişliği koruması) ────────────
	media := v1.Group("/media", middleware.Protected(), middleware.NewMediaLimiter())
	media.Post("/upload", mediaHandler.Upload)
	media.Get("/:imageId", mediaHandler.Download)

	// ── Mesajlar — genel API limiti ──────────────────────────────────────────
	messages := v1.Group("/messages", middleware.Protected(), middleware.NewAPILimiter())
	messages.Get("/inbox", messageHandler.GetInbox)
	messages.Post("/receipts", messageHandler.PostReceipts)

	// ── Gruplar ──────────────────────────────────────────────────────────────
	groups := v1.Group("/groups", middleware.Protected(), middleware.NewAPILimiter())
	groups.Post("/", groupHandler.CreateGroup)
	groups.Get("/", groupHandler.ListGroups)
	groups.Get("/:id", groupHandler.GetGroup)
	groups.Delete("/:id", groupHandler.DeleteGroup)
	groups.Post("/:id/members", groupHandler.AddMember)
	groups.Delete("/:id/members/:userId", groupHandler.RemoveMember)

	invites := v1.Group("/invites", middleware.Protected(), middleware.NewAPILimiter())
	invites.Post("/", groupHandler.CreateInvite)
	invites.Get("/:code", groupHandler.RedeemInvite)

	// ── WebSocket — JWT doğrulaması connection_manager içinde yapılıyor ──────
	v1.Get("/ws", wsHandler.UpgradeAndServe)
	v1.Get("/ws/sync/:session_id", syncHandler.UpgradeAndServe)

	// ── Graceful Shutdown ────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-quit
		logger.Log.Info("shutdown signal received — stopping gracefully")

		// Auth service cleanup goroutine'ini durdur
		authService.Shutdown()

		// Yeni bağlantıları reddet, mevcut istekleri tamamla
		if err := app.Shutdown(); err != nil {
			logger.Log.Error("server shutdown error", zap.Error(err))
		}

		logger.Log.Info("server stopped cleanly")
		os.Exit(0)
	}()

	// ── Başlat ───────────────────────────────────────────────────────────────
	logger.Log.Info("CoreGuard API starting",
		zap.String("port", config.AppConfig.AppPort),
		zap.String("env", "development"),
	)

	if err := app.Listen(config.AppConfig.AppPort); err != nil {
		logger.Log.Fatal("server failed to start", zap.Error(err))
	}
}
