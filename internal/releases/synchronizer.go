package releases

import (
	"context"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type SourceRepository interface {
	GitHubSource(context.Context, string) (domain.GitHubSource, error)
}

type TokenRepository interface {
	Get(context.Context, string) (string, bool, error)
}

type Synchronizer struct {
	Client   Client
	Sources  SourceRepository
	Packages *content.PackageManager
	Secrets  TokenRepository
}

func (s Synchronizer) SyncLatest(ctx context.Context, sourceID string) (content.PackageVersion, error) {
	source, err := s.Sources.GitHubSource(ctx, sourceID)
	if err != nil {
		return content.PackageVersion{}, err
	}
	token := ""
	if s.Secrets != nil {
		token, _, _ = s.Secrets.Get(ctx, "github_token")
	}
	result, err := s.Client.FetchLatest(ctx, source.Repository, source.AssetPattern, token, s.Packages)
	return result.Package, err
}
