package rapidpro

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type UserID int

// gets the system user to use for contact audit fields
func getSystemUserID(ctx context.Context, db *sqlx.DB) (UserID, error) {
	var id UserID
	err := db.GetContext(ctx, &id, "SELECT id FROM users_user WHERE email = 'system'")
	if err != nil {
		return 0, fmt.Errorf("error looking up system user: %w", err)
	}
	return id, nil
}
