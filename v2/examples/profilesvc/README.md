# profilesvc

A complete REST-style example showing service, endpoint, middleware, HTTP
transport, and a service-discovery-aware client.

## Run

From the v2 module:

```bash
go run ./examples/profilesvc/cmd/profilesvc -http.addr=:8080
```

Create and read a profile:

```bash
curl -X POST http://localhost:8080/profiles/ \
  -H "Content-Type: application/json" \
  -d '{"id":"1234","name":"Go Kit"}'

curl http://localhost:8080/profiles/1234
```

## Layout

```text
profilesvc/
|-- service.go              business interface and implementation
|-- endpoints.go            endpoint adapters
|-- middlewares.go          service middleware
|-- transport.go            HTTP transport
|-- client/client.go        discovery-aware client
`-- cmd/profilesvc/main.go  process assembly
```

This example demonstrates manual component assembly. For a new generated
service, start with `microgen`; for a smaller assembly, see `examples/kit_basic`.
