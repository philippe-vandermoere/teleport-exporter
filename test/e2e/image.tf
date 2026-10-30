locals {
  repo_root = abspath("${path.module}/../..")
  # Everything the root Dockerfile copies (see .dockerignore).
  exporter_sources = concat(
    [for f in fileset(local.repo_root, "{cmd,internal}/**") : "${local.repo_root}/${f}"],
    ["${local.repo_root}/go.mod", "${local.repo_root}/go.sum", "${local.repo_root}/Dockerfile"],
  )
}

resource "docker_image" "exporter" {
  name = "localhost:${var.registry_host_port}/teleport-exporter:e2e"

  build {
    context = local.repo_root
  }

  triggers = {
    sources = sha1(join("", [for f in local.exporter_sources : filesha1(f)]))
  }

  depends_on = [k3d_cluster.e2e]
}

resource "docker_registry_image" "exporter" {
  name          = docker_image.exporter.name
  keep_remotely = true

  triggers = {
    image_id = docker_image.exporter.image_id
  }
}
