package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"wobble/internal/ai"
	"wobble/internal/auth"
	"wobble/internal/chat"
	"wobble/internal/config"
	"wobble/internal/database"
	"wobble/internal/realtime"
	"wobble/internal/storage"
	"wobble/internal/telegram"
	"wobble/internal/tenant"
	"wobble/internal/ticket"
	"wobble/internal/worker"
)

func main() {
	// 1. Logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// 2. Load Config
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "err", err)
		os.Exit(1)
	}

	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	// 3. Database
	db, err := database.NewPostgresDB(cfg)
	if err != nil {
		slog.Error("failed to connect database", "err", err)
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("failed to get sql.DB handle", "err", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	// 4. Repositories
	tenantRepo := tenant.NewRepository(db)
	ticketRepo := ticket.NewRepository(db)
	chatRepo := chat.NewRepository(db)
	dedupStore := telegram.NewDedupStore(db)

	// 5. Services & Realtime Hub
	tokenSvc := auth.NewTokenService(cfg.TokenSecret, cfg.TokenTTL)

	hub := realtime.NewHub()
	go hub.Run()

	tgClient := telegram.NewClient(cfg.TelegramBotToken, "")
	tgNotifier := telegram.NewNotifier(tgClient, cfg.TelegramChatID, tenantRepo, cfg.UploadDir)

	var aiClient chat.AIClient
	if cfg.AIEnabled() {
		aiClient = ai.NewClient(cfg)
	}

	storageSvc := storage.NewService(cfg.UploadDir, cfg.FileSigningSecret)
	urlSigner := storage.NewURLSigner(storageSvc, cfg.BaseURL, time.Hour)

	ticketSvc := ticket.NewService(ticketRepo, tokenSvc, tgNotifier)
	chatSvc := chat.NewService(
		chatRepo,
		ticketRepo,
		hub,
		tgNotifier,
		aiClient,
		urlSigner,
		cfg.AIHistoryLimit,
		"",
	)

	// 6. Background Workers (Outbox, Auto-Close, Cleanup)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	workerMgr := worker.NewManager(db, chatRepo, ticketRepo, tgNotifier, hub)
	workerMgr.Start(workerCtx)

	// 7. Handlers
	ticketHandler := ticket.NewHandler(ticketSvc)
	chatHandler := chat.NewHandler(chatSvc, ticketRepo, storageSvc, cfg.MaxUploadImageMB*1024*1024, cfg.MaxUploadVideoMB*1024*1024)
	tgWebhookHandler := telegram.NewWebhookHandler(tgClient, dedupStore, ticketRepo, chatRepo, tenantRepo, hub, cfg.TelegramChatID, storageSvc, urlSigner)
	storageHandler := storage.NewHandler(storageSvc, chatRepo)

	// 8. Middlewares
	requireAPIKey := auth.RequireAPIKey(tenantRepo)
	requireTicketToken := auth.RequireTicketToken(tokenSvc)
	verifyTelegramSecret := auth.VerifyTelegramSecret(cfg.TelegramWebhookSecret)

	// Rate limiters
	rateLimitInit := auth.RateLimitByTenant(rate.Every(2*time.Second), 30)   // 30 req burst, 0.5 req/detik per tenant
	rateLimitChat := auth.RateLimitByTicket(rate.Every(3*time.Second), 20)   // 20 pesan/menit per tiket
	rateLimitUpload := auth.RateLimitByTicket(rate.Every(12*time.Second), 5) // 5 upload/menit per tiket

	// 9. Router
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	router.Use(gin.Recovery())
	router.Use(auth.RequestIDMiddleware())
	router.Use(auth.CORSMiddleware())

	// Health & Readiness check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": true,
			"app":    cfg.AppName,
			"env":    cfg.AppEnv,
		})
	})
	router.GET("/ready", func(c *gin.Context) {
		if err := sqlDB.PingContext(c.Request.Context()); err != nil {
			slog.Error("readiness check failed", "err", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"ready": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ready": true})
	})

	// Realtime WebSocket
	router.GET("/ws", realtime.ServeWS(hub, tokenSvc))

	// Telegram Webhook
	tgWebhookHandler.RegisterRoutes(router, verifyTelegramSecret)

	// Signed File Downloads
	storageHandler.RegisterRoutes(router)

	// API V1
	apiV1 := router.Group("/api/v1")
	{
		ticketHandler.RegisterRoutes(apiV1, requireAPIKey, requireTicketToken, rateLimitInit)
		chatHandler.RegisterRoutes(apiV1, requireTicketToken, rateLimitChat, rateLimitUpload)
	}

	// Static Web Assets & Widget
	if _, err := os.Stat("./web"); err == nil {
		router.Static("/demo", "./web")
		router.Static("/assets", "./web/assets")
	}

	// 10. HTTP Server
	srv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 11. Start Server in Goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", srv.Addr, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// 12. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		slog.Error("server startup error", "err", err)
	case sig := <-quit:
		slog.Info("shutting down server...", "signal", sig.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "err", err)
	}

	slog.Info("server exited cleanly")
}
