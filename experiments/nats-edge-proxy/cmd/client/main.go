package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"
	"zeitfun/pkg/db"
	db2 "zeitfun/pkg/db/db"
	"zeitfun/pkg/nats"

	"github.com/google/uuid"
	"github.com/lmittmann/tint"
	"github.com/samber/do/v2"
	"github.com/urfave/cli/v2"

	"github.com/rodaine/table"
	"github.com/samber/lo"
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

	db := do.MustInvoke[*db.DB](injector)
	nats := do.MustInvoke[*nats.NATS](injector)
	defer nats.Close()

	app := &cli.App{
		Name:  "client",
		Usage: "HTTP Proxy management client",
		Commands: []*cli.Command{
			{
				Name:  "httpproxy",
				Usage: "Manage HTTP proxies",
				Subcommands: []*cli.Command{
					{
						Name:  "list",
						Usage: "List all HTTP proxies",
						Action: func(c *cli.Context) error {
							proxies, err := db.HttpProxyFindAll(c.Context)
							if err != nil {
								return err
							}

							tbl := table.New("ID", "FQDN", "# Endpoints", "# Healthy")
							for _, proxy := range proxies {
								endpoints, err := db.HttpProxyEndpointFindByProxyId(c.Context, proxy.ID)
								if err != nil {
									return err
								}

								healthy := 0
								for _, endpoint := range endpoints {
									if endpoint.Healthy {
										healthy++
									}
								}

								tbl.AddRow(proxy.ID, proxy.Fqdn, len(endpoints), healthy)
							}
							tbl.Print()

							return nil
						},
					},
					{
						Name:  "endpoint",
						Usage: "Manage endpoints for a specific HTTP proxy",
						Subcommands: []*cli.Command{
							{
								Name:  "list",
								Usage: "List endpoints for the specified proxy",
								Flags: []cli.Flag{
									&cli.StringFlag{Name: "proxy-id", Aliases: []string{"p"}, Required: true},
								},
								Action: func(c *cli.Context) error {
									id := lo.Must(uuid.Parse(c.String("proxy-id")))

									endpoints, err := db.HttpProxyEndpointFindByProxyId(c.Context, id)
									if err != nil {
										return err
									}

									tbl := table.New("ID", "Endpoint", "Healthy")
									for _, endpoint := range endpoints {
										tbl.AddRow(endpoint.ID, endpoint.Endpoint, endpoint.Healthy)
									}
									tbl.Print()

									return nil
								},
							},
							{
								Name:  "set",
								Usage: "Set an endpoint to the specified proxy",
								Flags: []cli.Flag{
									&cli.StringFlag{Name: "proxy-id", Aliases: []string{"p"}, Required: true},
									&cli.StringFlag{Name: "endpoint", Required: true},
									&cli.BoolFlag{Name: "healthy", Required: true},
								},
								Action: func(c *cli.Context) error {
									id := lo.Must(uuid.Parse(c.String("proxy-id")))

									endpoint, err := db.HttpProxyEndpointUpsert(c.Context, db2.HttpProxyEndpointUpsertParams{
										HttpProxyID: id,
										Endpoint:    c.String("endpoint"),
										Healthy:     c.Bool("healthy"),
									})
									if err != nil {
										return err
									}

									// nats stuff
									err = nats.Publish(fmt.Sprintf("http_proxy.%s.%s", endpoint.HttpProxyID.String(), id.String()), endpoint.HttpProxyID[:])
									if err != nil {
										return err
									}

									return nil
								},
							},
							{
								Name:  "delete",
								Usage: "Delete an endpoint",
								Flags: []cli.Flag{
									&cli.StringFlag{Name: "endpoint-id", Aliases: []string{"id"}, Required: true},
								},
								Action: func(c *cli.Context) error {
									id := lo.Must(uuid.Parse(c.String("endpoint-id")))

									endpoint, err := db.HttpProxyEndpointDelete(c.Context, id)
									if err != nil {
										return err
									}

									// nats stuff
									err = nats.Publish(fmt.Sprintf("http_proxy.%s.%s", endpoint.HttpProxyID.String(), id.String()), endpoint.HttpProxyID[:])
									if err != nil {
										return err
									}

									return nil
								},
							},
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
