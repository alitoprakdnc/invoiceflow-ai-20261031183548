#!/usr/bin/env bash
set -euo pipefail

# ----------------------------------------------------------------------
# generate_openapi.sh
# ----------------------------------------------------------------------
# Generates the OpenAPI 3.0 specification for the InvoiceFlow AI backend.
#   1. Runs `swag init` against the Go entry point to produce a Swagger
#      definition (JSON & YAML) in the backend/docs directory.
#   2. If a manual spec (openapi/manual.yaml) exists, merges it with the
#      generated file using `yq` (deep merge, manual overrides take
#      precedence).
#   3. Writes the final merged document to openapi/openapi.yaml.
#
# The script aborts on any error and prints helpful messages.
# ----------------------------------------------------------------------

# Resolve script location and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ----------------------------------------------------------------------
# Configuration
# ----------------------------------------------------------------------
SWAG_CMD="swag"
YQ_CMD="yq"

# Paths
GO_ENTRYPOINT="$PROJECT_ROOT/backend/cmd/server/main.go"
SWAG_OUTPUT_DIR="$PROJECT_ROOT/backend/docs"
GENERATED_YAML="$SWAG_OUTPUT_DIR/swagger.yaml"

MANUAL_SPEC="$PROJECT_ROOT/openapi/manual.yaml"
MERGED_OUTPUT="$PROJECT_ROOT/openapi/openapi.yaml"

# ----------------------------------------------------------------------
# Helper: ensure required binaries are available
# ----------------------------------------------------------------------
for cmd in "$SWAG_CMD" "$YQ_CMD"; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: required command '$cmd' not found in PATH." >&2
    exit 1
  fi
done

# ----------------------------------------------------------------------
# Step 1 – Generate Swagger spec with swag
# ----------------------------------------------------------------------
echo "▶️  Generating Swagger spec via swag..."
"$SWAG_CMD" init \
  -g "$GO_ENTRYPOINT" \
  -o "$SWAG_OUTPUT_DIR" \
  --outputTypes yaml,json \
  >/dev/null

if [[ ! -f "$GENERATED_YAML" ]]; then
  echo "Error: swagger.yaml not found after swag execution (expected at $GENERATED_YAML)." >&2
  exit 1
fi
echo "✅  Generated $GENERATED_YAML"

# ----------------------------------------------------------------------
# Step 2 – Merge with manual overrides (if any)
# ----------------------------------------------------------------------
if [[ -f "$MANUAL_SPEC" ]]; then
  echo "▶️  Merging generated spec with manual overrides ($MANUAL_SPEC)..."
  # Deep merge: manual spec overrides generated spec where keys collide
  "$YQ_CMD" eval-all '
    select(fileIndex == 0) * select(fileIndex == 1)
  ' "$GENERATED_YAML" "$MANUAL_SPEC" > "$MERGED_OUTPUT"
else
  echo "⚠️  No manual spec found at $MANUAL_SPEC – using generated spec as final output."
  cp "$GENERATED_YAML" "$MERGED_OUTPUT"
fi

# ----------------------------------------------------------------------
# Step 3 – Validate final YAML (basic syntax check)
# ----------------------------------------------------------------------
if ! "$YQ_CMD" eval '.' "$MERGED_OUTPUT" >/dev/null; then
  echo "Error: merged OpenAPI spec is not valid YAML." >&2
  exit 1
fi

echo "✅  OpenAPI specification ready at $MERGED_OUTPUT"
echo "🎉  Generation complete."