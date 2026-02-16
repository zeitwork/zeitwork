package telemetry

import (
	"context"
	"log/slog"
	"os"

	"github.com/zeitwork/zeitwork/internal/shared/uuid"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if reconcileObjectID, ok := ctx.Value("reconcile_object_id").(uuid.UUID); ok {
		r.AddAttrs(slog.String("reconcile_object_id", reconcileObjectID.String()))
	}

	if reconcilerName, ok := ctx.Value("reconciler_name").(string); ok {
		r.AddAttrs(slog.String("reconcile_id", reconcilerName))
	}

	return h.Handler.Handle(ctx, r)
}
func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

func InitLogger(ctx context.Context) (func(ctx context.Context) error, slog.Handler, error) {
	if os.Getenv("OTEL_SERVICE_NAME") == "" {
		return func(ctx context.Context) error { return nil }, nil, nil
	}

	exporter, err := otlploghttp.New(ctx)
	if err != nil {
		panic(err)
	}

	// Reads OTEL_SERVICE_NAME from environment and adds host/process/OS attributes
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcess(),
	)
	if err != nil {
		return nil, nil, err
	}

	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(res),
	)

	handler := otelslog.NewHandler("zeitwork", otelslog.WithLoggerProvider(loggerProvider))
	wrappedHandler := contextHandler{handler}
	return loggerProvider.Shutdown, &wrappedHandler, nil
}
