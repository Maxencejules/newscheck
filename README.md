# NewsCheck

NewsCheck is a powerful command-line intelligence tool for discovering, extracting, summarizing, and analyzing news articles from around the world. It allows targeted searches by country, region, or topic, provides relevance scoring, verifies cross-source consensus, and generates coherent AI-powered summaries.

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
-   **Content Extraction & Translation:**
    -   Extracts clean article text (removing navigation, ads, headers) using `trafilatura` and `BeautifulSoup`.
    -   Optionally translates content to a pivot language (English/French) while preserving metadata.
-   **AI Summarization (Global Resume):**
    -   **Gemini AI:** Uses Google's Gemini API (if configured) to generate a coherent "Global Intelligence Resume" synthesizing all extracted articles.
    -   **Local Fallback:** Falls back to local LSA summarization (`sumy`) if no API key is provided.
-   **Comprehensive Reports (DOCX):**
    -   `reports/articles_....docx`: Full text of extracted articles.
    -   `scores/scores_....docx`: Relevance and consensus scores for all candidates.
    -   `summaries/resume_....docx`: A synthesized executive summary of the topic.

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

## Configuration

### API Keys (Optional)

To enable high-quality AI summarization, set the `GEMINI_API_KEY` environment variable.

**Linux/macOS:**
```bash
export GEMINI_API_KEY="your_api_key_here"
```

**Windows (PowerShell):**
```powershell
$env:GEMINI_API_KEY="your_api_key_here"
```

If the key is not set, NewsCheck will automatically use a local summarization algorithm (Sumy/LSA).

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
6.  **Extract Content:** Choose how many top articles to extract (default 5).
7.  **Get Reports:** The tool automatically generates reports in `reports/`, `scores/`, and `summaries/`.

## Architecture

The project follows a hybrid architecture:
-   **Go (`cmd/newscheck`, `internal/`):** Handles CLI interaction, search planning, RSS discovery, scoring, and report generation.
-   **Python (`python_worker/worker.py`):** A dedicated worker for content extraction and summarization. It uses:
    -   `playwright`: To resolve complex JavaScript redirects (Google News wrappers).
    -   `trafilatura` & `beautifulsoup4`: For high-quality text extraction.
    -   `google-generativeai`: For AI summarization.
    -   `sumy`: For local fallback summarization.

## Troubleshooting

-   **"Worker not configured"**: Ensure `python` is in your PATH or update `internal/extract/extract.go` to point to the correct python executable.
-   **Extraction fails / Google News links not resolving**: Ensure Playwright is installed (`playwright install chromium`). The worker logs `[DEBUG]` info to stderr which can be seen in the console.
-   **Summarization fails**: Check your `GEMINI_API_KEY` if you expect AI summaries. If using local fallback, ensure `nltk` data is downloaded (the tool attempts to download `punkt` automatically).

## License

MIT License
