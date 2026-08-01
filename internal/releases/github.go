package releases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	HTTP       *http.Client
	MaxBytes   int64
	Downloader Downloader
}
type FetchResult struct {
	Package content.PackageVersion
	Updated bool
}
type release struct {
	Name        string `json:"name"`
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name   string `json:"name"`
		APIURL string `json:"url"`
		URL    string `json:"browser_download_url"`
		Size   int64  `json:"size"`
	} `json:"assets"`
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func (c Client) FetchLatest(ctx context.Context, repository, assetPattern, token string, packages *content.PackageManager) (FetchResult, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(repository) {
		return FetchResult{}, errors.New("invalid GitHub repository")
	}
	pattern, err := regexp.Compile(assetPattern)
	if err != nil {
		return FetchResult{}, err
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	client := c.httpClient()
	jobs.Logf(ctx, "github", joblogs.Info, "checking latest release repository=%s asset_pattern=%q", repository, assetPattern)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+repository+"/releases/latest", nil)
	if err != nil {
		return FetchResult{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return FetchResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return FetchResult{}, fmt.Errorf("GitHub release API returned %s", response.Status)
	}
	var found release
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&found); err != nil {
		return FetchResult{}, err
	}
	var assetName, assetURL, assetAPIURL string
	var assetSize int64
	for _, asset := range found.Assets {
		if pattern.MatchString(asset.Name) {
			assetName, assetURL, assetAPIURL, assetSize = asset.Name, asset.URL, asset.APIURL, asset.Size
			break
		}
	}
	if assetURL == "" {
		return FetchResult{}, errors.New("matching release asset not found")
	}
	jobs.Logf(ctx, "github", joblogs.Info, "selected release repository=%s release=%q tag=%s published=%s asset=%s advertised_size=%d (%s)", repository, found.Name, found.TagName, found.PublishedAt, assetName, assetSize, jobs.FormatBytes(assetSize))
	if item, ok, err := packages.FindSourceVersion(repository, found.TagName, assetName); err != nil {
		return FetchResult{}, err
	} else if ok {
		jobs.Logf(ctx, "github", joblogs.Info, "package reused repository=%s tag=%s asset=%s package_id=%s size=%s", repository, found.TagName, assetName, item.ID, jobs.FormatBytes(item.Size))
		return FetchResult{Package: item}, nil
	}
	resolvedURL, err := c.resolveAssetURL(ctx, client, assetURL, assetAPIURL, token, base)
	if err != nil {
		return FetchResult{}, err
	}
	parsed, err := url.Parse(resolvedURL)
	if err != nil || !c.allowedAssetHost(parsed, base) {
		return FetchResult{}, errors.New("untrusted release asset URL")
	}
	limit := c.MaxBytes
	if limit <= 0 {
		limit = 2 << 30
	}
	if assetSize > limit {
		return FetchResult{}, errors.New("release asset exceeds size limit")
	}
	if c.Downloader == nil {
		return FetchResult{}, errors.New("release downloader unavailable")
	}
	temporary, err := packages.CreateDownloadTemp()
	if err != nil {
		return FetchResult{}, err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		return FetchResult{}, err
	}
	defer os.Remove(temporaryPath)
	written, err := c.Downloader.Download(ctx, DownloadRequest{URL: resolvedURL, Destination: temporaryPath, Filename: assetName, Total: assetSize, MaxBytes: limit})
	if err != nil {
		return FetchResult{}, err
	}
	if written > limit {
		return FetchResult{}, errors.New("release asset exceeds size limit")
	}
	temporary, err = os.Open(temporaryPath)
	if err != nil {
		return FetchResult{}, err
	}
	jobs.Logf(ctx, "github", joblogs.Info, "download completed source=github file=%s bytes=%d (%s)", assetName, written, jobs.FormatBytes(written))
	item, err := packages.AddUpload(assetName, found.TagName, temporary, written)
	temporary.Close()
	if err != nil {
		return FetchResult{}, err
	}
	item.SourceRepository = repository
	if err := packages.UpdateMetadata(item); err != nil {
		return FetchResult{}, err
	}
	jobs.Logf(ctx, "github", joblogs.Info, "package downloaded repository=%s tag=%s asset=%s package_id=%s size=%s", repository, found.TagName, assetName, item.ID, jobs.FormatBytes(written))
	return FetchResult{Package: item, Updated: true}, nil
}

func (c Client) resolveAssetURL(ctx context.Context, client *http.Client, browserURL, apiURL, token, base string) (string, error) {
	if token == "" || apiURL == "" {
		return browserURL, nil
	}
	parsed, err := url.Parse(apiURL)
	if err != nil || !c.allowedAssetHost(parsed, base) {
		return "", errors.New("untrusted release asset API URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("Authorization", "Bearer "+token)
	redirectClient := *client
	redirectClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := redirectClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 300 || response.StatusCode > 399 {
		return "", fmt.Errorf("GitHub asset API returned %s without a download redirect", response.Status)
	}
	location, err := response.Location()
	if err != nil || !c.allowedAssetHost(location, base) {
		return "", errors.New("untrusted release asset redirect")
	}
	return location.String(), nil
}

func (c Client) allowedAssetHost(asset *url.URL, base string) bool {
	baseURL, _ := url.Parse(base)
	if asset.Host == baseURL.Host {
		return asset.Scheme == baseURL.Scheme
	}
	if asset.Scheme != "https" {
		return false
	}
	allowed := map[string]bool{"github.com": true, "objects.githubusercontent.com": true, "github-releases.githubusercontent.com": true, "release-assets.githubusercontent.com": true}
	return allowed[asset.Host]
}
