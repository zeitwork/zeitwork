package testsuite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"github.com/stretchr/testify/suite"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/edgeproxy"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
	"github.com/zeitwork/zeitwork/internal/zeitwork"
)

const Server2IP = "192.168.0.2"

var Server1ID = uuid.MustParse("019c58f1-6073-7c26-8b4d-d11e020aab50")
var Server1IP = "192.168.0.1"
var Server2ID = uuid.MustParse("019c58f1-6049-7bc7-8714-06883c6ba18a")

type Suite struct {
	suite.Suite
	DB  *database.DB
	Cfg Config
}

// TODO REMOVE DUPLICATES SHARED WITH cmd/zeitwork
type Config struct {
	IPAdress               string `env:"LOAD_BALANCER_IP,required"`
	DatabaseURL            string `env:"DATABASE_URL,required"`        // PgBouncer pooled connection (for queries)
	DatabaseDirectURL      string `env:"DATABASE_DIRECT_URL,required"` // Direct connection (for WAL replication)
	InternalIP             string `env:"INTERNAL_IP,required"`         // This server's VLAN IP
	DockerRegistryURL      string `env:"DOCKER_REGISTRY_URL,required"`
	DockerRegistryUsername string `env:"DOCKER_REGISTRY_USERNAME,required"`
	DockerRegistryPAT      string `env:"DOCKER_REGISTRY_PAT,required"` // GitHub PAT with write:packages scope
	GitHubAppID            string `env:"GITHUB_APP_ID"`
	GitHubAppPrivateKey    string `env:"GITHUB_APP_PRIVATE_KEY"` // base64-encoded

	// S3/MinIO for shared image storage (optional — only needed for multi-node)
	S3Endpoint  string `env:"S3_ENDPOINT"`
	S3Bucket    string `env:"S3_BUCKET"`
	S3AccessKey string `env:"S3_ACCESS_KEY"`
	S3SecretKey string `env:"S3_SECRET_KEY"`
	S3UseSSL    bool   `env:"S3_USE_SSL" envDefault:"false"`

	// Edge proxy config
	EdgeProxyHTTPAddr    string `env:"EDGEPROXY_HTTP_ADDR" envDefault:":80"`
	EdgeProxyHTTPSAddr   string `env:"EDGEPROXY_HTTPS_ADDR" envDefault:":443"`
	EdgeProxyACMEEmail   string `env:"EDGEPROXY_ACME_EMAIL" envDefault:"admin@zeitwork.com"`
	EdgeProxyACMEStaging bool   `env:"EDGEPROXY_ACME_STAGING" envDefault:"false"`
}

func NewSuite() Suite {
	logger := slog.New(tint.NewHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Parse configuration from environment variables
	err := godotenv.Load("/data/zeitwork.env")
	if err != nil {
		panic(err)
	}
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		panic("failed to parse config: " + err.Error())
	}

	// Initialize database (pooled connection via PgBouncer)
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		panic("failed to init database: " + err.Error())
	}

	return Suite{
		DB:  db,
		Cfg: cfg,
	}
}

func (s *Suite) SetupSuite() {
	c, err := s.DB.Pool.Acquire(s.Context())
	s.NoError(err)

	var isDev bool
	err = c.QueryRow(s.Context(), "select inet_client_addr() = '172.24.0.1'").Scan(&isDev)
	s.NoError(err)

	if !isDev {
		panic("It looks like you are running tests against a non docker db")
	}

	// ensure zeitwork is not currently running on either server
	s.RunCommand("systemctl", "stop", "zeitwork.service")
	s.RunCommandRemote(Server2IP, "systemctl", "stop", "zeitwork.service")

	// clean up VM image directories on both servers
	s.RunCommand("rm", "-rf", "/data/work/*", "/data/base/*")
	s.RunCommandRemote(Server2IP, "rm -rf /data/work/* /data/base/*")

	_, err = c.Exec(s.Context(), "truncate table vms cascade")
	s.NoError(err)

	// start zeitwork stuff
	// Load or create stable server identity
	serverID, err := zeitwork.LoadOrCreateServerID()
	s.NoError(err)

	routeChangeNotify := make(chan struct{}, 1)
	service, err := zeitwork.New(zeitwork.Config{
		DB:                     s.DB,
		IPAdress:               s.Cfg.IPAdress,
		DatabaseDirectURL:      s.Cfg.DatabaseDirectURL,
		InternalIP:             s.Cfg.InternalIP,
		ServerID:               serverID,
		RouteChangeNotify:      routeChangeNotify,
		DockerRegistryURL:      s.Cfg.DockerRegistryURL,
		DockerRegistryUsername: s.Cfg.DockerRegistryUsername,
		DockerRegistryPAT:      s.Cfg.DockerRegistryPAT,
		GitHubAppID:            s.Cfg.GitHubAppID,
		GitHubAppPrivateKey:    s.Cfg.GitHubAppPrivateKey,
	})
	if err != nil {
		panic(err)
	}

	go func() {
		err = service.Start(s.Context())
		if err != nil && !errors.Is(context.Canceled, err) {
			slog.Error("service error", "err", err)
		}
	}()

	edgeProxy, err := edgeproxy.NewService(edgeproxy.Config{
		HTTPAddr:          s.Cfg.EdgeProxyHTTPAddr,
		HTTPSAddr:         s.Cfg.EdgeProxyHTTPSAddr,
		ACMEEmail:         s.Cfg.EdgeProxyACMEEmail,
		ACMEStaging:       s.Cfg.EdgeProxyACMEStaging,
		DB:                s.DB,
		RouteChangeNotify: routeChangeNotify,
	}, slog.Default())
	if err != nil {
		slog.Error("failed to create edge proxy", "err", err)
	} else {
		go func() {
			if err := edgeProxy.Start(s.Context()); err != nil {
				slog.Error("edge proxy error", "err", err)
			}
		}()
	}

	time.Sleep(1 * time.Second)

	// Start zeitwork on server2 so it can reconcile VMs assigned to it
	s.RunCommandRemote(Server2IP, "systemctl", "start", "zeitwork.service")
}

func (s *Suite) Context() context.Context {
	return context.Background()
}

func (s *Suite) RunCommand(name string, args ...string) {
	output, err := s.TryRunCommand(name, args...)
	s.NoErrorf(err, "failed to run command: %s, %s", name, output)
}

func (s *Suite) TryRunCommand(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).CombinedOutput()
	return string(output), err
}

func (s *Suite) RunCommandRemote(remote string, command ...string) {
	output, err := s.TryRunCommandRemote(remote, command...)
	s.NoErrorf(err, "error while running command: %s", output)
}

func (s *Suite) TryRunCommandRemote(remote string, command ...string) (string, error) {
	args := append([]string{fmt.Sprintf("root@%s", remote)}, command...)
	output, err := exec.Command("ssh", args...).CombinedOutput()
	return string(output), err
}

func (s *Suite) WaitUntil(f func() bool) {
	ticker := time.NewTicker(time.Second)

	var success bool
	for i := 0; i < 60; i++ {
		<-ticker.C

		slog.Info("Running WaitUntil")
		success = f()

		if success {
			break
		}
	}
	s.Truef(success, "WaitUntil timed out")
}

type CreateVMArgs struct {
	Registry   string
	Repository string
	Tag        string
	Port       int32

	Server uuid.UUID
}

func (s *Suite) CreateVM(args CreateVMArgs) queries.Vm {
	var vm queries.Vm
	err := s.DB.WithTx(s.Context(), func(q *queries.Queries) error {
		server, err := q.ServerFindByID(s.Context(), args.Server)
		s.NoError(err)

		image, err := q.ImageFindOrCreate(s.Context(), queries.ImageFindOrCreateParams{
			ID:         uuid.New(),
			Registry:   args.Registry,
			Repository: args.Repository,
			Tag:        args.Tag,
		})
		s.NoError(err)

		nextIp, err := q.VMNextIPAddress(s.Context(), queries.VMNextIPAddressParams{
			IpRange:  server.IpRange,
			ServerID: server.ID,
		})
		s.NoError(err)

		vm, err = q.VMCreate(s.Context(), queries.VMCreateParams{
			ID:        uuid.New(),
			Vcpus:     1,
			Memory:    2048,
			ImageID:   image.ID,
			ServerID:  server.ID,
			IpAddress: nextIp,
			Port:      pgtype.Int4{Valid: true, Int32: args.Port},
			Status:    queries.VmStatusPending,
		})
		s.NoError(err)

		return nil
	})
	s.NoError(err)

	return vm
}

// DeleteVM soft-deletes a VM and waits for it to be cleaned up (status=failed, no longer pingable).
func (s *Suite) DeleteVM(vmID uuid.UUID) {
	err := s.DB.VMSoftDelete(s.Context(), vmID)
	s.NoError(err)

	s.WaitUntil(func() bool {
		vm, err := s.DB.VMFirstByID(s.Context(), vmID)
		s.NoError(err)
		if vm.Status != queries.VmStatusFailed {
			return false
		}
		_, err = s.TryRunCommand("ping", "-c", "1", "-W", "2", vm.IpAddress.Addr().String())
		return err != nil
	})
}
