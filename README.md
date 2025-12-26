# NewsCheck

NewsCheck is a powerful intelligence tool for discovering, extracting, summarizing, and analyzing news articles from around the world. It features a **Modern Desktop GUI** for ease of use and a **CLI** for automation.

---

## 🚀 Quick Start (GUI)

### 1. Prerequisites
Ensure you have the following installed:
-   **Go** (v1.21+) - [Download](https://go.dev/dl/)
-   **Node.js** (v16+) - [Download](https://nodejs.org/)
-   **Python** (v3.10+) - [Download](https://www.python.org/)

### 2. Install Wails (The GUI Framework)
Open your terminal and run:
```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

> **Troubleshooting Windows:** If you see `wails : The term 'wails' is not recognized`, your Go bin directory is missing from your PATH.
>
> **Quick Fix (Current Session):**
> ```powershell
> $env:PATH += ";$env:USERPROFILE\go\bin"
> ```
> **Permanent Fix:** Add `%USERPROFILE%\go\bin` to your User PATH environment variable in Windows System Properties.

### 3. Setup Dependencies
Run these commands in the project root to install the necessary libraries:

**Go & Python:**
```bash
# Download Go modules
go mod tidy

# Install Python requirements
pip install -r requirements.txt

# Install Playwright browsers (for article extraction)
playwright install chromium
playwright install-deps
```

**Frontend (React):**
```bash
cd frontend
npm install
cd ..
```

### 4. Run the App
To start the application in **Development Mode** (with live reloading):
```bash
wails dev
```
The application window should appear shortly.

To **Build a Standalone App** (for your OS):
```bash
wails build
```
The executable will be located in the `build/bin/` directory.

---

## 🖥️ Features

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

---

## ⚙️ Configuration

### API Keys (Optional)
To enable high-quality AI summarization using Google Gemini:
1.  Launch the GUI.
2.  In the search screen, locate the **"Gemini API Key"** input field.
3.  Paste your API key (e.g., `AIzaSy...`).
4.  If left empty, the app will automatically use the local `sumy` summarizer.

---

## ⌨️ CLI Usage
If you prefer the command line:
```bash
go run cmd/newscheck/main.go
```
*Note: For the CLI, set the `GEMINI_API_KEY` environment variable to use AI summarization.*

## Architecture
-   **Backend (Go):** Handles orchestration, search planning, discovery, scoring, and report generation.
-   **Frontend (React + TypeScript):** Provides a modern, responsive user interface.
-   **Worker (Python):** Handles heavy lifting for content extraction (Playwright) and summarization.

## License
MIT License
