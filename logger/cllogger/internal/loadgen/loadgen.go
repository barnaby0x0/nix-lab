// Package loadgen génère des logs synthétiques en arrière-plan.
package loadgen

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// Emitter est la destination des logs générés.
type Emitter interface {
	Emit(level, message string)
}

// Generator lance et arrête des workers de génération de logs.
type Generator struct {
	emitter Emitter

	seq    atomic.Uint64
	active atomic.Int64

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// New crée un Generator qui écrit dans e.
func New(e Emitter) *Generator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Generator{emitter: e, ctx: ctx, cancel: cancel}
}

// Start lance un worker non bloquant qui émet count logs avec un délai delay entre chaque.
func (g *Generator) Start(count int, delay time.Duration) {
	g.mu.RLock()
	ctx := g.ctx
	g.mu.RUnlock()

	g.active.Add(1)
	go g.worker(ctx, count, delay)
}

// Reset stoppe tous les workers en cours mais permet d'en relancer ensuite.
func (g *Generator) Reset() {
	g.mu.Lock()
	g.cancel()
	g.ctx, g.cancel = context.WithCancel(context.Background())
	g.mu.Unlock()
}

// Stop stoppe tous les workers définitivement (arrêt du process).
func (g *Generator) Stop() {
	g.mu.Lock()
	g.cancel()
	g.mu.Unlock()
}

// ActiveWorkers renvoie le nombre de workers en cours.
func (g *Generator) ActiveWorkers() int64 { return g.active.Load() }

func (g *Generator) worker(ctx context.Context, count int, delay time.Duration) {
	defer g.active.Add(-1)

	// Numéro unique de cette goroutine
	workerID := g.seq.Add(1)

	for i := 1; i <= count; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		lvl := levels[rand.IntN(len(levels))]
		comp := components[rand.IntN(len(components))]
		msg := templates[rand.IntN(len(templates))]()

		g.emitter.Emit(lvl, fmt.Sprintf("[W-%d] [%d/%d] (%s) %s", workerID, i, count, comp, msg))

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
