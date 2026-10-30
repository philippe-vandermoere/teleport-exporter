resource "kubernetes_namespace_v1" "e2e" {
  metadata {
    name = "e2e"
  }
}

# The exporter's role: the same one the README documents.
resource "teleport_role" "exporter" {
  depends_on = [local_sensitive_file.terraform_token]

  version = "v7"
  metadata = {
    name = "teleport-exporter"
  }

  spec = {
    allow = {
      node_labels       = { "*" = ["*"] }
      kubernetes_labels = { "*" = ["*"] }
      db_labels         = { "*" = ["*"] }
      app_labels        = { "*" = ["*"] }
      rules = [{
        resources = ["node", "kube_server", "kube_cluster", "db_server", "db", "app_server", "app", "cluster_name"]
        verbs     = ["list", "read"]
      }]
    }
  }
}

resource "teleport_bot" "exporter" {
  metadata = {
    name = "teleport-exporter"
  }

  spec = {
    roles = [teleport_role.exporter.metadata.name]
  }
}

# The cluster's ServiceAccount signing keys, as `kubectl get --raw
# /openid/v1/jwks` returns them in the README procedure.
data "http" "jwks" {
  url             = "${k3d_cluster.e2e.credentials[0].host}/openid/v1/jwks"
  ca_cert_pem     = k3d_cluster.e2e.credentials[0].cluster_ca_certificate
  client_cert_pem = k3d_cluster.e2e.credentials[0].client_certificate
  client_key_pem  = k3d_cluster.e2e.credentials[0].client_key
}

# Joined by the exporter chart's tbot sidecar, exactly as documented.
resource "teleport_provision_token" "exporter" {
  version = "v2"
  metadata = {
    name = "teleport-exporter-bot"
  }

  spec = {
    roles       = ["Bot"]
    bot_name    = teleport_bot.exporter.metadata.name
    join_method = "kubernetes"
    kubernetes = {
      type        = "static_jwks"
      static_jwks = { jwks = data.http.jwks.response_body }
      allow       = [{ service_account = "${kubernetes_namespace_v1.e2e.metadata[0].name}:teleport-exporter" }]
    }
  }
}

# Joined by the SSH nodes (the first one also serving apps and databases)
# and Kubernetes agents: the auth service validates
# their pod's own ServiceAccount token with a TokenReview, no secret involved.
resource "teleport_provision_token" "agents" {
  depends_on = [local_sensitive_file.terraform_token]

  version = "v2"
  metadata = {
    name = "e2e-agents"
  }

  spec = {
    roles       = ["Node", "Kube", "App", "Db"]
    join_method = "kubernetes"
    kubernetes = {
      type = "in_cluster"
      allow = [
        for name in concat(keys(local.ssh_nodes), local.kube_nodes) :
        { service_account = "${kubernetes_namespace_v1.e2e.metadata[0].name}:${name}" }
      ]
    }
  }
}
