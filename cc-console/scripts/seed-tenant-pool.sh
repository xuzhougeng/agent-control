#!/usr/bin/env bash
set -euo pipefail

CC_CONTROL_URL="${CC_CONTROL_URL:?Set CC_CONTROL_URL}"
CC_ADMIN_TOKEN="${CC_ADMIN_TOKEN:?Set CC_ADMIN_TOKEN}"
COUNT="${1:-1000}"
NOW=$(date +%s)000

echo "Generating $COUNT tenants from $CC_CONTROL_URL ..."

SQL_VALUES=""
OK=0
FAIL=0

for i in $(seq 1 "$COUNT"); do
  RESP=$(curl -sk -X POST "$CC_CONTROL_URL/admin/tokens" \
    -H "Authorization: Bearer $CC_ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"type":"tenant"}' 2>/dev/null) || true

  TENANT_ID=$(echo "$RESP" | grep -o '"tenant_id":"[^"]*"' | head -1 | cut -d'"' -f4)
  TOKEN=$(echo "$RESP" | grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)

  if [ -z "$TENANT_ID" ] || [ -z "$TOKEN" ]; then
    FAIL=$((FAIL + 1))
    echo "  [$i] FAILED: $RESP"
    continue
  fi

  OK=$((OK + 1))
  SQL_VALUES="${SQL_VALUES}('${TENANT_ID}','${TOKEN}',${NOW}),"

  if [ $((OK % 50)) -eq 0 ]; then
    SQL_VALUES="${SQL_VALUES%,}"
    echo "INSERT INTO tenant_pool (tenant_id, tenant_token, created_at) VALUES ${SQL_VALUES};" > /tmp/seed_batch.sql
    npx wrangler d1 execute cc-console-db --remote --file=/tmp/seed_batch.sql
    SQL_VALUES=""
    echo "  Inserted $OK so far..."
  fi

  [ $((i % 10)) -eq 0 ] && echo "  [$i/$COUNT] ok=$OK fail=$FAIL"
done

if [ -n "$SQL_VALUES" ]; then
  SQL_VALUES="${SQL_VALUES%,}"
  echo "INSERT INTO tenant_pool (tenant_id, tenant_token, created_at) VALUES ${SQL_VALUES};" > /tmp/seed_batch.sql
  npx wrangler d1 execute cc-console-db --remote --file=/tmp/seed_batch.sql
fi

rm -f /tmp/seed_batch.sql
echo "Done. ok=$OK fail=$FAIL"
