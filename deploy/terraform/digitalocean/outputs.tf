output "load_balancer_address" {
  description = "Point the public_url hostname at this address. It can take several minutes to appear."
  value = try(
    kubernetes_service_v1.this.status[0].load_balancer[0].ingress[0].hostname,
    kubernetes_service_v1.this.status[0].load_balancer[0].ingress[0].ip,
    null,
  )
}

output "public_url" {
  value = var.public_url
}

output "kubernetes_cluster_id" {
  value = digitalocean_kubernetes_cluster.this.id
}

output "database_private_host" {
  value = digitalocean_database_cluster.this.private_host
}
