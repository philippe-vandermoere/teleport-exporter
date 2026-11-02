// Package teleport wraps the Teleport API client: it authenticates with an
// identity file (typically renewed by tbot) and fetches the cluster's
// inventory of nodes, Kubernetes clusters, databases and applications.
package teleport

import (
	"context"
	"fmt"
	"sync"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/defaults"
)

// Node is an SSH node registered in Teleport.
type Node struct {
	Name     string // UUID
	Hostname string
	Address  string
	Version  string
}

// Agent is a Teleport agent serving a Kubernetes cluster, database or
// application.
type Agent struct {
	NodeName string // UUID
	Version  string
}

// KubeCluster is a Kubernetes cluster registered in Teleport.
type KubeCluster struct {
	Name   string
	Agents []Agent
}

// Database is a database registered in Teleport.
type Database struct {
	Name     string
	Protocol string
	Type     string
	Agents   []Agent
}

// App is an application registered in Teleport.
type App struct {
	Name       string
	PublicAddr string
	URI        string
	Agents     []Agent
}

// Inventory is everything one collection fetches from a Teleport cluster.
// Kubernetes clusters, databases and apps are deduplicated by name: several
// agents may serve the same resource (HA), each with its own server entry,
// listed in the resource's Agents.
type Inventory struct {
	ClusterName  string
	Nodes        []Node
	KubeClusters []KubeCluster
	Databases    []Database
	Apps         []App
}

// Client fetches an Inventory from a single Teleport cluster. The
// underlying API connection is dialed lazily on the first Fetch (and again
// after a failed dial), so the exporter can start before tbot has written
// the identity file.
type Client struct {
	address      string
	identityFile string
	insecure     bool

	mu    sync.Mutex
	clt   *client.Client
	creds *client.DynamicIdentityFileCreds
}

// NewClient returns a Client for the Teleport proxy/auth server at address
// (host:port), authenticated with the identity file at identityFile.
// insecure skips TLS verification of the proxy's web certificate.
func NewClient(address, identityFile string, insecure bool) *Client {
	return &Client{address: address, identityFile: identityFile, insecure: insecure}
}

// Fetch reloads the identity file (tbot rotates the short-lived
// certificate on disk, and DynamicIdentityFileCreds deliberately never
// reloads on its own), then fetches the full inventory. Fetch is not meant
// to be called concurrently.
func (c *Client) Fetch(ctx context.Context) (Inventory, error) {
	clt, err := c.connect(ctx)
	if err != nil {
		return Inventory{}, err
	}

	var inv Inventory

	cn, err := clt.GetClusterName(ctx)
	if err != nil {
		return Inventory{}, fmt.Errorf("getting cluster name: %w", err)
	}
	inv.ClusterName = cn.GetClusterName()

	nodes, err := clt.GetNodes(ctx, defaults.Namespace)
	if err != nil {
		return Inventory{}, fmt.Errorf("listing nodes: %w", err)
	}
	for _, n := range nodes {
		inv.Nodes = append(inv.Nodes, Node{
			Name:     n.GetName(),
			Hostname: n.GetHostname(),
			Address:  n.GetAddr(),
			Version:  n.GetTeleportVersion(),
		})
	}

	kubeServers, err := clt.GetKubernetesServers(ctx)
	if err != nil {
		return Inventory{}, fmt.Errorf("listing kubernetes servers: %w", err)
	}
	index := make(map[string]int)
	for _, s := range kubeServers {
		kc := s.GetCluster()
		if kc == nil {
			continue
		}
		i, ok := index[kc.GetName()]
		if !ok {
			i = len(inv.KubeClusters)
			index[kc.GetName()] = i
			inv.KubeClusters = append(inv.KubeClusters, KubeCluster{Name: kc.GetName()})
		}
		inv.KubeClusters[i].Agents = append(inv.KubeClusters[i].Agents, agentOf(s))
	}

	dbServers, err := clt.GetDatabaseServers(ctx, defaults.Namespace)
	if err != nil {
		return Inventory{}, fmt.Errorf("listing database servers: %w", err)
	}
	index = make(map[string]int)
	for _, s := range dbServers {
		db := s.GetDatabase()
		if db == nil {
			continue
		}
		i, ok := index[db.GetName()]
		if !ok {
			i = len(inv.Databases)
			index[db.GetName()] = i
			inv.Databases = append(inv.Databases, Database{Name: db.GetName(), Protocol: db.GetProtocol(), Type: db.GetType()})
		}
		inv.Databases[i].Agents = append(inv.Databases[i].Agents, agentOf(s))
	}

	appServers, err := clt.GetApplicationServers(ctx, defaults.Namespace)
	if err != nil {
		return Inventory{}, fmt.Errorf("listing application servers: %w", err)
	}
	index = make(map[string]int)
	for _, s := range appServers {
		app := s.GetApp()
		if app == nil {
			continue
		}
		i, ok := index[app.GetName()]
		if !ok {
			i = len(inv.Apps)
			index[app.GetName()] = i
			inv.Apps = append(inv.Apps, App{Name: app.GetName(), PublicAddr: app.GetPublicAddr(), URI: app.GetURI()})
		}
		inv.Apps[i].Agents = append(inv.Apps[i].Agents, agentOf(s))
	}

	return inv, nil
}

// agentOf returns the agent behind a Kubernetes, database or application
// server entry.
func agentOf(s interface {
	GetHostID() string
	GetTeleportVersion() string
}) Agent {
	return Agent{NodeName: s.GetHostID(), Version: s.GetTeleportVersion()}
}

// connect returns the API client, dialing it first if needed, with the
// identity file freshly reloaded.
func (c *Client) connect(ctx context.Context) (*client.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.clt != nil {
		if err := c.creds.Reload(); err != nil {
			return nil, fmt.Errorf("reloading identity file %s: %w", c.identityFile, err)
		}
		return c.clt, nil
	}

	creds, err := client.NewDynamicIdentityFileCreds(c.identityFile)
	if err != nil {
		return nil, fmt.Errorf("loading identity file %s: %w", c.identityFile, err)
	}
	clt, err := client.New(ctx, client.Config{
		Addrs:                    []string{c.address},
		Credentials:              []client.Credentials{creds},
		InsecureAddressDiscovery: c.insecure,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", c.address, err)
	}
	c.clt, c.creds = clt, creds
	return clt, nil
}

// Close closes the API connection, if one was ever established.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.clt == nil {
		return nil
	}
	err := c.clt.Close()
	c.clt, c.creds = nil, nil
	return err
}
