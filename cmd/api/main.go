package main

import (
	"log"
	"net/http"
	"os"

	"digital-wallet/internal/auth"
	"digital-wallet/internal/database"
	"digital-wallet/internal/deposit"
	"digital-wallet/internal/history"
	"digital-wallet/internal/transfer"
	"digital-wallet/internal/wallet"

	"github.com/gin-gonic/gin"
)

func main() {
	db, err := database.Connect()
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()

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
	router := gin.Default()

	api := router.Group("/api/v1")

	{

		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
		protected := api.Group("")
		protected.Use(auth.Middleware(jwtSecret))

		protected.POST("/wallets", walletHandler.CreateWallet)
		protected.GET("/wallets/:id/balance", walletHandler.GetBalance)
		protected.POST("/transfers", transferHandler.CreateTransfer)
		protected.POST("/deposits", depositHandler.CreateDeposit)

		protected.GET(
	"/wallets/:id/transactions",
	historyHandler.List,
)
protected.POST(
	"/transfers/:id/reverse",
	reversalHandler.Reverse,
)

	}

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	router.GET("/health/db", func(c *gin.Context) {
		if err := db.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unavailable",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"database": "connected",
		})
	})

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
