package meta

import (
	"strings"

	"github.com/nyaruka/gocommon/urns"
)

func IsFacebookRef(u urns.URN) bool {
	return u.Scheme() == urns.Facebook.Prefix && strings.HasPrefix(u.Path(), urns.FacebookRefPrefix)
}

// FacebookRef returns the facebook referral portion of our path, this return empty string in the case where we aren't a Facebook scheme
func FacebookRef(u urns.URN) string {
	if IsFacebookRef(u) {
		return strings.TrimPrefix(u.Path(), urns.FacebookRefPrefix)
	}
	return ""
}
