#!/bin/sh
set -eu
umask 077

BASE="${GARAGE_ADMIN_URL:-http://garage:3903}"
AUTH="Authorization: Bearer ${GARAGE_ADMIN_TOKEN:?GARAGE_ADMIN_TOKEN is required}"
CREDS_DIR="${GARAGE_CREDENTIALS_DIR:-/run/garage}"
KEY_NAME="UMKM App Key"

api() {
  method="$1"
  endpoint="$2"
  shift 2
  curl --fail --silent --show-error --connect-timeout 3 --max-time 10 \
    -X "$method" -H "$AUTH" -H 'Content-Type: application/json' \
    "$BASE/v2/$endpoint" "$@"
}

echo "Waiting for Garage admin API..."
attempt=0
until STATUS="$(api GET GetClusterStatus 2>/dev/null)"; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    echo "ERROR: Garage admin API is unavailable; check GARAGE_ADMIN_TOKEN and Garage logs." >&2
    exit 1
  fi
  sleep 2
done

# This Compose stack manages exactly one local Garage node.
printf '%s' "$STATUS" | jq -e '.nodes | length == 1' >/dev/null
NODE_ID="$(printf '%s' "$STATUS" | jq -er '.nodes[0].id')"
LAYOUT_VERSION="$(printf '%s' "$STATUS" | jq -er '.layoutVersion')"
if printf '%s' "$STATUS" | jq -e '.nodes[0].role == null' >/dev/null; then
  LAYOUT="$(jq -nc --arg id "$NODE_ID" \
    '{roles:[{id:$id,zone:"dc1",capacity:1073741824000,tags:[]}],parameters:{zoneRedundancy:{atLeast:1}}}')"
  api POST UpdateClusterLayout -d "$LAYOUT" >/dev/null
  api POST ApplyClusterLayout -d "{\"version\":$((LAYOUT_VERSION + 1))}" >/dev/null
fi

# Recover the same named key even if only the credential volume was lost.
# Never delete unrelated keys or rotate credentials on a normal restart.
KEYS="$(api GET ListKeys)"
MATCHES="$(printf '%s' "$KEYS" | jq -c --arg name "$KEY_NAME" '[.[] | select(.name == $name)]')"
case "$(printf '%s' "$MATCHES" | jq 'length')" in
  0) KEY="$(api POST CreateKey -d "$(jq -nc --arg name "$KEY_NAME" '{name:$name}')")" ;;
  1)
    KEY_ID="$(printf '%s' "$MATCHES" | jq -er '.[0].id')"
    KEY="$(api GET "GetKeyInfo?id=$KEY_ID&showSecretKey=true")"
    ;;
  *)
    echo "ERROR: Multiple keys named '$KEY_NAME'; resolve the duplicate names in Garage before retrying." >&2
    exit 1
    ;;
esac

# Garage-generated credentials are alphanumeric; validate before writing a shell env file.
printf '%s' "$KEY" | jq -e '
  (.expired != true) and
  (.accessKeyId | type == "string" and test("^[A-Za-z0-9]+$")) and
  (.secretAccessKey | type == "string" and test("^[A-Za-z0-9]+$"))' >/dev/null
ACCESS_KEY="$(printf '%s' "$KEY" | jq -r '.accessKeyId')"
SECRET_KEY="$(printf '%s' "$KEY" | jq -r '.secretAccessKey')"

for bucket in \
  "${OBJECT_STORAGE_BUCKET_DOCUMENTS:-documents}" \
  "${OBJECT_STORAGE_BUCKET_PARTNERSHIP_FILES:-partnership-files}"; do
  BUCKETS="$(api GET ListBuckets)"
  BUCKET_ID="$(printf '%s' "$BUCKETS" | jq -r --arg name "$bucket" \
    '.[] | select(.globalAliases | index($name)) | .id')"
  if [ -z "$BUCKET_ID" ]; then
    BUCKET="$(api POST CreateBucket -d "$(jq -nc --arg name "$bucket" '{globalAlias:$name}')")"
    BUCKET_ID="$(printf '%s' "$BUCKET" | jq -er '.id')"
  fi
  PERMISSIONS="$(jq -nc --arg bucket "$BUCKET_ID" --arg key "$ACCESS_KEY" \
    '{bucketId:$bucket,accessKeyId:$key,permissions:{read:true,write:true,owner:true}}')"
  api POST AllowBucketKey -d "$PERMISSIONS" >/dev/null
done

# Publish only after all setup steps succeed. Both consumers run as UID 10001.
mkdir -p "$CREDS_DIR"
TEMP_FILE="$(mktemp "$CREDS_DIR/.garage.env.XXXXXX")"
trap 'rm -f "$TEMP_FILE"' EXIT HUP INT TERM
printf 'OBJECT_STORAGE_ACCESS_KEY=%s\nOBJECT_STORAGE_SECRET_KEY=%s\n' \
  "$ACCESS_KEY" "$SECRET_KEY" > "$TEMP_FILE"
chmod 600 "$TEMP_FILE"
mv "$TEMP_FILE" "$CREDS_DIR/garage.env"
echo "Garage layout, buckets, and shared credentials are ready."
