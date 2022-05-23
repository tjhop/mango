package mango

import (
	"path/filepath"
	"sync"

	_ "github.com/tjhop/mango/internal/logging"

	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	// prometheus metrics
	metricMangoWatcherEventsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "mango_mango_watcher_events_total",
			Help: "Total number of filesystem events detected within mango tree",
		},
	)

	metricMangoWatcherErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "mango_mango_watcher_errors_total",
			Help: "Total number of errors received watching for filesystem events within mango tree",
		},
	)
)

func NewMangoWatcher(path string) {
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

		go func() {
			for {
				select {
				case event := <-watcher.Events:
					if IsMangoExtValid(event.Name) {
						log.WithFields(log.Fields{
							"path":  path,
							"event": event,
						}).Debug("Filesystem event received in mango tree directory, reloading mango tree")

						metricMangoWatcherEventsTotal.Inc()

						ReloadTree(path)
					}

				case err := <-watcher.Errors:
					log.WithFields(log.Fields{
						"error": err,
					}).Error("Failed to handle event from fsnotify watcher")

					metricMangoWatcherErrorsTotal.Inc()
				}
			}
		}()

		err = watcher.Add(path)
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"path":  path,
			}).Error("Failed to add mango tree directory to mango watcher")
		}
		wg.Wait()
	}()
}
