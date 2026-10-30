terraform {
  required_version = ">= 1.9"

  required_providers {
    k3d = {
      source  = "moio/k3d"
      version = "0.0.12"
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 4.6"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 3.4"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 3.3"
    }
    http = {
      source  = "hashicorp/http"
      version = "~> 3.6"
    }
    teleport = {
      source  = "terraform.releases.teleport.dev/gravitational/teleport"
      version = "18.10.0"
    }
  }
}
