package reconciler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ReconcileFunc func(ctx context.Context, objectID uuid.UUID) error

type Scheduler struct {
	mu            sync.RWMutex
	wg            sync.WaitGroup
	schedule      map[uuid.UUID]time.Time
	dueRun        chan uuid.UUID
	reconcileFunc ReconcileFunc
}

func NewScheduler(reconcileFunc ReconcileFunc) *Scheduler {
	return &Scheduler{
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
			s.mu.Lock()
			for objectID, nextRun := range s.schedule {
				if !nextRun.IsZero() && nextRun.Before(now) {
					s.dueRun <- objectID
					s.schedule[objectID] = time.Time{}
				}
			}
			s.mu.Unlock()
		}
	}()
}

func (s *Scheduler) worker() {
	defer s.wg.Done()

	for {
		id := <-s.dueRun

		slog.Info("Running reconcile for", "id", id)
		err := s.reconcileFunc(context.Background(), id)
		if err != nil {
			slog.Error("Reconcile failed", "id", id, "err", err)
			s.Schedule(id, time.Now().Add(5*time.Second))
			continue
		}

		slog.Info("Reconcile done", "id", id)
		s.Schedule(id, time.Now().Add(1*time.Hour))
	}
}
