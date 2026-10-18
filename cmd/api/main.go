package main

import (
	"context"
	"digital-wallet/internal/auth"
	"digital-wallet/internal/database"
	"digital-wallet/internal/deposit"
	"digital-wallet/internal/history"
	"digital-wallet/internal/messaging"
	"digital-wallet/internal/ratelimit"
	"digital-wallet/internal/transfer"
	"digital-wallet/internal/wallet"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	db, err := database.Connect()
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()
	rabbitConn, rabbitChannel, err := messaging.Connect()
	if err != nil {
		log.Fatalf("initialize RabbitMQ: %v", err)
	}
	defer rabbitChannel.Close()
	defer rabbitConn.Close()

	log.Println("RabbitMQ connected successfully")

	queueName := os.Getenv("RABBITMQ_QUEUE")
	outboxPublisher := messaging.NewOutboxPublisher(db, rabbitChannel, queueName)
	publisherCtx, stopPublisher := context.WithCancel(context.Background())
	defer stopPublisher()
	go outboxPublisher.Start(publisherCtx)

	log.Println("Outbox publisher started")

	go func() {
		if err := messaging.StartConsumer(
			publisherCtx,
			rabbitConn,
			queueName,
		); err != nil {
			log.Printf("notification consumer stopped: %v", err)
		}
	}()

	walletRepo := wallet.NewRepository(db)
	walletService := wallet.NewService(walletRepo)
	walletHandler := wallet.NewHandler(walletService)

	transferRepo := transfer.NewRepository(db)
	transferService := transfer.NewService(transferRepo)
	transferHandler := transfer.NewHandler(transferService)

	depositRepo := deposit.NewRepository(db)
	depositService := deposit.NewService(depositRepo)
	depositHandler := deposit.NewHandler(depositService)

	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET must be configured with at least 32 characters")
	}

	authRepo := auth.NewPostgresUserRepository(db)
	authService := auth.NewService(authRepo, jwtSecret)
	authHandler := auth.NewHandler(authService)

	historyRepo := history.NewPostgresRepository(db)
	historyService := history.NewService(historyRepo)
	historyHandler := history.NewHandler(historyService)

	reversalRepo := transfer.NewPostgresReversalRepository(db)
	reversalService := transfer.NewReversalService(reversalRepo)
	reversalHandler := transfer.NewReversalHandler(reversalService)

	depositReversalRepo := deposit.NewPostgresReversalRepository(db)
	depositReversalService := deposit.NewReversalService(depositReversalRepo)
	depositReversalHandler := deposit.NewReversalHandler(depositReversalService)
	startupCtx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	redisClient, err := ratelimit.NewRedisClient(startupCtx)
	if err != nil {
		log.Fatalf("initialize Redis: %v", err)
	}
	defer redisClient.Close()
	rateLimiter := ratelimit.NewLimiter(redisClient)

	router := gin.Default()

	api := router.Group("/api/v1")

	// Initialize rate limiting for all API routes.
	authRoutes := api.Group("/auth")
	authRoutes.Use(ratelimit.Middleware(
		rateLimiter,
		10,
		time.Minute,
		"ratelimit:auth",
	))
	authRoutes.POST("/register", authHandler.Register)
	authRoutes.POST("/login", authHandler.Login)

	// Authenticated routes.
	protected := api.Group("")
	protected.Use(auth.Middleware(jwtSecret))
	protected.Use(ratelimit.Middleware(
		rateLimiter,
		100,
		time.Minute,
		"ratelimit:api",
	))

	protected.POST("/wallets", walletHandler.CreateWallet)
	protected.GET("/wallets/:id/balance", walletHandler.GetBalance)

	protected.POST("/transfers", transferHandler.CreateTransfer)
	protected.POST("/transfers/:id/reverse", reversalHandler.Reverse)

	protected.POST("/deposits", depositHandler.CreateDeposit)
	protected.POST("/deposits/:id/reverse", depositReversalHandler.Reverse)

	protected.GET("/wallets/:id/transactions", historyHandler.List)

	// Health checks.
	// Health checks.
	instanceID := os.Getenv("INSTANCE_ID")
	if instanceID == "" {
		instanceID = "local"
	}

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":      "ok",
			"instance_id": instanceID,
		})
	})

	router.GET("/health/db", func(c *gin.Context) {
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
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if err := router.Run(":" + port); err != nil {
		log.Fatalf("server failed: %v", err)
	}

}
