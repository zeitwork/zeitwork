package nats

import "github.com/nats-io/nats.go"
import "github.com/samber/do/v2"

type NATS struct {
	*nats.Conn
}

func NewNATS(i do.Injector) (*NATS, error) {
	conn, err := nats.Connect("localhost:4222")
	if err != nil {
		return nil, err
	}

	return &NATS{
		Conn: conn,
	}, nil
}
