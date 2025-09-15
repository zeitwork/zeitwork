package listener

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zeitwork/zeitwork/internal/shared/config"
	"github.com/zeitwork/zeitwork/internal/shared/nats"
)

// Service represents the listener service that streams PostgreSQL changes to NATS
type Service struct {
	logger                *slog.Logger
	config                *config.ListenerConfig
	natsClient            *nats.Client
	pgConn                *pgconn.PgConn
	relations             map[uint32]*pglogrepl.RelationMessageV2
	typeMap               *pgtype.Map
	inStream              bool
	clientXLogPos         pglogrepl.LSN
	standbyMessageTimeout time.Duration
}

// NewService creates a new listener service
func NewService(cfg *config.ListenerConfig, logger *slog.Logger) (*Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	// Set defaults
	if cfg.ReplicationSlotName == "" {
		cfg.ReplicationSlotName = "zeitwork_listener"
	}
	if cfg.PublicationName == "" {
		cfg.PublicationName = "zeitwork_changes"
	}
	if cfg.StandbyTimeout == 0 {
		cfg.StandbyTimeout = 10 * time.Second
	}
	if len(cfg.PluginArgs) == 0 {
		cfg.PluginArgs = []string{
			"proto_version '2'",
			fmt.Sprintf("publication_names '%s'", cfg.PublicationName),
			"messages 'true'",
			"streaming 'true'",
		}
	}

	// Create NATS client
	natsClient, err := nats.NewClient(cfg.NATS, "listener")
	if err != nil {
		return nil, fmt.Errorf("failed to create NATS client: %w", err)
	}

	return &Service{
		logger:                logger,
		config:                cfg,
		natsClient:            natsClient,
		relations:             make(map[uint32]*pglogrepl.RelationMessageV2),
		typeMap:               pgtype.NewMap(),
		standbyMessageTimeout: cfg.StandbyTimeout,
	}, nil
}

// Start starts the listener service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting listener service")

	// Connect to PostgreSQL with replication mode
	// Parse the existing URL and add replication parameter properly
	replicationURL := s.buildReplicationURL(s.config.DatabaseURL)
	conn, err := pgconn.Connect(ctx, replicationURL)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	s.pgConn = conn

	// Setup publication and replication slot
	if err := s.setupReplication(ctx); err != nil {
		return fmt.Errorf("failed to setup replication: %w", err)
	}

	// Start replication
	if err := s.startReplication(ctx); err != nil {
		return fmt.Errorf("failed to start replication: %w", err)
	}

	// Start the main replication loop
	return s.replicationLoop(ctx)
}

// setupReplication creates the publication and replication slot
func (s *Service) setupReplication(ctx context.Context) error {
	s.logger.Info("Setting up PostgreSQL replication",
		"publication", s.config.PublicationName,
		"slot", s.config.ReplicationSlotName)

	// Create publication for relevant tables
	createPubSQL := fmt.Sprintf(`
		CREATE PUBLICATION %s FOR TABLE
			projects,
			image_builds,
			deployments, 
			domains, 
			instances, 
			deployment_instances,
			ssl_certs
		`, s.config.PublicationName)

	// Drop and recreate publication (for development)
	dropPubSQL := fmt.Sprintf("DROP PUBLICATION IF EXISTS %s", s.config.PublicationName)

	result := s.pgConn.Exec(ctx, dropPubSQL)
	_, err := result.ReadAll()
	if err != nil {
		s.logger.Warn("Failed to drop existing publication", "error", err)
	}

	result = s.pgConn.Exec(ctx, createPubSQL)
	_, err = result.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to create publication: %w", err)
	}

	s.logger.Info("Created publication", "name", s.config.PublicationName)

	// Identify system
	sysident, err := pglogrepl.IdentifySystem(ctx, s.pgConn)
	if err != nil {
		return fmt.Errorf("failed to identify system: %w", err)
	}

	s.logger.Info("PostgreSQL system identified",
		"system_id", sysident.SystemID,
		"timeline", sysident.Timeline,
		"xlog_pos", sysident.XLogPos,
		"db_name", sysident.DBName)

	s.clientXLogPos = sysident.XLogPos

	// Create temporary replication slot
	_, err = pglogrepl.CreateReplicationSlot(
		ctx,
		s.pgConn,
		s.config.ReplicationSlotName,
		"pgoutput",
		pglogrepl.CreateReplicationSlotOptions{Temporary: true},
	)
	if err != nil {
		return fmt.Errorf("failed to create replication slot: %w", err)
	}

	s.logger.Info("Created replication slot", "name", s.config.ReplicationSlotName)
	return nil
}

// startReplication starts the logical replication
func (s *Service) startReplication(ctx context.Context) error {
	err := pglogrepl.StartReplication(
		ctx,
		s.pgConn,
		s.config.ReplicationSlotName,
		s.clientXLogPos,
		pglogrepl.StartReplicationOptions{
			PluginArgs: s.config.PluginArgs,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to start replication: %w", err)
	}

	s.logger.Info("Logical replication started", "slot", s.config.ReplicationSlotName)
	return nil
}

// replicationLoop is the main loop that processes replication messages
func (s *Service) replicationLoop(ctx context.Context) error {
	nextStandbyMessageDeadline := time.Now().Add(s.standbyMessageTimeout)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Replication loop stopped by context")
			return ctx.Err()
		default:
		}

		// Send standby status update if needed
		if time.Now().After(nextStandbyMessageDeadline) {
			err := pglogrepl.SendStandbyStatusUpdate(
				ctx,
				s.pgConn,
				pglogrepl.StandbyStatusUpdate{WALWritePosition: s.clientXLogPos},
			)
			if err != nil {
				s.logger.Error("Failed to send standby status update", "error", err)
				return fmt.Errorf("failed to send standby status update: %w", err)
			}
			s.logger.Debug("Sent standby status message", "wal_pos", s.clientXLogPos.String())
			nextStandbyMessageDeadline = time.Now().Add(s.standbyMessageTimeout)
		}

		// Receive message with timeout
		msgCtx, cancel := context.WithDeadline(ctx, nextStandbyMessageDeadline)
		rawMsg, err := s.pgConn.ReceiveMessage(msgCtx)
		cancel()

		if err != nil {
			if pgconn.Timeout(err) {
				continue
			}
			return fmt.Errorf("failed to receive message: %w", err)
		}

		// Handle error messages
		if errMsg, ok := rawMsg.(*pgproto3.ErrorResponse); ok {
			return fmt.Errorf("received PostgreSQL error: %+v", errMsg)
		}

		// Process copy data messages
		msg, ok := rawMsg.(*pgproto3.CopyData)
		if !ok {
			s.logger.Debug("Received unexpected message type", "type", fmt.Sprintf("%T", rawMsg))
			continue
		}

		if err := s.processCopyData(ctx, msg); err != nil {
			s.logger.Error("Failed to process copy data", "error", err)
			// Continue processing instead of failing completely
		}
	}
}

// processCopyData processes a copy data message from PostgreSQL
func (s *Service) processCopyData(ctx context.Context, msg *pgproto3.CopyData) error {
	switch msg.Data[0] {
	case pglogrepl.PrimaryKeepaliveMessageByteID:
		return s.handleKeepalive(msg.Data[1:])
	case pglogrepl.XLogDataByteID:
		return s.handleXLogData(ctx, msg.Data[1:])
	default:
		s.logger.Debug("Unknown message type", "byte_id", msg.Data[0])
		return nil
	}
}

// handleKeepalive handles keepalive messages from PostgreSQL
func (s *Service) handleKeepalive(data []byte) error {
	pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(data)
	if err != nil {
		return fmt.Errorf("failed to parse keepalive message: %w", err)
	}

	s.logger.Debug("Received keepalive",
		"server_wal_end", pkm.ServerWALEnd,
		"server_time", pkm.ServerTime,
		"reply_requested", pkm.ReplyRequested)

	if pkm.ServerWALEnd > s.clientXLogPos {
		s.clientXLogPos = pkm.ServerWALEnd
	}

	return nil
}

// handleXLogData handles WAL data messages
func (s *Service) handleXLogData(ctx context.Context, data []byte) error {
	xld, err := pglogrepl.ParseXLogData(data)
	if err != nil {
		return fmt.Errorf("failed to parse XLog data: %w", err)
	}

	s.logger.Debug("Received XLog data",
		"wal_start", xld.WALStart,
		"server_wal_end", xld.ServerWALEnd,
		"server_time", xld.ServerTime)

	if err := s.processLogicalMessage(ctx, xld.WALData); err != nil {
		return fmt.Errorf("failed to process logical message: %w", err)
	}

	if xld.WALStart > s.clientXLogPos {
		s.clientXLogPos = xld.WALStart
	}

	return nil
}

// processLogicalMessage processes a logical replication message
func (s *Service) processLogicalMessage(ctx context.Context, walData []byte) error {
	logicalMsg, err := pglogrepl.ParseV2(walData, s.inStream)
	if err != nil {
		return fmt.Errorf("failed to parse logical message: %w", err)
	}

	s.logger.Debug("Received logical message", "type", logicalMsg.Type())

	switch msg := logicalMsg.(type) {
	case *pglogrepl.RelationMessageV2:
		s.relations[msg.RelationID] = msg
		s.logger.Debug("Stored relation", "id", msg.RelationID, "name", msg.RelationName)

	case *pglogrepl.BeginMessage:
		s.logger.Debug("Transaction begin", "xid", msg.Xid)

	case *pglogrepl.CommitMessage:
		s.logger.Debug("Transaction commit")

	case *pglogrepl.InsertMessageV2:
		return s.handleInsert(ctx, msg)

	case *pglogrepl.UpdateMessageV2:
		return s.handleUpdate(ctx, msg)

	case *pglogrepl.DeleteMessageV2:
		return s.handleDelete(ctx, msg)

	case *pglogrepl.StreamStartMessageV2:
		s.inStream = true
		s.logger.Debug("Stream start", "xid", msg.Xid)

	case *pglogrepl.StreamStopMessageV2:
		s.inStream = false
		s.logger.Debug("Stream stop")

	case *pglogrepl.StreamCommitMessageV2:
		s.logger.Debug("Stream commit", "xid", msg.Xid)

	default:
		s.logger.Debug("Unhandled message type", "type", fmt.Sprintf("%T", msg))
	}

	return nil
}

// handleInsert handles INSERT operations
func (s *Service) handleInsert(ctx context.Context, msg *pglogrepl.InsertMessageV2) error {
	return s.handleDMLOperation(ctx, msg.RelationID, msg.Tuple, "INSERT")
}

// handleUpdate handles UPDATE operations
func (s *Service) handleUpdate(ctx context.Context, msg *pglogrepl.UpdateMessageV2) error {
	return s.handleDMLOperation(ctx, msg.RelationID, msg.NewTuple, "UPDATE")
}

// handleDelete handles DELETE operations
func (s *Service) handleDelete(ctx context.Context, msg *pglogrepl.DeleteMessageV2) error {
	return s.handleDMLOperation(ctx, msg.RelationID, msg.OldTuple, "DELETE")
}

// buildReplicationURL properly adds the replication parameter to the database URL
func (s *Service) buildReplicationURL(dbURL string) string {
	// Parse the URL
	u, err := url.Parse(dbURL)
	if err != nil {
		// If parsing fails, fall back to simple string concatenation
		s.logger.Warn("Failed to parse database URL, using simple concatenation", "error", err)
		if strings.Contains(dbURL, "?") {
			return dbURL + "&replication=database"
		}
		return dbURL + "?replication=database"
	}

	// Add replication parameter to query string
	query := u.Query()
	query.Set("replication", "database")
	u.RawQuery = query.Encode()

	return u.String()
}

// Close closes the listener service
func (s *Service) Close() error {
	s.logger.Info("Closing listener service")

	if s.natsClient != nil {
		if err := s.natsClient.Close(); err != nil {
			s.logger.Error("Failed to close NATS client", "error", err)
		}
	}

	if s.pgConn != nil {
		if err := s.pgConn.Close(context.Background()); err != nil {
			s.logger.Error("Failed to close PostgreSQL connection", "error", err)
		}
	}

	return nil
}
