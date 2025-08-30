package client

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log/slog"
	"zeitfun/proto"
)

func Start() {
	con, err := grpc.NewClient("localhost:8080", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("unable to create client", "err", err)
		return
	}

	// connect to grpc server
	client := proto.NewEdgeProxyAPIClient(con)

	slog.Info("Starting client and listening to changes")
	subscription, err := client.StreamChanges(context.Background(), &proto.StreamChangesRequest{})
	if err != nil {
		slog.Error("unable to listen msg", "err", err)
	}

	for {
		change, err := subscription.Recv()
		if err != nil {
			slog.Error("unable to listen msg", "err", err)
		}

		slog.Info("received change", "table", change.Table, "id", change.Id)
	}

}
