import { escapeHtml, decodeHtmlEntities } from "../shared/utils.js";

function sanitizeMarkdownHref(rawHref) {
  const decoded = decodeHtmlEntities(String(rawHref || "")).trim().replace(/^<+|>+$/g, "");
  if (!decoded) return "";
  const href = decoded.split(/\s+/)[0];
  if (/^(https?:\/\/|mailto:)/i.test(href)) return escapeHtml(href);
  if (/^(\/|#)/.test(href)) return escapeHtml(href);
  return "";
}

function renderInlineMarkdown(raw) {
  let html = escapeHtml(raw == null ? "" : raw);
  const placeholders = [];
  const stash = (markup) => {
    const token = `@@MD${placeholders.length}@@`;
    placeholders.push(markup);
    return token;
  };

  html = html.replace(/`([^`\n]+)`/g, (_m, code) => stash(`<code>${code}</code>`));
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_m, label, target) => {
    const href = sanitizeMarkdownHref(target);
    if (!href) return label;
    return stash(`<a href="${href}" target="_blank" rel="noopener noreferrer">${label}</a>`);
  });

  html = html.replace(/(\*\*|__)(?=\S)([\s\S]*?\S)\1/g, "<strong>$2</strong>");
  html = html.replace(/~~(?=\S)([\s\S]*?\S)~~/g, "<del>$1</del>");
  html = html.replace(/(^|[^*])\*(?=\S)([\s\S]*?\S)\*(?!\*)/g, "$1<em>$2</em>");
  html = html.replace(/(^|[^_])_(?=\S)([\s\S]*?\S)_(?!_)/g, "$1<em>$2</em>");

  return html.replace(/@@MD(\d+)@@/g, (_m, idx) => placeholders[Number(idx)] || "");
}

function isMarkdownBlockBoundary(line) {
  if (!line.trim()) return true;
  if (/^\s*```/.test(line)) return true;
  if (/^\s*#{1,6}\s+/.test(line)) return true;
  if (/^\s{0,3}([-*_])(\s*\1){2,}\s*$/.test(line)) return true;
  if (/^\s*>\s?/.test(line)) return true;
  if (/^\s*[-+*]\s+/.test(line)) return true;
  if (/^\s*\d+\.\s+/.test(line)) return true;
  return false;
}

function splitMarkdownTableRow(rawLine) {
  let line = String(rawLine == null ? "" : rawLine).trim();
  if (line.startsWith("|")) line = line.slice(1);
  if (line.endsWith("|")) line = line.slice(0, -1);
  return line.split("|").map((cell) => cell.trim());
}

function isMarkdownTableDivider(line) {
  const cells = splitMarkdownTableRow(line);
  if (cells.length < 2) return false;
  return cells.every((cell) => /^:?-{3,}:?$/.test(cell));
}

function parseTableAlignments(dividerLine) {
  return splitMarkdownTableRow(dividerLine).map((cell) => {
    const left = cell.startsWith(":");
    const right = cell.endsWith(":");
    if (left && right) return "center";
    if (right) return "right";
    return "left";
  });
}

function isMarkdownTableStart(lines, idx) {
  if (idx < 0 || idx + 1 >= lines.length) return false;
  const header = lines[idx];
  const divider = lines[idx + 1];
  if (!header || !divider) return false;
  if (header.indexOf("|") < 0) return false;
  return isMarkdownTableDivider(divider);
}

export function renderChatMarkdown(raw) {
  const text = String(raw == null ? "" : raw).replace(/\r\n?/g, "\n");
  const lines = text.split("\n");
  const blocks = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) {
      i += 1;
      continue;
    }

    if (/^\s*```/.test(line)) {
      const lang = line.trim().slice(3).trim().split(/\s+/)[0] || "";
      i += 1;
      const codeLines = [];
      while (i < lines.length && !/^\s*```/.test(lines[i])) {
        codeLines.push(lines[i]);
        i += 1;
      }
      if (i < lines.length) i += 1;
      const langClass = lang ? ` class="language-${escapeHtml(lang)}"` : "";
      blocks.push(`<pre class="chat-md-code"><code${langClass}>${escapeHtml(codeLines.join("\n"))}</code></pre>`);
      continue;
    }

    const heading = line.match(/^\s*(#{1,6})\s+(.*)$/);
    if (heading) {
      const level = heading[1].length;
      blocks.push(`<h${level}>${renderInlineMarkdown(heading[2])}</h${level}>`);
      i += 1;
      continue;
    }

    if (/^\s{0,3}([-*_])(\s*\1){2,}\s*$/.test(line)) {
      blocks.push("<hr>");
      i += 1;
      continue;
    }

    if (/^\s*>\s?/.test(line)) {
      const quoteLines = [];
      while (i < lines.length && /^\s*>\s?/.test(lines[i])) {
        quoteLines.push(lines[i].replace(/^\s*>\s?/, ""));
        i += 1;
      }
      const quoteHTML = renderInlineMarkdown(quoteLines.join("\n")).replace(/\n/g, "<br>");
      blocks.push(`<blockquote>${quoteHTML}</blockquote>`);
      continue;
    }

    if (/^\s*[-+*]\s+/.test(line)) {
      const items = [];
      while (i < lines.length && /^\s*[-+*]\s+/.test(lines[i])) {
        items.push(`<li>${renderInlineMarkdown(lines[i].replace(/^\s*[-+*]\s+/, ""))}</li>`);
        i += 1;
      }
      blocks.push(`<ul>${items.join("")}</ul>`);
      continue;
    }

    if (/^\s*\d+\.\s+/.test(line)) {
      const items = [];
      while (i < lines.length && /^\s*\d+\.\s+/.test(lines[i])) {
        items.push(`<li>${renderInlineMarkdown(lines[i].replace(/^\s*\d+\.\s+/, ""))}</li>`);
        i += 1;
      }
      blocks.push(`<ol>${items.join("")}</ol>`);
      continue;
    }

    if (isMarkdownTableStart(lines, i)) {
      const headerCells = splitMarkdownTableRow(lines[i]);
      const aligns = parseTableAlignments(lines[i + 1]);
      i += 2;
      const bodyRows = [];
      while (i < lines.length) {
        const rowLine = lines[i];
        if (!rowLine.trim()) break;
        if (rowLine.indexOf("|") < 0) break;
        if (isMarkdownTableDivider(rowLine)) break;
        bodyRows.push(splitMarkdownTableRow(rowLine));
        i += 1;
      }

      const headerHTML = headerCells.map((cell, colIdx) => {
        const align = aligns[colIdx] || "left";
        return `<th style="text-align:${align}">${renderInlineMarkdown(cell)}</th>`;
      }).join("");

      const bodyHTML = bodyRows.map((row) => {
        const rowHTML = headerCells.map((_unused, colIdx) => {
          const align = aligns[colIdx] || "left";
          const cell = row[colIdx] || "";
          return `<td style="text-align:${align}">${renderInlineMarkdown(cell)}</td>`;
        }).join("");
        return `<tr>${rowHTML}</tr>`;
      }).join("");

      blocks.push(`<div class="chat-md-table-wrap"><table><thead><tr>${headerHTML}</tr></thead><tbody>${bodyHTML}</tbody></table></div>`);
      continue;
    }

    const para = [];
    while (i < lines.length && !isMarkdownBlockBoundary(lines[i]) && !isMarkdownTableStart(lines, i)) {
      para.push(lines[i]);
      i += 1;
    }
    blocks.push(`<p>${renderInlineMarkdown(para.join("\n")).replace(/\n/g, "<br>")}</p>`);
  }

  return blocks.join("");
}
