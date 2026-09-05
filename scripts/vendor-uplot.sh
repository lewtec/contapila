#!/usr/bin/env bash
# Vendor uPlot into internal/web/static/vendor/uplot for offline embed.
# Usage: scripts/vendor-uplot.sh [version]
#   omit version → latest on npm
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/internal/web/static/vendor/uplot"
VERSION="${1:-}"

if [[ -z "$VERSION" ]]; then
  VERSION="$(curl -fsSL https://registry.npmjs.org/uplot/latest | sed -n 's/.*"version":"\([^"]*\)".*/\1/p' | head -1)"
fi
if [[ -z "$VERSION" ]]; then
  echo "could not resolve uPlot version" >&2
  exit 1
fi

BASE="https://cdn.jsdelivr.net/npm/uplot@${VERSION}/dist"
mkdir -p "$OUT"
curl -fsSL "$BASE/uPlot.iife.min.js" -o "$OUT/uplot.min.js"
curl -fsSL "$BASE/uPlot.min.css" -o "$OUT/uplot.min.css"
printf '%s\n' "$VERSION" > "$OUT/VERSION"
cat > "$OUT/README.md" << MD
# uPlot (vendored)

Version: **${VERSION}**

Update:

\`\`\`bash
./scripts/vendor-uplot.sh          # latest
./scripts/vendor-uplot.sh 1.6.32   # pin
\`\`\`

Source: ${BASE}
MD
echo "vendored uPlot $VERSION -> $OUT"
ls -la "$OUT"
