package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type Article struct {
	URL         string  `json:"url"`
	FinalURL    string  `json:"final_url"`
	Site        string  `json:"site"`
	Title       string  `json:"title"`
	Author      *string `json:"author"`
	PublishedAt *string `json:"published_at"`
	Lang        *string `json:"lang"`
	Text        string  `json:"text"`
	FetchedAt   string  `json:"fetched_at"`
}

type workerResponse struct {
	OK        bool    `json:"ok"`
	ElapsedMS int     `json:"elapsed_ms"`
	Data      Article `json:"data"`
	Error     string  `json:"error"`
}

type Worker struct {
	PythonExe string // "python"
	Script    string // "python_worker/worker.py"
}

func NewWorker() *Worker {
	return &Worker{
		PythonExe: "python",
		Script:    "python_worker/worker.py",
	}
}

func (w *Worker) Summarize(ctx context.Context, text string) (string, error) {
	if w.PythonExe == "" || w.Script == "" {
		return "", errors.New("worker not configured")
	}
	if text == "" {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	args := []string{w.Script, "--mode", "summarize"}
	cmd := exec.CommandContext(ctx, w.PythonExe, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = bytes.NewBufferString(text)

	// Pass Gemini Key explicitly if needed, though os.Environ() covers it if set in current process
	cmd.Env = append(os.Environ(), "GEMINI_API_KEY="+os.Getenv("GEMINI_API_KEY"))

	err := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("summarize timeout: %w", ctx.Err())
	}
	if err != nil {
		return "", fmt.Errorf("summarize failed: %v (stderr=%s)", err, stderr.String())
	}

	type summaryResp struct {
		OK        bool   `json:"ok"`
		Summary   string `json:"summary"`
		Error     string `json:"error"`
	}

	var resp summaryResp
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("bad summary json: %v (out=%s)", err, stdout.String())
	}
	if !resp.OK {
		return "", fmt.Errorf("summary worker error: %s", resp.Error)
	}

	return resp.Summary, nil
}

func (w *Worker) Extract(ctx context.Context, url string, targetLang string) (Article, error) {
	if w.PythonExe == "" || w.Script == "" {
		return Article{}, errors.New("worker not configured")
	}

	// Increase timeout for translation
	timeout := 25 * time.Second
	if targetLang != "" {
		timeout = 45 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{w.Script, "--url", url}
	if targetLang != "" {
		args = append(args, "--target-lang", targetLang)
	}

	cmd := exec.CommandContext(ctx, w.PythonExe, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		return Article{}, fmt.Errorf("python worker timeout: %w", ctx.Err())
	}
	if err != nil {
		return Article{}, fmt.Errorf("python worker failed: %v (stderr=%s)", err, stderr.String())
	}

	var resp workerResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return Article{}, fmt.Errorf("bad worker json: %v (out=%s)", err, stdout.String())
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "unknown error"
		}
		return Article{}, fmt.Errorf("worker error: %s", resp.Error)
	}

	return resp.Data, nil
}
