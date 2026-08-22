# Database migrations

Migration files are an append-only production ledger. Never edit, rename, or delete an existing `.sql` file, even when its schema is no longer part of the current product. Make every schema change in a new, uniquely numbered migration and add its checksum to `checksums.sha256` in the same change. The historical duplicate `0020` sequence is frozen as applied history; do not repeat it.

The runtime records and verifies the checksum when it applies a migration. The repository test also verifies the complete history against `checksums.sha256`, so an accidental historical edit fails before deployment.
