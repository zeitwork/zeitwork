package local

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zeitwork/zeitwork/internal/certmanager/types"
	"github.com/zeitwork/zeitwork/internal/database"
	"github.com/zeitwork/zeitwork/internal/shared/config"
	shareduuid "github.com/zeitwork/zeitwork/internal/shared/uuid"
)

type LocalRuntime struct {
	logger  *slog.Logger
	config  *config.CertManagerConfig
	db      *pgxpool.Pool
	queries *database.Queries
}

func NewLocalRuntime(cfg *config.CertManagerConfig, logger *slog.Logger) (types.Runtime, error) {
	// Reuse DB connection per runtime
	dbConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}
	db, err := pgxpool.NewWithConfig(context.Background(), dbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := db.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &LocalRuntime{
		logger:  logger,
		config:  cfg,
		db:      db,
		queries: database.New(db),
	}, nil
}

func (r *LocalRuntime) Name() string { return "local" }

func (r *LocalRuntime) Cleanup() error {
	if r.db != nil {
		r.db.Close()
	}
	return nil
}

func (r *LocalRuntime) EnsureCertificate(ctx context.Context, name string, isWildcard bool) error {
	keyName := fmt.Sprintf("certs/%s", name)

	// Lookup existing and validate expiry window
	existing, err := r.queries.SslCertsGetByKey(ctx, keyName)
	if err == nil && existing != nil {
		cert, _ := tls.X509KeyPair([]byte(existing.Value), []byte(existing.Value))
		if len(cert.Certificate) > 0 {
			if parsed, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
				if time.Until(parsed.NotAfter) > r.config.RenewBefore {
					return nil
				}
			}
		}
	}

	// Generate new self-signed
	pemBundle, notAfter, err := generateSelfSignedPEM(name)
	if err != nil {
		return err
	}
	// Upsert row
	expires := pgtype.Timestamptz{}
	_ = expires.Scan(notAfter)
	_, err = r.queries.SslCertsUpsert(ctx, &database.SslCertsUpsertParams{
		ID:        shareduuid.GeneratePgUUID(),
		Key:       keyName,
		Value:     pemBundle,
		ExpiresAt: expires,
	})
	return err
}

// generateSelfSignedPEM returns a combined PEM (cert+key) and NotAfter.
func generateSelfSignedPEM(host string) (string, time.Time, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", time.Time{}, err
	}

	now := time.Now()
	notAfter := now.Add(90 * 24 * time.Hour)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return "", time.Time{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    now,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if strings.HasPrefix(host, "*.") {
		template.DNSNames = []string{host}
	} else {
		template.DNSNames = []string{host}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", time.Time{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return string(append(certPEM, keyPEM...)), notAfter, nil
}
