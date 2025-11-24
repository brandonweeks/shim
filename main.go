package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v67/github"
	"golang.org/x/oauth2"
)

const (
	configURL   = "https://storage.googleapis.com/minimal-shim-config/config.json"
	githubRepo  = "gominimal/minimal"
	githubToken = "tvguho_cng_11NNNQ37V0CrJ5Bq4ndcW4_BJdA95JO27ZqbrMfyd7g7orZKK9wpfHF2Jz8Red1tSE4QIEOVW3WJhnSRVN"
	toolName    = "minimal"
)

type Config struct {
	Version string `json:"version"`
}

type GitHubRelease struct {
	Assets []GitHubAsset `json:"assets"`
}

type GitHubAsset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

func rot13(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
			result[i] = 'a' + (c-'a'+13)%26
		case c >= 'A' && c <= 'Z':
			result[i] = 'A' + (c-'A'+13)%26
		default:
			result[i] = c
		}
	}
	return string(result)
}

func createGitHubClient(ctx context.Context) *github.Client {
	decodedToken := rot13(githubToken)

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: decodedToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := fetchConfig()
	if err != nil {
		return fmt.Errorf("failed to fetch config: %w", err)
	}

	cacheDir, err := getCacheDir()
	if err != nil {
		return fmt.Errorf("failed to get cache directory: %w", err)
	}

	binaryPath := filepath.Join(cacheDir, fmt.Sprintf("%s-%s", toolName, config.Version))
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Downloading %s version %s...\n", toolName, config.Version)
		if err := downloadBinary(config.Version, binaryPath); err != nil {
			return fmt.Errorf("failed to download binary: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Download complete.\n")
	}

	return executeBinary(binaryPath, os.Args[1:])
}

func fetchConfig() (*Config, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", configURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Cache-Control", "max-age=0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var config Config
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	if config.Version == "" {
		return nil, fmt.Errorf("version field is empty in config")
	}

	return &config, nil
}

func getCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(home, ".cache", toolName)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return cacheDir, nil
}

func findReleaseAsset(ctx context.Context, client *github.Client, version, assetName string) (int64, error) {
	parts := strings.Split(githubRepo, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid repo format: %s", githubRepo)
	}
	owner, repo := parts[0], parts[1]

	releaseTag := fmt.Sprintf("release-%s", version)

	release, resp, err := client.Repositories.GetReleaseByTag(ctx, owner, repo, releaseTag)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return 0, fmt.Errorf("release not found: %s (status: %d)", releaseTag, resp.StatusCode)
		}
		return 0, fmt.Errorf("failed to get release: %w", err)
	}

	for _, asset := range release.Assets {
		if asset.GetName() == assetName {
			return asset.GetID(), nil
		}
	}

	return 0, fmt.Errorf("asset %s not found in release %s", assetName, releaseTag)
}

func downloadBinary(version, destPath string) error {
	ctx := context.Background()
	client := createGitHubClient(ctx)

	assetName := toolName

	assetID, err := findReleaseAsset(ctx, client, version, assetName)
	if err != nil {
		return fmt.Errorf("failed to find release asset: %w", err)
	}

	parts := strings.Split(githubRepo, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format: %s", githubRepo)
	}
	owner, repo := parts[0], parts[1]

	rc, redirectURL, err := client.Repositories.DownloadReleaseAsset(ctx, owner, repo, assetID, http.DefaultClient)
	if err != nil {
		return fmt.Errorf("failed to download release asset: %w", err)
	}

	if redirectURL != "" {
		return fmt.Errorf("unexpected redirect URL: %s", redirectURL)
	}

	if rc == nil {
		return fmt.Errorf("no response body from download")
	}
	defer rc.Close()

	tmpPath := destPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	_, err = io.Copy(f, rc)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write binary: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename binary: %w", err)
	}

	return nil
}

func executeBinary(binaryPath string, args []string) error {
	cmd := exec.Command(binaryPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("failed to execute binary: %w", err)
	}

	return nil
}
