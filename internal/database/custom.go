package database

import (
	"context"
)

// InstanceIpInUse returns true if any non-deleted instance already has the given IP address.
func (q *Queries) InstanceIpInUse(ctx context.Context, ip string) (bool, error) {
	row := q.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM instances WHERE ip_address = $1 AND deleted_at IS NULL)", ip)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
