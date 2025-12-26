#!/usr/bin/env python3
import argparse
import json
import re
import sys
import time
import traceback
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Optional
from urllib.parse import urlparse, urljoin, unquote, parse_qs

import requests
from bs4 import BeautifulSoup
import trafilatura
from deep_translator import GoogleTranslator

try:
    from playwright.sync_api import sync_playwright
except ImportError:
    sync_playwright = None


@dataclass
class Extracted:
    url: str
    resolved_url: str
    final_url: str
    site: str
    title: str
    author: Optional[str]
    published_at: Optional[str]
    lang: Optional[str]
    text: str
    fetched_at: str


def iso_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def normalize_space(s: str) -> str:
    s = re.sub(r"[ \t]+", " ", s)
    s = re.sub(r"\n{3,}", "\n\n", s)
    return s.strip()


def pick_meta(soup: BeautifulSoup, *keys: str) -> Optional[str]:
    for k in keys:
        tag = soup.find("meta", attrs={"name": k})
        if tag and tag.get("content"):
            return tag["content"].strip()
        tag = soup.find("meta", attrs={"property": k})
        if tag and tag.get("content"):
            return tag["content"].strip()
    return None


def detect_lang(soup: BeautifulSoup) -> Optional[str]:
    html = soup.find("html")
    if not html:
        return None
    lang = html.get("lang")
    if not lang:
        return None
    return lang.split("-")[0].lower().strip() or None


def extract_main_text(soup: BeautifulSoup, html_text: str) -> str:
    # Try trafilatura first as it's specialized for article extraction
    try:
        text = trafilatura.extract(html_text, include_comments=False, include_tables=False)
        if text:
            return normalize_space(text)
    except Exception:
        pass

    # Fallback to BeautifulSoup logic
    for tag in soup(["script", "style", "noscript", "header", "footer", "nav", "aside"]):
        tag.decompose()

    article = soup.find("article")
    if article:
        return normalize_space(article.get_text("\n", strip=True))

    candidates = []
    for sel in ["main", "div", "section"]:
        for node in soup.find_all(sel):
            t = normalize_space(node.get_text("\n", strip=True))
            if len(t) >= 800:
                candidates.append((len(t), t))

    if candidates:
        candidates.sort(key=lambda x: x[0], reverse=True)
        return candidates[0][1]

    body = soup.find("body")
    if body:
        return normalize_space(body.get_text("\n", strip=True))

    return ""


def fetch_html(url: str, timeout_s: int, max_bytes: int) -> tuple[str, str, str]:
    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 newscheck/0.1",
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
        "Accept-Language": "en-US,en;q=0.9",
        "Accept-Encoding": "gzip, deflate, br",
        "Cache-Control": "no-cache",
        "Pragma": "no-cache",
    }

    with requests.get(url, headers=headers, timeout=timeout_s, allow_redirects=True, stream=True) as r:
        r.raise_for_status()

        ctype = (r.headers.get("Content-Type") or "").lower()
        if ctype and ("text/html" not in ctype and "application/xhtml+xml" not in ctype and "application/xml" not in ctype):
            raise RuntimeError(f"unsupported content-type: {ctype}")

        chunks = []
        total = 0
        for chunk in r.iter_content(chunk_size=64 * 1024):
            if not chunk:
                continue
            total += len(chunk)
            if total > max_bytes:
                raise RuntimeError("response too large")
            chunks.append(chunk)

        data = b"".join(chunks)

        enc = r.encoding or "utf-8"
        try:
            html_text = data.decode(enc, errors="replace")
        except Exception:
            html_text = data.decode("utf-8", errors="replace")

        return html_text, r.url, ctype


def is_google_news_wrapper(u: str) -> bool:
    """Check if URL is a Google News wrapper that needs unwrapping"""
    try:
        p = urlparse(u)
        host = (p.netloc or "").lower()

        # Check for news.google.com domain
        if host not in ("news.google.com", "www.google.com", "google.com"):
            return False

        # Google News article wrappers
        if "/articles/" in p.path or "/rss/articles/" in p.path:
            return True

        # Google redirect URLs
        if host in ("www.google.com", "google.com") and p.path == "/url":
            return True

        return False
    except Exception:
        return False


def try_google_news_redirect(url: str, timeout_s: int) -> Optional[str]:
    """
    Google News URLs often redirect to the actual article if you:
    1. Use a browser-like User-Agent
    2. Don't include certain query parameters
    3. Follow redirects

    Returns the final URL after redirects, or None if still on Google News
    """
    try:
        # Try without query params first
        parsed = urlparse(url)
        clean_url = f"{parsed.scheme}://{parsed.netloc}{parsed.path}"

        headers = {
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.9",
            "Referer": "https://news.google.com/",
        }

        # Follow redirects with a session
        with requests.Session() as session:
            session.max_redirects = 5
            response = session.get(clean_url, headers=headers, timeout=timeout_s, allow_redirects=True)

            # Check if we ended up somewhere other than Google News
            final_host = urlparse(response.url).netloc.lower()
            if "google" not in final_host:
                return response.url

    except Exception:
        pass

    return None


def unwrap_google_redirect(href: str) -> str:
    """Handle https://www.google.com/url?...&url=<real>..."""
    try:
        p = urlparse(href)
        host = (p.netloc or "").lower()
        if host not in ("www.google.com", "google.com"):
            return href
        if p.path != "/url":
            return href
        qs = parse_qs(p.query)
        if "url" in qs and qs["url"]:
            return qs["url"][0]
        if "q" in qs and qs["q"]:
            return qs["q"][0]
        return href
    except Exception:
        return href


def decode_google_news_url(source_url: str) -> str:
    """
    Decodes Google News URLs using a headless browser (Playwright).
    The URL structure is complex and often requires executing JS to get the final redirect.
    """
    try:
        url = urlparse(source_url)
        # Check if it looks like a Google News wrapper
        if not (url.hostname in ("news.google.com", "www.google.com", "google.com") and
                ("/articles/" in url.path or "/rss/articles/" in url.path)):
            return source_url

        if sync_playwright:
            with sync_playwright() as p:
                browser = p.chromium.launch(headless=True, args=["--no-sandbox"])
                context = browser.new_context(
                    user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
                )
                page = context.new_page()

                # Navigate to the URL
                try:
                    # We expect a redirect. Wait for navigation to a non-google URL or timeout.
                    # However, sometimes it redirects to another google URL.
                    # Let's just navigate and wait for the final URL.
                    response = page.goto(source_url, timeout=30000, wait_until="domcontentloaded")

                    # Wait a bit for JS redirects if any
                    page.wait_for_timeout(2000)

                    final_url = page.url
                    final_host = urlparse(final_url).netloc.lower()

                    if "google" not in final_host:
                        return final_url

                    # If still on google, maybe we need to click something?
                    # Usually the JS redirect happens automatically.
                    # Let's try to find the link in the DOM if we are still on google

                    # Try to find link in canonical
                    canon = page.locator("link[rel='canonical']").get_attribute("href")
                    if canon and "google" not in urlparse(canon).netloc:
                        return canon

                    # Try to find "Read full article" link
                    # Note: The text might vary by locale
                    links = page.locator("a")
                    count = links.count()
                    for i in range(count):
                        href = links.nth(i).get_attribute("href")
                        if href:
                            href = unwrap_google_redirect(href)
                            if href.startswith("http") and "google" not in urlparse(href).netloc:
                                return href

                except Exception:
                    pass
                finally:
                    browser.close()
        else:
            # Fallback or error if playwright not available
            print("[WARN] Playwright not installed, skipping advanced decoding", file=sys.stderr)

        return source_url
    except Exception:
        return source_url


def extract_publisher_url_from_google_news(html_text: str, base_url: str) -> Optional[str]:
    """Extract the real article URL from Google News wrapper page"""
    soup = BeautifulSoup(html_text, "html.parser")

    # Strategy 1: Look for the "read full article" button/link (c-wiz component)
    # Google News uses specific data attributes for the article link
    for a in soup.find_all("a", href=True):
        href = a.get("href", "").strip()
        if not href:
            continue

        # Check if this link has article-like text
        link_text = a.get_text(strip=True).lower()
        if any(phrase in link_text for phrase in ["read full article", "view full coverage", "go to article"]):
            href = unwrap_google_redirect(href)
            if href.startswith("http"):
                host = urlparse(href).netloc
                if host and host != "news.google.com":
                    return href

    # Strategy 2: Look for canonical link
    canon = soup.find("link", attrs={"rel": "canonical"})
    if canon and canon.get("href"):
        href = canon["href"].strip()
        if href.startswith("http") and "news.google.com" not in (urlparse(href).netloc or ""):
            return href

    # Strategy 3: Check all external links, prioritizing those with article indicators
    external_links = []
    for a in soup.find_all("a", href=True):
        href = a["href"].strip()
        if not href:
            continue

        if href.startswith("./"):
            href = urljoin(base_url, href)
        elif href.startswith("/"):
            href = urljoin("https://news.google.com", href)

        href = unwrap_google_redirect(href)

        if href.startswith("http"):
            host = urlparse(href).netloc
            if host and host != "news.google.com":
                # Prioritize links that look like articles (have a path)
                parsed = urlparse(href)
                if parsed.path and len(parsed.path) > 1:
                    return href
                external_links.append(href)

    # If we found any external links, return the first one
    if external_links:
        return external_links[0]

    return None


def clean_lang(lang: Optional[str]) -> Optional[str]:
    if not isinstance(lang, str):
        return None
    lang = lang.strip()
    if "_" in lang:
        lang = lang.split("_")[0]
    if "-" in lang:
        lang = lang.split("-")[0]
    lang = lang.lower().strip()
    return lang or None


def safe_json_output(payload: dict) -> None:
    """Safely output JSON even with encoding issues"""
    try:
        # Try UTF-8 first
        output = json.dumps(payload, ensure_ascii=False)
        print(output, flush=True)
    except Exception:
        # Fallback to ASCII if UTF-8 fails
        try:
            output = json.dumps(payload, ensure_ascii=True)
            print(output, flush=True)
        except Exception:
            # Last resort: basic error
            print('{"ok": false, "error": "JSON encoding failed"}', flush=True)


def main() -> int:
    # Fix Windows console encoding issues
    try:
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8")
    except Exception:
        pass

    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--timeout", type=int, default=20)
    ap.add_argument("--max-bytes", type=int, default=3_000_000)
    ap.add_argument("--debug", action="store_true", help="Print debug info to stderr")
    ap.add_argument("--target-lang", help="Target language code to translate to (e.g. 'en', 'fr')")
    args = ap.parse_args()

    started = time.time()
    original_url = args.url.strip()
    resolved_url = original_url

    try:
        if args.debug:
            print(f"[DEBUG] Fetching: {original_url}", file=sys.stderr, flush=True)

        # Special handling for Google News - try decode first
        if is_google_news_wrapper(original_url):
            if args.debug:
                print(f"[DEBUG] Detected Google News wrapper, trying decode...", file=sys.stderr, flush=True)

            decoded = decode_google_news_url(original_url)
            if decoded != original_url:
                if args.debug:
                    print(f"[DEBUG] Decode successful: {decoded}", file=sys.stderr, flush=True)
                original_url = decoded
                resolved_url = decoded
            else:
                # Fallback to redirect logic if decode fails or returns same URL
                if args.debug:
                    print(f"[DEBUG] Decode failed/same, trying redirect...", file=sys.stderr, flush=True)

                redirected = try_google_news_redirect(original_url, args.timeout)
                if redirected:
                    if args.debug:
                        print(f"[DEBUG] Redirect successful: {redirected}", file=sys.stderr, flush=True)
                    original_url = redirected
                    resolved_url = redirected
                else:
                    if args.debug:
                        print(f"[DEBUG] Redirect failed, will try HTML parsing", file=sys.stderr, flush=True)

        html_text, final_url, _ctype = fetch_html(original_url, args.timeout, args.max_bytes)
        resolved_url = final_url

        if args.debug:
            print(f"[DEBUG] Resolved to: {final_url}", file=sys.stderr, flush=True)

        # If still on GN wrapper after fetch, try to extract publisher URL from page
        max_unwrap_attempts = 2
        unwrap_count = 0

        while (is_google_news_wrapper(final_url) or is_google_news_wrapper(original_url)) and unwrap_count < max_unwrap_attempts:
            if args.debug:
                print(f"[DEBUG] Still on Google News, parsing HTML (attempt {unwrap_count + 1})", file=sys.stderr, flush=True)

            pub = extract_publisher_url_from_google_news(html_text, final_url)
            if pub:
                if args.debug:
                    print(f"[DEBUG] Found publisher URL in HTML: {pub}", file=sys.stderr, flush=True)
                html_text, final_url, _ctype = fetch_html(pub, args.timeout, args.max_bytes)
                resolved_url = final_url
                unwrap_count += 1
            else:
                if args.debug:
                    print(f"[DEBUG] Could not extract publisher URL from Google News HTML", file=sys.stderr, flush=True)
                # Give up - return empty result rather than Google News page
                raise RuntimeError("Could not unwrap Google News article - no publisher link found")
                break

        soup = BeautifulSoup(html_text, "html.parser")

        site = urlparse(final_url).netloc
        title = pick_meta(soup, "og:title", "twitter:title") or (soup.title.get_text(strip=True) if soup.title else "")
        author = pick_meta(soup, "author", "article:author")
        published = pick_meta(soup, "article:published_time", "og:updated_time", "date", "pubdate")

        lang = clean_lang(detect_lang(soup) or pick_meta(soup, "og:locale"))
        text = extract_main_text(soup, html_text)

        # Translation logic
        if args.target_lang and args.target_lang != lang:
            if args.debug:
                print(f"[DEBUG] Translating content to {args.target_lang}...", file=sys.stderr, flush=True)

            translator = GoogleTranslator(source='auto', target=args.target_lang)

            # Translate Title
            if title:
                try:
                    title = translator.translate(title)
                except Exception as e:
                    if args.debug:
                        print(f"[DEBUG] Title translation failed: {e}", file=sys.stderr, flush=True)

            # Translate Text (chunked to avoid limits)
            if text:
                try:
                    # Split into chunks ~4500 chars (safe limit)
                    chunks = [text[i:i+4500] for i in range(0, len(text), 4500)]
                    translated_chunks = []
                    for chunk in chunks:
                        translated_chunks.append(translator.translate(chunk))
                    text = " ".join(translated_chunks)
                except Exception as e:
                    if args.debug:
                        print(f"[DEBUG] Text translation failed: {e}", file=sys.stderr, flush=True)

            # NOTE: We specifically DO NOT translate 'site' or 'author' as requested.

        out = Extracted(
            url=original_url,
            resolved_url=resolved_url,
            final_url=final_url,
            site=site,
            title=title or "",
            author=author,
            published_at=published,
            lang=lang,
            text=text,
            fetched_at=iso_now(),
        )

        elapsed = int((time.time() - started) * 1000)
        payload = {"ok": True, "elapsed_ms": elapsed, "data": out.__dict__}
        safe_json_output(payload)
        return 0

    except requests.exceptions.HTTPError as e:
        elapsed = int((time.time() - started) * 1000)
        error_msg = f"HTTP {e.response.status_code}: {str(e)}"
        payload = {"ok": False, "elapsed_ms": elapsed, "error": error_msg, "data": None}
        safe_json_output(payload)
        return 2

    except requests.exceptions.Timeout:
        elapsed = int((time.time() - started) * 1000)
        payload = {"ok": False, "elapsed_ms": elapsed, "error": "Request timeout", "data": None}
        safe_json_output(payload)
        return 2

    except Exception as e:
        elapsed = int((time.time() - started) * 1000)
        error_msg = f"{type(e).__name__}: {str(e)}"
        if args.debug:
            print(f"[DEBUG] Exception: {traceback.format_exc()}", file=sys.stderr, flush=True)
        payload = {"ok": False, "elapsed_ms": elapsed, "error": error_msg, "data": None}
        safe_json_output(payload)
        return 2


if __name__ == "__main__":
    sys.exit(main())