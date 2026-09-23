# Device enrollment

## Create an enrollment code

```bash
curl -sS -X POST https://codebridge.example.com/admin/enrollments \
  -H "Authorization: Bearer $CODEBRIDGE_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"ttl_seconds":600,"account_id":"your-oauth-subject"}'
```

Enrollment codes:

- are random 256-bit values;
- are stored only as SHA-256 digests by the Manager;
- expire automatically;
- can be consumed exactly once;
- carry the `account_id` (1-128 bytes) that the enrolled device will belong to; when omitted, the device joins the `default` account.

## Accounts and device visibility

Every device belongs to exactly one account. An MCP caller authenticated through OAuth only sees and can only call devices in their own account; a wrong-account device ID behaves exactly like an unknown device ID, so accounts cannot be probed. See [oauth.md](oauth.md#accounts-and-device-visibility) for how subjects map to accounts.

Device IDs are unique per Manager: a device ID that is already enrolled cannot be enrolled into another account. Revoke it first if the ID must be reused.

The admin API is deployment-wide by design (loopback-bound, bearer-token protected): `GET /admin/devices` lists every device with its `account_id`, and revocation/rotation work across accounts.

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
