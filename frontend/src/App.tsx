import { useState } from 'react';
import './App.css';

// Define types manually since we can't auto-generate in this env
interface SearchParams {
    query: string;
    days: number;
    scope: number;
    chosenCountry: string;
    pivotLang: string;
}

interface Candidate {
    url: string;
    title: string;
    source: string;
    published_at: string; // ISO string
    relevance_score: number;
    consensus_score: number;
}

interface SearchResult {
    Candidates: Candidate[];
    // ... other fields if needed
}

interface ExtractParams {
    urls: string[];
    pivotLang: string;
    query: string;
    apiKey: string;
}

interface ExtractResult {
    articles: any[]; // simplify for now
    summary: string;
}

// Access Wails runtime
const wails = (window as any).go.main.App;

function App() {
    // Search State
    const [query, setQuery] = useState("");
    const [days, setDays] = useState(7);
    const [scope, setScope] = useState(0);
    const [chosenCountry, setChosenCountry] = useState("");
    const [pivotLang, setPivotLang] = useState("en");
    const [apiKey, setApiKey] = useState("");

    // Data State
    const [candidates, setCandidates] = useState<Candidate[]>([]);
    const [selectedUrls, setSelectedUrls] = useState<Set<string>>(new Set());
    const [extractResult, setExtractResult] = useState<ExtractResult | null>(null);

    // UI State
    const [view, setView] = useState<"search" | "results" | "extracted">("search");
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");

    const handleSearch = async () => {
        if (!query) return;
        setLoading(true);
        setError("");
        try {
            const params: SearchParams = {
                query,
                days: Number(days),
                scope: Number(scope),
                chosenCountry,
                pivotLang
            };
            const res = await wails.Search(params);
            if (res && res.Candidates) {
                setCandidates(res.Candidates);
                setView("results");
            } else {
                setCandidates([]);
                setView("results"); // Show empty state
            }
        } catch (e: any) {
            setError("Search failed: " + e);
        } finally {
            setLoading(false);
        }
    };

    const toggleSelect = (url: string) => {
        const next = new Set(selectedUrls);
        if (next.has(url)) next.delete(url);
        else next.add(url);
        setSelectedUrls(next);
    };

    const handleSelectAll = () => {
        const all = new Set(candidates.map(c => c.url));
        setSelectedUrls(all);
    };

    const handleClearSelection = () => {
        setSelectedUrls(new Set());
    };

    const handleExtract = async () => {
        if (selectedUrls.size === 0) return;
        setLoading(true);
        setError("");
        try {
            const params: ExtractParams = {
                urls: Array.from(selectedUrls),
                pivotLang,
                query,
                apiKey
            };
            const res = await wails.ExtractAndSummarize(params);
            setExtractResult(res);
            setView("extracted");
        } catch (e: any) {
            setError("Extraction failed: " + e);
        } finally {
            setLoading(false);
        }
    };

    const saveArticles = async () => {
        if (!extractResult) return;
        try {
            await wails.SaveArticleReport(extractResult.articles);
        } catch (e: any) {
            alert("Error saving: " + e);
        }
    };

    const saveResume = async () => {
        if (!extractResult) return;
        try {
            await wails.SaveResumeReport(extractResult.summary, query, extractResult.articles);
        } catch (e: any) {
            alert("Error saving: " + e);
        }
    };

    const goHome = () => {
        setView("search");
        setCandidates([]);
        setSelectedUrls(new Set());
        setExtractResult(null);
        setError("");
    };

    return (
        <div className="container">
            <header>
                <h1 onClick={goHome} style={{cursor: 'pointer'}}>NewsCheck</h1>
                {loading && <div className="loader">Processing...</div>}
                {error && <div className="error">{error}</div>}
            </header>

            {view === "search" && (
                <div className="card">
                    <h2>Global Intelligence Search</h2>
                    <div className="form-group">
                        <label>Query / Topic</label>
                        <textarea
                            value={query}
                            onChange={e => setQuery(e.target.value)}
                            placeholder="e.g. Artificial Intelligence Regulation in EU"
                            rows={3}
                        />
                    </div>

                    <div className="row">
                        <div className="form-group">
                            <label>Time Range</label>
                            <select value={days} onChange={e => setDays(Number(e.target.value))}>
                                <option value={1}>Last 24 Hours</option>
                                <option value={7}>Last 7 Days</option>
                                <option value={30}>Last 30 Days</option>
                            </select>
                        </div>
                        <div className="form-group">
                            <label>Pivot Language</label>
                            <select value={pivotLang} onChange={e => setPivotLang(e.target.value)}>
                                <option value="en">English</option>
                                <option value="fr">French</option>
                            </select>
                        </div>
                    </div>

                    <div className="form-group">
                        <label>Scope</label>
                        <select value={scope} onChange={e => setScope(Number(e.target.value))}>
                            <option value={0}>Auto-Detect</option>
                            <option value={1}>Specific Country</option>
                            <option value={2}>Global</option>
                        </select>
                    </div>

                    {scope === 1 && (
                        <div className="form-group">
                            <label>Country Name</label>
                            <input
                                type="text"
                                value={chosenCountry}
                                onChange={e => setChosenCountry(e.target.value)}
                                placeholder="e.g. Brazil"
                            />
                        </div>
                    )}

                    <div className="form-group">
                        <label>Gemini API Key (Optional)</label>
                        <input
                            type="password"
                            value={apiKey}
                            onChange={e => setApiKey(e.target.value)}
                            placeholder="AIzaSy..."
                        />
                        <small style={{color: '#777'}}>Leave empty to use local summarization (Sumy)</small>
                    </div>

                    <button className="btn primary" onClick={handleSearch} disabled={loading || !query}>
                        Start Discovery
                    </button>
                </div>
            )}

            {view === "results" && (
                <div className="results-view">
                    <div className="toolbar">
                        <button onClick={goHome}>&larr; Back</button>
                        <span>Found {candidates.length} candidates</span>
                        <div className="actions">
                            <button className="btn" onClick={() => wails.SaveScoresReport(candidates)}>Save Scores DOCX</button>
                            <button
                                className="btn primary"
                                onClick={handleExtract}
                                disabled={selectedUrls.size === 0 || loading}
                            >
                                Extract ({selectedUrls.size})
                            </button>
                        </div>
                    </div>

                    <div className="list">
                        {candidates.map((c, i) => (
                            <div key={i} className={`item ${selectedUrls.has(c.url) ? 'selected' : ''}`} onClick={() => toggleSelect(c.url)}>
                                <div className="checkbox">
                                    <input
                                        type="checkbox"
                                        checked={selectedUrls.has(c.url)}
                                        readOnly
                                    />
                                </div>
                                <div className="content">
                                    <h3>{c.title}</h3>
                                    <div className="meta">
                                        <span>{c.source}</span>
                                        <span>{new Date(c.published_at).toLocaleDateString()}</span>
                                        <span className="badge">Rel: {c.relevance_score}</span>
                                        {c.consensus_score > 1 && <span className="badge consensus">Consensus: {c.consensus_score}</span>}
                                    </div>
                                    <div className="url">{c.url}</div>
                                </div>
                            </div>
                        ))}
                    </div>
                </div>
            )}

            {view === "extracted" && extractResult && (
                <div className="extracted-view">
                    <div className="toolbar">
                        <button onClick={() => setView("results")}>&larr; Back to Results</button>
                        <div className="actions">
                            <button className="btn" onClick={saveArticles}>Save Articles DOCX</button>
                            <button className="btn primary" onClick={saveResume}>Save Resume DOCX</button>
                        </div>
                    </div>

                    <div className="split-view">
                        <div className="summary-panel">
                            <h2>Global Intelligence Resume</h2>
                            <div className="summary-content">
                                {extractResult.summary.split('\n').map((line, i) => (
                                    <p key={i}>{line}</p>
                                ))}
                            </div>
                        </div>
                        <div className="articles-panel">
                            <h2>Extracted Articles ({extractResult.articles.length})</h2>
                            {extractResult.articles.map((art, i) => (
                                <div key={i} className="article-card">
                                    <h3>{art.title}</h3>
                                    <div className="meta">{art.site} | {art.lang}</div>
                                    <p className="preview">
                                        {art.text.slice(0, 200)}...
                                    </p>
                                </div>
                            ))}
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

export default App;
