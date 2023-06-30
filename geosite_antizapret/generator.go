package geosite_antizapret

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v53/github"
	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/sagernet/sing-box/common/geosite"
	"golang.org/x/text/encoding/charmap"
)

const DefaultDownloadURL = "https://raw.githubusercontent.com/zapret-info/z-i/master/dump.csv"

type Generator struct {
	downloadURL string

	githubClient *github.Client
	githubOwner  string
	githubRepo   string

	httpClient *http.Client
}

type GeneratorOption func(*Generator)

func WithDownloadURL(downloadURL string) GeneratorOption {
	return func(g *Generator) {
		g.downloadURL = downloadURL
	}
}

func WithGitHubClient(pat, owner, repo string) GeneratorOption {
	return func(g *Generator) {
		g.githubClient = github.NewTokenClient(context.Background(), pat)
		g.githubOwner = owner
		g.githubRepo = repo
	}
}

func WithHTTPClient(httpClient *http.Client) GeneratorOption {
	return func(g *Generator) {
		g.httpClient = httpClient
	}
}

func NewGenerator(opts ...GeneratorOption) *Generator {
	g := &Generator{
		downloadURL: DefaultDownloadURL,
		httpClient:  http.DefaultClient,
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

func generateSites(in io.Reader, outSites, outIPs io.Writer) error {
	// skip first line
	bufio.NewScanner(in).Scan()

	// create csv reader with CP1251 decoder
	r := csv.NewReader(charmap.Windows1251.NewDecoder().Reader(in))
	r.Comma = ';'

	mmdb, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "sing-geoip",
		Languages:    []string{"antizapret"},
	})
	if err != nil {
		return fmt.Errorf("cannot create new mmdb: %w", err)
	}

	var domains []geosite.Item

	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("cannot parse csv file: %w", err)
		}

		{
			ips := strings.Split(rec[0], "|")

			for _, ipStr := range ips {
				ip := net.ParseIP(ipStr)
				if ip == nil {
					log.Println("invalid ip")
					continue
				}

				ipNet := &net.IPNet{
					IP:   ip,
					Mask: net.IPv4Mask(255, 255, 255, 255),
				}

				if ip[0] != 0 {
					ipNet.Mask = net.CIDRMask(128, 128)
				}

				if err := mmdb.Insert(ipNet, mmdbtype.String("antizapret")); err != nil {
					return fmt.Errorf("cannot insert into mmdb: %w", err)
				}
			}
		}

		{
			var item geosite.Item
			if strings.HasPrefix(rec[1], "*") {
				item.Type = geosite.RuleTypeDomainSuffix
				item.Value = strings.Replace(rec[1], "*", "", 1)
			} else {
				item.Type = geosite.RuleTypeDomain
				item.Value = rec[1]
			}

			domains = append(domains, item)
		}
	}

	if err := geosite.Write(outSites, map[string][]geosite.Item{
		"antizapret": domains,
	}); err != nil {
		return fmt.Errorf("cannot write into geosite file: %w", err)
	}

	if _, err := mmdb.WriteTo(outIPs); err != nil {
		return fmt.Errorf("cannot write into geoip file: %w", err)
	}

	return nil
}

func (g *Generator) GenerateAndWrite() error {
	resp, err := g.httpClient.Get(g.downloadURL)
	if err != nil {
		return fmt.Errorf("cannot get dump from github: %w", err)
	}
	defer resp.Body.Close()

	outSites, err := os.Create("geosite.db")
	if err != nil {
		return fmt.Errorf("cannot create geosite file: %w", err)
	}
	defer outSites.Close()

	outIPs, err := os.Create("geoip.db")
	if err != nil {
		return fmt.Errorf("cannot create geoip file: %w", err)
	}
	defer outIPs.Close()

	if err := generateSites(resp.Body, outSites, outIPs); err != nil {
		return fmt.Errorf("cannot generate: %w", err)
	}

	return nil
}

func (g *Generator) GenerateAndUpload(ctx context.Context) error {
	if g.githubClient == nil {
		return errors.New("cannot generate and upload without github client")
	}

	resp, err := g.httpClient.Get(g.downloadURL)
	if err != nil {
		return fmt.Errorf("cannot get dump from github: %w", err)
	}
	defer resp.Body.Close()

	geositeFile, err := os.CreateTemp("", "geosite_antizapret")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer geositeFile.Close()

	hasher := sha256.New()

	if err := generateSites(resp.Body, io.MultiWriter(hasher, geositeFile), nil); err != nil {
		return fmt.Errorf("cannot generate: %w", err)
	}

	if _, err := geositeFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	geositeFileHashSumFile, err := os.CreateTemp("", "geosite_antizapret")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer geositeFileHashSumFile.Close()
	if _, err := geositeFileHashSumFile.Write([]byte(hex.EncodeToString(hasher.Sum(nil)) + "  geosite.db\n")); err != nil {
		return err
	}

	if _, err := geositeFileHashSumFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	tagName := time.Now().Format("20060102150405")
	repositoryRelease, _, err := g.githubClient.Repositories.CreateRelease(ctx, g.githubOwner, g.githubRepo, &github.RepositoryRelease{
		TagName: &tagName,
	})
	if err != nil {
		return fmt.Errorf("cannot create github release: %w", err)
	}

	if _, _, err := g.githubClient.Repositories.UploadReleaseAsset(ctx, g.githubOwner, g.githubRepo, *repositoryRelease.ID, &github.UploadOptions{
		Name: "geosite.db",
	}, geositeFile); err != nil {
		return fmt.Errorf("cannot upload release asset: %w", err)
	}

	if _, _, err := g.githubClient.Repositories.UploadReleaseAsset(ctx, g.githubOwner, g.githubRepo, *repositoryRelease.ID, &github.UploadOptions{
		Name: "geosite.db.sha256sum",
	}, geositeFileHashSumFile); err != nil {
		return fmt.Errorf("cannot upload release asset: %w", err)
	}

	return nil
}
