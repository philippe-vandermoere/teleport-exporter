// Package collector implements the prometheus.Collector exposing the
// Teleport inventory. The inventory is fetched in the background every
// refresh interval (see Run), and every scrape is served from the last
// snapshot, so a Prometheus scrape never calls the Teleport API itself.
package collector

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/pvandermoere/teleport-exporter/internal/teleport"
)

const namespace = "teleport_exporter"

var (
	upDesc = prometheus.NewDesc(
		namespace+"_up",
		"Whether the last collection from Teleport succeeded (1) or not (0).",
		nil, nil,
	)
	nodesTotalDesc = prometheus.NewDesc(
		namespace+"_nodes_total",
		"Number of SSH nodes registered in Teleport.",
		[]string{"cluster_name"}, nil,
	)
	nodeClusterInfoDesc = prometheus.NewDesc(
		namespace+"_node_cluster_info",
		"Always 1; carries descriptive info about an SSH node registered in Teleport as labels.",
		[]string{"cluster_name", "node_name", "hostname", "address", "version"}, nil,
	)
	kubeClustersTotalDesc = prometheus.NewDesc(
		namespace+"_kubernetes_clusters_total",
		"Number of Kubernetes clusters registered in Teleport.",
		[]string{"cluster_name"}, nil,
	)
	kubeClusterInfoDesc = prometheus.NewDesc(
		namespace+"_kubernetes_cluster_info",
		"Always 1; one series per Kubernetes cluster registered in Teleport.",
		[]string{"cluster_name", "kube_cluster_name"}, nil,
	)
	databasesTotalDesc = prometheus.NewDesc(
		namespace+"_databases_total",
		"Number of databases registered in Teleport.",
		[]string{"cluster_name"}, nil,
	)
	databasesByProtocolDesc = prometheus.NewDesc(
		namespace+"_databases_by_protocol_total",
		"Number of databases registered in Teleport, by protocol.",
		[]string{"cluster_name", "protocol"}, nil,
	)
	databasesByTypeDesc = prometheus.NewDesc(
		namespace+"_databases_by_type_total",
		"Number of databases registered in Teleport, by type.",
		[]string{"cluster_name", "type"}, nil,
	)
	databaseClusterInfoDesc = prometheus.NewDesc(
		namespace+"_database_cluster_info",
		"Always 1; carries descriptive info about a database registered in Teleport as labels.",
		[]string{"cluster_name", "database_name", "protocol", "type"}, nil,
	)
	appsTotalDesc = prometheus.NewDesc(
		namespace+"_apps_total",
		"Number of applications registered in Teleport.",
		[]string{"cluster_name"}, nil,
	)
	appClusterInfoDesc = prometheus.NewDesc(
		namespace+"_app_cluster_info",
		"Always 1; carries descriptive info about an application registered in Teleport as labels.",
		[]string{"cluster_name", "app_name", "public_addr", "uri"}, nil,
	)
	collectDurationDesc = prometheus.NewDesc(
		namespace+"_collect_duration_seconds",
		"Duration of the last collection from Teleport, including failed attempts.",
		[]string{"cluster_name"}, nil,
	)
	collectErrorsDesc = prometheus.NewDesc(
		namespace+"_collect_errors_total",
		"Number of failed collections from Teleport since the exporter started.",
		[]string{"cluster_name"}, nil,
	)
	lastSuccessDesc = prometheus.NewDesc(
		namespace+"_last_successful_collect_timestamp_seconds",
		"Unix timestamp of the last successful collection from Teleport.",
		[]string{"cluster_name"}, nil,
	)
)

// Source fetches the Teleport inventory. Implemented by *teleport.Client.
type Source interface {
	Fetch(ctx context.Context) (teleport.Inventory, error)
}

// Collector refreshes the Teleport inventory in the background and exposes
// the last snapshot.
type Collector struct {
	source   Source
	interval time.Duration
	timeout  time.Duration
	logger   *slog.Logger

	mu sync.Mutex
	// clusterName labels the exporter health metrics. It starts as the
	// fallback given to NewCollector (the host part of the configured
	// address), and becomes the name Teleport reports after the first
	// successful collection.
	clusterName string
	// inventory is the last successful collection, nil until there is one.
	// A failed collection keeps the previous inventory rather than
	// publishing a partial one; up and last_successful_collect tell the
	// two apart.
	inventory   *teleport.Inventory
	up          bool
	duration    time.Duration
	errors      int
	lastSuccess time.Time
}

// NewCollector builds a Collector fetching from source every interval,
// each collection bounded by timeout. fallbackClusterName labels the health
// metrics until Teleport has reported its real cluster name. logger may be
// nil, in which case collection errors are discarded.
func NewCollector(source Source, interval, timeout time.Duration, fallbackClusterName string, logger *slog.Logger) *Collector {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Collector{source: source, interval: interval, timeout: timeout, clusterName: fallbackClusterName, logger: logger}
}

// Run collects once immediately, then every interval until ctx is done.
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		c.Refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Refresh performs a single collection and updates the snapshot.
func (c *Collector) Refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	start := time.Now()
	inv, err := c.source.Fetch(ctx)
	duration := time.Since(start)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.duration = duration
	if err != nil {
		if c.up || c.errors == 0 {
			c.logger.Warn("collection from Teleport failed", "error", err)
		} else {
			c.logger.Debug("collection from Teleport failed again", "error", err)
		}
		c.up = false
		c.errors++
		return
	}
	if !c.up {
		c.logger.Info("collection from Teleport succeeded", "cluster_name", inv.ClusterName,
			"nodes", len(inv.Nodes), "kubernetes_clusters", len(inv.KubeClusters),
			"databases", len(inv.Databases), "apps", len(inv.Apps))
	}
	c.up = true
	c.clusterName = inv.ClusterName
	c.inventory = &inv
	c.lastSuccess = time.Now()
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- upDesc
	ch <- nodesTotalDesc
	ch <- nodeClusterInfoDesc
	ch <- kubeClustersTotalDesc
	ch <- kubeClusterInfoDesc
	ch <- databasesTotalDesc
	ch <- databasesByProtocolDesc
	ch <- databasesByTypeDesc
	ch <- databaseClusterInfoDesc
	ch <- appsTotalDesc
	ch <- appClusterInfoDesc
	ch <- collectDurationDesc
	ch <- collectErrorsDesc
	ch <- lastSuccessDesc
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	up := 0.0
	if c.up {
		up = 1
	}
	ch <- prometheus.MustNewConstMetric(upDesc, prometheus.GaugeValue, up)
	ch <- prometheus.MustNewConstMetric(collectErrorsDesc, prometheus.CounterValue, float64(c.errors), c.clusterName)

	// Nothing has been attempted yet: a zero duration would be misleading.
	if c.up || c.errors > 0 {
		ch <- prometheus.MustNewConstMetric(collectDurationDesc, prometheus.GaugeValue, c.duration.Seconds(), c.clusterName)
	}

	inv := c.inventory
	if inv == nil {
		return
	}
	ch <- prometheus.MustNewConstMetric(lastSuccessDesc, prometheus.GaugeValue, float64(c.lastSuccess.Unix()), inv.ClusterName)
	collectInventory(ch, inv)
}

func collectInventory(ch chan<- prometheus.Metric, inv *teleport.Inventory) {
	cluster := inv.ClusterName

	ch <- prometheus.MustNewConstMetric(nodesTotalDesc, prometheus.GaugeValue, float64(len(inv.Nodes)), cluster)
	for _, n := range inv.Nodes {
		ch <- prometheus.MustNewConstMetric(nodeClusterInfoDesc, prometheus.GaugeValue, 1, cluster, n.Name, n.Hostname, n.Address, n.Version)
	}

	ch <- prometheus.MustNewConstMetric(kubeClustersTotalDesc, prometheus.GaugeValue, float64(len(inv.KubeClusters)), cluster)
	for _, k := range inv.KubeClusters {
		ch <- prometheus.MustNewConstMetric(kubeClusterInfoDesc, prometheus.GaugeValue, 1, cluster, k.Name)
	}

	ch <- prometheus.MustNewConstMetric(databasesTotalDesc, prometheus.GaugeValue, float64(len(inv.Databases)), cluster)
	byProtocol := make(map[string]int)
	byType := make(map[string]int)
	for _, db := range inv.Databases {
		byProtocol[db.Protocol]++
		byType[db.Type]++
		ch <- prometheus.MustNewConstMetric(databaseClusterInfoDesc, prometheus.GaugeValue, 1, cluster, db.Name, db.Protocol, db.Type)
	}
	for protocol, n := range byProtocol {
		ch <- prometheus.MustNewConstMetric(databasesByProtocolDesc, prometheus.GaugeValue, float64(n), cluster, protocol)
	}
	for typ, n := range byType {
		ch <- prometheus.MustNewConstMetric(databasesByTypeDesc, prometheus.GaugeValue, float64(n), cluster, typ)
	}

	ch <- prometheus.MustNewConstMetric(appsTotalDesc, prometheus.GaugeValue, float64(len(inv.Apps)), cluster)
	for _, a := range inv.Apps {
		ch <- prometheus.MustNewConstMetric(appClusterInfoDesc, prometheus.GaugeValue, 1, cluster, a.Name, a.PublicAddr, a.URI)
	}
}
