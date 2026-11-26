// Package logstream gère le buffer mémoire des logs, leur formatage et leur
// écriture vers stdout et/ou un fichier.
package logstream

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// LogFilePath est le fichier utilisé pour les modes OutputFile et OutputBoth.
const LogFilePath = "/var/logs/logger.log"

// OutputMode définit la destination des logs.
type OutputMode int32

const (
	OutputStdout OutputMode = 0 // Uniquement console / CloudWatch
	OutputFile   OutputMode = 1 // Uniquement /var/logs/logger.log
	OutputBoth   OutputMode = 2 // Les deux en même temps
)

// Valid indique si le mode est connu.
func (m OutputMode) Valid() bool { return m >= OutputStdout && m <= OutputBoth }

// String renvoie un libellé lisible du mode.
func (m OutputMode) String() string {
	switch m {
	case OutputStdout:
		return "0 (stdout)"
	case OutputFile:
		return "1 (file: " + LogFilePath + ")"
	case OutputBoth:
		return "2 (stdout + file)"
	default:
		return "unknown"
	}
}

// Stream est un buffer de logs avec compteurs et mode de sortie à chaud.
type Stream struct {
	ch      chan string
	mode    atomic.Int32
	sent    atomic.Uint64
	dropped atomic.Uint64
}

// New crée un Stream avec un buffer de bufferSize logs.
func New(bufferSize int, mode OutputMode) *Stream {
	s := &Stream{ch: make(chan string, bufferSize)}
	s.mode.Store(int32(mode))
	return s
}

// Emit formate un log et le met en file d'attente. Si le buffer est plein,
// le log est droppé et comptabilisé.
func (s *Stream) Emit(level, message string) {
	now := time.Now().UTC()
	// Format exact : 2026-11-16 09:43:07,383 (CloudWatch ajoute automatiquement le préfixe ISO8601)
	ts := now.Format("2006-01-02 15:04:05") + fmt.Sprintf(",%03d", now.Nanosecond()/1e6)
	line := fmt.Sprintf("%s - %s - %s", ts, level, message)

	select {
	case s.ch <- line:
		s.sent.Add(1)
	default:
		s.dropped.Add(1)
	}
}

// Drain vide le buffer sans bloquer.
func (s *Stream) Drain() {
	for {
		select {
		case <-s.ch:
		default:
			return
		}
	}
}

func (s *Stream) Mode() OutputMode     { return OutputMode(s.mode.Load()) }
func (s *Stream) SetMode(m OutputMode) { s.mode.Store(int32(m)) }
func (s *Stream) Sent() uint64         { return s.sent.Load() }
func (s *Stream) Dropped() uint64      { return s.dropped.Load() }
func (s *Stream) Len() int             { return len(s.ch) }
func (s *Stream) Cap() int             { return cap(s.ch) }

// Run consomme le buffer et écrit vers stdout / fichier jusqu'à l'annulation
// de ctx. À l'arrêt, les logs restants sont écrits puis les writers flushés.
func (s *Stream) Run(ctx context.Context) {
	stdout := bufio.NewWriterSize(os.Stdout, 64*1024)

	var file *bufio.Writer
	if err := os.MkdirAll(filepath.Dir(LogFilePath), 0o755); err == nil {
		if f, err := os.OpenFile(LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			defer f.Close()
			file = bufio.NewWriterSize(f, 64*1024)
		} else {
			fmt.Fprintf(os.Stderr, "Erreur ouverture logger.log : %v\n", err)
		}
	}

	write := func(msg string) {
		mode := s.Mode()

		if mode == OutputStdout || mode == OutputBoth {
			stdout.WriteString(msg)
			stdout.WriteByte('\n')
			if stdout.Buffered() >= 32*1024 {
				stdout.Flush()
			}
		}

		if file != nil && (mode == OutputFile || mode == OutputBoth) {
			file.WriteString(msg)
			file.WriteByte('\n')
			if file.Buffered() >= 32*1024 {
				file.Flush()
			}
		}
	}

	flush := func() {
		if stdout.Buffered() > 0 {
			stdout.Flush()
		}
		if file != nil && file.Buffered() > 0 {
			file.Flush()
		}
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case msg := <-s.ch:
			write(msg)

		case <-ticker.C:
			flush()

		case <-ctx.Done():
			for {
				select {
				case msg := <-s.ch:
					write(msg)
				default:
					flush()
					return
				}
			}
		}
	}
}
