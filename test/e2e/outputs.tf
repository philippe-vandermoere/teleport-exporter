output "teleport_version" {
  description = "Teleport version the agents run, reported by node_cluster_info's version label."
  value       = var.teleport_version
}

output "kubeconfig" {
  description = "Kubeconfig of the k3d cluster, written by run.sh for its kubectl calls."
  value       = k3d_cluster.e2e.credentials[0].raw
  sensitive   = true
}
