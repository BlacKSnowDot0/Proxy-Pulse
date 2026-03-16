package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"proxy-pulse/internal/proxy"
)

func main() {
	cfg := proxy.LoadConfigFromEnv()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := proxy.Run(ctx, cfg); err != nil {
		log.Fatalf("proxy-updater failed: %v", err)
	}
}
