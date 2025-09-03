package main

import (
	"github.com/lmittmann/tint"
	"github.com/samber/do/v2"
	"log/slog"
	"os"
	"time"
	"zeitfun/pkg/db"
	"zeitfun/pkg/edgeProxy"
	"zeitfun/pkg/nats"
)

func main() {
	slog.SetDefault(slog.New(
		tint.NewHandler(os.Stdout, &tint.Options{
			Level:      slog.LevelDebug,
			TimeFormat: time.Kitchen,
		}),
	))

	injector := do.New()

	do.Provide(injector, db.NewDB)
	do.Provide(injector, nats.NewNATS)
	do.Provide(injector, edgeProxy.New)

	do.MustInvoke[*edgeProxy.EdgeProxy](injector).Start()
}
