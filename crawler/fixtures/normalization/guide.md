---
title: Payments SDK
---

# Payments {#payments}

Use the [authentication guide](./authentication) before creating a client.

## Install

- Install the package.
  - Pin an exact version.
- Configure credentials.

| Runtime | Command |
| :--- | ---: |
| Node.js | `pnpm add @acme/payments@2.1.0` |
| Python | `pip install acme-payments==2.1.0` |

```ts
import { Payments } from "@acme/payments";

const payments = new Payments({ token: process.env.PAYMENTS_TOKEN });
```

## Errors

Retry `429` responses using the server-provided backoff value.

