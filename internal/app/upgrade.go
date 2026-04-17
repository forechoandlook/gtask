package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"github.com/forechoandlook/gtask/internal/version"
	"github.com/minio/selfupdate"
)

const (
	versionURL = "https://github.com/forechoandlook/gtask/releases/latest/download/VERSION"
	binaryURL  = "https://github.com/forechoandlook/gtask/releases/latest/download/gtask_%s_%s"
)

func runSelfUpgrade(ctx context.Context, stdout io.Writer) error {
	fmt.Fprintf(stdout, "Current version: %s\n", version.Version)
	if version.Version == "dev" {
		fmt.Fprintln(stdout, "Development version, skipping upgrade.")
		return nil
	}

	fmt.Fprintln(stdout, "Checking for updates...")
	latestVersion, err := fetchLatestVersion(ctx)
	if err != nil {
		return fmt.Errorf("check version: %w", err)
	}

	if latestVersion == version.Version {
		fmt.Fprintln(stdout, "You are already on the latest version.")
		return nil
	}

	fmt.Fprintf(stdout, "New version available: %s. Upgrading...\n", latestVersion)

	downloadURL := fmt.Sprintf(binaryURL, runtime.GOOS, runtime.GOARCH)
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download binary failed: status %s", resp.Status)
	}

	err = selfupdate.Apply(resp.Body, selfupdate.Options{})
	if err != nil {
		return fmt.Errorf("apply update: %w", err)
	}

	fmt.Fprintln(stdout, "Successfully upgraded to", latestVersion)
	return nil
}

func fetchLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}
