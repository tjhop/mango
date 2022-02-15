package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
)

func run(ctx context.Context) error {
	log.Info("Mango server started")
	defer log.Info("Mango server finished")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sigs:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Mango server recieved error")
		os.Exit(1)
	}
}
