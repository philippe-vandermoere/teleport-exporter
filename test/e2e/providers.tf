provider "kubernetes" {
  host                   = k3d_cluster.e2e.credentials[0].host
  client_certificate     = k3d_cluster.e2e.credentials[0].client_certificate
  client_key             = k3d_cluster.e2e.credentials[0].client_key
  cluster_ca_certificate = k3d_cluster.e2e.credentials[0].cluster_ca_certificate
}

provider "helm" {
  kubernetes = {
    host                   = k3d_cluster.e2e.credentials[0].host
    client_certificate     = k3d_cluster.e2e.credentials[0].client_certificate
    client_key             = k3d_cluster.e2e.credentials[0].client_key
    cluster_ca_certificate = k3d_cluster.e2e.credentials[0].cluster_ca_certificate
  }
}

provider "docker" {
  registry_auth {
    address  = "http://localhost:${var.registry_host_port}"
    username = "fake"
    password = "fake"
  }
}

provider "teleport" {
  addr        = "127.0.0.1:${var.auth_host_port}"
  insecure    = true
  join_method = "kubernetes"
  join_token  = var.terraform_join_token
}
