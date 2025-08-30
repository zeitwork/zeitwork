package edgeProxy

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/url"
	"sync"
	"zeitfun/internal/reconciler"
	"zeitfun/pkg/db"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/google/uuid"
	"github.com/samber/do/v2"
)

type Route struct {
	VHost    string
	Endpoint []*url.URL
}

type EdgeProxy struct {
	DB         *db.DB
	Reconciler *reconciler.Reconciler

	routesMu sync.RWMutex
	routes   map[uuid.UUID]Route
}

func New(i do.Injector) (*EdgeProxy, error) {
	var err error
	ep := &EdgeProxy{
		DB:       do.MustInvoke[*db.DB](i),
		routes:   map[uuid.UUID]Route{},
		routesMu: sync.RWMutex{},
	}

	ep.Reconciler, err = reconciler.NewReconciler(i, ep.ReconcileHTTPProxy, ep.DB.HttpProxyFindAllIDs, "http_proxies.>")
	if err != nil {
		slog.Error("error while trying to start reconciler", "err", err)
	}

	return ep, nil
}

func (ep *EdgeProxy) Start() {
	err := ep.Reconciler.GoRun()
	if err != nil {
		slog.Error("error while trying to start reconciler", "err", err)
		return
	}

	ep.StartServer()

	select {}
}

func (ep *EdgeProxy) ReconcileHTTPProxy(ctx context.Context, id uuid.UUID) (*reconciler.Result, error) {
	proxy, err := ep.DB.HttpProxyFindByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	slog.Info("Reconciling HTTPProxy", "id", id, "fqdn", proxy.Fqdn)

	// if the proxy was deleted, ensure it's no longer in our service map.
	if proxy.DeletedAt.Valid {
		delete(ep.routes, proxy.ID)
		return nil, nil
	}

	// find all available endpoints
	endpoints, err := ep.DB.HttpProxyEndpointFindByProxyId(ctx, proxy.ID)
	if err != nil {
		return nil, err
	}

	endpointURLs := make([]*url.URL, 0, len(endpoints))
	for _, endpoint := range endpoints {
		// if the endpoint is not healthy, skip it
		if !endpoint.Healthy {
			slog.Debug("Skipping unhealthy endpoint", "id", endpoint.ID)
			continue
		}

		// try to parse endpoint
		urlI, err := url.Parse(endpoint.Endpoint)
		if err != nil {
			slog.Error("Unable to parse endpoint", "url", endpoint.Endpoint)
			continue
		}

		endpointURLs = append(endpointURLs, urlI)
	}

	ep.routesMu.Lock()
	ep.routes[proxy.ID] = Route{
		VHost:    proxy.Fqdn,
		Endpoint: endpointURLs,
	}
	ep.routesMu.Unlock()

	return nil, nil
}

type HandlerWrapper struct {
	Name string `json:"handler"`
	*reverseproxy.Handler
}
