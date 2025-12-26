# NewsCheck

NewsCheck is a powerful intelligence tool for discovering, extracting, summarizing, and analyzing news articles from around the world. It provides both a modern Desktop UI and a robust CLI.

## Features

-   **Flexible Search Scopes:**
    -   **Auto:** Automatically detects countries and regions from your query.
    -   **Chosen Country:** Force-search within a specific country (e.g., "Bulgaria").
    -   **Global:** Search worldwide sources.
-   **Intelligent Discovery:**
    -   Leverages Google News RSS with localized parameters.
    -   Includes curated RSS feeds (BBC, NYT, Guardian, Al Jazeera).
    -   Robustly handles Google News redirect URLs using Playwright.
-   **Relevance & Consensus Scoring:**
    -   **Relevance Score:** Scores articles based on keyword matches and country context.
    -   **Consensus Score:** Verifies story significance via cross-source overlap.
-   **Content Extraction & Translation:**
    -   Extracts clean article text using `trafilatura`.
    -   Optionally translates content to a pivot language (English/French).
-   **AI Summarization (Global Resume):**
    -   **Gemini AI:** Uses Google's Gemini API to generate a coherent executive summary.
    -   **Local Fallback:** Falls back to local LSA summarization (`sumy`) if no API key is provided.
-   **Comprehensive Reports (DOCX):**
    -   Save detailed reports of extracted articles and executive summaries.

## Prerequisites

1.  **Go:** Version 1.21+
2.  **Node.js:** Version 16+ (for GUI)
3.  **Python:** Version 3.10+
4.  **Wails:** `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## Python Setup

1.  Install dependencies:
    ```bash
    pip install -r requirements.txt
    ```
2.  Install Playwright browsers:
    ```bash
    playwright install chromium
    playwright install-deps
    ```

## Running the Application

### Desktop GUI (Recommended)

1.  Navigate to the project root.
2.  Install frontend dependencies:
    ```bash
    cd frontend && npm install && cd ..
    ```
3.  Run in development mode:
    ```bash
    wails dev
    ```
4.  Or build for production:
    ```bash
    wails build
    ```

### CLI Mode

You can still use the command-line interface:

```bash
go run cmd/newscheck/main.go
```

## Configuration

### API Keys

To enable high-quality AI summarization:
-   **GUI:** Enter your Gemini API key in the search screen settings.
-   **CLI:** Set the `GEMINI_API_KEY` environment variable.

## Architecture

-   **Backend (Go):** Handles orchestration, search planning, discovery, scoring, and report generation.
-   **Frontend (React + TypeScript):** Provides a modern, responsive user interface.
-   **Worker (Python):** Handles heavy lifting for content extraction (Playwright) and summarization.

## License

MIT License
