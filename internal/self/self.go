package self

import (
	"os/user"
	"runtime"
	"strconv"

	log "github.com/sirupsen/logrus"
)


// GetCurrentUserInfo returns, in order, the following information about the
// user that launched the `mango` daemon:
// - user name
// - user ID
// - group name
// - group ID
// It exits fatally if any of the calls involved to get user info fail.
func GetCurrentUserInfo() (username string, uid int, group string, gid int) {
	u, err := user.Current()
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Fatal("Failed to lookup current user")
	}

	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Fatal("Failed to lookup current group")
	}

	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Fatal("Failed to convert UID from string to int")
	}

	gid, err = strconv.Atoi(g.Gid)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Fatal("Failed to convert GID from string to int")
	}

	return u.Username, uid, g.Name, gid
}

// GetRuntimeVersion is a convenience wrapper for `runtime.Version()` to get
// the version from the go runtime
func GetRuntimeVersion() string {
	return runtime.Version()
}
