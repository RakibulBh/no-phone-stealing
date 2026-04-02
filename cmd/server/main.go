package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	openaiClient "github.com/rakibulbh/safe-london/internal/infrastructure/openai"
	"github.com/rakibulbh/safe-london/internal/infrastructure/policeuk"
	sqliterepo "github.com/rakibulbh/safe-london/internal/infrastructure/sqlite"
	httpHandler "github.com/rakibulbh/safe-london/internal/interface/http"
	"github.com/rakibulbh/safe-london/internal/interface/ws"
	"github.com/rakibulbh/safe-london/internal/usecase"
)

func main() {
	godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		slog.Warn("OPENAI_API_KEY not set, LLM calls will fail")
	}

	// --- Infrastructure ---
	repo, err := sqliterepo.NewRepository("safe-london.db")
	if err != nil {
		slog.Error("failed to init sqlite", "err", err)
		os.Exit(1)
	}
	defer repo.Close()

	llm := openaiClient.NewClient(apiKey)
	hub := ws.NewHub()
	go hub.Run()

	// Seed historical data in background (non-blocking)
	seedCtx, seedCancel := context.WithCancel(context.Background())
	defer seedCancel()
	go policeuk.SeedHistoricalData(seedCtx, repo, &http.Client{Timeout: 30 * time.Second})

	// --- Use Cases ---
	reportUC := usecase.NewReportUseCase(repo, llm, repo, hub)

	// --- HTTP ---
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
	}))

	handler := httpHandler.NewHandler(reportUC, repo)

	api := e.Group("/api/v1")
	api.POST("/reports", handler.PostReport)
	api.GET("/trends", handler.GetTrends)
	api.GET("/ws", hub.HandleWS)

	// Health check
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Graceful shutdown
	go func() {
		if err := e.Start(":" + port); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	slog.Info("shutting down...")
	seedCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}
