package reconciler

import (
	"context"
	"log/slog"
	"math"
	"time"
	natsService "zeitfun/pkg/nats"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/samber/do/v2"
)

type Result struct {
	RescheduleAfter time.Duration
}
type Func func(context.Context, uuid.UUID) (*Result, error)
type FindAllIDsFunc func(context.Context) ([]uuid.UUID, error)

type Reconciler struct {
	// todo: this should use a much better data structure.
	scheduledReconciliations map[uuid.UUID]time.Time
	exponentialBackoff       map[uuid.UUID]int
	reconcilerFunc           Func
	findAllIDsFunc           FindAllIDsFunc
	NATS                     *natsService.NATS
	topic                    string
}

func NewReconciler(i do.Injector, f Func, find FindAllIDsFunc, topic string) (*Reconciler, error) {
	r := &Reconciler{
		scheduledReconciliations: make(map[uuid.UUID]time.Time),
		exponentialBackoff:       make(map[uuid.UUID]int),
		reconcilerFunc:           f,
		findAllIDsFunc:           find,
		NATS:                     do.MustInvoke[*natsService.NATS](i),
		topic:                    topic,
	}

	return r, nil
}

func (r *Reconciler) AddScheduledReconciliation(uuid uuid.UUID, time time.Time) {
	r.scheduledReconciliations[uuid] = time
}

func (r *Reconciler) popIfDue() uuid.UUID {
	for id, at := range r.scheduledReconciliations {
		if time.Now().After(at) {
			delete(r.scheduledReconciliations, id)
			return id
		}
	}

	return uuid.UUID{}
}

func (r *Reconciler) GoRun() error {
	// use the findAllIDs function to schedule all rows
	ids, err := r.findAllIDsFunc(context.Background())
	if err != nil {
		return err
	}

	for _, id := range ids {
		r.AddScheduledReconciliation(id, time.Now())
	}

	r.subscribeToTopic()

	go func() {
		for {
			reconciliationId := r.popIfDue()
			if reconciliationId == uuid.Nil {
				time.Sleep(1 * time.Second)
				continue
			}

			slog.Debug("Starting reconciliation", "id", reconciliationId)

			res, err := r.reconcilerFunc(context.Background(), reconciliationId)
			slog.Debug("Finished reconciliation", "id", reconciliationId, "res", res, "err", err)

			if err != nil {
				slog.Error("Reconciler failed", "id", reconciliationId, "err", err)
				r.exponentialBackoff[reconciliationId]++

				// retry after 10s
				r.scheduledReconciliations[reconciliationId] = time.Now().Add(time.Duration(min(200*math.Pow(2, float64(r.exponentialBackoff[reconciliationId])), 60_000)) * time.Millisecond)
				continue
			}

			delete(r.exponentialBackoff, reconciliationId)

			if res != nil {
				// if a result is returned, but no RescheduleAfter, we retry immediately.
				if res.RescheduleAfter == 0 {
					r.scheduledReconciliations[reconciliationId] = time.Now()
				} else {
					r.scheduledReconciliations[reconciliationId] = time.Now().Add(res.RescheduleAfter)
				}
				continue
			}

			// if no res and no error, it went good. Retry in 6h
			r.scheduledReconciliations[reconciliationId] = time.Now().Add(6 * time.Hour)
		}
	}()

	return nil
}

func (r *Reconciler) subscribeToTopic() {
	// todo: retry nd stuff
	_, err := r.NATS.Subscribe(r.topic, func(msg *nats.Msg) {
		r.AddScheduledReconciliation(uuid.UUID(msg.Data), time.Now())
	})
	if err != nil {
		panic(err)
	}

	slog.Info("Successfully Subscribed to topic", "topic", r.topic)
}
