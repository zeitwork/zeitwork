package reconciler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgxlisten"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
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

func (s *Scheduler) SetupPGXListener(ctx context.Context, db *pgxpool.Pool, tableName string) {
	listener := &pgxlisten.Listener{
		Connect: func(ctx context.Context) (*pgx.Conn, error) {
			con, err := db.Acquire(ctx)
			if err != nil {
				return nil, err
			}
			return con.Conn(), err
		},
	}

	listener.Handle(tableName, pgxlisten.HandlerFunc(func(ctx context.Context, notification *pgconn.Notification, conn *pgx.Conn) error {
		// payload must be an uuid
		id, err := uuid.Parse(notification.Payload)
		if err != nil {
			return err
		}

		s.Schedule(id, time.Now())
		return nil
	}))

	go listener.Listen(ctx)
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

	logger := slog.With("reconciler_name", s.name)

	for {
		id := <-s.dueRun

		logger.Info("running reconcile for", "id", id)
		err := s.reconcileFunc(context.Background(), id)
		if err != nil {
			logger.Error("reconcile failed", "id", id, "err", err)
			s.Schedule(id, time.Now().Add(5*time.Second))
			continue
		}

		logger.Info("reconcile done", "id", id)
		s.Schedule(id, time.Now().Add(1*time.Hour))
	}
}
