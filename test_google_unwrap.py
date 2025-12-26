#!/usr/bin/env python3
"""Test script to debug Google News unwrapping"""
import requests
from bs4 import BeautifulSoup
from urllib.parse import urlparse, urljoin

def test_unwrap(google_url):
    print(f"Testing: {google_url}\n")

    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
    }

    print("Fetching Google News page...")
    r = requests.get(google_url, headers=headers, timeout=10, allow_redirects=True)
    print(f"Status: {r.status_code}")
    print(f"Final URL: {r.url}")
    print(f"Content length: {len(r.text)} chars\n")

    # Check if we were redirected away from Google News
    final_host = urlparse(r.url).netloc
    if final_host != "news.google.com":
        print(f"✓ SUCCESS: Redirected directly to publisher!")
        print(f"  Publisher: {final_host}")
        print(f"  URL: {r.url}")
        return r.url

    # Still on Google News, need to parse
    print("Still on Google News, parsing for article link...")
    soup = BeautifulSoup(r.text, "html.parser")

    # Save HTML for inspection
    with open("google_news_page.html", "w", encoding="utf-8") as f:
        f.write(r.text)
    print("Saved HTML to: google_news_page.html\n")

    # Strategy 1: Look for all external links
    print("Looking for external links...")
    external_links = []
    for a in soup.find_all("a", href=True):
        href = a["href"]

        if href.startswith("/"):
            href = urljoin("https://news.google.com", href)
        elif href.startswith("./"):
            href = urljoin(r.url, href)

        if href.startswith("http"):
            host = urlparse(href).netloc
            if host and "google" not in host:
                link_text = a.get_text(strip=True)[:50]
                print(f"  Found: {host} - {link_text}")
                external_links.append(href)

    if external_links:
        print(f"\n✓ Found {len(external_links)} external link(s)")
        print(f"  First link: {external_links[0]}")
        return external_links[0]
    else:
        print("\n✗ No external links found")
        return None

if __name__ == "__main__":
    # Test with the URL from your output
    test_url = "https://news.google.com/rss/articles/CBMigwFBVV95cUxQTUFtd1J6NFd2ZVNxMGhXdjNmOFA3Qk83TmZ0X1NVbzlSWVl6cHN1b0xQaUNYVmg4MFBLUG5TcGdXZktLR2d3V3UwY1NnVjRTelpBNE4zQVZuSzRIUU9iR05PenRCNDZ2Vnk0NjhvSElWcm9zOEhVN3BVMWJ2SGRsbWFtTQ?oc=5"

    result = test_unwrap(test_url)

    if result:
        print(f"\n{'='*60}")
        print("SUCCESS! Article URL found:")
        print(result)
    else:
        print(f"\n{'='*60}")
        print("FAILED - Could not extract article URL")
        print("Check google_news_page.html to see what Google returned")