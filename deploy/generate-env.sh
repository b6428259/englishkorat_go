#!/usr/bin/env bash
set -euo pipefail

# If DEBUG=1 show commands
[ "${DEBUG:-0}" = "1" ] && set -x

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
AWS_PROFILE_OPT=""
if [ -n "${AWS_PROFILE:-}" ]; then
  AWS_PROFILE_OPT="--profile $AWS_PROFILE"
fi
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
ATTEMPT=0
MAX_ATTEMPTS=3
SLEEP_BASE=2
while :; do
  if [ -n "$NEXT_TOKEN" ]; then
    RESP_JSON=$(aws ssm get-parameters-by-path $AWS_PROFILE_OPT --with-decryption --path "$FULL_PREFIX" --recursive --region "$AWS_REGION_ENV" --max-results 10 --next-token "$NEXT_TOKEN" --output json 2>&1 || true)
  else
    RESP_JSON=$(aws ssm get-parameters-by-path $AWS_PROFILE_OPT --with-decryption --path "$FULL_PREFIX" --recursive --region "$AWS_REGION_ENV" --max-results 10 --output json 2>&1 || true)
  fi
  
  # Treat whitespace-only as empty
  if ! echo "$RESP_JSON" | grep -q '[^[:space:]]'; then
    if [ $COUNT -eq 0 ] && [ -z "$NEXT_TOKEN" ]; then
      ATTEMPT=$((ATTEMPT+1))
      if [ $ATTEMPT -lt $MAX_ATTEMPTS ]; then
        WAIT=$((SLEEP_BASE * ATTEMPT))
        echo "WARN: Empty SSM response, retrying in ${WAIT}s (attempt ${ATTEMPT}/${MAX_ATTEMPTS})" >&2
        sleep $WAIT
        continue
      else
        echo "ERROR: Empty/whitespace response from SSM after retries (check credentials / region / permissions)" >&2
        rm -f "$TMP_FILE"
        exit 1
      fi
    fi
    break
  fi

  # Parse parameters and next token using python3
  PARSED=$(echo "$RESP_JSON" | python3 -c "
import sys, json
try:
  data = json.load(sys.stdin)
  params = data.get('Parameters', [])
  for p in params:
    name = p.get('Name','')
    value = p.get('Value','')
    print(name + '\t' + value.replace('\n',' '))
  nt = data.get('NextToken')
  print('NEXT_TOKEN::' + (nt or ''))
except Exception as e:
  print('PARSE_ERROR')
")

  if [ -z "$PARSED" ] || echo "$PARSED" | grep -q '^PARSE_ERROR$'; then
    ATTEMPT=$((ATTEMPT+1))
    if [ $ATTEMPT -lt $MAX_ATTEMPTS ]; then
      WAIT=$((SLEEP_BASE * ATTEMPT))
      echo "WARN: Parse error on SSM response, retrying in ${WAIT}s (attempt ${ATTEMPT}/${MAX_ATTEMPTS})" >&2
      sleep $WAIT
      continue
    else
      echo "ERROR: Failed to parse SSM response after retries" >&2
      rm -f "$TMP_FILE"
      exit 1
    fi
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

# Validate required keys exist before writing final .env to avoid partial/invalid files
required_keys=(DB_PASSWORD DB_HOST DB_USER)
missing=()
for k in "${required_keys[@]}"; do
  if ! grep -q "^${k}=" "$TMP_FILE" 2>/dev/null; then
    missing+=("$k")
  fi
done
if [ ${#missing[@]} -ne 0 ]; then
  echo "ERROR: Missing required keys from SSM: ${missing[*]}" >&2
  rm -f "$TMP_FILE"
  exit 1
fi

mv "$TMP_FILE" .env
echo "Written .env with $COUNT parameters (secrets hidden)." >&2
grep -Ev 'PASSWORD=|SECRET=|KEY=' .env || true
echo "Done." >&2
