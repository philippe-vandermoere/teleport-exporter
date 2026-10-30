# teleport-exporter

Helm chart for [teleport-exporter](https://github.com/philippe-vandermoere/teleport-exporter),
a Prometheus exporter for the Teleport inventory (SSH nodes, Kubernetes
clusters, databases, applications).

## Installing

Create the Teleport role, bot and join token first (see below), then:

```
helm install teleport-exporter oci://ghcr.io/philippe-vandermoere/charts/teleport-exporter \
  --version <version> --namespace monitoring \
  --set teleport.address=teleport.example.com:443 \
  --set tbot.token=teleport-exporter-bot
```

See [CHANGELOG.md](CHANGELOG.md) for this chart's release history (the
exporter image itself has its own, [../../CHANGELOG.md](../../CHANGELOG.md)).

## Creating the Teleport bot and join token

Run the following with `tctl`, as a Teleport user allowed to manage roles,
bots and tokens.

### 1. The role

Read-only access to nodes, Kubernetes clusters, databases, applications and
the cluster name. The label matchers are required: without them, the list
calls succeed but return nothing.

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

```
tctl create -f role.yaml
```

### 2. The bot

```
tctl bots add teleport-exporter --roles=teleport-exporter
```

Ignore the one-time join token `tctl` prints: the bot joins with the
`kubernetes` token below instead.

### 3. The join token

The `service_account` to allow is `<release namespace>:<ServiceAccount
name>`. The ServiceAccount is named after the release (`teleport-exporter`
for a release named `teleport-exporter`, otherwise
`<release>-teleport-exporter`), or `serviceAccount.name` if set.

**`static_jwks`**, when Teleport can't reach the Kubernetes cluster's OIDC
issuer (typical on-premises). Paste the output of
`kubectl get --raw /openid/v1/jwks` into the token, and update it whenever
the cluster's signing keys rotate:

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
        {"keys":[...]}
    allow:
      - service_account: "monitoring:teleport-exporter"
```

**`oidc`**, when the cluster's ServiceAccount issuer is publicly reachable
(EKS, GKE, AKS...). Teleport fetches the signing keys itself, so key
rotation needs no update. The issuer is
`kubectl get --raw /.well-known/openid-configuration | jq -r .issuer`:

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
      issuer: https://oidc.eks.eu-west-1.amazonaws.com/id/EXAMPLE
    allow:
      - service_account: "monitoring:teleport-exporter"
```

```
tctl create -f token.yaml
```

Then install the chart with `tbot.token=teleport-exporter-bot`.

## How it authenticates: the tbot sidecar

The pod runs two containers sharing an in-memory `emptyDir`:

- `tbot` (Teleport Machine ID), declared as a
  [native sidecar](https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/)
  (an init container with `restartPolicy: Always`, hence Kubernetes ≥ 1.29).
  It joins Teleport with the `kubernetes` join method, presenting a
  projected ServiceAccount token, and keeps a short-lived identity file
  renewed in the shared volume. Its diagnostics endpoint is enabled
  (`tbot.diag.port`) and backs its startup, liveness and readiness probes:
  the exporter container only starts once the startup probe passes, i.e.
  once the first identity file has been written.
- `teleport-exporter`, which reads that identity file (re-read before every
  collection, so renewals are picked up without a restart).

Both `static_jwks` and `oidc` token types work with the same chart values:
they only differ in how the Teleport join token validates the
ServiceAccount token, see the project README. Either way, the token's
`spec.kubernetes.allow` must list this release's ServiceAccount
(`<namespace>:<serviceAccount name>`, printed by `helm install`'s notes).

The projected token's audience must be the Teleport cluster name. It
defaults to the host part of `teleport.address` (`teleport.example.com:443`
→ `teleport.example.com`), the cluster name in the common case; set
`teleport.clusterName` when they differ (`tctl status` shows the cluster
name).

Without tbot (`tbot.enabled: false`), point `identity.existingSecret` at a
Secret holding an identity file (key `identity.key`, `identity` by default),
for instance one written by a tbot running elsewhere with a
`kubernetes_secret` destination. Renewing it is then up to you.

Naming fields (`nameOverride`, `fullnameOverride`, `serviceAccount.name`),
`teleport.address`/`teleport.clusterName`, `tbot.token`,
`identity.existingSecret` and `extraArgs`/`extraEnv`/`extraVolumes`/
`extraVolumeMounts` are all passed through Helm's `tpl`, so they may contain
template expressions evaluated against the release.

## Prometheus Operator integration

`serviceMonitor.enabled` is `false` by default, since the
`monitoring.coreos.com` CRDs aren't guaranteed to be installed in every
target cluster. Set it to `true` (and optionally `serviceMonitor.labels` to
match your Prometheus Operator's `serviceMonitorSelector`) to register a
`ServiceMonitor`.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| config.collectTimeout | string | `"20s"` | Value for `TELEPORT_EXPORTER_COLLECT_TIMEOUT`: upper bound for one whole collection. |
| config.listenAddress | string | `":9180"` | Value for `TELEPORT_EXPORTER_LISTEN_ADDRESS`. Its port must match `service.port`. |
| config.refreshInterval | string | `"30s"` | Value for `TELEPORT_EXPORTER_REFRESH_INTERVAL`: how often the inventory is collected from Teleport in the background (scrapes are served from the last collection). |
| controller.affinity | object | `{}` | Affinity rules. |
| controller.annotations | object | `{}` | Annotations for the Deployment object itself. |
| controller.containerSecurityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Container-level securityContext, applied to both the exporter and tbot. |
| controller.nodeSelector | object | `{}` | Node selector. |
| controller.podAnnotations | object | `{}` | Annotations for the pod template. |
| controller.podLabels | object | `{}` | Labels for the pod template. |
| controller.podSecurityContext | object | `{"fsGroup":65535,"runAsGroup":65535,"runAsNonRoot":true,"runAsUser":65535}` | Pod-level securityContext, shared by the exporter and tbot. Both read/write the identity volume as this UID/GID. |
| controller.replicas | int | `1` | Replica count. Each replica runs its own tbot and queries Teleport independently; more than one only adds load on the Teleport API. |
| controller.resources | object | `{}` | Resource requests/limits for the exporter container. |
| controller.tolerations | list | `[]` | Tolerations. |
| controller.updateStrategy | object | `{}` | Deployment `strategy`. |
| extraArgs | list | `[]` | Extra command-line args for the exporter binary. Rendered through Helm templating as a whole, so any entry may contain expressions like `"{{ .Release.Name }}"`. |
| extraEnv | list | `[]` | Extra environment variables for the exporter container. Supports Helm templating (see `extraArgs` above). |
| extraVolumeMounts | list | `[]` | Extra volume mounts for the exporter container, paired with `extraVolumes`. Supports Helm templating. |
| extraVolumes | list | `[]` | Extra volumes. Supports Helm templating (see `extraArgs` above). |
| fullnameOverride | string | `""` | Override the fully qualified object name entirely (otherwise built from the release name and chart name). Supports Helm templating. |
| identity.existingSecret | string | `""` | Name of an existing Secret holding a Teleport identity file, used instead of tbot when `tbot.enabled` is false (you are then responsible for renewing it). Supports Helm templating. |
| identity.key | string | `"identity"` | Key within `identity.existingSecret` holding the identity file. |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy. |
| image.repository | string | `"ghcr.io/philippe-vandermoere/teleport-exporter"` | Image repository. |
| image.tag | string | `""` | Image tag. Defaults to the chart's appVersion if unset. |
| imagePullSecrets | list | `[]` | Image pull secrets, for private registries. |
| livenessProbe.enabled | bool | `true` | Enable the exporter's liveness probe. |
| livenessProbe.failureThreshold | int | `3` | Consecutive failures before the container is considered unhealthy. |
| livenessProbe.httpGet.path | string | `"/"` | Path the probe checks. `/` rather than `/metrics`: it answers without touching the collector, so a Teleport outage never restarts the exporter (it shows up as `teleport_exporter_up 0` instead). |
| livenessProbe.httpGet.port | string | `"http"` | Port the probe checks (the container port name). |
| livenessProbe.initialDelaySeconds | int | `5` | Seconds before the first probe. |
| livenessProbe.periodSeconds | int | `30` | Seconds between probes. |
| livenessProbe.timeoutSeconds | int | `5` | Probe timeout in seconds. |
| nameOverride | string | `""` | Override the chart name used to build object names (see `fullnameOverride` for the common case of controlling the whole name directly). Supports Helm templating, e.g. `"{{ .Release.Namespace }}"`. |
| readinessProbe.enabled | bool | `true` | Enable the exporter's readiness probe. |
| readinessProbe.failureThreshold | int | `3` | Consecutive failures before the pod is removed from Service endpoints. |
| readinessProbe.httpGet.path | string | `"/"` | Path the probe checks. See the comment on livenessProbe.httpGet.path above — kept as `/`, not `/metrics`, for the same reason. |
| readinessProbe.httpGet.port | string | `"http"` | Port the probe checks (the container port name). |
| readinessProbe.initialDelaySeconds | int | `5` | Seconds before the first probe. |
| readinessProbe.periodSeconds | int | `10` | Seconds between probes. |
| readinessProbe.timeoutSeconds | int | `5` | Probe timeout in seconds. |
| service.annotations | object | `{}` | Annotations for the Service. |
| service.port | int | `9180` | Service port (and the exporter's listen port, see `config.listenAddress`). |
| serviceAccount.annotations | object | `{}` | Annotations for the ServiceAccount. |
| serviceAccount.create | bool | `true` | Create a ServiceAccount. Its identity (`<namespace>:<name>`) is what the Teleport join token allows (`spec.kubernetes.allow`). No RBAC is attached: neither the exporter nor tbot call the Kubernetes API. |
| serviceAccount.name | string | `""` | Name of the ServiceAccount to use. Defaults to the chart's fullname when `create` is true. Supports Helm templating. |
| serviceMonitor.enabled | bool | `false` | Create a Prometheus Operator ServiceMonitor. Disabled by default: the `monitoring.coreos.com` CRDs aren't guaranteed to be installed in every target cluster. |
| serviceMonitor.interval | string | `"30s"` | Scrape interval. |
| serviceMonitor.labels | object | `{}` | Extra labels, so the ServiceMonitor matches your Prometheus Operator's `serviceMonitorSelector`. |
| serviceMonitor.metricRelabelings | list | `[]` | `metricRelabelings` on the scrape endpoint. |
| serviceMonitor.namespace | string | `""` | Namespace for the ServiceMonitor. Defaults to the release namespace. |
| serviceMonitor.relabelings | list | `[]` | `relabelings` on the scrape endpoint. |
| serviceMonitor.scrapeTimeout | string | `"10s"` | Scrape timeout. |
| tbot.certificateTTL | string | `"1h"` | TTL of the certificates tbot issues. |
| tbot.diag.port | int | `3001` | Port of tbot's diagnostics endpoint (`/livez`, `/readyz`), used by its probes below. |
| tbot.enabled | bool | `true` | Run a tbot sidecar that joins Teleport with the pod's ServiceAccount (`kubernetes` join method, `static_jwks` or `oidc` token type — see the project README to create the bot and its token) and keeps a short-lived identity file renewed for the exporter. When false, `identity.existingSecret` must hold an identity file instead. |
| tbot.extraEnv | list | `[]` | Extra environment variables for the tbot container. Supports Helm templating. |
| tbot.image.pullPolicy | string | `"IfNotPresent"` | tbot image pull policy. |
| tbot.image.repository | string | `"public.ecr.aws/gravitational/tbot-distroless"` | tbot image repository. |
| tbot.image.tag | string | `"18.10.0"` | tbot image tag. Keep it on the same major version as the Teleport cluster. |
| tbot.livenessProbe | object | `{"enabled":true,"failureThreshold":3,"httpGet":{"path":"/livez","port":"diag"},"periodSeconds":30,"timeoutSeconds":5}` | Liveness probe on tbot's `/livez`. |
| tbot.readinessProbe | object | `{"enabled":true,"failureThreshold":3,"httpGet":{"path":"/readyz","port":"diag"},"periodSeconds":10,"timeoutSeconds":5}` | Readiness probe on tbot's `/readyz`: not ready while tbot fails to renew its identity. |
| tbot.renewalInterval | string | `"20m"` | How often tbot renews its certificates. Must be shorter than `tbot.certificateTTL`. |
| tbot.resources | object | `{}` | Resource requests/limits for the tbot container. |
| tbot.serviceAccountTokenExpirationSeconds | int | `600` | Lifetime of the projected ServiceAccount token tbot joins with. Kubernetes rotates it automatically; 600 is the minimum Kubernetes accepts. |
| tbot.startupProbe | object | `{"enabled":true,"failureThreshold":60,"httpGet":{"path":"/readyz","port":"diag"},"periodSeconds":2,"timeoutSeconds":2}` | Startup probe on tbot's `/readyz`. tbot runs as a native sidecar (an init container with `restartPolicy: Always`): the exporter container only starts once this probe passes, i.e. once the first identity file has been written. |
| tbot.token | required when `tbot.enabled` | `""` | Name of the Teleport join token (a `kind: token` with `join_method: kubernetes`) the bot joins with. Supports Helm templating. |
| teleport.address | required | `""` | Teleport proxy address, as `host:port` (e.g. `teleport.example.com:443`). Used both by tbot to join and renew its identity, and by the exporter to query the inventory. Supports Helm templating. |
| teleport.clusterName | string | `""` | Teleport cluster name, used as the audience of the projected ServiceAccount token tbot joins with (Teleport rejects any other audience). Defaults to the host part of `teleport.address` (`teleport.example.com:443` → `teleport.example.com`), which matches the cluster name in the common case; set it when they differ (`tctl status` shows the cluster name). Supports Helm templating. |
| teleport.insecure | bool | `false` | Skip TLS verification of the proxy's web certificate (self-signed test clusters only). Applies to both tbot and the exporter. |
