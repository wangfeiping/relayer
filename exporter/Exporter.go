package exporter

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var collector *relayerCollector

func init() {
	collector = &relayerCollector{
		serviceStatusDesc: prometheus.NewDesc(
			"client_update_status",
			"Status of relayer client update ",
			[]string{"time", "chain_id"}, nil),
		mapper: make(map[string]*callRecord)}
}

type callRecord struct {
	status int
	time   string
}

type relayerCollector struct {
	serviceStatusDesc *prometheus.Desc

	mapper map[string]*callRecord
	mux    sync.RWMutex
}

// Collector returns a collector
// which exports metrics about status code of network service response
func Collector() prometheus.Collector {
	return collector
}

// Describe returns all descriptions of the collector.
func (c *relayerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.serviceStatusDesc
}

// Collect returns the current state of all metrics of the collector.
func (c *relayerCollector) Collect(ch chan<- prometheus.Metric) {
	c.mux.RLock()
	defer c.mux.RUnlock()

	for chainID, record := range c.mapper {
		ch <- prometheus.MustNewConstMetric(
			c.serviceStatusDesc,
			prometheus.GaugeValue,
			float64(record.status), record.time, chainID)
	}
}

func (c *relayerCollector) setStatusCode(
	status int, time string, chainID string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.mapper[chainID] = &callRecord{
		status: status,
		time:   time}
}

// SetStatusCode set status to the collector mapper
func SetStatusCode(status int, time string, chainID string) {
	collector.setStatusCode(status, time, chainID)
}
