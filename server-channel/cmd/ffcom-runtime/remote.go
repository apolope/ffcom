package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

const (
	maxIndexBytes   = 1 << 20
	maxSigBytes     = 4 << 10
	indexTimeout    = 30 * time.Second
	artifactTimeout = 10 * time.Minute
)

// fetcher baixa o índice assinado e os artefatos.
type fetcher struct {
	client   *http.Client
	indexURL string // base; baixa <base>/index.json e <base>/index.json.sig
	pub      ed25519.PublicKey
}

// fetchIndex baixa index.json e index.json.sig, verifica a assinatura sobre
// os bytes exatos baixados e só então decodifica.
func (f fetcher) fetchIndex(ctx context.Context) (*release.Index, error) {
	ctx, cancel := context.WithTimeout(ctx, indexTimeout)
	defer cancel()
	data, err := f.get(ctx, f.indexURL+"/index.json", maxIndexBytes)
	if err != nil {
		return nil, err
	}
	sig, err := f.get(ctx, f.indexURL+"/index.json.sig", maxSigBytes)
	if err != nil {
		return nil, err
	}
	if err := release.Verify(f.pub, data, string(sig)); err != nil {
		return nil, err
	}
	return release.ParseIndex(data)
}

func (f fetcher) get(ctx context.Context, url string, max int64) ([]byte, error) {
	body, err := f.open(ctx, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, max+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%s: resposta maior que %d bytes", url, max)
	}
	return data, nil
}

func (f fetcher) open(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ffcom-runtime/"+version)
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return resp.Body, nil
}

// downloadArtifact baixa o artefato de v para platform e instala em st.
func (f fetcher) downloadArtifact(ctx context.Context, st store, v release.Version, platform string) error {
	art, ok := v.Artifacts[platform]
	if !ok {
		return fmt.Errorf("versão %s sem artefato para %s", v.Version, platform)
	}
	ctx, cancel := context.WithTimeout(ctx, artifactTimeout)
	defer cancel()
	body, err := f.open(ctx, art.URL)
	if err != nil {
		return err
	}
	defer body.Close()
	if err := st.install(v.Version, body, art.SHA256); err != nil {
		return fmt.Errorf("versão %s: %w", v.Version, err)
	}
	return nil
}
