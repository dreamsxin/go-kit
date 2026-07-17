# Generated TypeScript SDK

This directory is generated from the same normalized service contract as the
Go SDK, HTTP routes, OpenAPI document, and JSON Schema bundle. Do not edit it by
hand.

The client uses the standard Fetch API and has no runtime dependencies.

```ts
import { APIClient } from "./client";

const client = new APIClient("http://localhost:8080");
const response = await client.catalogService.createUser({
  "username": "value",
  "email": "value",
});
```

Use the service clients exposed by `APIClient` and pass an `AbortSignal` or
per-request headers through the optional second argument. Non-2xx responses
throw `APIError` with the HTTP status and response body.

Type-check the generated source with the release-pinned compiler:

```bash
npx --yes --package typescript@7.0.2 tsc -p sdk/typescript/tsconfig.json
```

The source SDK covers unary HTTP operations. Use the generated Go gRPC SDK for
streaming RPCs.
