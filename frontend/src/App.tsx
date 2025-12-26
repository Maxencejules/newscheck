import { useState } from 'react';
import './fonts.css';
import './App.css';

// Define types manually since we can't auto-generate in this env
interface SearchParams {
    query: string;
    days: number;
    customFrom?: string;
    customTo?: string;
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

// Icons (Simple SVGs)
const Icons = {
    Search: () => <svg className="icon" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" /></svg>,
    Back: () => <svg className="icon" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" /></svg>,
    Download: () => <svg className="icon" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" /></svg>,
    Globe: () => <svg className="icon" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>,
    Clock: () => <svg className="icon" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>,
    Filter: () => <svg className="icon" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" /></svg>,
    News: () => <svg className="icon" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 20H5a2 2 0 01-2-2V6a2 2 0 012-2h10a2 2 0 012 2v1m2 13a2 2 0 01-2-2V7m2 13a2 2 0 002-2V9a2 2 0 00-2-2h-2m-4-3H9M7 16h6M7 8h6v4H7V8z" /></svg>,
};

function App() {
    // Search State
    const [query, setQuery] = useState("");
    const [days, setDays] = useState(7);
    const [customFrom, setCustomFrom] = useState("");
    const [customTo, setCustomTo] = useState("");
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
                customFrom,
                customTo,
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
                <h1 onClick={goHome} style={{cursor: 'pointer'}}>
                    <Icons.Globe /> NewsCheck
                </h1>
                {loading && <div className="loader">Processing...</div>}
                {error && <div className="error">{error}</div>}
            </header>

            {error && <div className="error">{error}</div>}

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
                            autoFocus
                        />
                    </div>

                    <div className="row">
                        <div className="form-group">
                            <label><Icons.Clock /> Time Range</label>
                            <select value={days} onChange={e => setDays(Number(e.target.value))}>
                                <option value={1}>Last 24 Hours</option>
                                <option value={7}>Last 7 Days</option>
                                <option value={30}>Last 30 Days</option>
                                <option value={-1}>Custom Range</option>
                            </select>
                            {days === -1 && (
                                <div style={{display:'flex', gap:'10px', marginTop:'10px'}}>
                                    <input type="date" value={customFrom} onChange={e => setCustomFrom(e.target.value)} placeholder="From" />
                                    <input type="date" value={customTo} onChange={e => setCustomTo(e.target.value)} placeholder="To" />
                                </div>
                            )}
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
                        <label><Icons.Filter /> Scope</label>
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
                        <small>Leave empty to use local summarization (Sumy)</small>
                    </div>

                    <button className="btn primary full-width" onClick={handleSearch} disabled={loading || !query}>
                        <Icons.Search /> Start Discovery
                    </button>
                </div>
            )}

            {view === "results" && (
                <div className="results-view">
                    <div className="toolbar">
                        <div style={{display:'flex', gap:'1rem', alignItems:'center'}}>
                            <button className="btn" onClick={goHome}><Icons.Back /> Back</button>
                            <span>Found {candidates.length} candidates</span>
                            <div style={{display:'flex', gap:'0.5rem', marginLeft:'1rem', borderLeft:'1px solid #eee', paddingLeft:'1rem'}}>
                                <button className="btn small" onClick={handleSelectAll}>Select All</button>
                                <button className="btn small" onClick={handleClearSelection}>Clear</button>
                            </div>
                        </div>
                        <div className="actions" style={{display:'flex', gap:'0.5rem'}}>
                            <button className="btn" onClick={() => wails.SaveScoresReport(candidates)}>
                                <Icons.Download /> Save Scores
                            </button>
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
                                        <span><Icons.News /> {c.source}</span>
                                        <span>{new Date(c.published_at).toLocaleDateString()}</span>
                                        <span className="badge rel">Rel: {c.relevance_score}</span>
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
                        <button className="btn" onClick={() => setView("results")}><Icons.Back /> Back to Results</button>
                        <div className="actions" style={{display:'flex', gap:'0.5rem'}}>
                            <button className="btn" onClick={saveArticles}><Icons.Download /> Save Articles</button>
                            <button className="btn primary" onClick={saveResume}><Icons.Download /> Save Resume</button>
                        </div>
                    </div>

                    <div className="split-view">
                        <div className="summary-panel">
                            <div className="panel-header">
                                <h2>Global Intelligence Resume</h2>
                            </div>
                            <div className="summary-content">
                                {extractResult.summary.split('\n').map((line, i) => (
                                    <p key={i}>{line}</p>
                                ))}
                            </div>
                        </div>
                        <div className="articles-panel">
                            <div className="panel-header">
                                <h2>Extracted Articles ({extractResult.articles.length})</h2>
                            </div>
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
