package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
)

const repo = "cubetiqlabs/wkr"

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func cmdUpdate() {
	fmt.Println("Checking for updates...")

	latest, err := getLatestRelease()
	if err != nil {
		fatal("failed to check for updates: " + err.Error())
	}

	latestVer := strings.TrimPrefix(latest.TagName, "wkr-cli-v")
	if latestVer == version {
		fmt.Printf("✓ Already up to date (v%s)\n", version)
		return
	}

	fmt.Printf("  Current: v%s\n  Latest:  v%s\n\n", version, latestVer)

	// Find matching asset
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	assetName := fmt.Sprintf("wkr-%s-%s", goos, goarch)
	if goos == "windows" {
		assetName += ".exe"
	}

	var downloadURL string
	for _, a := range latest.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		fatal(fmt.Sprintf("no binary found for %s/%s", goos, goarch))
	}

	fmt.Printf("Downloading %s...\n", assetName)

	resp, err := http.Get(downloadURL)
	if err != nil {
		fatal("download failed: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fatal(fmt.Sprintf("download failed: %s", resp.Status))
	}

	// Write to temp file then replace self
	exe, err := os.Executable()
	if err != nil {
		fatal("cannot locate current binary: " + err.Error())
	}

	tmp, err := os.CreateTemp("", "wkr-update-*")
	if err != nil {
		fatal("cannot create temp file: " + err.Error())
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		fatal("download failed: " + err.Error())
	}
	tmp.Close()

	if err := os.Chmod(tmp.Name(), 0755); err != nil {
		fatal("chmod failed: " + err.Error())
	}

	if err := os.Rename(tmp.Name(), exe); err != nil {
		// Rename may fail cross-device; fall back to copy
		if err := copyFile(tmp.Name(), exe); err != nil {
			fatal("update failed: " + err.Error() + "\n  Try: curl -fsSL https://raw.githubusercontent.com/" + repo + "/main/scripts/install.sh | sh")
		}
	}

	fmt.Printf("✓ Updated to v%s\n", latestVer)
}

func getLatestRelease() (*ghRelease, error) {
	resp, err := http.Get("https://api.github.com/repos/" + repo + "/releases?per_page=20")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("no releases found")
	}

	for _, r := range releases {
		if strings.HasPrefix(r.TagName, "wkr-cli-v") {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("no wkr-cli release found")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
