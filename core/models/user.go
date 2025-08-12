package models

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type UserID int

// GetSystemUserID gets the system user to use for contact audit fields
func GetSystemUserID(ctx context.Context, db *sqlx.DB) (UserID, error) {
	var id UserID

	if err := db.GetContext(ctx, &id, "SELECT id FROM users_user WHERE email = 'system'"); err != nil {
		return 0, fmt.Errorf("error looking up system user: %w", err)
	}
	return id, nil
}
