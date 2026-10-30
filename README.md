# teleport-exporter

A Prometheus exporter for the inventory of a [Teleport](https://goteleport.com/)
cluster: SSH nodes, Kubernetes clusters, databases and applications, as
counts and as one `*_cluster_info` series per resource. It authenticates
with a Teleport identity file, typically written and renewed by a
[tbot](https://goteleport.com/docs/machine-workload-identity/) sidecar (see
the Helm chart under `charts/teleport-exporter/`).

- The inventory is collected **in the background** every
  `refresh_interval` (30s by default), and every scrape is served from the
  last collection: a Prometheus scrape never calls the Teleport API itself,
  however many Prometheus servers scrape the exporter.
- A collection is all-or-nothing: if any API call fails, the previous
  inventory stays published (`teleport_exporter_up 0` and
  `teleport_exporter_last_successful_collect_timestamp_seconds` tell you
  it's stale), never a partial one.
- The identity file is re-read before every collection, so a certificate
  renewed by tbot is picked up without restarting the exporter. It does not
  need to exist at startup either: until it does, collections simply fail
  (`teleport_exporter_up 0`).

## Metrics

`cluster_name` is the Teleport cluster name, as reported by Teleport.

| Metric | Type | Labels | Notes |
|---|---|---|---|
| `teleport_exporter_up` | gauge | — | 1 if the last collection succeeded, 0 otherwise (including before the first one). |
| `teleport_exporter_nodes_total` | gauge | `cluster_name` | SSH nodes registered in Teleport. |
| `teleport_exporter_node_cluster_info` | gauge | `cluster_name`, `node_name`, `hostname`, `address`, `version` | Always 1, one series per SSH node. `node_name` is the node's UUID, `version` its Teleport version. `address` is empty for nodes connected through a reverse tunnel (the default when a node joins via the proxy). |
| `teleport_exporter_kubernetes_clusters_total` | gauge | `cluster_name` | Kubernetes clusters registered in Teleport. |
| `teleport_exporter_kubernetes_cluster_info` | gauge | `cluster_name`, `kube_cluster_name` | Always 1, one series per Kubernetes cluster. |
| `teleport_exporter_databases_total` | gauge | `cluster_name` | Databases registered in Teleport. |
| `teleport_exporter_databases_by_protocol_total` | gauge | `cluster_name`, `protocol` | Databases by protocol (`postgres`, `mysql`, ...). |
| `teleport_exporter_databases_by_type_total` | gauge | `cluster_name`, `type` | Databases by type (`self-hosted`, `rds`, ...). |
| `teleport_exporter_database_cluster_info` | gauge | `cluster_name`, `database_name`, `protocol`, `type` | Always 1, one series per database. |
| `teleport_exporter_apps_total` | gauge | `cluster_name` | Applications registered in Teleport. |
| `teleport_exporter_app_cluster_info` | gauge | `cluster_name`, `app_name`, `public_addr`, `uri` | Always 1, one series per application. |
| `teleport_exporter_collect_duration_seconds` | gauge | `cluster_name` | Duration of the last collection, including failed ones. Absent before the first one. |
| `teleport_exporter_collect_errors_total` | counter | `cluster_name` | Failed collections since the exporter started. |
| `teleport_exporter_last_successful_collect_timestamp_seconds` | gauge | `cluster_name` | Unix timestamp of the last successful collection. Absent until there is one. |

Kubernetes clusters, databases and applications served by several agents
(high availability) are counted once, by name. The resource metrics are
absent until the first successful collection.

Until Teleport has reported its cluster name once (i.e. before the first
successful collection), the three `collect_*` health metrics use the host
part of the configured `address` as `cluster_name` instead.

Standard `process_*` metrics (CPU, memory, file descriptors) are also
exposed; Go runtime metrics (`go_*`) are not.

Compared to [giantswarm/teleport-exporter](https://github.com/giantswarm/teleport-exporter#metrics),
which this metric set started from, the Giant Swarm-specific heuristics
(`nodes_identified_total`, `nodes_unidentified_total`,
`nodes_by_kubernetes_cluster`, `kubernetes_management_clusters_total`,
`kubernetes_workload_clusters_total`) are not exported, and
`node_cluster_info`, `database_cluster_info` and `app_cluster_info` are
added alongside the existing `kubernetes_cluster_info`.

Suggested alerting (not shipped with this repository):
- Exporter can't reach Teleport: `teleport_exporter_up == 0` for 5 minutes.
- Stale data: `time() - teleport_exporter_last_successful_collect_timestamp_seconds > 600`.
- A resource disappeared: `absent_over_time(teleport_exporter_kubernetes_cluster_info{kube_cluster_name="prod"}[10m])`.

## Configuration

### YAML file

```yaml
address: teleport.example.com:443                    # required, Teleport proxy host:port
identity_file: /var/run/teleport-identity/identity   # required
listen_address: ":9180"    # optional, this is the default
refresh_interval: 30s      # optional, this is the default
collect_timeout: 20s       # optional, this is the default: bounds one whole collection
insecure: false            # optional: skip TLS verification of the proxy (self-signed test clusters only)
```

Pass the file with `--config /path/to/config.yaml`. Startup fails fast if
`address` is missing or isn't `host:port`, or `identity_file` is missing.
The identity file itself is not checked at startup (see above).

### Environment variables (container-friendly)

`--config` is optional. These variables apply whether or not a YAML file is
used, and always take precedence over the corresponding YAML value:

| Variable | Equivalent to |
|---|---|
| `TELEPORT_EXPORTER_CONFIG` | `--config` |
| `TELEPORT_EXPORTER_ADDRESS` | `address` |
| `TELEPORT_EXPORTER_IDENTITY_FILE` | `identity_file` |
| `TELEPORT_EXPORTER_LISTEN_ADDRESS` | `listen_address` |
| `TELEPORT_EXPORTER_REFRESH_INTERVAL` | `refresh_interval` |
| `TELEPORT_EXPORTER_COLLECT_TIMEOUT` | `collect_timeout` |
| `TELEPORT_EXPORTER_INSECURE` | `insecure` (`true`/`false`) |

## Creating the Teleport bot and join token

The exporter authenticates as a Teleport
[bot](https://goteleport.com/docs/machine-workload-identity/machine-id/introduction/)
with a read-only role. In Kubernetes, the chart's tbot sidecar joins with
the `kubernetes` join method, presenting a projected ServiceAccount token:
no long-lived secret is involved. Run the following with `tctl`, as a
Teleport user allowed to manage roles, bots and tokens.

### 1. The role

The role grants `list` and `read` on nodes, Kubernetes clusters,
databases, applications and the cluster name, and label matchers on all of
them (without those, the list calls succeed but return nothing). The
end-to-end test declares the same role in Terraform
(`test/e2e/teleport-resources.tf`).

```yaml
kind: role
version: v7
metadata:
  name: teleport-exporter
spec:
  allow:
    node_labels:
      "*": "*"
    kubernetes_labels:
      "*": "*"
    db_labels:
      "*": "*"
    app_labels:
      "*": "*"
    rules:
      - resources:
          - node
          - kube_server
          - kube_cluster
          - db_server
          - db
          - app_server
          - app
          - cluster_name
        verbs:
          - list
          - read
```

Save it as `role.yaml`, then:

```
tctl create -f role.yaml
```

### 2. The bot

```
tctl bots add teleport-exporter --roles=teleport-exporter
```

`tctl` prints a one-time join token: ignore it, the bot joins with the
`kubernetes` token created below instead.

### 3. The join token

Pick the token type matching how Teleport can verify the pod's
ServiceAccount token. Both are used with the same chart values.

**`static_jwks`**: Teleport can't reach the Kubernetes cluster's OIDC
issuer (typical for an on-premises cluster). Paste the cluster's
ServiceAccount signing keys into the token:

```
kubectl get --raw /openid/v1/jwks
```

then create the token with it:

```yaml
kind: token
version: v2
metadata:
  name: teleport-exporter-bot
spec:
  roles: [Bot]
  bot_name: teleport-exporter
  join_method: kubernetes
  kubernetes:
    type: static_jwks
    static_jwks:
      jwks: |
        {"keys":[...]}   # output of the kubectl command above
    allow:
      - service_account: "monitoring:teleport-exporter"   # <namespace>:<ServiceAccount>
```

Update the token whenever the cluster's signing keys rotate.

**`oidc`**: the cluster's ServiceAccount issuer is publicly reachable (EKS,
GKE, AKS, or a self-managed cluster publishing its OIDC discovery
document). Teleport fetches the signing keys itself, so key rotation needs
no update. Get the issuer:

```
kubectl get --raw /.well-known/openid-configuration | jq -r .issuer
```

then create the token with it:

```yaml
kind: token
version: v2
metadata:
  name: teleport-exporter-bot
spec:
  roles: [Bot]
  bot_name: teleport-exporter
  join_method: kubernetes
  kubernetes:
    type: oidc
    oidc:
      issuer: https://oidc.eks.eu-west-1.amazonaws.com/id/EXAMPLE   # output of the command above
    allow:
      - service_account: "monitoring:teleport-exporter"   # <namespace>:<ServiceAccount>
```

In both cases, create it with `tctl create -f <file>`. The `service_account`
is `<release namespace>:<ServiceAccount name>`; the chart's ServiceAccount
is named after the release (`teleport-exporter` for a release named
`teleport-exporter`, otherwise `<release>-teleport-exporter`), or
`serviceAccount.name` if set.

### 4. Installing the chart

```
helm install teleport-exporter oci://ghcr.io/philippe-vandermoere/charts/teleport-exporter \
  --version <version> --namespace monitoring \
  --set teleport.address=teleport.example.com:443 \
  --set tbot.token=teleport-exporter-bot
```

The projected token's audience must be the Teleport cluster name: it
defaults to the host part of `teleport.address`, set `teleport.clusterName`
if they differ (`tctl status` shows the cluster name). See the
[chart README](charts/teleport-exporter/README.md) for every value.

## Building

```
make build   # bin/teleport-exporter, static binary
make test    # go test ./... -race
make vet
```

## Docker image

```
make docker-build   # teleport-exporter:dev
```

The image is built `FROM scratch`: a static binary running as a non-root
numeric user (`65535:65535`), plus the CA certificate bundle (the Teleport
proxy usually presents a publicly trusted certificate, verified during
address discovery). No shell, no `/etc/passwd`.

## Testing

Unit tests (`internal/config`, `internal/collector`) don't need Docker:

```
go test ./... -race
```

`./test/e2e/run.sh` (`make e2e-test`) deploys a real Teleport cluster
with Terraform (`test/e2e/`, a single root module) and runs the exporter
against it, through this repository's Helm chart. It needs Docker,
Terraform and `kubectl`:

- a **k3d** cluster (`moio/k3d` provider) with a local registry, to which
  the exporter image built from the root Dockerfile is pushed;
- **Teleport** from the official `teleport-cluster` chart. Its auth
  service creates a `terraform` bot and join token at startup
  (`--apply-on-startup`), with which the Teleport Terraform provider joins
  natively (Machine ID, `kubernetes` join method), so no admin identity is
  ever extracted with `tctl`. The provider joins as soon as it is
  configured, so `run.sh` applies Teleport first (`-target`), then
  everything else;
- the exporter's role, bot and `static_jwks` join token, declared with the
  Teleport provider exactly as documented above (the JWKS is read from the
  cluster's `/openid/v1/jwks`);
- **2 SSH nodes** (`ssh-node-1`, `ssh-node-2`), each its own StatefulSet,
  joining with an `in_cluster` kubernetes token; `ssh-node-1` also
  registers 2 static applications and 2 static databases (`postgres`,
  `mysql`) — nothing has to listen behind them for Teleport to list them;
- **2 Kubernetes clusters** (`k3s-1`, `k3s-2`), each its own StatefulSet
  whose pod runs a k3s control plane (`--disable-agent`, unprivileged)
  next to a Teleport `kubernetes_service`;
- the **exporter**, installed with `charts/teleport-exporter`: its tbot
  sidecar joins with the `static_jwks` token, and `teleport.clusterName` is
  left unset to exercise the audience derived from `teleport.address`.

The test checks every metric against that inventory (2 nodes, 2
Kubernetes clusters, 2 apps, 2 databases, all `*_cluster_info` series,
the database breakdowns and health metrics), then scales the Teleport
auth service down and checks that the exporter reports `up 0` with a
growing `collect_errors_total` while keeping the last inventory, and
recovers once it is back. Everything is destroyed afterwards, including the temporary directory
holding the kubeconfig and the provider's ServiceAccount token; run with
`KEEP=1` to keep the environment for debugging (the script then prints the
environment variables and the command to remove it). Versions are pinned in `test/e2e/variables.tf` and
`test/e2e/versions.tf`.

## CI/CD

GitHub Actions (`.github/workflows/`): `ci.yml` runs Go lint
(`golangci-lint`), a Dockerfile lint (`hadolint`), unit tests, and the
end-to-end test above on every pull request (and push to `main`).
`chart.yml` runs `ct lint`, `helm unittest` and `kubeconform` on the chart.
`release.yml`, on `v*` tags, builds linux/amd64+arm64 binaries and a GitHub
Release via GoReleaser (`.goreleaser.yaml`), and builds/pushes a multi-arch
image to `ghcr.io/<repo>`. `chart-release.yml`, on `chart-v*` tags, pushes
the chart to `oci://ghcr.io/<owner>/charts`. Dependabot
(`.github/dependabot.yml`) keeps Go modules and Actions up to date weekly.

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for the exporter's release history (the
Helm chart has its own, [charts/teleport-exporter/CHANGELOG.md](charts/teleport-exporter/CHANGELOG.md)).
