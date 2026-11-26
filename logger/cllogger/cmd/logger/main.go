package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"logger/internal/loadgen"
	"logger/internal/logstream"
	"logger/internal/platform"
	"logger/internal/server"
	"logger/internal/telemetry"
)

const (
	serviceName = "logger"
	bufferSize  = 100_000 // Tampon de 100k logs en mémoire
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Annulé sur SIGINT/SIGTERM (arrêt des tâches ECS)
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	containerID := platform.ContainerID()

	stream := logstream.New(bufferSize, logstream.OutputBoth)
	gen := loadgen.New(stream)

	// Le flusher a son propre contexte : il doit s'arrêter en dernier pour
	// écrire les logs restants.
	streamCtx, stopStream := context.WithCancel(context.Background())
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		stream.Run(streamCtx)
	}()

	shutdownMetrics, metricsHandler, err := telemetry.Init(sigCtx, serviceName, containerID, stream, gen)
	if err != nil {
		stopStream()
		<-streamDone
		return fmt.Errorf("init OTel : %w", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}
	srv := server.New(":"+port, stream, gen, containerID, metricsHandler)

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "Server listening on port %s...\n", port)
		errCh <- srv.ListenAndServe()
	}()

	var runErr error
	select {
	case runErr = <-errCh: // le serveur a échoué
	case <-sigCtx.Done():
	}

	// Arrêt ordonné : serveur, générateurs, flusher de logs, puis métriques.
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = srv.Shutdown(sctx)
	gen.Stop()
	stopStream()
	<-streamDone

	if err := shutdownMetrics(sctx); err != nil && runErr == nil {
		runErr = err
	}
	return runErr
}
