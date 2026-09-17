package models

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/nyaruka/courier/v26/runtime"
	"github.com/vinovest/sqlx"
)

type UserID int
type UserUUID string

const NilUserID = UserID(0)

// User is a platform user as a message's sender is shown to the recipient - just the name and avatar a chat
// client renders. Loaded through the user cache created by Start.
type User struct {
	ID_        UserID         `db:"id"`
	UUID_      UserUUID       `db:"uuid"`
	FirstName_ string         `db:"first_name"`
	LastName_  string         `db:"last_name"`
	Avatar_    sql.NullString `db:"avatar"` // the path of the avatar in public storage, if they have one
}

func (u *User) ID() UserID     { return u.ID_ }
func (u *User) UUID() UserUUID { return u.UUID_ }

// Name returns the user's full name, as the platform shows it
func (u *User) Name() string {
	return strings.TrimSpace(u.FirstName_ + " " + u.LastName_)
}

// AvatarURL returns the URL of the user's avatar in public storage, or an empty string if they don't have one
func (u *User) AvatarURL(rt *runtime.Runtime) string {
	if !u.Avatar_.Valid || u.Avatar_.String == "" {
		return ""
	}
	return rt.S3.ObjectURL(rt.Config.S3PublicBucket, u.Avatar_.String)
}

const sqlSelectUser = `
SELECT id, uuid, first_name, last_name, avatar
  FROM users_user
 WHERE id = $1`

// a user that isn't there is loaded as a nil so the cache can hold the absence - see loadChannelByUUID
func loadUser(ctx context.Context, rt *runtime.Runtime, id UserID) (*User, error) {
	user := &User{}
	err := rt.DB.GetContext(ctx, user, sqlSelectUser, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUser returns the user with the given id, or nil if there's no such user. It reads through the user cache
// created by Start, which is why - like GetChannel - it doesn't take a runtime.
func GetUser(ctx context.Context, id UserID) (*User, error) {
	timeout, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	user, err := usersByID.GetOrFetch(timeout, id)
	if err != nil {
		return nil, fmt.Errorf("error looking up user #%d: %w", id, err)
	}
	return user, nil
}

// GetSystemUserID gets the system user to use for contact audit fields
func GetSystemUserID(ctx context.Context, db *sqlx.DB) (UserID, error) {
	var id UserID

	if err := db.GetContext(ctx, &id, "SELECT id FROM users_user WHERE email = 'system'"); err != nil {
		return 0, fmt.Errorf("error looking up system user: %w", err)
	}
	return id, nil
}
