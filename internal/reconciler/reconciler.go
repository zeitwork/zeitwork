package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/zeitwork/zeitwork/internal/shared/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type ReconcileFunc func(ctx context.Context, objectID uuid.UUID) error

type Scheduler struct {
	name          string
	mu            sync.RWMutex
	wg            sync.WaitGroup
	schedule      map[uuid.UUID]time.Time
	dueRun        chan uuid.UUID
	reconcileFunc ReconcileFunc
}

func New(reconcileFunc ReconcileFunc) *Scheduler {
	return &Scheduler{
		name:          "unknown",
		mu:            sync.RWMutex{},
		wg:            sync.WaitGroup{},
		schedule:      make(map[uuid.UUID]time.Time),
		dueRun:        make(chan uuid.UUID),
		reconcileFunc: reconcileFunc,
	}
}

func NewWithName(name string, reconcileFunc ReconcileFunc) *Scheduler {
	return &Scheduler{
		name:          name,
		mu:            sync.RWMutex{},
		wg:            sync.WaitGroup{},
		schedule:      make(map[uuid.UUID]time.Time),
		dueRun:        make(chan uuid.UUID),
		reconcileFunc: reconcileFunc,
	}
}

func (s *Scheduler) Schedule(objectID uuid.UUID, nextRun time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.schedule[objectID] = nextRun
}

func (s *Scheduler) Start() {
	for i := 0; i <= 5; i++ {
		s.wg.Go(s.worker)
	}

	// run scheduler
	go func() {
		timer := time.NewTicker(1 * time.Second)
		for {
			<-timer.C
			now := time.Now()

			// Collect due items while holding lock (fast operation)
			var dueItems []uuid.UUID
			s.mu.Lock()
			for objectID, nextRun := range s.schedule {
				if !nextRun.IsZero() && nextRun.Before(now) {
					dueItems = append(dueItems, objectID)
					s.schedule[objectID] = time.Time{}
				}
			}
			s.mu.Unlock()

			// Send to channel without holding lock (may block, but won't deadlock)
			for _, objectID := range dueItems {
				s.dueRun <- objectID
			}
		}
	}()
}

func (s *Scheduler) worker() {
	defer s.wg.Done()

	tracer := otel.Tracer("reconciler")
	logger := slog.With("reconciler_name", s.name).With("reconcile_id", uuid.New().String())

	for {
		id := <-s.dueRun

		logger = logger.With("reconcile_object_id", id)

		ctx := context.WithValue(context.Background(), "reconcile_object_id", id)
		ctx = context.WithValue(ctx, "reconciler_name", s.name)

		ctx, span := tracer.Start(ctx, fmt.Sprintf("reconcile_%s_%s", s.name, id.String()))
		span.SetAttributes(attribute.String("reconcile_object_id", id.String()), attribute.String("reconciler", s.name))
		logger.InfoContext(ctx, "running reconcile")

		err := s.reconcileFunc(ctx, id)
		if err != nil {
			logger.ErrorContext(ctx, "reconcile failed", "id", id, "err", err)
			s.Schedule(id, time.Now().Add(5*time.Second))
			continue
		}

		logger.InfoContext(ctx, "reconcile done", "id", id)
		span.End()

		// Only apply the 1-hour default if the reconcile function
		// did not already schedule a custom next-run time.
		s.mu.RLock()
		alreadyScheduled := !s.schedule[id].IsZero()
		s.mu.RUnlock()
		if !alreadyScheduled {
			s.Schedule(id, time.Now().Add(1*time.Hour))
		}
	}
}
