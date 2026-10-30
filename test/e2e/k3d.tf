resource "k3d_cluster" "e2e" {
  name    = var.cluster_name
  servers = 1
  image   = var.k3s_image

  kube_api {
    host_ip = "127.0.0.1"
  }

  port {
    host           = "127.0.0.1"
    host_port      = var.auth_host_port
    container_port = 3025
    node_filters   = ["loadbalancer"]
  }

  registries {
    create {
      name      = "registry.localhost"
      host_port = tostring(var.registry_host_port)
    }

    config = yamlencode({
      mirrors = {
        "public.ecr.aws" = {
          endpoint = ["https://ghcr.io"]
          rewrite  = { "^gravitational/(.*)" = "philippe-vandermoere/$1" }
        }
      }
    })
  }

  kubeconfig {
    update_default_kubeconfig = false
    switch_current_context    = false
  }
}
