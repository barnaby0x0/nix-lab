package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var workerSeq atomic.Uint64

var (
	activeTasks   atomic.Int64
	totalLogsSent atomic.Uint64
	logsDropped   atomic.Uint64

	ctxMu     sync.RWMutex
	globalCtx context.Context
	cancelAll context.CancelFunc

	logChan = make(chan string, 100000) // Tampon de 100k logs en mémoire
)

var (
	levels     = []string{"INFO", "INFO", "INFO", "WARNING", "ERROR"}
	components = []string{"NETWORK", "FINOPS_ALRT", "FIREWALL", "SERVERLESS", "SEC", "IAM_SERVICE", "KERNEL", "DISK"}
	templates  = []func() string{
		func() string { return "WebAssembly sandbox isolated and killed: CPU execution timeout exceeded" },
		func() string {
			return fmt.Sprintf("Microservice 'payment-v2' reported high latency (%dms)", rand.IntN(900)+50)
		},
		func() string {
			return fmt.Sprintf("HNSW index build completed for 'knowledge-base' (%dms)", rand.IntN(1500)+100)
		},
		func() string { return "Forward proxy blocked connection to blacklisted category 'Gambling'" },
		func() string {
			return fmt.Sprintf("Process %d (node) sacrificed by Out-Of-Memory killer", rand.IntN(9000)+100)
		},
		func() string {
			return fmt.Sprintf("Connection closed prematurely by client: Ping timeout (%ds)", rand.IntN(600)+30)
		},
		func() string { return "Nginx buffer overflow prevented: client sent header line too long" },
		func() string {
			return fmt.Sprintf("Disk usage reached %d%% on partition /dev/sda1", rand.IntN(25)+75)
		},
		func() string {
			return fmt.Sprintf("SMTP relay rejected %d emails: Rate limit exceeded for sender", rand.IntN(1000)+10)
		},
	}
)

const (
	OutputStdout int32 = 0 // Uniquement console / CloudWatch
	OutputFile   int32 = 1 // Uniquement /var/logs/logger.log
	OutputBoth   int32 = 2 // Les deux en même temps
)

var logOutputMode atomic.Int32

var containerID = getContainerID()

func getContainerID() string {
	// Si sur AWS ECS Fargate, l'URI de métadonnées contient le Task ID à la fin
	if metaURI := os.Getenv("ECS_CONTAINER_METADATA_URI_V4"); metaURI != "" {
		parts := strings.Split(metaURI, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1] // Renvoie l'ID unique du conteneur/task ECS
		}
	}

	// Sinon (Docker classique, Kubernetes, EC2), le Hostname est l'ID du conteneur
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return "unknown"
}

func init() {
	logOutputMode.Store(OutputBoth)
	globalCtx, cancelAll = context.WithCancel(context.Background())
}

var fileLogger *log.Logger

// ---------------------------------------------------------------------------
// Métriques OpenTelemetry (export OTLP/HTTP)
// ---------------------------------------------------------------------------

func initMetrics(ctx context.Context) (func(context.Context) error, error) {
	// Endpoint, headers, protocole, etc. : variables d'env OTEL_EXPORTER_OTLP_*
	exp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("logger"),
			semconv.ServiceInstanceID(containerID),
		),
		resource.WithFromEnv(), // OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES surchargent les valeurs ci-dessus
	)
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(15*time.Second))),
	)
	otel.SetMeterProvider(mp)

	meter := mp.Meter("logger")

	obs := func(v func() int64) metric.Int64Callback {
		return func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(v())
			return nil
		}
	}

	if _, err := meter.Int64ObservableCounter("logger.logs.emitted",
		metric.WithDescription("Logs mis en file d'attente."),
		metric.WithUnit("{log}"),
		metric.WithInt64Callback(obs(func() int64 { return int64(totalLogsSent.Load()) })),
	); err != nil {
		return nil, err
	}

	if _, err := meter.Int64ObservableCounter("logger.logs.dropped",
		metric.WithDescription("Logs droppés car le buffer était plein."),
		metric.WithUnit("{log}"),
		metric.WithInt64Callback(obs(func() int64 { return int64(logsDropped.Load()) })),
	); err != nil {
		return nil, err
	}

	if _, err := meter.Int64ObservableUpDownCounter("logger.workers.active",
		metric.WithDescription("Goroutines de génération actives."),
		metric.WithUnit("{worker}"),
		metric.WithInt64Callback(obs(func() int64 { return activeTasks.Load() })),
	); err != nil {
		return nil, err
	}

	if _, err := meter.Int64ObservableGauge("logger.buffer.usage",
		metric.WithDescription("Logs actuellement dans le buffer."),
		metric.WithUnit("{log}"),
		metric.WithInt64Callback(obs(func() int64 { return int64(len(logChan)) })),
	); err != nil {
		return nil, err
	}

	if _, err := meter.Int64ObservableGauge("logger.buffer.capacity",
		metric.WithDescription("Capacité totale du buffer."),
		metric.WithUnit("{log}"),
		metric.WithInt64Callback(obs(func() int64 { return int64(cap(logChan)) })),
	); err != nil {
		return nil, err
	}

	if _, err := meter.Int64ObservableGauge("logger.output.mode",
		metric.WithDescription("Mode de sortie (0=stdout, 1=file, 2=both)."),
		metric.WithInt64Callback(obs(func() int64 { return int64(logOutputMode.Load()) })),
	); err != nil {
		return nil, err
	}

	return mp.Shutdown, nil
}

// ---------------------------------------------------------------------------

func startLogFlusher() {
	stdoutWriter := bufio.NewWriterSize(os.Stdout, 64*1024)

	var fileWriter *bufio.Writer
	if err := os.MkdirAll("/var/logs", 0755); err == nil {
		if file, err := os.OpenFile("/var/logs/logger.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			defer file.Close()
			fileWriter = bufio.NewWriterSize(file, 64*1024)
		} else {
			fmt.Fprintf(os.Stderr, "Erreur ouverture logger.log : %v\n", err)
		}
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case msg := <-logChan:
			mode := logOutputMode.Load()

			// Écriture Stdout (si mode 0 ou 2)
			if mode == OutputStdout || mode == OutputBoth {
				stdoutWriter.WriteString(msg)
				stdoutWriter.WriteByte('\n')
				if stdoutWriter.Buffered() >= 32*1024 {
					stdoutWriter.Flush()
				}
			}

			// Écriture Fichier (si mode 1 ou 2)
			if fileWriter != nil && (mode == OutputFile || mode == OutputBoth) {
				fileWriter.WriteString(msg)
				fileWriter.WriteByte('\n')
				if fileWriter.Buffered() >= 32*1024 {
					fileWriter.Flush()
				}
			}

		case <-ticker.C:
			if stdoutWriter.Buffered() > 0 {
				stdoutWriter.Flush()
			}
			if fileWriter != nil && fileWriter.Buffered() > 0 {
				fileWriter.Flush()
			}
		}
	}
}

func emitLog(level, message string) {
	now := time.Now().UTC()
	// Format exact : 2026-11-16 09:43:07,383 (CloudWatch ajoute automatiquement le préfixe ISO8601)
	ts := now.Format("2006-01-02 15:04:05") + fmt.Sprintf(",%03d", now.Nanosecond()/1e6)
	line := fmt.Sprintf("%s - %s - %s", ts, level, message)

	select {
	case logChan <- line:
		totalLogsSent.Add(1)
	default:
		// Buffer de 100k plein : on drop, mais on le comptabilise
		logsDropped.Add(1)
	}
}

// Génère les logs en arrière-plan dans une goroutine dédiée
func generateLogsWorker(ctx context.Context, count int, delay time.Duration) {
	activeTasks.Add(1)
	defer activeTasks.Add(-1)

	// Attribue un numéro unique à cette goroutine
	workerID := workerSeq.Add(1)

	for i := 1; i <= count; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		lvl := levels[rand.IntN(len(levels))]
		comp := components[rand.IntN(len(components))]
		msg := templates[rand.IntN(len(templates))]()

		// On inclut [W-%d] dans le message formaté
		formatted := fmt.Sprintf("[W-%d] [%d/%d] (%s) %s", workerID, i, count, comp, msg)
		emitLog(lvl, formatted)

		if delay > 0 && i < count {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

// Middleware pour journaliser les requêtes HTTP au format Apache/Flask
func accessLogMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r)
		now := time.Now().UTC().Format("02/Jan/2006 15:04:05")
		msg := fmt.Sprintf(`%s - - [%s] "%s %s %s" 200 -`, r.RemoteAddr, now, r.Method, r.URL.RequestURI(), r.Proto)
		emitLog("INFO", msg)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Libellé lisible pour le mode actif
	modeLabels := map[int32]string{
		OutputStdout: "0 (stdout)",
		OutputFile:   "1 (file: /var/logs/logger.log)",
		OutputBoth:   "2 (stdout + file)",
	}
	currentMode := logOutputMode.Load()

	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"status":          "running",
		"container_id":    containerID,
		"log_output_mode": modeLabels[currentMode],
		"active_workers":  activeTasks.Load(),
		"total_logs_sent": totalLogsSent.Load(),
		"logs_dropped":    logsDropped.Load(),
		"buffer_usage":    len(logChan),
		"endpoints": map[string]string{
			"/":       "Affiche le statut actuel et les instructions",
			"/log":    "Lance une génération asynchrone. Params: ?count=100&delay=0.3 (delay en secondes)",
			"/config": "Change la sortie des logs à chaud. Params: ?mode=0 (stdout), 1 (file), 2 (both)",
			"/reset":  "Stoppe immédiatement toutes les générations de logs en cours et vide le buffer",
		},
		"metrics": "Exportées en OTLP (variables OTEL_EXPORTER_OTLP_*), pas de endpoint /metrics",
	}
	json.NewEncoder(w).Encode(resp)
}

func handleLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	count := 100
	if cStr := q.Get("count"); cStr != "" {
		if c, err := strconv.Atoi(cStr); err == nil && c > 0 {
			count = c
		}
	}

	delaySec := 0.0
	if dStr := q.Get("delay"); dStr != "" {
		if d, err := strconv.ParseFloat(dStr, 64); err == nil && d >= 0 {
			delaySec = d
		}
	}

	delay := time.Duration(delaySec * float64(time.Second))

	ctxMu.RLock()
	ctx := globalCtx
	ctxMu.RUnlock()

	// Lancement non-bloquant (multi-thread via goroutine)
	go generateLogsWorker(ctx, count, delay)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "started",
		"count":   count,
		"delay_s": delaySec,
	})
}

func handleReset(w http.ResponseWriter, r *http.Request) {
	ctxMu.Lock()
	cancelAll()
	globalCtx, cancelAll = context.WithCancel(context.Background())
	ctxMu.Unlock()

	for len(logChan) > 0 {
		<-logChan
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "all log generations stopped",
	})
}

func handleSetLogMode(w http.ResponseWriter, r *http.Request) {
	valStr := r.URL.Query().Get("mode")
	val, err := strconv.Atoi(valStr)
	if err != nil || val < 0 || val > 2 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Mode invalide (attendu : 0=stdout, 1=file, 2=both)",
		})
		return
	}

	logOutputMode.Store(int32(val))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "updated",
		"mode":   val,
	})
}

func main() {
	go startLogFlusher()

	// Contexte annulé sur SIGINT/SIGTERM (arrêt des tâches ECS) pour un flush propre
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownMetrics, err := initMetrics(sigCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OTel init error: %v\n", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", accessLogMiddleware(handleRoot))
	mux.HandleFunc("/log", accessLogMiddleware(handleLog))
	mux.HandleFunc("/reset", accessLogMiddleware(handleReset))
	mux.HandleFunc("/config", accessLogMiddleware(handleSetLogMode))

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		<-sigCtx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(sctx)
		_ = shutdownMetrics(sctx) // flush final des métriques
	}()

	fmt.Fprintf(os.Stderr, "Server listening on port %s...\n", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}

	<-done // attend la fin du flush avant de quitter
}
