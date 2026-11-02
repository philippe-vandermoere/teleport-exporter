package collector

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/pvandermoere/teleport-exporter/internal/teleport"
)

// fakeSource returns inv, or err if set.
type fakeSource struct {
	inv teleport.Inventory
	err error
}

func (f *fakeSource) Fetch(ctx context.Context) (teleport.Inventory, error) {
	return f.inv, f.err
}

var testInventory = teleport.Inventory{
	ClusterName: "teleport.example.com",
	Nodes: []teleport.Node{
		{Name: "uuid-1", Hostname: "ssh-node-1", Address: "10.0.0.1:3022", Version: "18.10.0"},
		{Name: "uuid-2", Hostname: "ssh-node-2", Address: "10.0.0.2:3022", Version: "18.10.0"},
	},
	// k3s-1 and pg-1 are served by two agents (HA), one of them mid-upgrade:
	// one info series per agent, but each resource counted once.
	KubeClusters: []teleport.KubeCluster{
		{Name: "k3s-1", Agents: []teleport.Agent{{NodeName: "uuid-3", Version: "18.10.0"}, {NodeName: "uuid-4", Version: "18.9.2"}}},
		{Name: "k3s-2", Agents: []teleport.Agent{{NodeName: "uuid-5", Version: "18.10.0"}}},
	},
	Databases: []teleport.Database{
		{Name: "pg-1", Protocol: "postgres", Type: "self-hosted", Agents: []teleport.Agent{{NodeName: "uuid-6", Version: "18.10.0"}, {NodeName: "uuid-7", Version: "18.10.0"}}},
		{Name: "pg-2", Protocol: "postgres", Type: "rds", Agents: []teleport.Agent{{NodeName: "uuid-6", Version: "18.10.0"}}},
		{Name: "mysql-1", Protocol: "mysql", Type: "self-hosted", Agents: []teleport.Agent{{NodeName: "uuid-6", Version: "18.10.0"}}},
	},
	Apps: []teleport.App{
		{Name: "grafana", PublicAddr: "grafana.teleport.example.com", URI: "http://grafana:3000", Agents: []teleport.Agent{{NodeName: "uuid-8", Version: "18.10.0"}}},
	},
}

func newTestCollector(src Source) *Collector {
	return NewCollector(src, time.Minute, time.Second, "fallback.example.com", nil)
}

const inventoryMetrics = `
# HELP teleport_exporter_app_cluster_info Always 1; carries descriptive info about an application registered in Teleport and an agent serving it as labels.
# TYPE teleport_exporter_app_cluster_info gauge
teleport_exporter_app_cluster_info{app_name="grafana",cluster_name="teleport.example.com",node_name="uuid-8",public_addr="grafana.teleport.example.com",uri="http://grafana:3000",version="18.10.0"} 1
# HELP teleport_exporter_apps_total Number of applications registered in Teleport.
# TYPE teleport_exporter_apps_total gauge
teleport_exporter_apps_total{cluster_name="teleport.example.com"} 1
# HELP teleport_exporter_database_cluster_info Always 1; carries descriptive info about a database registered in Teleport and an agent serving it as labels.
# TYPE teleport_exporter_database_cluster_info gauge
teleport_exporter_database_cluster_info{cluster_name="teleport.example.com",database_name="mysql-1",node_name="uuid-6",protocol="mysql",type="self-hosted",version="18.10.0"} 1
teleport_exporter_database_cluster_info{cluster_name="teleport.example.com",database_name="pg-1",node_name="uuid-6",protocol="postgres",type="self-hosted",version="18.10.0"} 1
teleport_exporter_database_cluster_info{cluster_name="teleport.example.com",database_name="pg-1",node_name="uuid-7",protocol="postgres",type="self-hosted",version="18.10.0"} 1
teleport_exporter_database_cluster_info{cluster_name="teleport.example.com",database_name="pg-2",node_name="uuid-6",protocol="postgres",type="rds",version="18.10.0"} 1
# HELP teleport_exporter_databases_by_protocol_total Number of databases registered in Teleport, by protocol.
# TYPE teleport_exporter_databases_by_protocol_total gauge
teleport_exporter_databases_by_protocol_total{cluster_name="teleport.example.com",protocol="mysql"} 1
teleport_exporter_databases_by_protocol_total{cluster_name="teleport.example.com",protocol="postgres"} 2
# HELP teleport_exporter_databases_by_type_total Number of databases registered in Teleport, by type.
# TYPE teleport_exporter_databases_by_type_total gauge
teleport_exporter_databases_by_type_total{cluster_name="teleport.example.com",type="rds"} 1
teleport_exporter_databases_by_type_total{cluster_name="teleport.example.com",type="self-hosted"} 2
# HELP teleport_exporter_databases_total Number of databases registered in Teleport.
# TYPE teleport_exporter_databases_total gauge
teleport_exporter_databases_total{cluster_name="teleport.example.com"} 3
# HELP teleport_exporter_kubernetes_cluster_info Always 1; one series per Kubernetes cluster registered in Teleport and agent serving it.
# TYPE teleport_exporter_kubernetes_cluster_info gauge
teleport_exporter_kubernetes_cluster_info{cluster_name="teleport.example.com",kube_cluster_name="k3s-1",node_name="uuid-3",version="18.10.0"} 1
teleport_exporter_kubernetes_cluster_info{cluster_name="teleport.example.com",kube_cluster_name="k3s-1",node_name="uuid-4",version="18.9.2"} 1
teleport_exporter_kubernetes_cluster_info{cluster_name="teleport.example.com",kube_cluster_name="k3s-2",node_name="uuid-5",version="18.10.0"} 1
# HELP teleport_exporter_kubernetes_clusters_total Number of Kubernetes clusters registered in Teleport.
# TYPE teleport_exporter_kubernetes_clusters_total gauge
teleport_exporter_kubernetes_clusters_total{cluster_name="teleport.example.com"} 2
# HELP teleport_exporter_node_cluster_info Always 1; carries descriptive info about an SSH node registered in Teleport as labels.
# TYPE teleport_exporter_node_cluster_info gauge
teleport_exporter_node_cluster_info{address="10.0.0.1:3022",cluster_name="teleport.example.com",hostname="ssh-node-1",node_name="uuid-1",version="18.10.0"} 1
teleport_exporter_node_cluster_info{address="10.0.0.2:3022",cluster_name="teleport.example.com",hostname="ssh-node-2",node_name="uuid-2",version="18.10.0"} 1
# HELP teleport_exporter_nodes_total Number of SSH nodes registered in Teleport.
# TYPE teleport_exporter_nodes_total gauge
teleport_exporter_nodes_total{cluster_name="teleport.example.com"} 2
`

var inventoryMetricNames = []string{
	"teleport_exporter_app_cluster_info",
	"teleport_exporter_apps_total",
	"teleport_exporter_database_cluster_info",
	"teleport_exporter_databases_by_protocol_total",
	"teleport_exporter_databases_by_type_total",
	"teleport_exporter_databases_total",
	"teleport_exporter_kubernetes_cluster_info",
	"teleport_exporter_kubernetes_clusters_total",
	"teleport_exporter_node_cluster_info",
	"teleport_exporter_nodes_total",
}

func TestCollect_Success(t *testing.T) {
	c := newTestCollector(&fakeSource{inv: testInventory})
	c.Refresh(context.Background())

	if err := testutil.CollectAndCompare(c, strings.NewReader(inventoryMetrics), inventoryMetricNames...); err != nil {
		t.Fatal(err)
	}

	health := `
# HELP teleport_exporter_collect_errors_total Number of failed collections from Teleport since the exporter started.
# TYPE teleport_exporter_collect_errors_total counter
teleport_exporter_collect_errors_total{cluster_name="teleport.example.com"} 0
# HELP teleport_exporter_up Whether the last collection from Teleport succeeded (1) or not (0).
# TYPE teleport_exporter_up gauge
teleport_exporter_up 1
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(health), "teleport_exporter_up", "teleport_exporter_collect_errors_total"); err != nil {
		t.Fatal(err)
	}

	assertPresent(t, c, "teleport_exporter_collect_duration_seconds", "teleport_exporter_last_successful_collect_timestamp_seconds")
}

func TestCollect_BeforeFirstRefresh(t *testing.T) {
	c := newTestCollector(&fakeSource{inv: testInventory})

	want := `
# HELP teleport_exporter_collect_errors_total Number of failed collections from Teleport since the exporter started.
# TYPE teleport_exporter_collect_errors_total counter
teleport_exporter_collect_errors_total{cluster_name="fallback.example.com"} 0
# HELP teleport_exporter_up Whether the last collection from Teleport succeeded (1) or not (0).
# TYPE teleport_exporter_up gauge
teleport_exporter_up 0
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want)); err != nil {
		t.Fatal(err)
	}
}

func TestCollect_FailureBeforeAnySuccess(t *testing.T) {
	c := newTestCollector(&fakeSource{err: errors.New("identity file not found")})
	c.Refresh(context.Background())
	c.Refresh(context.Background())

	want := `
# HELP teleport_exporter_collect_errors_total Number of failed collections from Teleport since the exporter started.
# TYPE teleport_exporter_collect_errors_total counter
teleport_exporter_collect_errors_total{cluster_name="fallback.example.com"} 2
# HELP teleport_exporter_up Whether the last collection from Teleport succeeded (1) or not (0).
# TYPE teleport_exporter_up gauge
teleport_exporter_up 0
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want), "teleport_exporter_up", "teleport_exporter_collect_errors_total"); err != nil {
		t.Fatal(err)
	}
	assertPresent(t, c, "teleport_exporter_collect_duration_seconds")
	if n := testutil.CollectAndCount(c, inventoryMetricNames...); n != 0 {
		t.Errorf("got %d inventory series before any successful collection, want 0", n)
	}
	if n := testutil.CollectAndCount(c, "teleport_exporter_last_successful_collect_timestamp_seconds"); n != 0 {
		t.Errorf("got last_successful_collect before any success")
	}
}

func TestCollect_FailureKeepsLastInventory(t *testing.T) {
	src := &fakeSource{inv: testInventory}
	c := newTestCollector(src)
	c.Refresh(context.Background())

	src.err = errors.New("access denied")
	c.Refresh(context.Background())

	if err := testutil.CollectAndCompare(c, strings.NewReader(inventoryMetrics), inventoryMetricNames...); err != nil {
		t.Fatal(err)
	}
	want := `
# HELP teleport_exporter_collect_errors_total Number of failed collections from Teleport since the exporter started.
# TYPE teleport_exporter_collect_errors_total counter
teleport_exporter_collect_errors_total{cluster_name="teleport.example.com"} 1
# HELP teleport_exporter_up Whether the last collection from Teleport succeeded (1) or not (0).
# TYPE teleport_exporter_up gauge
teleport_exporter_up 0
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want), "teleport_exporter_up", "teleport_exporter_collect_errors_total"); err != nil {
		t.Fatal(err)
	}
}

func TestCollect_RemovedResourcesDisappear(t *testing.T) {
	src := &fakeSource{inv: testInventory}
	c := newTestCollector(src)
	c.Refresh(context.Background())

	src.inv = teleport.Inventory{ClusterName: "teleport.example.com", Nodes: testInventory.Nodes[:1]}
	c.Refresh(context.Background())

	if n := testutil.CollectAndCount(c, "teleport_exporter_node_cluster_info"); n != 1 {
		t.Errorf("node_cluster_info series = %d, want 1", n)
	}
	for _, name := range []string{"teleport_exporter_kubernetes_cluster_info", "teleport_exporter_database_cluster_info", "teleport_exporter_app_cluster_info", "teleport_exporter_databases_by_protocol_total"} {
		if n := testutil.CollectAndCount(c, name); n != 0 {
			t.Errorf("%s series = %d, want 0", name, n)
		}
	}
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	c := NewCollector(&fakeSource{inv: testInventory}, 10*time.Millisecond, time.Second, "fallback", nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	deadline := time.After(time.Second)
	for testutil.ToFloat64(prometheus.Collector(upOnly{c})) != 1 {
		select {
		case <-deadline:
			t.Fatal("first collection never happened")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

// upOnly exposes just teleport_exporter_up, for testutil.ToFloat64.
type upOnly struct{ c *Collector }

func (u upOnly) Describe(ch chan<- *prometheus.Desc) { ch <- upDesc }
func (u upOnly) Collect(ch chan<- prometheus.Metric) {
	all := make(chan prometheus.Metric, 64)
	go func() {
		u.c.Collect(all)
		close(all)
	}()
	for m := range all {
		if m.Desc() == upDesc {
			ch <- m
		}
	}
}

func assertPresent(t *testing.T, c prometheus.Collector, names ...string) {
	t.Helper()
	for _, name := range names {
		if n := testutil.CollectAndCount(c, name); n != 1 {
			t.Errorf("%s series = %d, want 1", name, n)
		}
	}
}
