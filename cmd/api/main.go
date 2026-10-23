// @title           LedgerX API
// @version         1.0
// @description     Digital wallet and double-entry ledger API.
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
package main

import (
	"context"
	"database/sql"
	"digital-wallet/internal/auth"
	"digital-wallet/internal/database"
	"digital-wallet/internal/deposit"
	"digital-wallet/internal/history"
	"digital-wallet/internal/messaging"
	"digital-wallet/internal/notification"
	"digital-wallet/internal/observability"
	"digital-wallet/internal/ratelimit"
	"digital-wallet/internal/transfer"
	"digital-wallet/internal/wallet"
	"log"
	"net/http"
	"os"
	"time"

	_ "digital-wallet/docs"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
	// Initialize distributed tracing.
	tracingShutdown, err := observability.InitTracing(
		context.Background(),
	)
	if err != nil {
		log.Fatalf("initialize tracing: %v", err)
	}

	defer func() {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		if err := tracingShutdown(ctx); err != nil {
			log.Printf("shutdown tracing: %v", err)
		}
	}()

	// Initialize PostgreSQL.
	db, err := database.Connect()
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

	// Initialize RabbitMQ.
	rabbitConn, rabbitChannel, err := messaging.Connect()
	if err != nil {
		log.Fatalf("initialize RabbitMQ: %v", err)
	}
	defer rabbitChannel.Close()
	defer rabbitConn.Close()

	log.Println("RabbitMQ connected successfully")

	queueName := os.Getenv("RABBITMQ_QUEUE")

	// Start the transactional outbox publisher.
	publisherCtx, stopPublisher := context.WithCancel(
		context.Background(),
	)
	defer stopPublisher()

	outboxPublisher := messaging.NewOutboxPublisher(
		db,
		rabbitChannel,
		queueName,
	)
	go outboxPublisher.Start(publisherCtx)

	log.Println("Outbox publisher started")

	// Validate application secrets.
	webhookSecret := os.Getenv("MOCK_PAYMENT_WEBHOOK_SECRET")
	if len(webhookSecret) < 32 {
		log.Fatal(
			"MOCK_PAYMENT_WEBHOOK_SECRET must be at least 32 characters",
		)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		log.Fatal(
			"JWT_SECRET must be configured with at least 32 characters",
		)
	}

	// Create a bounded context for startup initialization.
	startupCtx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	// Initialize AWS SES.
	sesClient, err := notification.NewSESClient(startupCtx)
	if err != nil {
		log.Fatalf("initialize AWS SES: %v", err)
	}

	log.Println("AWS SES client initialized successfully")

	// Start the RabbitMQ notification consumer.
	go func() {
		if err := messaging.StartConsumer(
			publisherCtx,
			rabbitConn,
			queueName,
			db,
			sesClient,
		); err != nil {
			log.Printf("notification consumer stopped: %v", err)
		}
	}()

	// Initialize Redis rate limiting.
	redisClient, err := ratelimit.NewRedisClient(startupCtx)
	if err != nil {
		log.Fatalf("initialize Redis: %v", err)
	}
	defer redisClient.Close()

	rateLimiter := ratelimit.NewLimiter(redisClient)

	// Initialize wallet module.
	walletRepo := wallet.NewRepository(db)
	walletService := wallet.NewService(walletRepo)
	walletHandler := wallet.NewHandler(walletService)

	// Initialize transfer module.
	transferRepo := transfer.NewRepository(db)
	transferService := transfer.NewService(transferRepo)
	transferHandler := transfer.NewHandler(transferService)

	// Initialize deposit module.
	depositRepo := deposit.NewRepository(db)
	depositService := deposit.NewService(depositRepo)
	depositHandler := deposit.NewHandler(depositService)

	webhookHandler := deposit.NewWebhookHandler(
		depositService,
		webhookSecret,
	)

	// Initialize authentication module.
	authRepo := auth.NewPostgresUserRepository(db)
	authService := auth.NewService(authRepo, jwtSecret)
	authHandler := auth.NewHandler(authService)

	// Initialize transaction history module.
	historyRepo := history.NewPostgresRepository(db)
	historyService := history.NewService(historyRepo)
	historyHandler := history.NewHandler(historyService)

	// Initialize transfer reversal module.
	reversalRepo := transfer.NewPostgresReversalRepository(db)
	reversalService := transfer.NewReversalService(reversalRepo)
	reversalHandler := transfer.NewReversalHandler(reversalService)

	// Initialize deposit reversal module.
	depositReversalRepo := deposit.NewPostgresReversalRepository(db)
	depositReversalService := deposit.NewReversalService(
		depositReversalRepo,
	)
	depositReversalHandler := deposit.NewReversalHandler(
		depositReversalService,
	)

	// Configure structured logging.
	logger := observability.NewLogger()

	// Configure Gin middleware.
	router := gin.New()
	router.Use(
		gin.Recovery(),
		otelgin.Middleware("ledgerx"),
		observability.RequestLoger(logger),
		observability.MetricsMiddleware(),
	)

	// Swagger documentation UI.
	router.GET(
		"/swagger/*any",
		ginSwagger.WrapHandler(swaggerFiles.Handler),
	)
	router.GET("/metrics", observability.MetricsHandler())

	// API routes.
	api := router.Group("/api/v1")

	// Mock payment provider webhook.
	api.POST(
		"/webhooks/mock-payment",
		webhookHandler.Handle,
	)

	// Public authentication routes with rate limiting.
	authRoutes := api.Group("/auth")
	authRoutes.Use(
		ratelimit.Middleware(
			rateLimiter,
			10,
			time.Minute,
			"ratelimit:auth",
		),
	)
	authRoutes.POST("/register", authHandler.Register)
	authRoutes.POST("/login", authHandler.Login)

	// Authenticated API routes.
	protected := api.Group("")
	protected.Use(auth.Middleware(jwtSecret))
	protected.Use(
		ratelimit.Middleware(
			rateLimiter,
			100,
			time.Minute,
			"ratelimit:api",
		),
	)

	protected.POST("/wallets", walletHandler.CreateWallet)
	protected.GET("/wallets/:id/balance", walletHandler.GetBalance)

	protected.POST("/transfers", transferHandler.CreateTransfer)
	protected.POST(
		"/transfers/:id/reverse",
		reversalHandler.Reverse,
	)

	protected.POST("/deposits", depositHandler.CreateDeposit)
	protected.POST(
		"/deposits/:id/reverse",
		depositReversalHandler.Reverse,
	)

	protected.GET(
		"/wallets/:id/transactions",
		historyHandler.List,
	)

	// Health checks.
	instanceID := os.Getenv("INSTANCE_ID")
	if instanceID == "" {
		instanceID = "local"
	}

	router.GET("/health", func(c *gin.Context) {
		healthCheck(c, instanceID)
	})
	router.GET("/health/db", func(c *gin.Context) {
		databaseHealthCheck(c, db)
	})

	// Start the HTTP server.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("LedgerX API starting on port %s", port)

	if err := router.Run(":" + port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// HealthCheck godoc
// @Summary Check API health
// @Description Returns the API status and instance ID.
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]interface{} "API is healthy"
// @Router /health [get]
func healthCheck(c *gin.Context, instanceID string) {
	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"instance_id": instanceID,
	})
}

// DatabaseHealthCheck godoc
// @Summary Check database health
// @Description Checks database connectivity using a ping.
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]interface{} "Database is connected"
// @Failure 503 {object} map[string]interface{} "Database is unavailable"
// @Router /health/db [get]
func databaseHealthCheck(c *gin.Context, db *sql.DB) {
	if err := db.PingContext(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"database": "connected",
	})
}
