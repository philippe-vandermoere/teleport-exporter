locals {
  ssh_nodes = {
    ssh-node-1 = {
      services = {
        app_service = {
          enabled = true
          apps = [
            { name = "grafana", uri = "http://grafana.e2e.test:3000", public_addr = "grafana.teleport.test" },
            { name = "prometheus", uri = "http://prometheus.e2e.test:9090", public_addr = "prometheus.teleport.test" },
          ]
        }
        db_service = {
          enabled = true
          databases = [
            { name = "postgres", protocol = "postgres", uri = "postgres.e2e.test:5432" },
            { name = "mysql", protocol = "mysql", uri = "mysql.e2e.test:3306" },
          ]
        }
      }
    }
    ssh-node-2 = {}
  }
}

resource "kubernetes_service_account_v1" "ssh_node" {
  for_each = local.ssh_nodes

  metadata {
    name      = each.key
    namespace = kubernetes_namespace_v1.e2e.metadata[0].name
  }
}

resource "kubernetes_config_map_v1" "ssh_node" {
  for_each = local.ssh_nodes

  metadata {
    name      = each.key
    namespace = kubernetes_namespace_v1.e2e.metadata[0].name
  }

  data = {
    "teleport.yaml" = yamlencode(merge({
      version = "v3"
      teleport = {
        nodename     = each.key
        data_dir     = "/var/lib/teleport"
        proxy_server = "${helm_release.teleport.name}.${helm_release.teleport.namespace}.svc.cluster.local:443"
        join_params  = { method = "kubernetes", token_name = teleport_provision_token.agents.metadata.name }
        log          = { output = "stderr", severity = "INFO" }
      }
      auth_service  = { enabled = false }
      proxy_service = { enabled = false }
      ssh_service   = { enabled = true }
    }, try(each.value.services, {})))
  }
}

resource "kubernetes_stateful_set_v1" "ssh_node" {
  for_each = local.ssh_nodes

  metadata {
    name      = each.key
    namespace = kubernetes_namespace_v1.e2e.metadata[0].name
  }

  spec {
    replicas     = 1
    service_name = each.key

    selector {
      match_labels = { "app.kubernetes.io/name" = each.key }
    }

    template {
      metadata {
        labels = { "app.kubernetes.io/name" = each.key }
        annotations = {
          "checksum/config" = sha256(kubernetes_config_map_v1.ssh_node[each.key].data["teleport.yaml"])
        }
      }

      spec {
        service_account_name = kubernetes_service_account_v1.ssh_node[each.key].metadata[0].name

        container {
          name    = "teleport"
          image   = "public.ecr.aws/gravitational/teleport-distroless:${var.teleport_version}"
          command = ["/usr/bin/dumb-init", "/usr/local/bin/teleport"]
          # --insecure: the test proxy's certificate is self-signed.
          args = ["start", "-c", "/etc/teleport/teleport.yaml", "--insecure", "--diag-addr=0.0.0.0:3000"]

          port {
            name           = "diag"
            container_port = 3000
          }

          readiness_probe {
            http_get {
              path = "/readyz"
              port = "diag"
            }
            period_seconds = 2
          }

          volume_mount {
            name       = "config"
            mount_path = "/etc/teleport"
            read_only  = true
          }
          volume_mount {
            name       = "data"
            mount_path = "/var/lib/teleport"
          }
        }

        volume {
          name = "config"
          config_map {
            name = kubernetes_config_map_v1.ssh_node[each.key].metadata[0].name
          }
        }
        volume {
          name = "data"
          empty_dir {}
        }
      }
    }
  }
}
