# Device enrollment

## Create an enrollment code

```bash
curl -sS -X POST https://codebridge.example.com/admin/enrollments \
  -H "Authorization: Bearer $CODEBRIDGE_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"ttl_seconds":600}'
```

Enrollment codes:

- are random 256-bit values;
- are stored only as SHA-256 digests by the Manager;
- expire automatically;
- can be consumed exactly once.

## Enroll a client

```bash
CODEBRIDGE_ENROLL_CODE='enr_...' \
CODEBRIDGE_MANAGER_URL='wss://codebridge.example.com/agent' \
CODEBRIDGE_DEVICE_ID='mbp-m1' \
CODEBRIDGE_DEVICE_NAME='MacBook Pro M1' \
CODEBRIDGE_WORKSPACES='pms=/Users/me/code/pms' \
./codebridge-client
```

The returned device credential is immediately persisted locally before normal request processing begins. This prevents a successful enrollment from being lost if the connection drops immediately afterwards.

## Rotation

```bash
curl -sS -X POST https://codebridge.example.com/admin/devices/mbp-m1/rotate \
  -H "Authorization: Bearer $CODEBRIDGE_ADMIN_TOKEN"
```

The previous credential becomes invalid immediately. The new raw credential is returned once. On the local machine, persist it by starting the client once with:

```bash
CODEBRIDGE_DEVICE_CREDENTIAL='dev_...' ./codebridge-client ...
```

## Revocation

```bash
curl -sS -X DELETE https://codebridge.example.com/admin/devices/mbp-m1 \
  -H "Authorization: Bearer $CODEBRIDGE_ADMIN_TOKEN"
```

The active agent connection is closed and the stored credential digest is removed. A revoked device needs a fresh enrollment code to register again.
