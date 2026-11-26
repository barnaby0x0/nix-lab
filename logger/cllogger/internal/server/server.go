// Package server expose l'API HTTP de contrôle du logger.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"logger/internal/loadgen"
	"logger/internal/logstream"
)

type handler struct {
	stream      *logstream.Stream
	gen         *loadgen.Generator
	containerID string
}

// New construit le serveur HTTP avec toutes les routes.
func New(addr string, stream *logstream.Stream, gen *loadgen.Generator, containerID string, metricsHandler http.Handler) *http.Server {
	h := &handler{stream: stream, gen: gen, containerID: containerID}

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.accessLog(h.root))
	mux.HandleFunc("/log", h.accessLog(h.log))
	mux.HandleFunc("/reset", h.accessLog(h.reset))
	mux.HandleFunc("/config", h.accessLog(h.setLogMode))

	// Volontairement sans accessLog : chaque scrape générerait un log, ce qui
	// incrémenterait les compteurs qu'on cherche à mesurer.
	mux.Handle("/metrics", metricsHandler)

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// accessLog journalise les requêtes HTTP au format Apache/Flask.
func (h *handler) accessLog(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r)
		now := time.Now().UTC().Format("02/Jan/2006 15:04:05")
		h.stream.Emit("INFO", fmt.Sprintf(`%s - - [%s] "%s %s %s" 200 -`,
			r.RemoteAddr, now, r.Method, r.URL.RequestURI(), r.Proto))
	}
}

func (h *handler) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "running",
		"container_id":    h.containerID,
		"log_output_mode": h.stream.Mode().String(),
		"active_workers":  h.gen.ActiveWorkers(),
		"total_logs_sent": h.stream.Sent(),
		"logs_dropped":    h.stream.Dropped(),
		"buffer_usage":    h.stream.Len(),
		"endpoints": map[string]string{
			"/":        "Affiche le statut actuel et les instructions",
			"/log":     "Lance une génération asynchrone. Params: ?count=100&delay=0.3 (delay en secondes)",
			"/config":  "Change la sortie des logs à chaud. Params: ?mode=0 (stdout), 1 (file), 2 (both)",
			"/reset":   "Stoppe immédiatement toutes les générations de logs en cours et vide le buffer",
			"/metrics": "Métriques au format Prometheus (débogage ; l'export principal est OTLP)",
		},
	})
}

func (h *handler) log(w http.ResponseWriter, r *http.Request) {
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

	h.gen.Start(count, time.Duration(delaySec*float64(time.Second)))

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "started",
		"count":   count,
		"delay_s": delaySec,
	})
}

func (h *handler) reset(w http.ResponseWriter, r *http.Request) {
	h.gen.Reset()
	h.stream.Drain()

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "all log generations stopped",
	})
}

func (h *handler) setLogMode(w http.ResponseWriter, r *http.Request) {
	val, err := strconv.Atoi(r.URL.Query().Get("mode"))
	mode := logstream.OutputMode(val)
	if err != nil || !mode.Valid() {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Mode invalide (attendu : 0=stdout, 1=file, 2=both)",
		})
		return
	}

	h.stream.SetMode(mode)

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "updated",
		"mode":   val,
	})
}
