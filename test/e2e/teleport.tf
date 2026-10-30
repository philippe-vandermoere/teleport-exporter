resource "kubernetes_namespace_v1" "teleport" {
  metadata {
    name = "teleport"
  }
}

# Applied by the auth service on every start (--apply-on-startup), so the
# Teleport provider can join as a bot with no admin identity to extract
# with tctl. The chart uses --apply-on-startup itself for the proxy's join
# token, and only one file can be given: that token is declared here too,
# with the chart's own one disabled (createProxyToken = false).
resource "kubernetes_config_map_v1" "teleport_bootstrap" {
  metadata {
    name      = "teleport-bootstrap"
    namespace = kubernetes_namespace_v1.teleport.metadata[0].name
  }

  data = {
    "resources.yaml" = join("---\n", [
      yamlencode({
        kind     = "token"
        version  = "v2"
        metadata = { name = "teleport-proxy" }
        spec = {
          roles       = ["Proxy"]
          join_method = "kubernetes"
          kubernetes  = { allow = [{ service_account = "teleport:teleport-proxy" }] }
        }
      }),
      yamlencode({
        kind     = "bot"
        version  = "v1"
        metadata = { name = "terraform" }
        spec     = { roles = ["terraform-provider"] }
      }),
      yamlencode({
        kind     = "token"
        version  = "v2"
        metadata = { name = var.terraform_join_token }
        spec = {
          roles       = ["Bot"]
          bot_name    = "terraform"
          join_method = "kubernetes"
          kubernetes = {
            type  = "in_cluster"
            allow = [{ service_account = "teleport:${kubernetes_service_account_v1.terraform.metadata[0].name}" }]
          }
        }
      }),
    ])
  }
}

resource "helm_release" "teleport" {
  name       = "teleport"
  namespace  = kubernetes_namespace_v1.teleport.metadata[0].name
  repository = "https://charts.releases.teleport.dev"
  chart      = "teleport-cluster"
  version    = var.teleport_version
  timeout    = 600

  values = [yamlencode({
    # The FQDN of the proxy Service (named after the release): the exporter
    # chart derives its tbot token audience from teleport.address, and this
    # test relies on that default (see exporter.tf).
    clusterName       = "teleport.${kubernetes_namespace_v1.teleport.metadata[0].name}.svc.cluster.local"
    proxyListenerMode = "multiplex"
    # No ACME and no tls.existingSecretName: the proxy serves a self-signed
    # certificate, hence --insecure/insecure everywhere below.
    service          = { type = "ClusterIP" }
    createProxyToken = false

    auth = {
      extraArgs = ["--apply-on-startup=/etc/teleport-bootstrap/resources.yaml"]
      extraVolumes = [{
        name      = "bootstrap"
        configMap = { name = kubernetes_config_map_v1.teleport_bootstrap.metadata[0].name }
      }]
      extraVolumeMounts = [{
        name      = "bootstrap"
        mountPath = "/etc/teleport-bootstrap"
        readOnly  = true
      }]
      # The chart registers the hosting k3d cluster by default: only the two
      # k3s clusters of nodes.tf should be listed.
      teleportConfig = {
        kubernetes_service = { enabled = false }
      }
    }
  })]
}

# The Teleport provider joins through the auth service directly: through
# the proxy, it would be sent on to the proxy's public address
# (teleport.teleport.svc.cluster.local), which doesn't resolve on the host.
resource "kubernetes_service_v1" "teleport_auth_external" {
  metadata {
    name      = "teleport-auth-external"
    namespace = kubernetes_namespace_v1.teleport.metadata[0].name
  }

  spec {
    type = "LoadBalancer"
    selector = {
      "app.kubernetes.io/name"      = "teleport-cluster"
      "app.kubernetes.io/instance"  = helm_release.teleport.name
      "app.kubernetes.io/component" = "auth"
    }

    port {
      name        = "auth"
      port        = 3025
      target_port = 3025
    }
  }
}

# --- The Teleport provider's identity ---------------------------------------
#
# The provider joins natively as the "terraform" bot with the kubernetes
# join method. Teleport only accepts pod-bound ServiceAccount tokens, so the
# token is requested for a pause pod running as the "terraform"
# ServiceAccount, and written where the provider reads it
# (KUBERNETES_TOKEN_PATH, exported by run.sh).

resource "kubernetes_service_account_v1" "terraform" {
  metadata {
    name      = "terraform"
    namespace = kubernetes_namespace_v1.teleport.metadata[0].name
  }
}

resource "kubernetes_pod_v1" "terraform" {
  metadata {
    name      = "terraform"
    namespace = kubernetes_namespace_v1.teleport.metadata[0].name
  }

  spec {
    service_account_name            = kubernetes_service_account_v1.terraform.metadata[0].name
    automount_service_account_token = false

    container {
      name  = "pause"
      image = "registry.k8s.io/pause:3.10"
    }
  }
}

resource "kubernetes_token_request_v1" "terraform" {
  metadata {
    name      = kubernetes_service_account_v1.terraform.metadata[0].name
    namespace = kubernetes_namespace_v1.teleport.metadata[0].name
  }

  spec {
    audiences          = [jsondecode(helm_release.teleport.metadata.values).clusterName]
    expiration_seconds = 7200

    bound_object_ref {
      api_version = "v1"
      kind        = "Pod"
      name        = kubernetes_pod_v1.terraform.metadata[0].name
      uid         = kubernetes_pod_v1.terraform.metadata[0].uid
    }
  }
}

# The Teleport provider joins as soon as it is configured, i.e. at plan
# time: run.sh first applies this file (and with it Teleport) with -target,
# then everything else. Teleport resources depend on it too, so a destroy
# removes them before Teleport itself.
resource "local_sensitive_file" "terraform_token" {
  content  = kubernetes_token_request_v1.terraform.token
  filename = var.kubernetes_token_path

  depends_on = [helm_release.teleport, kubernetes_service_v1.teleport_auth_external]
}
