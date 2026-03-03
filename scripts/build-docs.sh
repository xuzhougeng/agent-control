#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# build-docs.sh — Convert docs/*.md into a browsable static site at docs/_site/
#
# Dependencies: bash + sed (no Python/Node/pandoc needed)
# Rendering:    Browser-side via marked.js + highlight.js + mermaid (CDN)
# ---------------------------------------------------------------------------

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCS_DIR="${ROOT_DIR}/docs"
SITE_DIR="${DOCS_DIR}/_site"

# --- Clean & create output dirs -------------------------------------------
rm -rf "${SITE_DIR}"
mkdir -p "${SITE_DIR}/deploy-public-server" "${SITE_DIR}/assets/css" "${SITE_DIR}/assets/images"

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
echo "Building docs → ${SITE_DIR}"
echo

# --- Copy static assets ---------------------------------------------------
cp "${DOCS_DIR}/assets/css/style.css" "${SITE_DIR}/assets/css/style.css"
cp "${ROOT_DIR}/scripts/install.sh" "${SITE_DIR}/install.sh"
cp "${ROOT_DIR}/scripts/install.ps1" "${SITE_DIR}/install.ps1"
if [ -d "${DOCS_DIR}/assets/images" ]; then
    cp -R "${DOCS_DIR}/assets/images/." "${SITE_DIR}/assets/images/"
fi
echo "  Copied: install.sh, install.ps1"

# --- Copy bilingual landing pages --------------------------------------
cp "${DOCS_DIR}/assets/templates/index.en.html" "${SITE_DIR}/index.html"
cp "${DOCS_DIR}/assets/templates/index.en.html" "${SITE_DIR}/index.en.html"
cp "${DOCS_DIR}/assets/templates/index.zh.html" "${SITE_DIR}/index.zh.html"
echo "  Generated: index.html, index.en.html, index.zh.html"

# ---------------------------------------------------------------------------
# generate_doc_page  MD_FILE  HTML_FILE  TITLE  ASSETS_PREFIX  BACK_LINK  BACK_TEXT
# ---------------------------------------------------------------------------
generate_doc_page() {
    local md_file="$1"
    local html_file="$2"
    local title="$3"
    local assets_prefix="$4"   # "" for root-level, "../" for sub-dir pages
    local back_link="$5"
    local back_text="$6"

    # Read markdown and convert cross-references:
    #   `02-tls.md`  →  [02-tls.md](02-tls.html)   (backtick refs → links)
    #   (file.md)    →  (file.html)                  (existing link targets)
    local md_content
    md_content=$(sed \
        -e 's|`\(deploy-public-server/[0-9a-z-]*\.md\)`|[\1](\1)|g' \
        -e 's|`\([0-9][0-9a-z]*-[a-z_-]*\.md\)`|[\1](\1)|g' \
        -e 's|\.md)|.html)|g' \
        "$md_file")

    # Read template and inject variables + markdown content
    local tmp_md="${html_file}.md.tmp"
    echo "$md_content" > "$tmp_md"
    sed -e "s|{{TITLE}}|$title|g" \
        -e "s|{{ASSETS_PREFIX}}|$assets_prefix|g" \
        -e "s|{{BACK_LINK}}|$back_link|g" \
        -e "s|{{BACK_TEXT}}|$back_text|g" \
        "${DOCS_DIR}/assets/templates/doc-page.html" | \
        sed -e '/{{MARKDOWN_CONTENT}}/{
r '"$tmp_md"'
d
}' > "$html_file"
    rm -f "$tmp_md"

    echo "  Generated: ${html_file#"${SITE_DIR}"/}"
}

# ---------------------------------------------------------------------------
# Generate all pages
# ---------------------------------------------------------------------------

# Root-level docs
generate_doc_page \
    "${DOCS_DIR}/deploy-public-server.md" \
    "${SITE_DIR}/deploy-public-server.html" \
    "Deployment Guide" "" "index.html" "Back to Home"

generate_doc_page \
    "${DOCS_DIR}/architecture.md" \
    "${SITE_DIR}/architecture.html" \
    "Architecture" "" "index.html" "Back to Home"

generate_doc_page \
    "${DOCS_DIR}/api.md" \
    "${SITE_DIR}/api.html" \
    "API Reference" "" "index.html" "Back to Home"

generate_doc_page \
    "${DOCS_DIR}/getting-started.md" \
    "${SITE_DIR}/getting-started.html" \
    "Tutorial" "" "index.html" "Back to Home"

generate_doc_page \
    "${DOCS_DIR}/use-cases.md" \
    "${SITE_DIR}/use-cases.html" \
    "Use Cases" "" "getting-started.html" "Back to Tutorial"

generate_doc_page \
    "${DOCS_DIR}/privacy-policy.md" \
    "${SITE_DIR}/privacy-policy.html" \
    "Privacy Policy" "" "index.html" "Back to Home"

# Sub-directory docs (deploy-public-server/)
generate_doc_page \
    "${DOCS_DIR}/deploy-public-server/01-direct-http.md" \
    "${SITE_DIR}/deploy-public-server/01-direct-http.html" \
    "Part 1: Direct HTTP" "../" "../deploy-public-server.html" "Back to Deployment Guide"

generate_doc_page \
    "${DOCS_DIR}/deploy-public-server/02-tls.md" \
    "${SITE_DIR}/deploy-public-server/02-tls.html" \
    "Part 2: TLS" "../" "../deploy-public-server.html" "Back to Deployment Guide"

generate_doc_page \
    "${DOCS_DIR}/deploy-public-server/02a-cloudflare-tunnel.md" \
    "${SITE_DIR}/deploy-public-server/02a-cloudflare-tunnel.html" \
    "Part 2a: Cloudflare Tunnel" "../" "../deploy-public-server.html" "Back to Deployment Guide"

generate_doc_page \
    "${DOCS_DIR}/deploy-public-server/03-operations.md" \
    "${SITE_DIR}/deploy-public-server/03-operations.html" \
    "Part 3: Operations" "../" "../deploy-public-server.html" "Back to Deployment Guide"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo
FILE_COUNT=$(find "${SITE_DIR}" -name '*.html' | wc -l)
echo "Done! ${FILE_COUNT} HTML files generated in docs/_site/"
echo "Open docs/_site/index.html in a browser to preview."
echo
echo "Deploy to Cloudflare Pages:"
echo "  npx wrangler pages deploy docs/_site --project-name=agent-control"
echo ""
echo "After deploy, install cc-agent one-liners:"
echo "  Linux/macOS: curl -fsSL https://cc-remote.app/install.sh | bash"
echo "  Windows:    irm https://cc-remote.app/install.ps1 | iex"
