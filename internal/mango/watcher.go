package mango

import (
	"path/filepath"
	"sync"

	_ "github.com/tjhop/mango/internal/logging"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func NewMangoWatcher() {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Failed to watch mango tree for changes")
		}
		defer watcher.Close()

		mTree := filepath.Clean(viper.GetString("mango.tree"))

		go func() {
			for {
				select {
				case event := <-watcher.Events:
					log.WithFields(log.Fields{
						"path": mTree,
						"event": event,
					}).Debug("Filesystem event received in mango tree directory, reloading mango tree")

					ReloadTree(mTree)

				case err := <-watcher.Errors:
					log.WithFields(log.Fields{
						"error": err,
					}).Error("Failed to handle event from fsnotify watcher")
				}
			}
		}()

		err = watcher.Add(mTree)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"path": mTree,
			}).Error("Failed to add mango tree directory to mango watcher")
		}
		wg.Wait()
	}()
}
