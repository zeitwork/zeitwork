package main

import (
	"github.com/lmittmann/tint"
	"log/slog"
	"os"
	"time"
	"zeitfun/pkg/client"
)

func main() {
	slog.SetDefault(slog.New(
		tint.NewHandler(os.Stdout, &tint.Options{
			Level:      slog.LevelDebug,
			TimeFormat: time.Kitchen,
		}),
	))

	client.Start()
}
