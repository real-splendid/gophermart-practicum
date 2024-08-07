package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v4/pgxpool"
	"go.uber.org/zap"

	"github.com/real-splendid/gophermart-practicum/internal/accrual"
	"github.com/real-splendid/gophermart-practicum/internal/app"
	"github.com/real-splendid/gophermart-practicum/internal/storage"
)

type config struct {
	ServerAddress            string
	AccrualSystemAddress     string
	DatabaseConnectionString string
}

func main() {
	cfg := config{
		ServerAddress: ":8080",
	}

	flag.StringVar(&cfg.ServerAddress, "a", os.Getenv("RUN_ADDRESS"), "")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", os.Getenv("ACCRUAL_SYSTEM_ADDRESS"), "")
	flag.StringVar(&cfg.DatabaseConnectionString, "d", os.Getenv("DATABASE_URI"), "")

	flag.Parse()

	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Printf("failed to initialize logger: %+v", err)
		os.Exit(1)
	}
	defer logger.Sync()

	if len(cfg.DatabaseConnectionString) == 0 {
		logger.Fatal("Empty database connection string")
	}

	dbConn, err := pgxpool.Connect(context.Background(), cfg.DatabaseConnectionString)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer dbConn.Close()

	storageCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	storage, err := storage.NewDatabaseStorage(storageCtx, dbConn, logger)
	if err != nil {
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}

	serverCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updaterCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	accCfg := accrual.Config{
		BaseAddr:   cfg.AccrualSystemAddress,
		Logger:     logger,
		AppStorage: storage,
	}
	accrual := accrual.NewAccrual(updaterCtx, accCfg)
	defer accrual.Stop()

	app.Run(serverCtx, cfg.ServerAddress, logger, storage)
}
