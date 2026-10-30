output "teleport_version" {
  description = "Teleport version the agents run, reported by node_cluster_info's version label."
  value       = var.teleport_version
}

output "terraform_sa_token" {
  description = "ServiceAccount token the Teleport provider joins with, written by run.sh where the provider reads it (KUBERNETES_TOKEN_PATH)."
  value       = kubernetes_token_request_v1.terraform.token
  sensitive   = true
}

output "kubeconfig" {
  description = "Kubeconfig of the k3d cluster, written by run.sh for its kubectl calls."
  value       = k3d_cluster.e2e.credentials[0].raw
  sensitive   = true
}
