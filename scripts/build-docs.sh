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
mkdir -p "${SITE_DIR}/deploy-public-server" "${SITE_DIR}/assets/css"

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
echo "Building docs → ${SITE_DIR}"
echo

# --- Copy static assets ---------------------------------------------------
cp "${DOCS_DIR}/assets/css/style.css" "${SITE_DIR}/assets/css/style.css"

# --- Generate index.html (landing page) -----------------------------------
cat > "${SITE_DIR}/index.html" <<'INDEX_EOF'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Agent Control - Unified Control for AI Coding Tools</title>
    <meta name="description" content="A single unified control interface for managing Claude Code, Codex, and Gemini sessions.">
    <link rel="stylesheet" href="assets/css/style.css">
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.0.0/css/all.min.css">
</head>
<body>
    <header>
        <div class="container">
            <div class="logo">
                <i class="fas fa-network-wired"></i> Agent Control
            </div>
            <nav>
                <ul>
                    <li><a href="#features">Features</a></li>
                    <li><a href="#architecture">Architecture</a></li>
                    <li><a href="deploy-public-server.html">Docs</a></li>
                    <li><a href="https://github.com/xuzhougeng/agent-control" target="_blank"><i class="fab fa-github"></i> GitHub</a></li>
                </ul>
            </nav>
        </div>
    </header>

    <section class="hero">
        <div class="hero-content">
            <h1>Orchestrate Your AI Agents.</h1>
            <p>The centralized control plane for AI coding tools. Manage Claude Code, Codex, Gemini, and OpenCode sessions across multiple servers.</p>
            <div class="hero-buttons">
                <a href="deploy-public-server.html" class="btn btn-primary">Get Started</a>
                <a href="https://github.com/xuzhougeng/agent-control" class="btn btn-secondary">Star on GitHub</a>
            </div>
        </div>

        <div class="terminal-window">
            <div class="terminal-header">
                <div class="dot red"></div>
                <div class="dot yellow"></div>
                <div class="dot green"></div>
            </div>
            <div class="terminal-body">
                <div class="comment"># Start the control plane</div>
                <div class="command">
                    <span class="prompt">$</span>
                    <span class="cmd-text">./cc-control -addr :18080 -admin-token $ADMIN_TOKEN</span>
                </div>
                <div class="comment"># Issue tenant tokens via Admin API</div>
                <div class="command">
                    <span class="prompt">$</span>
                    <span class="cmd-text">curl -X POST https://api.ac.dev/admin/tokens</span>
                </div>
                <div class="comment"># Connect an agent securely</div>
                <div class="command">
                    <span class="prompt">$</span>
                    <span class="cmd-text">./cc-agent -control-url wss://api.ac.dev/ws/agent</span>
                </div>
                <div class="comment"># Agent connected. Ready to code.</div>
                <div class="command">
                    <span class="prompt">$</span>
                    <span class="cmd-text blinking-cursor">_</span>
                </div>
            </div>
        </div>
    </section>

    <section id="features" class="features">
        <h2 class="section-title">Why Agent Control?</h2>
        <div class="grid">
            <div class="card">
                <div class="card-icon"><i class="fas fa-server"></i></div>
                <h3>Multi-Server</h3>
                <p>One dashboard to rule them all. Manage diverse environments from a single pane of glass.</p>
            </div>
            <div class="card">
                <div class="card-icon"><i class="fas fa-lock"></i></div>
                <h3>Secure by Design</h3>
                <p>Outbound-only WebSocket connections. No inbound ports. Zero Trust architecture ready.</p>
            </div>
            <div class="card">
                <div class="card-icon"><i class="fas fa-terminal"></i></div>
                <h3>Real-time PTY</h3>
                <p>Full xterm.js integration. It feels like SSH, but smarter and web-native.</p>
            </div>
            <div class="card">
                <div class="card-icon"><i class="fas fa-history"></i></div>
                <h3>Audit Trails</h3>
                <p>Every command, every output. Full JSONL logging for compliance and replay.</p>
            </div>
            <div class="card">
                <div class="card-icon"><i class="fas fa-mobile-alt"></i></div>
                <h3>Native Apps</h3>
                <p>Control your agents on the go with dedicated macOS and iOS applications.</p>
            </div>
            <div class="card">
                <div class="card-icon"><i class="fas fa-code-branch"></i></div>
                <h3>Open Source</h3>
                <p>MIT Licensed. Hackable. Deploy it on your own infrastructure.</p>
            </div>
        </div>
    </section>

    <section id="architecture" class="architecture">
        <div class="container">
            <h2 class="section-title">Architecture</h2>
            <div class="mermaid-wrapper">
                <div class="mermaid">
    flowchart LR
        subgraph Clients
            Browser["Browser UI"]
            App["Native App"]
        end

        subgraph Control["Control Plane"]
            CC["cc-control"]
        end

        subgraph Agents
            A1["Agent 1"]
            A2["Agent 2"]
        end

        Clients -->|"REST/WS"| CC
        CC <-->|"Secure WS"| A1
        CC <-->|"Secure WS"| A2

        style Clients fill:#111,stroke:#333,color:#fff
        style Control fill:#111,stroke:#333,color:#fff
        style Agents fill:#111,stroke:#333,color:#fff
        style CC fill:#2563eb,stroke:#1d4ed8,color:#fff
                </div>
            </div>
        </div>
    </section>

    <footer>
        <div class="social-links">
            <a href="https://github.com/xuzhougeng/agent-control"><i class="fab fa-github"></i></a>
        </div>
        <p>&copy; 2026 Agent Control. MIT License.</p>
    </footer>

    <script type="module">
        import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
        mermaid.initialize({ startOnLoad: true, theme: 'dark', themeVariables: { darkMode: true, background: '#000000', primaryColor: '#2563eb', lineColor: '#666' } });
    </script>
</body>
</html>
INDEX_EOF
echo "  Generated: index.html"

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

    # --- Part 1: HTML head + header + nav (single-quote heredoc, no expansion) ---
    cat > "$html_file" <<'PART1_EOF'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
PART1_EOF

    # Title & asset paths (needs variable expansion)
    cat >> "$html_file" <<EOF
    <title>${title} - Agent Control</title>
    <link rel="stylesheet" href="${assets_prefix}assets/css/style.css">
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.0.0/css/all.min.css">
EOF

    cat >> "$html_file" <<'PART1B_EOF'
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/atom-one-dark.min.css">
    <style>
        .doc-content {
            max-width: 900px;
            margin: 0 auto;
            padding: 4rem 1.5rem;
            color: #d4d4d4;
        }
        .doc-content h1, .doc-content h2, .doc-content h3 {
            color: #fff;
            margin-top: 2rem;
            margin-bottom: 1rem;
        }
        .doc-content h1 { font-size: 2.5rem; border-bottom: 1px solid #333; padding-bottom: 1rem; }
        .doc-content h2 { font-size: 1.8rem; border-bottom: 1px solid #333; padding-bottom: 0.5rem; }
        .doc-content h3 { font-size: 1.4rem; color: #fff; margin-top: 1.5rem; }
        .doc-content p { line-height: 1.7; margin-bottom: 1.2rem; }
        .doc-content code { background: #2d2d2d; padding: 0.2rem 0.4rem; border-radius: 4px; font-family: 'JetBrains Mono', monospace; font-size: 0.9em; color: #e06c75; }
        .doc-content pre { background: #1e1e1e; padding: 1rem; border-radius: 8px; overflow-x: auto; margin-bottom: 1.5rem; border: 1px solid #333; }
        .doc-content pre code { background: none; color: inherit; padding: 0; }
        .doc-content table { width: 100%; border-collapse: collapse; margin-bottom: 1.5rem; }
        .doc-content th, .doc-content td { padding: 0.75rem; border: 1px solid #333; text-align: left; }
        .doc-content th { background: #252525; color: #fff; }
        .doc-content a { color: #3b82f6; text-decoration: none; }
        .doc-content a:hover { text-decoration: underline; }
        .doc-content ul, .doc-content ol { padding-left: 1.5rem; margin-bottom: 1.5rem; }
        .doc-content li { margin-bottom: 0.5rem; }
        .doc-content blockquote { border-left: 3px solid #3b82f6; padding-left: 1rem; color: #aaa; margin-bottom: 1.5rem; }
        .mermaid { background: #000; padding: 1rem; border-radius: 8px; margin-bottom: 1.5rem; text-align: center; }
        .back-link { display: inline-flex; align-items: center; color: #888; margin-bottom: 2rem; font-weight: 500; }
        .back-link:hover { color: #fff; text-decoration: none; }
        .back-link i { margin-right: 0.5rem; }
    </style>
</head>
<body>
    <header>
        <div class="container">
            <div class="logo">
PART1B_EOF

    # Logo & nav links (needs assets_prefix)
    cat >> "$html_file" <<EOF
                <a href="${assets_prefix}index.html" style="color: inherit; text-decoration: none; display: flex; align-items: center; gap: 0.5rem;">
                    <i class="fas fa-network-wired"></i> Agent Control
                </a>
            </div>
            <nav>
                <ul>
                    <li><a href="${assets_prefix}index.html#features">Features</a></li>
                    <li><a href="${assets_prefix}index.html#architecture">Architecture</a></li>
                    <li><a href="${assets_prefix}deploy-public-server.html" style="color: #fff;">Docs</a></li>
                    <li><a href="https://github.com/xuzhougeng/agent-control" target="_blank"><i class="fab fa-github"></i> GitHub</a></li>
                </ul>
            </nav>
        </div>
    </header>

    <div class="doc-content" id="content">
        <a href="${back_link}" class="back-link"><i class="fas fa-arrow-left"></i> ${back_text}</a>
        <div id="markdown-body"></div>
    </div>

    <footer>
        <div class="social-links">
            <a href="https://github.com/xuzhougeng/agent-control"><i class="fab fa-github"></i></a>
        </div>
        <p>&copy; 2026 Agent Control. MIT License.</p>
    </footer>

    <script type="text/markdown" id="markdown-source">
EOF

    # --- Part 2: markdown content (printf to avoid expansion of $ and backticks) ---
    printf '%s\n' "$md_content" >> "$html_file"

    # --- Part 3: JS rendering (single-quote heredoc, no expansion) ---
    cat >> "$html_file" <<'PART3_EOF'
    </script>

    <script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js"></script>
    <script type="module">
        import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';

        // Configure marked with highlight.js
        marked.setOptions({
            highlight: function(code, lang) {
                if (lang && hljs.getLanguage(lang)) {
                    return hljs.highlight(code, { language: lang }).value;
                }
                return hljs.highlightAuto(code).value;
            },
            breaks: true
        });

        // Initialize mermaid
        mermaid.initialize({ startOnLoad: false, theme: 'dark', themeVariables: { darkMode: true, background: '#000000', primaryColor: '#2563eb' } });

        // Render markdown
        const markdownSource = document.getElementById('markdown-source').textContent;
        const contentDiv = document.getElementById('markdown-body');
        contentDiv.innerHTML = marked.parse(markdownSource);

        // Render mermaid diagrams
        const mermaidBlocks = contentDiv.querySelectorAll('.language-mermaid');
        mermaidBlocks.forEach(async (block, index) => {
            const graphDefinition = block.textContent;
            const newDiv = document.createElement('div');
            newDiv.className = 'mermaid';
            newDiv.textContent = graphDefinition;
            block.parentNode.replaceWith(newDiv);
        });

        mermaid.run();
    </script>
</body>
</html>
PART3_EOF

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
