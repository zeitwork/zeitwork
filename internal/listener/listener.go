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
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

// Handler is called when a table change is detected
type Handler func(ctx context.Context, id uuid.UUID)

// Config holds the configuration for the listener
type Config struct {
	DatabaseURL         string
	ReplicationSlotName string
	PublicationName     string

	// Handlers for each table
	OnDeployment Handler
	OnBuild      Handler
	OnVM         Handler
	OnDomain     Handler
	OnServer     Handler
}

// Listener streams PostgreSQL WAL changes and dispatches to handlers
type Listener struct {
	config    Config
	pgConn    *pgconn.PgConn
	relations map[uint32]*pglogrepl.RelationMessageV2
	typeMap   *pgtype.Map

	clientXLogPos         pglogrepl.LSN
	standbyMessageTimeout time.Duration
	inStream              bool
}

// New creates a new WAL listener
func New(cfg Config) *Listener {
	// Set defaults
	if cfg.ReplicationSlotName == "" {
		cfg.ReplicationSlotName = "zeitwork_listener"
	}
	if cfg.PublicationName == "" {
		cfg.PublicationName = "zeitwork_changes"
	}

	return &Listener{
		config:                cfg,
		relations:             make(map[uint32]*pglogrepl.RelationMessageV2),
		typeMap:               pgtype.NewMap(),
		standbyMessageTimeout: 10 * time.Second,
	}
}

// Start starts the WAL listener and blocks until context is cancelled
func (l *Listener) Start(ctx context.Context) error {
	slog.Info("starting WAL listener")

	// Connect with replication mode
	replicationURL := l.buildReplicationURL(l.config.DatabaseURL)
	conn, err := pgconn.Connect(ctx, replicationURL)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL for replication: %w", err)
	}
	l.pgConn = conn

	defer func() {
		if l.pgConn != nil {
			l.pgConn.Close(context.Background())
		}
	}()

	// Setup publication and replication slot
	if err := l.setupReplication(ctx); err != nil {
		return fmt.Errorf("failed to setup replication: %w", err)
	}

	// Start replication
	if err := l.startReplication(ctx); err != nil {
		return fmt.Errorf("failed to start replication: %w", err)
	}

	// Run the main replication loop
	return l.replicationLoop(ctx)
}

// setupReplication creates the publication and replication slot
func (l *Listener) setupReplication(ctx context.Context) error {
	slog.Info("setting up PostgreSQL replication",
		"publication", l.config.PublicationName,
		"slot", l.config.ReplicationSlotName)

	// Create publication for relevant tables
	createPubSQL := fmt.Sprintf(`
		CREATE PUBLICATION %s FOR TABLE
			deployments,
			builds,
			images,
			vms,
			domains,
			servers
		`, l.config.PublicationName)

	// Drop and recreate publication (idempotent setup)
	dropPubSQL := fmt.Sprintf("DROP PUBLICATION IF EXISTS %s", l.config.PublicationName)

	result := l.pgConn.Exec(ctx, dropPubSQL)
	_, err := result.ReadAll()
	if err != nil {
		slog.Warn("failed to drop existing publication", "error", err)
	}

	result = l.pgConn.Exec(ctx, createPubSQL)
	_, err = result.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to create publication: %w", err)
	}

	slog.Info("created publication", "name", l.config.PublicationName)

	// Identify system to get current WAL position
	sysident, err := pglogrepl.IdentifySystem(ctx, l.pgConn)
	if err != nil {
		return fmt.Errorf("failed to identify system: %w", err)
	}

	slog.Info("PostgreSQL system identified",
		"system_id", sysident.SystemID,
		"timeline", sysident.Timeline,
		"xlog_pos", sysident.XLogPos,
		"db_name", sysident.DBName)

	l.clientXLogPos = sysident.XLogPos

	// Create temporary replication slot (auto-cleanup on disconnect)
	_, err = pglogrepl.CreateReplicationSlot(
		ctx,
		l.pgConn,
		l.config.ReplicationSlotName,
		"pgoutput",
		pglogrepl.CreateReplicationSlotOptions{Temporary: true},
	)
	if err != nil {
		return fmt.Errorf("failed to create replication slot: %w", err)
	}

	slog.Info("created replication slot", "name", l.config.ReplicationSlotName)
	return nil
}

// startReplication starts the logical replication stream
func (l *Listener) startReplication(ctx context.Context) error {
	pluginArgs := []string{
		"proto_version '2'",
		fmt.Sprintf("publication_names '%s'", l.config.PublicationName),
		"messages 'true'",
		"streaming 'true'",
	}

	err := pglogrepl.StartReplication(
		ctx,
		l.pgConn,
		l.config.ReplicationSlotName,
		l.clientXLogPos,
		pglogrepl.StartReplicationOptions{
			PluginArgs: pluginArgs,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to start replication: %w", err)
	}

	slog.Info("logical replication started", "slot", l.config.ReplicationSlotName)
	return nil
}

// replicationLoop processes WAL messages
func (l *Listener) replicationLoop(ctx context.Context) error {
	nextStandbyMessageDeadline := time.Now().Add(l.standbyMessageTimeout)

	for {
		select {
		case <-ctx.Done():
			slog.Info("WAL listener stopped by context")
			return ctx.Err()
		default:
		}

		// Send standby status update if needed
		if time.Now().After(nextStandbyMessageDeadline) {
			err := pglogrepl.SendStandbyStatusUpdate(
				ctx,
				l.pgConn,
				pglogrepl.StandbyStatusUpdate{WALWritePosition: l.clientXLogPos},
			)
			if err != nil {
				slog.Error("failed to send standby status update", "error", err)
				return fmt.Errorf("failed to send standby status update: %w", err)
			}
			//slog.Debug("sent standby status message", "wal_pos", l.clientXLogPos.String())
			nextStandbyMessageDeadline = time.Now().Add(l.standbyMessageTimeout)
		}

		// Receive message with timeout
		msgCtx, cancel := context.WithDeadline(ctx, nextStandbyMessageDeadline)
		rawMsg, err := l.pgConn.ReceiveMessage(msgCtx)
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
			slog.Debug("received unexpected message type", "type", fmt.Sprintf("%T", rawMsg))
			continue
		}

		if err := l.processCopyData(ctx, msg); err != nil {
			slog.Error("failed to process copy data", "error", err)
			// Continue processing instead of failing completely
		}
	}
}

// processCopyData handles copy data messages from PostgreSQL
func (l *Listener) processCopyData(ctx context.Context, msg *pgproto3.CopyData) error {
	switch msg.Data[0] {
	case pglogrepl.PrimaryKeepaliveMessageByteID:
		return l.handleKeepalive(msg.Data[1:])
	case pglogrepl.XLogDataByteID:
		return l.handleXLogData(ctx, msg.Data[1:])
	default:
		slog.Debug("unknown message type", "byte_id", msg.Data[0])
		return nil
	}
}

// handleKeepalive handles keepalive messages
func (l *Listener) handleKeepalive(data []byte) error {
	pkm, err := pglogrepl.ParsePrimaryKeepaliveMessage(data)
	if err != nil {
		return fmt.Errorf("failed to parse keepalive message: %w", err)
	}

	//slog.Debug("received keepalive",
	//	"server_wal_end", pkm.ServerWALEnd,
	//	"server_time", pkm.ServerTime,
	//	"reply_requested", pkm.ReplyRequested)

	if pkm.ServerWALEnd > l.clientXLogPos {
		l.clientXLogPos = pkm.ServerWALEnd
	}

	return nil
}

// handleXLogData handles WAL data messages
func (l *Listener) handleXLogData(ctx context.Context, data []byte) error {
	xld, err := pglogrepl.ParseXLogData(data)
	if err != nil {
		return fmt.Errorf("failed to parse XLog data: %w", err)
	}

	//slog.Debug("received XLog data",
	//	"wal_start", xld.WALStart,
	//	"server_wal_end", xld.ServerWALEnd,
	//	"server_time", xld.ServerTime)

	if err := l.processLogicalMessage(ctx, xld.WALData); err != nil {
		return fmt.Errorf("failed to process logical message: %w", err)
	}

	if xld.WALStart > l.clientXLogPos {
		l.clientXLogPos = xld.WALStart
	}

	return nil
}

// processLogicalMessage processes a logical replication message
func (l *Listener) processLogicalMessage(ctx context.Context, walData []byte) error {
	logicalMsg, err := pglogrepl.ParseV2(walData, l.inStream)
	if err != nil {
		return fmt.Errorf("failed to parse logical message: %w", err)
	}

	//slog.Debug("received logical message", "type", logicalMsg.Type())

	switch msg := logicalMsg.(type) {
	case *pglogrepl.RelationMessageV2:
		l.relations[msg.RelationID] = msg
		slog.Debug("stored relation", "id", msg.RelationID, "name", msg.RelationName)

	case *pglogrepl.BeginMessage:
		slog.Debug("transaction begin", "xid", msg.Xid)

	case *pglogrepl.CommitMessage:
		slog.Debug("transaction commit")

	case *pglogrepl.InsertMessageV2:
		return l.handleChange(ctx, msg.RelationID, msg.Tuple, "INSERT")

	case *pglogrepl.UpdateMessageV2:
		return l.handleChange(ctx, msg.RelationID, msg.NewTuple, "UPDATE")

	case *pglogrepl.DeleteMessageV2:
		// We use soft-delete, so DELETE events are not expected
		slog.Debug("ignoring DELETE (soft-delete used)")

	case *pglogrepl.StreamStartMessageV2:
		l.inStream = true
		slog.Debug("stream start", "xid", msg.Xid)

	case *pglogrepl.StreamStopMessageV2:
		l.inStream = false
		slog.Debug("stream stop")

	case *pglogrepl.StreamCommitMessageV2:
		slog.Debug("stream commit", "xid", msg.Xid)

	default:
		slog.Debug("unhandled message type", "type", fmt.Sprintf("%T", msg))
	}

	return nil
}

// handleChange handles INSERT and UPDATE operations
func (l *Listener) handleChange(ctx context.Context, relationID uint32, tuple *pglogrepl.TupleData, operation string) error {
	relation, ok := l.relations[relationID]
	if !ok {
		return fmt.Errorf("unknown relation ID %d", relationID)
	}

	if tuple == nil {
		return fmt.Errorf("tuple is nil for %s change", relation.RelationName)
	}

	// Extract the ID from the tuple
	id, err := l.extractID(tuple, relation)
	if err != nil {
		return fmt.Errorf("failed to extract ID from %s: %w", relation.RelationName, err)
	}

	//slog.Info("detected change",
	//	"table", relation.RelationName,
	//	"operation", operation,
	//	"id", id)

	// Dispatch to appropriate handler
	switch relation.RelationName {
	case "deployments":
		if l.config.OnDeployment != nil {
			l.config.OnDeployment(ctx, id)
		}
	case "builds":
		if l.config.OnBuild != nil {
			l.config.OnBuild(ctx, id)
		}
	case "vms":
		if l.config.OnVM != nil {
			l.config.OnVM(ctx, id)
		}
	case "domains":
		if l.config.OnDomain != nil {
			l.config.OnDomain(ctx, id)
		}
	case "servers":
		if l.config.OnServer != nil {
			l.config.OnServer(ctx, id)
		}
	default:
		slog.Debug("ignoring change for unhandled table", "table", relation.RelationName)
	}

	return nil
}

// extractID extracts the ID column from a tuple
func (l *Listener) extractID(tuple *pglogrepl.TupleData, relation *pglogrepl.RelationMessageV2) (uuid.UUID, error) {
	for i, col := range tuple.Columns {
		if i >= len(relation.Columns) {
			continue
		}

		colName := relation.Columns[i].Name
		if colName == "id" && col.Data != nil {
			return uuid.Parse(string(col.Data))
		}
	}

	return uuid.UUID{}, fmt.Errorf("id column not found in tuple")
}

// buildReplicationURL adds the replication parameter to the database URL
func (l *Listener) buildReplicationURL(dbURL string) string {
	u, err := url.Parse(dbURL)
	if err != nil {
		slog.Warn("failed to parse database URL, using simple concatenation", "error", err)
		if strings.Contains(dbURL, "?") {
			return dbURL + "&replication=database"
		}
		return dbURL + "?replication=database"
	}

	query := u.Query()
	query.Set("replication", "database")
	u.RawQuery = query.Encode()

	return u.String()
}
