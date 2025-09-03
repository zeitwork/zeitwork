package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"zeitfun/proto"

	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type EdgeProxyAPI struct {
	proto.UnimplementedEdgeProxyAPIServer

	nc *nats.Conn
}

func (e *EdgeProxyAPI) GetHttpRoutes(ctx context.Context, req *proto.GetHttpRoutesRequest) (*proto.GetHttpRoutesResponse, error) {
	return &proto.GetHttpRoutesResponse{HttpRoutes: []*proto.HttpRoute{
		{
			Fqdn:     "app.dokedu.org" + req.NodeId,
			Endpoint: "localhost:8080",
		},
	}}, nil
}

func (e *EdgeProxyAPI) StreamChanges(req *proto.StreamChangesRequest, src grpc.ServerStreamingServer[proto.Change]) error {
	slog.Info("client connected")

	channel := make(chan *nats.Msg, 5)
	subscription, err := e.nc.ChanSubscribe("httproute.>", channel)
	if err != nil {
		return err
	}
	defer subscription.Unsubscribe()

	for {
		if !subscription.IsValid() {
			slog.Error("nats subscription is invalid")
			return errors.New("nats subscription expired")
		}

		select {
		case <-src.Context().Done():
			slog.Info("client disconnected")
			return nil
		case msg := <-channel:
			err = src.Send(&proto.Change{Table: msg.Subject, Id: string(msg.Data)})
			if err != nil {
				slog.Error("unable to send msg", "err", err)
				return err
			}
		}
	}
}

func (e *EdgeProxyAPI) UpsertHttpRoute(ctx context.Context, req *proto.UpsertHttpRouteRequest) (*proto.UpsertHttpRouteResponse, error) {
	slog.Info("Route was upserted.", "fqdn", req.HttpRoute.Fqdn, "endpoint", req.HttpRoute.Endpoint)

	err := e.nc.Publish("httproute.1", []byte(req.HttpRoute.Fqdn+req.HttpRoute.Endpoint))
	if err != nil {
		return nil, err
	}

	return &proto.UpsertHttpRouteResponse{}, nil
}

func Start() {
	natsUrl := "http://localhost:4222"
	slog.Info("Connecting to NATS", "url", natsUrl)

	nc, err := nats.Connect(natsUrl)
	if err != nil {
		slog.Error("unable to connect to nats!", "err", err)
		return
	}
	defer nc.Drain()

	s := grpc.NewServer()
	proto.RegisterEdgeProxyAPIServer(s, &EdgeProxyAPI{
		nc: nc,
	})
	reflection.Register(s)

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		slog.Error("failed to listen", "err", err)
		return
	}

	slog.Info("starting server", "addr", listener.Addr())
	err = s.Serve(listener)
	if err != nil {
		slog.Error("failed to serve", "err", err)
		return
	}
}
