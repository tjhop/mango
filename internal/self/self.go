package self

import (
	"os"
	"os/user"
	"strconv"

	log "github.com/sirupsen/logrus"
)

// GetHostname is a wrapper around `os.Hostname()`. Since the hostname is how
// Mango determines what configurations are applicable to the running system,
// the hostname is critical. It returns the hostname as a string if successful,
// and exits fatally if it fails.
func GetHostname() string {
	h, err := os.Hostname()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Failed to get hostname! Mango cannot determine the system's identity and is unable to determine what configurations are applicable.")
	}

	return h
}

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
