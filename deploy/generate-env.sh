#!/usr/bin/env bash
set -euo pipefail

# Dynamic SSM → .env generator.
# It fetches EVERY parameter under /englishkorat/<stage>/ (default stage=production)
# and maps the last path segment (after stage) to an uppercased env var.
# Example: /englishkorat/production/db_host -> DB_HOST
#          /englishkorat/production/redis_password -> REDIS_PASSWORD
# Add new secret: just create new SSM param with that naming pattern; redeploy.

BASE_PATH=${SSM_BASE_PATH:-/englishkorat}
STAGE=${STAGE:-production}
AWS_REGION_DEFAULT="ap-southeast-1"
AWS_REGION_ENV=${AWS_REGION:-$AWS_REGION_DEFAULT}
FULL_PREFIX="${BASE_PATH%/}/$STAGE"

echo "Scanning SSM path: $FULL_PREFIX (region: $AWS_REGION_ENV)" >&2

if ! command -v aws >/dev/null 2>&1; then
  echo "ERROR: aws cli not installed on host" >&2
  exit 1
fi

TMP_FILE=".env.new"
echo "# Auto-generated $(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$TMP_FILE"
echo "AWS_REGION=$AWS_REGION_ENV" >> "$TMP_FILE"
echo "STAGE=$STAGE" >> "$TMP_FILE"
echo "USE_SSM=true" >> "$TMP_FILE"

# Paginate through parameters reliably using JSON parsing (python3 required)
NEXT_TOKEN=""
COUNT=0
while :; do
  if [ -n "$NEXT_TOKEN" ]; then
    RESP_JSON=$(aws ssm get-parameters-by-path --with-decryption --path "$FULL_PREFIX" --recursive --region "$AWS_REGION_ENV" --max-results 10 --next-token "$NEXT_TOKEN" --output json || true)
  else
    RESP_JSON=$(aws ssm get-parameters-by-path --with-decryption --path "$FULL_PREFIX" --recursive --region "$AWS_REGION_ENV" --max-results 10 --output json || true)
  fi

  if [ -z "$RESP_JSON" ] || [ "$RESP_JSON" = "null" ]; then
    break
  fi

  # Parse parameters and next token using python3
  PARSED=$(printf '%s' "$RESP_JSON" | python3 - <<'PY'
import sys, json
data = json.load(sys.stdin)
params = data.get('Parameters', [])
for p in params:
    name = p.get('Name','')
    value = p.get('Value','')
    # output tab separated
    print(name + '\t' + value.replace('\n',' '))
nt = data.get('NextToken')
if nt is None:
    print('NEXT_TOKEN::')
else:
    print('NEXT_TOKEN::' + nt)
PY
)

  if [ -z "$PARSED" ]; then
    break
  fi

  while IFS=$'\t' read -r name value; do
    if [[ "$name" == NEXT_TOKEN::* ]]; then
      NT=${name#NEXT_TOKEN::}${value}
      NEXT_TOKEN=${NT}
      continue
    fi
    [ -z "$name" ] && continue
    key=${name##*/}
    env_key=$(echo "$key" | tr '[:lower:]' '[:upper:]')
    # Avoid overwriting if duplicate
    if grep -q "^$env_key=" "$TMP_FILE" 2>/dev/null; then
      echo "Skipping duplicate $env_key" >&2
      continue
    fi
    esc=$(printf '%s' "$value" | tr '\n' ' ')
    if echo "$env_key" | grep -qi 'PASSWORD\|SECRET\|KEY'; then
      echo "Adding $env_key=*** (secret)" >&2
    else
      echo "Adding $env_key" >&2
    fi
    echo "$env_key=$esc" >> "$TMP_FILE"
    COUNT=$((COUNT+1))
  done <<< "$PARSED"

  # Stop if no next token
  if [ -z "$NEXT_TOKEN" ]; then
    break
  fi
done

# Ensure essentials for compose if not in SSM (fallbacks)
grep -q '^PORT=' "$TMP_FILE" || echo 'PORT=3000' >> "$TMP_FILE"
grep -q '^APP_ENV=' "$TMP_FILE" || echo 'APP_ENV=production' >> "$TMP_FILE"

mv "$TMP_FILE" .env
echo "Written .env with $COUNT parameters (secrets hidden)." >&2
grep -Ev 'PASSWORD=|SECRET=|KEY=' .env || true
echo "Done." >&2
