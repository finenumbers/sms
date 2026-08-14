#!/usr/bin/env python3
"""Extract structured Runexis DIDAPI reference from vendor HTML.

Reads docs/vendor/runexis/DIDAPI Documentation.html and writes:
  - docs/reference/runexis/ENDPOINTS.json
  - docs/reference/runexis/{OVERVIEW,AUTH,SMS,NUMBERS,SIMS_SMS,WEBHOOKS}.md

Re-run after replacing the vendor HTML archive.
"""

from __future__ import annotations

import html as htmlmod
import json
import re
from collections import defaultdict
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
VENDOR_HTML = ROOT / "docs/vendor/runexis/DIDAPI Documentation.html"
OUT_DIR = ROOT / "docs/reference/runexis"

BASE_URL = "https://didapi.runexis.ru"

# Sections we materialize as focused markdown (plus full JSON catalog).
FOCUS_SECTIONS = {
    "auth": "AUTH.md",
    "sms": "SMS.md",
    "numbers": "NUMBERS.md",
    "sims": "SIMS_SMS.md",
    "webhook": "WEBHOOKS.md",
}

# For SIMS we only keep SMS-related endpoints in the focused markdown.
SIMS_SMS_TITLE_RE = re.compile(r"SMS|СМС|sms", re.I)


def clean_text(value: str) -> str:
    value = re.sub(r"<[^>]+>", " ", value)
    value = htmlmod.unescape(value)
    return re.sub(r"\s+", " ", value).strip()


def extract_balanced_json(text: str, start: int = 0) -> str | None:
    brace = text.find("{", start)
    if brace < 0:
        return None
    depth = 0
    in_str = False
    escape = False
    for i, ch in enumerate(text[brace:], start=brace):
        if in_str:
            if escape:
                escape = False
            elif ch == "\\":
                escape = True
            elif ch == '"':
                in_str = False
            continue
        if ch == '"':
            in_str = True
        elif ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return text[brace : i + 1]
    return None


def strip_tags_keep_newlines(chunk: str) -> str:
    chunk = re.sub(r"<(script|style)[^>]*>.*?</\1>", "", chunk, flags=re.I | re.S)
    chunk = re.sub(r"<br\s*/?>", "\n", chunk, flags=re.I)
    chunk = re.sub(r"</(p|div|li|tr|h\d|pre|code)>", "\n", chunk, flags=re.I)
    chunk = re.sub(r"<li[^>]*>", "- ", chunk, flags=re.I)
    chunk = re.sub(r"<[^>]+>", "", chunk)
    chunk = htmlmod.unescape(chunk)
    chunk = re.sub(r"[ \t]+", " ", chunk)
    chunk = re.sub(r"\n\s*\n+", "\n\n", chunk)
    return chunk.strip()


def split_h1_sections(raw: str) -> list[dict[str, Any]]:
    matches = list(re.finditer(r'<h1([^>]*)>(.*?)</h1>', raw, re.I | re.S))
    sections: list[dict[str, Any]] = []
    for i, match in enumerate(matches):
        attrs, title_html = match.group(1), match.group(2)
        sid_m = re.search(r'id="([^"]+)"', attrs)
        sid = sid_m.group(1) if sid_m else clean_text(title_html).lower().replace(" ", "-")
        end = matches[i + 1].start() if i + 1 < len(matches) else len(raw)
        sections.append(
            {
                "id": sid,
                "title": clean_text(title_html),
                "html": raw[match.end() : end],
            }
        )
    return sections


def parse_param_blocks(chunk: str, heading: str) -> list[dict[str, Any]]:
    """Parse Scribe-style Body/URL/Query/Response parameter blocks."""
    pattern = re.compile(
        rf"{re.escape(heading)}\s*(.*?)(?="
        r"Body Parameters|Query Parameters|URL Parameters|Response Fields|"
        r"Headers\s|Request\s|Example request:|Received response:|$)",
        re.I | re.S,
    )
    match = pattern.search(chunk)
    if not match:
        return []

    block = match.group(1)
    # Split on parameter name lines that look like: name\n type
    text = strip_tags_keep_newlines(block)
    params: list[dict[str, Any]] = []
    # Heuristic: lines with identifier then type keywords nearby in cleaned text.
    # Work from HTML instead for better accuracy.
    # Find strong/b parameter names in original block HTML.
    name_iter = list(
        re.finditer(
            r"(?:<strong>|<b[^>]*>|class=\"[^\"]*attribute[^\"]*\"[^>]*>)\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*<",
            block,
            re.I,
        )
    )
    if not name_iter:
        # Fallback: look for bare names before type badges
        for m in re.finditer(
            r">\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*<[^>]*>\s*(string|number|integer|boolean|object|array)",
            block,
            re.I,
        ):
            name_iter.append(m)

    # Simpler reliable approach for this HTML: after "Body Parameters" etc,
    # parameters appear as plain text names followed by type words in stripped text.
    lines = [ln.strip() for ln in text.splitlines() if ln.strip()]
    i = 0
    while i < len(lines):
        name = lines[i]
        if not re.fullmatch(r"[a-zA-Z_][a-zA-Z0-9_]*", name):
            i += 1
            continue
        typ = None
        optional = False
        desc_parts: list[str] = []
        j = i + 1
        while j < len(lines):
            nxt = lines[j]
            if re.fullmatch(r"[a-zA-Z_][a-zA-Z0-9_]*", nxt) and j + 1 < len(lines):
                # next param candidate
                if any(
                    lines[j + 1].lower().startswith(t)
                    for t in ("string", "number", "integer", "boolean", "object", "array")
                ):
                    break
            if nxt.lower() in {"string", "number", "integer", "boolean", "object"} or nxt.lower().startswith(
                ("string", "number", "integer", "boolean", "object", "array")
            ):
                typ = nxt
                j += 1
                continue
            if nxt.lower() in {"optional", "required"}:
                optional = nxt.lower() == "optional"
                j += 1
                continue
            if nxt.lower() in {"true", "false"}:
                j += 1
                continue
            if nxt.startswith("Example:"):
                desc_parts.append(nxt)
                j += 1
                continue
            if nxt.startswith("Must be") or nxt.startswith("Must start"):
                desc_parts.append(nxt)
                j += 1
                continue
            if nxt.startswith("- "):
                desc_parts.append(nxt)
                j += 1
                continue
            # description prose
            if not re.fullmatch(r"[a-zA-Z_][a-zA-Z0-9_]*", nxt):
                desc_parts.append(nxt)
                j += 1
                continue
            break
        if typ:
            params.append(
                {
                    "name": name,
                    "type": typ,
                    "optional": optional,
                    "description": " ".join(desc_parts).strip(),
                }
            )
            i = j
        else:
            i += 1
    return params


def parse_endpoint(section_id: str, title: str, chunk_html: str) -> dict[str, Any] | None:
    methods = re.findall(r"curl --request (GET|POST|PUT|PATCH|DELETE)", chunk_html)
    urls = re.findall(r"https://didapi\.runexis\.ru(/api/v1/[^\s\"'?]+)", chunk_html)

    # Scribe forms carry canonical templated paths:
    # <form data-method="PATCH" data-path="api/v1/numbers/{number}/sms/dlr-url" ...>
    form = re.search(
        r'<form[^>]*data-method="([A-Z]+)"[^>]*data-path="([^"]+)"[^>]*>',
        chunk_html,
        re.I,
    )
    if not form:
        form = re.search(
            r'<form[^>]*data-path="([^"]+)"[^>]*data-method="([A-Z]+)"[^>]*>',
            chunk_html,
            re.I,
        )
        if form:
            path_raw, method_raw = form.group(1), form.group(2)
            form_data = (method_raw, path_raw)
        else:
            form_data = None
    else:
        form_data = (form.group(1), form.group(2))

    method = None
    path = None
    if form_data:
        method = form_data[0].upper()
        path = "/" + form_data[1].lstrip("/")
    elif methods and urls:
        method = methods[0].upper()
        path = urls[0]
    elif urls:
        path = urls[0]
        method = methods[0].upper() if methods else None
    else:
        return None

    req_body = None
    mpay = re.search(r"payload\s*=\s*(\{.*?\})\s*headers\s*=", chunk_html, re.S)
    if mpay:
        req_body_raw = re.sub(r"<[^>]+>", "", mpay.group(1))
        req_body_raw = htmlmod.unescape(req_body_raw)
        try:
            req_body = json.loads(req_body_raw)
        except json.JSONDecodeError:
            req_body = req_body_raw.strip()

    resp = None
    mresp = re.search(r"Example response \(200\):", chunk_html)
    if mresp:
        sub = re.sub(r"<[^>]+>", "", chunk_html[mresp.end() : mresp.end() + 12000])
        sub = htmlmod.unescape(sub)
        candidate = extract_balanced_json(sub)
        if candidate:
            try:
                resp = json.loads(candidate)
            except json.JSONDecodeError:
                resp = candidate

    auth_required = "requires authentication" in chunk_html.lower()

    return {
        "section": section_id,
        "title": title,
        "method": method,
        "path": path,
        "auth_required": auth_required,
        "url_parameters": parse_param_blocks(chunk_html, "URL Parameters"),
        "query_parameters": parse_param_blocks(chunk_html, "Query Parameters"),
        "body_parameters": parse_param_blocks(chunk_html, "Body Parameters"),
        "response_fields": parse_param_blocks(chunk_html, "Response Fields"),
        "example_request_body": req_body,
        "example_response": resp,
        "example_urls": list(dict.fromkeys(urls))[:5],
    }


def parse_section_endpoints(section: dict[str, Any]) -> list[dict[str, Any]]:
    html = section["html"]
    h2s = list(re.finditer(r"<h2[^>]*>(.*?)</h2>", html, re.I | re.S))
    endpoints: list[dict[str, Any]] = []
    for i, h2 in enumerate(h2s):
        title = clean_text(h2.group(1))
        # Skip non-endpoint group headers in Operator section etc.
        chunk = html[h2.end() : (h2s[i + 1].start() if i + 1 < len(h2s) else len(html))]
        if "curl --request" not in chunk and "api/v1/" not in chunk:
            continue
        ep = parse_endpoint(section["id"], title, chunk)
        if ep:
            endpoints.append(ep)
    return endpoints


def md_escape(text: str) -> str:
    return text.replace("|", "\\|")


def render_params_table(params: list[dict[str, Any]]) -> str:
    if not params:
        return "_None documented._\n"
    lines = ["| Name | Type | Optional | Description |", "|---|---|---|---|"]
    for p in params:
        lines.append(
            "| {name} | {typ} | {opt} | {desc} |".format(
                name=md_escape(p.get("name", "")),
                typ=md_escape(str(p.get("type", ""))),
                opt="yes" if p.get("optional") else "no",
                desc=md_escape(p.get("description", "")),
            )
        )
    return "\n".join(lines) + "\n"


def render_endpoint_md(ep: dict[str, Any]) -> str:
    parts = [
        f"### {ep['title']}",
        "",
        f"- **Method:** `{ep.get('method') or 'UNKNOWN'}`",
        f"- **Path:** `{ep.get('path') or 'UNKNOWN'}`",
        f"- **Auth:** {'required' if ep.get('auth_required') else 'not required'}",
        "",
    ]
    if ep.get("url_parameters"):
        parts += ["**URL parameters**", "", render_params_table(ep["url_parameters"])]
    if ep.get("query_parameters"):
        parts += ["**Query parameters**", "", render_params_table(ep["query_parameters"])]
    if ep.get("body_parameters"):
        parts += ["**Body parameters**", "", render_params_table(ep["body_parameters"])]
    if ep.get("example_request_body") is not None:
        parts += [
            "**Example request body**",
            "",
            "```json",
            json.dumps(ep["example_request_body"], ensure_ascii=False, indent=2)
            if not isinstance(ep["example_request_body"], str)
            else ep["example_request_body"],
            "```",
            "",
        ]
    if ep.get("example_response") is not None:
        parts += [
            "**Example response**",
            "",
            "```json",
            json.dumps(ep["example_response"], ensure_ascii=False, indent=2)
            if not isinstance(ep["example_response"], str)
            else ep["example_response"],
            "```",
            "",
        ]
    elif ep.get("path") == "/api/v1/sms/send":
        parts += [
            "**Example response**",
            "",
            "_Not present in vendor HTML — see GAPS.md._",
            "",
        ]
    return "\n".join(parts)


def write_overview(path: Path) -> None:
    path.write_text(
        f"""# Runexis DIDAPI — Overview

Source of truth (immutable vendor archive):
[`docs/vendor/runexis/DIDAPI Documentation.html`](../../vendor/runexis/DIDAPI%20Documentation.html)

## Base URL

`{BASE_URL}`

## Authentication

Include header:

```http
Authorization: Bearer {{token}}
```

Obtain / refresh tokens via Auth routes (`/api/v1/login`, `/api/v1/refresh`). See [AUTH.md](AUTH.md).

## Response envelope

Success responses typically look like:

```json
{{
  "data": {{}},
  "success": true
}}
```

Error responses (4XX / 500):

```json
{{
  "code": 400,
  "message": "...",
  "request_id": "...",
  "success": false
}}
```

## HTTP status codes

| Code | Meaning |
|---|---|
| 200 | Success |
| 400 | Client error (validation, missing ID, etc.) |
| 401 | Missing / invalid auth token |
| 403 | Account lacks permission |
| 404 | URL not found |
| 405 | Method not allowed for URL |
| 500 | Internal server error |

## Focused references for Finenumbers SMS Service

| Doc | Scope |
|---|---|
| [AUTH.md](AUTH.md) | Platform login to Runexis |
| [SMS.md](SMS.md) | Send / stats / DLR & MO URL registration / directions |
| [NUMBERS.md](NUMBERS.md) | Partner number inventory (no purchase in our product) |
| [SIMS_SMS.md](SIMS_SMS.md) | Informational SIM SMS channel (not primary product send) |
| [WEBHOOKS.md](WEBHOOKS.md) | Lifecycle webhooks (not SMS DLR payloads) |
| [GAPS.md](GAPS.md) | Missing contracts that block implementation |
| [ENDPOINTS.json](ENDPOINTS.json) | Machine-readable catalog (all sections) |

## Regenerating

```bash
python3 scripts/extract_runexis_docs.py
```
""",
        encoding="utf-8",
    )


def write_section_md(
    out_path: Path,
    title: str,
    intro: str,
    endpoints: list[dict[str, Any]],
) -> None:
    parts = [
        f"# {title}",
        "",
        intro.strip(),
        "",
        f"Base URL: `{BASE_URL}`",
        "",
        "## Endpoints",
        "",
    ]
    for ep in endpoints:
        parts.append(render_endpoint_md(ep))
    out_path.write_text("\n".join(parts).rstrip() + "\n", encoding="utf-8")


def main() -> None:
    if not VENDOR_HTML.exists():
        raise SystemExit(f"Vendor HTML not found: {VENDOR_HTML}")

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    raw = VENDOR_HTML.read_text(encoding="utf-8", errors="replace")
    sections = split_h1_sections(raw)

    all_endpoints: list[dict[str, Any]] = []
    by_section: dict[str, list[dict[str, Any]]] = defaultdict(list)

    for section in sections:
        eps = parse_section_endpoints(section)
        for ep in eps:
            all_endpoints.append(ep)
            by_section[section["id"]].append(ep)

    catalog = {
        "base_url": BASE_URL,
        "source": "docs/vendor/runexis/DIDAPI Documentation.html",
        "section_count": len(sections),
        "endpoint_count": len(all_endpoints),
        "sections": [
            {
                "id": s["id"],
                "title": s["title"],
                "endpoint_count": len(by_section.get(s["id"], [])),
            }
            for s in sections
        ],
        "endpoints": all_endpoints,
    }
    (OUT_DIR / "ENDPOINTS.json").write_text(
        json.dumps(catalog, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )

    write_overview(OUT_DIR / "OVERVIEW.md")

    write_section_md(
        OUT_DIR / "AUTH.md",
        "Runexis DIDAPI — Auth",
        "Platform authentication for the agent account used by Finenumbers SMS Service.",
        by_section.get("auth", []),
    )
    write_section_md(
        OUT_DIR / "SMS.md",
        "Runexis DIDAPI — SMS",
        """Primary SMS surface for the product: outbound send, statistics, per-number / global
DLR & incoming (MO) handler URL registration, and SMS direction flags.

**Note:** callback *payload* formats for DLR / incoming SMS are **not** documented in the vendor HTML.
See [GAPS.md](GAPS.md).""",
        by_section.get("sms", []),
    )

    # Numbers: keep inventory-relevant endpoints first; include all for completeness.
    numbers_eps = by_section.get("numbers", [])
    preferred_order = [
        "Список номеров партнера",
        "Поиск номеров",
        "Получение номера",
        "Получение отчета по номерам",
    ]
    numbers_sorted = sorted(
        numbers_eps,
        key=lambda e: (
            preferred_order.index(e["title"]) if e["title"] in preferred_order else 999,
            e["title"],
        ),
    )
    write_section_md(
        OUT_DIR / "NUMBERS.md",
        "Runexis DIDAPI — Numbers (inventory)",
        """Number inventory & status APIs useful for reconciling purchased DEF numbers.

**Out of product scope:** booking, purchase, MNP, and agreement-binding flows exist in DIDAPI
but are not exposed by Finenumbers SMS Service (admin uploads already-purchased numbers).""",
        numbers_sorted,
    )

    sims_eps = [e for e in by_section.get("sims", []) if SIMS_SMS_TITLE_RE.search(e["title"])]
    write_section_md(
        OUT_DIR / "SIMS_SMS.md",
        "Runexis DIDAPI — SIM informational SMS",
        """Separate SIM channel: `POST /api/v1/numbers/{number}/sim/send-sms`.

This is **not** the primary product send path. Product outbound SMS uses `POST /api/v1/sms/send`
(see [SMS.md](SMS.md)).""",
        sims_eps,
    )

    write_section_md(
        OUT_DIR / "WEBHOOKS.md",
        "Runexis DIDAPI — WebHooks (lifecycle)",
        """Lifecycle / provisioning webhooks (`gu_verified`, `mnp`, `number_blocked`, SIM events, etc.).

These are **not** the SMS DLR / incoming-SMS callbacks. SMS callbacks are registered via
`/api/v1/sms/dlr-url`, `/api/v1/sms/hook-url` and per-number equivalents (see [SMS.md](SMS.md)).""",
        by_section.get("webhook", []),
    )

    print(f"Wrote {len(all_endpoints)} endpoints from {len(sections)} sections -> {OUT_DIR}")


if __name__ == "__main__":
    main()
