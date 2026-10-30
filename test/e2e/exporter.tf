resource "helm_release" "exporter" {
  name      = "teleport-exporter"
  namespace = kubernetes_namespace_v1.e2e.metadata[0].name
  chart     = "${local.repo_root}/charts/teleport-exporter"
  timeout   = 300

  values = [yamlencode({
    # Pulled from the k3d registry the image was pushed to (see image.tf),
    # pinned by digest so a rebuilt image always rolls the pod.
    image = {
      repository = "registry.localhost:5000/teleport-exporter"
      tag        = "e2e@${docker_registry_image.exporter.sha256_digest}"
    }
    teleport = {
      address  = "${helm_release.teleport.name}.${helm_release.teleport.namespace}.svc.cluster.local:443"
      insecure = true
    }
    tbot = {
      token = teleport_provision_token.exporter.metadata.name
    }
    config = {
      refreshInterval = "5s"
      collectTimeout  = "5s"
    }
  })]
}
