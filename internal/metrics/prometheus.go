package metrics

import (
	"fmt"
	"net/http"
	_ "net/http/pprof" // for profiling
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/tjhop/mango/internal/config"
)

const (
	defaultPrometheusPort = 9555
)

func init() {
	// expose build info metric
	promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "mango_build_info",
			Help: "A metric with a constant '1' value with labels for version, commit and build_date from which mango was built.",
			ConstLabels: prometheus.Labels{
				"version":    config.Version,
				"commit":     config.Commit,
				"build_date": config.BuildDate,
				"goversion":  runtime.Version(),
			},
		},
		func() float64 { return 1 },
	)
}

// ExportPrometheusMetrics sets up our HTTP server for prometheus metrics at
// the configured `interfac:port`. Designed to be run as a goroutine from main.
func ExportPrometheusMetrics() {
	http.Handle("/metrics", promhttp.Handler())

	viper.SetDefault("metrics.port", defaultPrometheusPort)
	iface := viper.GetString("metrics.interface")
	port := viper.GetInt("metrics.port")

	log.Panic(http.ListenAndServe(fmt.Sprintf("%s:%d", iface, port), nil))
}
