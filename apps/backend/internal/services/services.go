package services

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Config struct {
}
type Services struct {
	Github Github
	K8s    client.Client
}

func New(cfg Config, k8s client.Client) (Services, error) {
	github, err := NewGithub()
	if err != nil {
		return Services{}, err
	}

	return Services{
		Github: github,
		K8s:    k8s,
	}, nil
}
