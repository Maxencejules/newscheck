# NewsCheck

NewsCheck is a command-line tool for discovering, extracting, and analyzing news articles from around the world. It allows targeted searches by country, region, or topic, and provides relevance scoring and cross-source consensus verification.

## Features

-   **Flexible Search Scopes:**
    -   **Auto:** Automatically detects countries and regions from your query.
    -   **Chosen Country:** Force-search within a specific country (e.g., "Bulgaria") regardless of the topic keywords.
    -   **Global:** Search worldwide sources.
-   **Intelligent Discovery:**
    -   Leverages Google News RSS with localized parameters (language, region, CEID).
    -   Includes curated RSS feeds (BBC, NYT, Guardian, Al Jazeera).
    -   Robustly handles Google News redirect URLs using a headless browser (Playwright) to find the original publisher content.
-   **Relevance & Consensus Scoring:**
    -   **Relevance Score:** Scores articles based on keyword matches in titles and country context.
    -   **Consensus Score:** Identifies how many distinct sources are covering the same story to verify its significance.
-   **Content Extraction:**
    -   Extracts clean article text (removing navigation, ads, headers) using `trafilatura` and `BeautifulSoup`.
    -   Generates comprehensive reports in DOCX format.
-   **Reports:**
    -   `articles.docx`: Full text of extracted articles.
    -   `scores.docx`: Relevance and consensus scores for all discovered candidates.

## Architecture

The project follows a hybrid architecture:
-   **Go (`cmd/newscheck`, `internal/`):** Handles CLI interaction, search planning, RSS discovery, scoring, and report generation.
-   **Python (`python_worker/worker.py`):** A dedicated worker for content extraction. It uses:
    -   `playwright`: To resolve complex JavaScript redirects (Google News wrappers).
    -   `trafilatura` & `beautifulsoup4`: For high-quality text extraction.

## Prerequisites

1.  **Go:** Version 1.25+ (or compatible).
2.  **Python:** Version 3.10+.
3.  **Python Dependencies:**
    ```bash
    pip install -r requirements.txt
    ```
4.  **Playwright Browsers:**
    After installing Python dependencies, run:
    ```bash
    playwright install chromium
    playwright install-deps
    ```

## Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/newscheck.git
cd newscheck

# Install Go dependencies
go mod download

# Install Python dependencies
pip install -r requirements.txt
playwright install chromium
playwright install-deps
```

## Usage

Run the application:

```bash
go run cmd/newscheck/main.go
```

### Step-by-Step Guide

1.  **Enter Topic:** Type your search query (e.g., "artificial intelligence regulation"). Submit with a blank line.
2.  **Select Time Window:** Choose from Last 24h, 7 days, 30 days, or a custom range.
3.  **Select Search Scope:**
    -   `Auto`: Detects "France" from "protests in France".
    -   `Chosen Country`: You enter "France", and it searches for "artificial intelligence regulation" specifically in French sources (local & English).
    -   `Global`: Searches worldwide.
4.  **Select Pivot Language:** Choose a target language for potential translation (English/French).
5.  **View Results:** The tool displays a list of discovered articles with their **Relevance** and **Consensus** scores.
6.  **Extract Content:** Choose how many top articles to extract.
7.  **Get Reports:** The tool generates `articles.docx` and `scores.docx` in the current directory.

## Configuration

-   `data/country_languages.json`: Database of countries and their languages/ISO codes.
-   `data/country_auto_cache.json`: Cache for automatically resolved country queries.

## Troubleshooting

-   **"Worker not configured"**: Ensure `python` is in your PATH or update `internal/extract/extract.go` to point to the correct python executable.
-   **Extraction fails / Google News links not resolving**: Ensure Playwright is installed (`playwright install chromium`). The worker logs `[DEBUG]` info to stderr which can be seen in the console.
-   **Build errors**: Ensure you have run `go mod tidy` and have the `github.com/gingfrederik/docx` dependency.

## License

[Your License Here]
