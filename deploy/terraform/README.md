# DokoSoko Terraform deployments

These roots deploy the DokoSoko service, crawler, PostgreSQL, persistent file
storage, HTTPS ingress, and provider-native secrets on four clouds:

- [`aws/`](./aws): ECS Fargate, RDS PostgreSQL, EFS, and an Application Load Balancer.
- [`digitalocean/`](./digitalocean): DOKS, Managed PostgreSQL, a block-storage PVC, and a DigitalOcean Load Balancer.
- [`azure/`](./azure): Azure Container Apps, PostgreSQL Flexible Server, and Azure Files.
- [`gcp/`](./gcp): Cloud Run, Cloud SQL for PostgreSQL, and Filestore over NFS.

Each directory is an independent Terraform root. Pick one; do not apply all four.

## Important operating constraints

DokoSoko currently stores uploaded source material and crawl snapshots on a shared
filesystem. The app and crawler therefore run as one active pair backed by one
shared storage volume. These configurations intentionally prevent horizontal
scaling. Do not raise the replica count until that filesystem contract has been
replaced by object-storage-aware application code or a tested multi-writer volume.

The database and file storage are durable, but these examples are a production
baseline rather than a complete disaster-recovery system. Configure provider
backups, retention, DNS, alerting, and regional recovery to match your own RPO and
RTO before launch.

The shared mount contains separate `data` and `uploads` directories. Both
containers use UID/GID 65532, and the directories are created by the workload
rather than assuming ownership of a provider-managed mount root.

## Prerequisites

1. Build and push both `Dockerfile` and `Dockerfile.crawler` images to a registry
   the target platform can read.
2. Resolve both images to immutable `@sha256:` references. Mutable tags are
   rejected by the variable validation.
3. Generate a 32-byte base64 master key and a high-entropy setup token:

   ```sh
   openssl rand -base64 32
   openssl rand -hex 32
   ```

4. Copy the provider's `terraform.tfvars.example` to `terraform.tfvars` and fill
   in the required values.
5. Configure an encrypted, access-controlled remote Terraform state backend.

Secrets supplied as Terraform variables can be present in Terraform state even
when marked `sensitive`. Local state is not an acceptable production secret store.
The deployment copies runtime secrets into the target provider's secret facility,
but that does not remove them from state.

## Apply

Run these commands from the selected provider directory:

```sh
terraform init
terraform plan -out=tfplan
terraform apply tfplan
```

OpenTofu 1.8 or later can be used in place of Terraform.

After deployment, verify `/healthz`, complete the one-time setup flow, confirm a
crawl can read an uploaded source and write a snapshot, and then remove or rotate
the setup token according to your operational policy.

## Provider notes

### AWS

The AWS root spans two availability zones. ECS tasks run in private subnets,
PostgreSQL runs in isolated subnets, and one NAT gateway provides controlled
outbound access. Supply an ACM certificate in the deployment region, then point
the `public_url` hostname at `load_balancer_dns_name`. The root does not own your
public DNS zone.

### DigitalOcean

DigitalOcean App Platform is not used because its local filesystem is ephemeral.
The DOKS StatefulSet keeps the app and crawler in one pod on one block-storage
PVC. Supply the stable name of an existing DigitalOcean certificate and point
the `public_url` hostname at `load_balancer_address`. Images must be public or
available through a registry integration already attached to the cluster.

### Azure

The Container App runs both containers in one revision and mounts one Azure
Files share. PostgreSQL uses private VNet integration, and the required
`VECTOR`, `CITEXT`, and `PGCRYPTO` extensions are allowlisted before migrations
run. If `public_url` is empty, the generated Container Apps hostname is used.
The database has a Terraform `prevent_destroy` guard; removing a whole
environment requires a deliberate source change after taking a verified backup.
Images must be anonymously readable unless you add an ACR identity or registry
credential block for your registry.

### Google Cloud

Cloud Run uses instance-based CPU allocation and exactly one instance so the
crawler continues polling outside HTTP requests. The shared volume is Filestore
over NFS because DokoSoko relies on POSIX operations that Cloud Storage FUSE does
not guarantee. The `BASIC_HDD` Filestore tier has a 1024 GiB minimum and should
be cost-reviewed before apply. `public_url` must be a hostname routed to the
resulting `cloud_run_url`; domain mapping is intentionally left with your DNS
and certificate boundary.
