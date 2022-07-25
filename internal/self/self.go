package self

import (
	"os"

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
