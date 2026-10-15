package main

import (
	"log"
	"net/http"

	"digital-wallet/internal/database"
	"digital-wallet/internal/deposit"
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

	router := gin.Default()

	api := router.Group("/api/v1")
	{
		api.POST("/wallets", walletHandler.CreateWallet)
		api.GET("/wallets/:id/balance", walletHandler.GetBalance)
		api.POST("/transfers", transferHandler.CreateTransfer)
		api.POST("/deposits", depositHandler.CreateDeposit)
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
