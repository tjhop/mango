package mango

import (
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"golang.org/x/exp/slices"
)

var (
	once        sync.Once
	DefaultTree Tree

	mangoExts = []string{".mango", ".yaml", ".yml"}

	// prometheus metrics
	metricTreeTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mango_tree_total",
			Help: "Total number of mangoes found during the last load of the config tree",
		},
	)

	metricTreeReloadSeconds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mango_tree_reload_seconds",
			Help: "Unix timestamp of the last successful mango config tree reload",
		},
	)
)

func IsMangoExtValid(path string) bool {
	return slices.Contains(mangoExts, filepath.Ext(path))
}

type Mango struct {
	Config *viper.Viper

	ID string
}

// TODO: currently mangoes are more or less a wrapper around viper to provide
// easier support for reading arbritrary config structs This should be
// revisited soon. I dislike how implicitly and tightly coupled this is.
func NewMango(path string) Mango {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigName(filepath.Base(path))
	v.AddConfigPath(filepath.Dir(path))

	if err := v.ReadInConfig(); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to read mango configuration file")
	}

	m := Mango{
		ID:     path,
		Config: v,
	}

	return m
}

func (m Mango) String() string {
	return m.ID
}

type Tree struct {
	mangoes []Mango
}

func (t *Tree) AddMango(m Mango) {
	t.mangoes = append(t.mangoes, m)

	log.WithFields(log.Fields{
		"mango": m,
	}).Info("Added mango to tree")
}

func (t *Tree) Reload(tree string) {
	// stash old mangoes and clear list
	old := t.mangoes
	t.mangoes = nil

	err := filepath.WalkDir(tree,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !d.IsDir() && IsMangoExtValid(path) {
				mangoPath, err := filepath.Abs(path)
				if err != nil {
					return err
				}

				t.AddMango(NewMango(mangoPath))
			}

			return nil
		})

	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
			"tree":  tree,
		}).Error("Failed to reload mangoes for tree")

		// replace old list of mangoes, simce we failed to reload
		t.mangoes = old
	} else {
		metricTreeReloadSeconds.Set(float64(time.Now().Unix()))
	}

	metricTreeTotal.Set(float64(len(t.mangoes)))
}

func InitTree() {
	// on first load, do an initial search for all mangos in specified path
	once.Do(func() {
		DefaultTree.Reload(viper.GetString("mango.tree"))
	})
}

// GetCombinedMangoForThing will search all discovered mangoes for the requested thing type,
// collect the data from all mangoes, and merge it into a combined config map containing all
// of the things of the given type. Intended for consumption by individual Manager ipmlementations
// as they will need to refresh the list of things they manage periodically.
func GetCombinedMangoForThing(thingType string) Mango {
	v := viper.New()

	// TODO: handle dependencies/ordering/imports?
	// right now, this just squishes all the returned thing types back together
	for _, m := range DefaultTree.mangoes {
		thingData := map[string]any{
			thingType: m.Config.Get("things." + thingType),
		}

		v.MergeConfigMap(thingData)
	}

	m := Mango{
		ID:     "combined-" + thingType + "-thing",
		Config: v,
	}

	return m
}
