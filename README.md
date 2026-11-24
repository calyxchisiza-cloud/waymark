# Waymark

Waymark answers DNS for names you register. An address is returned while its TTL still has time left. When the TTL passes, the name disappears.

The demo registers `payments.prod` and queries it:

```text
payments.prod 127.0.0.1
```

## Run

```powershell
go test ./...
go test -race -count=1 ./...
go run ./cmd/demo
```

The server listens on loopback:

```powershell
go run ./cmd/waymark -dns 127.0.0.1:5300 -http 127.0.0.1:5301
```

```powershell
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:5301/v1/names/payments.prod -ContentType application/json -Body '{"addresses":["127.0.0.1"],"ttl_seconds":30}'
```

Point a resolver at `127.0.0.1:5300`. An unknown name is NXDOMAIN. A known name with an AAAA question gets an empty answer, because the name exists and has no IPv6 address.
