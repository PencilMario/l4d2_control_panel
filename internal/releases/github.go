package releases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type Client struct {
	BaseURL  string
	HTTP     *http.Client
	MaxBytes int64
}
type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (c Client) FetchLatest(ctx context.Context, repository, assetPattern, token string, packages *content.PackageManager) (content.PackageVersion, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repository) {
		return content.PackageVersion{}, errors.New("invalid GitHub repository")
	}
	pattern, err := regexp.Compile(assetPattern)
	if err != nil {
		return content.PackageVersion{}, err
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+repository+"/releases/latest", nil)
	if err != nil {
		return content.PackageVersion{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return content.PackageVersion{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return content.PackageVersion{}, fmt.Errorf("GitHub release API returned %s", response.Status)
	}
	var found release
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&found); err != nil {
		return content.PackageVersion{}, err
	}
	var assetName, assetURL string
	for _, asset := range found.Assets {
		if pattern.MatchString(asset.Name) {
			assetName, assetURL = asset.Name, asset.URL
			break
		}
	}
	if assetURL == "" {
		return content.PackageVersion{}, errors.New("matching release asset not found")
	}
	parsed, err := url.Parse(assetURL)
	if err != nil || !c.allowedAssetHost(parsed, base) {
		return content.PackageVersion{}, errors.New("untrusted release asset URL")
	}
	download, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return content.PackageVersion{}, err
	}
	if token != "" {
		download.Header.Set("Authorization", "Bearer "+token)
	}
	assetResponse, err := client.Do(download)
	if err != nil {
		return content.PackageVersion{}, err
	}
	defer assetResponse.Body.Close()
	if assetResponse.StatusCode != 200 {
		return content.PackageVersion{}, fmt.Errorf("release download returned %s", assetResponse.Status)
	}
	limit := c.MaxBytes
	if limit <= 0 {
		limit = 2 << 30
	}
	temporary, err := os.CreateTemp("", "l4d2-release-*.zip")
	if err != nil {
		return content.PackageVersion{}, err
	}
	defer os.Remove(temporary.Name())
	written, err := io.Copy(temporary, io.LimitReader(assetResponse.Body, limit+1))
	if err != nil {
		temporary.Close()
		return content.PackageVersion{}, err
	}
	if written > limit {
		temporary.Close()
		return content.PackageVersion{}, errors.New("release asset exceeds size limit")
	}
	if _, err := temporary.Seek(0, 0); err != nil {
		temporary.Close()
		return content.PackageVersion{}, err
	}
	item, err := packages.AddUpload(assetName, found.TagName, temporary, written)
	temporary.Close()
	return item, err
}
func (c Client) allowedAssetHost(asset *url.URL, base string) bool {
	baseURL, _ := url.Parse(base)
	if asset.Scheme != "https" && asset.Host != baseURL.Host {
		return false
	}
	allowed := map[string]bool{"github.com": true, "objects.githubusercontent.com": true, "github-releases.githubusercontent.com": true, baseURL.Host: true}
	return allowed[asset.Host]
}
