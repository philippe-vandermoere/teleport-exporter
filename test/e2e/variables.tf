variable "cluster_name" {
  description = "Name of the k3d cluster."
  type        = string
  default     = "teleport-exporter-e2e"
}

variable "k3s_image" {
  description = "k3s image for the k3d cluster and for the nested k3s of the kube nodes."
  type        = string
  default     = "docker.io/rancher/k3s:v1.36.2-k3s1"
}

variable "teleport_version" {
  description = "Teleport version: teleport-cluster chart, agents and tbot. Keep in step with the Teleport provider in versions.tf."
  type        = string
  default     = "18.10.0"
}

variable "auth_host_port" {
  description = "Host port (on 127.0.0.1) forwarded to the Teleport auth service, used by the Teleport provider."
  type        = number
  default     = 3025
}

variable "registry_host_port" {
  description = "Host port of the k3d registry the exporter image is pushed to."
  type        = number
  default     = 5111
}

variable "terraform_join_token" {
  description = "Name of the join token the Teleport provider's bot joins with, created at Teleport startup (see teleport.tf)."
  type        = string
  default     = "terraform"
}

variable "kubernetes_token_path" {
  description = "Where the Teleport provider's ServiceAccount token is written. Must match the KUBERNETES_TOKEN_PATH environment variable Terraform runs with (see run.sh)."
  type        = string
}
