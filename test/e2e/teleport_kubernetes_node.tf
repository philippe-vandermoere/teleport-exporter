locals {
  kube_nodes = ["k3s-1", "k3s-2"]
}

resource "kubernetes_service_account_v1" "kube_node" {
  for_each = toset(local.kube_nodes)

  metadata {
    name      = each.key
    namespace = kubernetes_namespace_v1.e2e.metadata[0].name
  }
}

resource "kubernetes_config_map_v1" "kube_node" {
  for_each = toset(local.kube_nodes)

  metadata {
    name      = each.key
    namespace = kubernetes_namespace_v1.e2e.metadata[0].name
  }

  data = {
    "teleport.yaml" = yamlencode({
      version = "v3"
      teleport = {
        nodename     = each.key
        data_dir     = "/var/lib/teleport"
        proxy_server = "${helm_release.teleport.name}.${helm_release.teleport.namespace}.svc.cluster.local:443"
        join_params  = { method = "kubernetes", token_name = teleport_provision_token.agents.metadata.name }
        log          = { output = "stderr", severity = "INFO" }
      }
      auth_service       = { enabled = false }
      proxy_service      = { enabled = false }
      ssh_service        = { enabled = false }
      kubernetes_service = { enabled = true, kubeconfig_file = "/tmp/kubeconfig.yaml" }
    })

    # Teleport names a kube cluster after its kubeconfig context, which k3s
    # calls "default": rename it (and the cluster/user entries) first. The
    # server stays https://127.0.0.1:6443, since k3s runs in the same pod.
    "start.sh" = <<-EOT
      #!/busybox/sh
      set -eu
      export PATH="/busybox:$PATH"
      until [ -s /shared/kubeconfig.yaml ]; do
          sleep 1
      done
      sed -E "s/(name|cluster|user|current-context): default$/\1: ${each.key}/" \
          /shared/kubeconfig.yaml > /tmp/kubeconfig.yaml
      exec /usr/local/bin/teleport start -c /etc/teleport/teleport.yaml \
          --insecure --diag-addr=0.0.0.0:3000
    EOT
  }
}

resource "kubernetes_stateful_set_v1" "kube_node" {
  for_each = toset(local.kube_nodes)

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
          "checksum/config" = sha256(jsonencode(kubernetes_config_map_v1.kube_node[each.key].data))
        }
      }

      spec {
        service_account_name = kubernetes_service_account_v1.kube_node[each.key].metadata[0].name

        # A k3s control plane only (--disable-agent: no kubelet, no
        # containerd), which is all Teleport needs and runs unprivileged
        # inside a pod.
        container {
          name  = "k3s"
          image = var.k3s_image
          args = [
            "server",
            "--disable-agent",
            "--disable=traefik,servicelb,metrics-server,local-storage,coredns",
            "--write-kubeconfig=/shared/kubeconfig.yaml",
            "--write-kubeconfig-mode=644",
          ]

          volume_mount {
            name       = "shared"
            mount_path = "/shared"
          }
          volume_mount {
            name       = "k3s-data"
            mount_path = "/var/lib/rancher"
          }
        }

        # The -debug image adds busybox, for start.sh.
        container {
          name    = "teleport"
          image   = "public.ecr.aws/gravitational/teleport-distroless-debug:${var.teleport_version}"
          command = ["/busybox/sh", "/etc/teleport/start.sh"]

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
            name       = "shared"
            mount_path = "/shared"
            read_only  = true
          }
          volume_mount {
            name       = "teleport-data"
            mount_path = "/var/lib/teleport"
          }
        }

        volume {
          name = "config"
          config_map {
            name = kubernetes_config_map_v1.kube_node[each.key].metadata[0].name
          }
        }
        volume {
          name = "shared"
          empty_dir {}
        }
        volume {
          name = "k3s-data"
          empty_dir {}
        }
        volume {
          name = "teleport-data"
          empty_dir {}
        }
      }
    }
  }
}
