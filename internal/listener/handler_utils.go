package listener

import (
	"context"
	"fmt"

	"github.com/jackc/pglogrepl"
	"google.golang.org/protobuf/proto"
)

// HandlerFunc defines the function signature for domain-specific event handlers
type HandlerFunc func(ctx context.Context, id string) error

// GenericHandler handles common PostgreSQL replication parsing and routing
func (s *Service) GenericHandler(
	ctx context.Context,
	tuple *pglogrepl.TupleData,
	operation string,
	relation *pglogrepl.RelationMessageV2,
	tableName string,
	createdHandler HandlerFunc,
	updatedHandler HandlerFunc,
) error {
	if tuple == nil {
		return fmt.Errorf("tuple is nil for %s change", tableName)
	}

	// Extract data from tuple
	data, err := s.extractTupleData(tuple, relation)
	if err != nil {
		return fmt.Errorf("failed to extract %s data: %w", tableName, err)
	}

	// Route to appropriate handler based on operation
	switch operation {
	case "INSERT":
		return createdHandler(ctx, data["id"])
	case "UPDATE":
		return updatedHandler(ctx, data["id"])
	default:
		s.logger.Debug("Ignoring operation", "table", tableName, "operation", operation, "id", data["id"])
		return nil
	}
}

// PublishEvent publishes a protobuf event to NATS
func (s *Service) PublishEvent(eventName string, message proto.Message, entityID string) error {
	data, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", eventName, err)
	}

	if err := s.natsClient.Publish(eventName, data); err != nil {
		return fmt.Errorf("failed to publish %s: %w", eventName, err)
	}

	s.logger.Info("Published event", "event", eventName, "entity_id", entityID)
	return nil
}

// extractTupleData extracts data from a PostgreSQL tuple (generic version)
func (s *Service) extractTupleData(tuple *pglogrepl.TupleData, relation *pglogrepl.RelationMessageV2) (map[string]string, error) {
	data := make(map[string]string)

	for i, col := range tuple.Columns {
		if i >= len(relation.Columns) {
			continue
		}

		colName := relation.Columns[i].Name
		var value string

		if col.Data != nil {
			value = string(col.Data)
		}

		data[colName] = value
	}

	return data, nil
}
