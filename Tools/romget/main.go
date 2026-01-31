package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultReferer   = "https://myrient.erista.me/files/No-Intro/"
	defaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0"
	bufferSize       = 1024 * 1024 // 1MB buffer for better throughput on large files
)

type ProgressWriter struct {
	Total      int64
	Downloaded int64
	StartTime  time.Time
	LastPrint  time.Time
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Downloaded += int64(n)

	// Print progress every 2 seconds
	now := time.Now()
	if now.Sub(pw.LastPrint) >= 2*time.Second || pw.Downloaded == pw.Total {
		elapsed := now.Sub(pw.StartTime).Seconds()
		speed := float64(pw.Downloaded) / elapsed / 1024 // KB/s
		progress := float64(pw.Downloaded) / float64(pw.Total) * 100

		fmt.Fprintf(os.Stderr, "\rProgress: %.1f%% (%s/%s) @ %.1f KB/s",
			progress,
			formatBytes(pw.Downloaded),
			formatBytes(pw.Total),
			speed)

		pw.LastPrint = now

		if pw.Downloaded == pw.Total {
			fmt.Fprintf(os.Stderr, "\n")
		}
	}

	return n, nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func createRequest(urlStr, referer, userAgent string) (*http.Request, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}

	// Set myrient-compatible headers
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// Don't set Accept-Encoding - let Go handle it, avoids compression issues
	req.Header.Set("Referer", referer)
	req.Header.Set("Connection", "keep-alive")

	return req, nil
}

func downloadFile(urlStr, outputPath string, retries int, timeout time.Duration, referer, userAgent string, quiet bool) error {
	var lastErr error

	for attempt := 1; attempt <= retries; attempt++ {
		if !quiet && retries > 1 {
			fmt.Fprintf(os.Stderr, "Attempt %d/%d...\n", attempt, retries)
		}

		err := downloadAttempt(urlStr, outputPath, timeout, referer, userAgent, quiet)
		if err == nil {
			return nil
		}

		lastErr = err
		if attempt < retries {
			if !quiet {
				fmt.Fprintf(os.Stderr, "Failed: %v, retrying in 2s...\n", err)
			}
			time.Sleep(2 * time.Second)
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", retries, lastErr)
}

func downloadAttempt(urlStr, outputPath string, timeout time.Duration, referer, userAgent string, quiet bool) error {
	// Create HTTP client optimized for large file downloads
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   10,
		WriteBufferSize:       bufferSize,
		ReadBufferSize:        bufferSize,
		DisableCompression:    true, // Avoid decompression overhead for binary files
	}
	client := &http.Client{
		Transport: transport,
		// No Timeout here - this would limit the entire request including download
	}

	// Create request with browser headers
	req, err := createRequest(urlStr, referer, userAgent)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	// Check status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d: %s", resp.StatusCode, resp.Status)
	}

	// Get content length
	totalSize := resp.ContentLength
	if !quiet {
		fmt.Fprintf(os.Stderr, "Size: %s\n", formatBytes(totalSize))
	}

	// Create temp file with buffered writer for better disk I/O
	tempPath := outputPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	
	// Use buffered writer to reduce disk I/O overhead
	bufferedFile := bufio.NewWriterSize(file, bufferSize)

	// Download with progress using large buffer for better throughput
	var writer io.Writer = bufferedFile
	if !quiet && totalSize > 0 {
		pw := &ProgressWriter{
			Total:     totalSize,
			StartTime: time.Now(),
			LastPrint: time.Now(),
		}
		writer = io.MultiWriter(bufferedFile, pw)
	}

	// Use large buffer for copying - significantly improves download speed
	buf := make([]byte, bufferSize)
	_, err = io.CopyBuffer(writer, resp.Body, buf)
	if err != nil {
		file.Close()
		os.Remove(tempPath)
		return fmt.Errorf("download: %w", err)
	}

	// Flush buffered writer and close file before rename
	if err := bufferedFile.Flush(); err != nil {
		file.Close()
		os.Remove(tempPath)
		return fmt.Errorf("flush: %w", err)
	}
	file.Close()

	// Move temp to final location
	err = os.Rename(tempPath, outputPath)
	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

func inferReferer(urlStr string) string {
	// Parse URL to extract parent directory for referer
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return defaultReferer
	}

	// Extract parent directory path
	dir := filepath.Dir(parsedURL.Path)
	if dir == "." || dir == "/" {
		return defaultReferer
	}

	// Build referer from parent directory
	return fmt.Sprintf("%s://%s%s/", parsedURL.Scheme, parsedURL.Host, dir)
}

func main() {
	// Command-line flags
	urlFlag := flag.String("url", "", "URL to download (required)")
	outputFlag := flag.String("o", "", "Output file path (default: filename from URL)")
	retriesFlag := flag.Int("r", 3, "Number of retry attempts")
	timeoutFlag := flag.Int("t", 60, "Timeout in seconds")
	refererFlag := flag.String("referer", "", "Referer header (default: auto-detect from URL)")
	userAgentFlag := flag.String("ua", defaultUserAgent, "User-Agent header")
	quietFlag := flag.Bool("q", false, "Quiet mode (no progress)")
	flag.Parse()

	// Validate required flags
	if *urlFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: -url is required")
		fmt.Fprintln(os.Stderr, "\nUsage: romget -url <URL> [-o output] [-r retries] [-t timeout] [-referer <referer>] [-ua <user-agent>] [-q]")
		fmt.Fprintln(os.Stderr, "\nExamples:")
		fmt.Fprintln(os.Stderr, `  romget -url "https://myrient.erista.me/files/.../game.zip"`)
		fmt.Fprintln(os.Stderr, `  romget -url "https://example.com/rom.zip" -o /path/to/save.zip`)
		fmt.Fprintln(os.Stderr, `  romget -url "https://example.com/rom.zip" -r 5 -t 120`)
		os.Exit(1)
	}

	// Determine output path
	outputPath := *outputFlag
	if outputPath == "" {
		// Extract filename from URL
		parsedURL, err := url.Parse(*urlFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid URL: %v\n", err)
			os.Exit(1)
		}
		outputPath = filepath.Base(parsedURL.Path)
		if outputPath == "" || outputPath == "." || outputPath == "/" {
			fmt.Fprintln(os.Stderr, "Error: cannot determine filename from URL, use -o to specify output")
			os.Exit(1)
		}
	}

	// Determine referer
	referer := *refererFlag
	if referer == "" {
		referer = inferReferer(*urlFlag)
	}

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil {
		fmt.Fprintf(os.Stderr, "File already exists: %s\n", outputPath)
		os.Exit(0)
	}

	// Download file
	timeout := time.Duration(*timeoutFlag) * time.Second
	if !*quietFlag {
		fmt.Fprintf(os.Stderr, "Downloading: %s\n", filepath.Base(outputPath))
	}

	err := downloadFile(*urlFlag, outputPath, *retriesFlag, timeout, referer, *userAgentFlag, *quietFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !*quietFlag {
		fmt.Fprintf(os.Stderr, "Saved to: %s\n", outputPath)
	}
}
