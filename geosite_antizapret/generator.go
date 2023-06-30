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
	"net/netip"
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

func (g *Generator) generate(in io.Reader, outSites, outIPs io.Writer) error {
	antizapretConfigs, err := g.fetchAntizapretConfigs()
	if err != nil {
		return fmt.Errorf("cannot fetch antizapret configs: %w", err)
	}

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
				if ipStr == "" {
					continue
				}

				var ipNet *net.IPNet
				if strings.Contains(ipStr, "/") {
					_, ipNet, err = net.ParseCIDR(ipStr)
					if err != nil {
						log.Println(err)
						continue
					}
				} else {
					addr, err := netip.ParseAddr(ipStr)
					if err != nil {
						log.Println(err)
						continue
					}

					exclude := false
					for _, ip := range antizapretConfigs.ExcludeIPs {
						if addr.Compare(ip) == 0 {
							exclude = true
							break
						}
					}
					if exclude {
						break
					}

					ipNet = &net.IPNet{
						IP: addr.AsSlice(),
					}
					if addr.Is4() {
						ipNet.Mask = net.CIDRMask(32, 32)
					} else if addr.Is6() {
						ipNet.Mask = net.CIDRMask(128, 128)
					}
				}

				if err := mmdb.Insert(ipNet, mmdbtype.String("antizapret")); err != nil {
					return fmt.Errorf("cannot insert into mmdb: %w", err)
				}
			}
		}

		{
			if rec[1] == "" {
				continue
			}

			/*exclude := false
			for _, host := range antizapretConfigs.ExcludeHosts {
				if rec[1] == host || rec[1] == "*."+host {
					exclude = true
					break
				}
			}
			for _, rx := range antizapretConfigs.ExcludeRegexp {
				if rx.MatchString(rec[1]) {
					exclude = true
					break
				}
			}
			if exclude {
				break
			}*/

			if strings.HasPrefix(rec[1], "*") {
				domains = append(domains, geosite.Item{
					Type:  geosite.RuleTypeDomainSuffix,
					Value: strings.Replace(rec[1], "*", "", 1),
				})
				domains = append(domains, geosite.Item{
					Type:  geosite.RuleTypeDomain,
					Value: strings.Replace(rec[1], "*.", "", 1),
				})
			} else {
				domains = append(domains, geosite.Item{
					Type:  geosite.RuleTypeDomain,
					Value: rec[1],
				})
			}
		}
	}

	for _, host := range antizapretConfigs.IncludeHosts {
		domains = append(domains, geosite.Item{
			Type:  geosite.RuleTypeDomainSuffix,
			Value: "." + host,
		})
		domains = append(domains, geosite.Item{
			Type:  geosite.RuleTypeDomain,
			Value: host,
		})
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

	if err := g.generate(resp.Body, outSites, outIPs); err != nil {
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

	geoipFile, err := os.CreateTemp("", "geosite_antizapret")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer geoipFile.Close()

	geositeHasher := sha256.New()
	geoipHasher := sha256.New()

	if err := g.generate(
		resp.Body,
		io.MultiWriter(geositeHasher, geositeFile),
		io.MultiWriter(geoipHasher, geoipFile),
	); err != nil {
		return fmt.Errorf("cannot generate: %w", err)
	}

	if _, err := geositeFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}
	if _, err := geoipFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	geositeFileHashSumFile, err := os.CreateTemp("", "geosite_antizapret")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer geositeFileHashSumFile.Close()
	if _, err := geositeFileHashSumFile.Write([]byte(hex.EncodeToString(geositeHasher.Sum(nil)) + "  geosite.db\n")); err != nil {
		return err
	}

	if _, err := geositeFileHashSumFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	geoipFileHashSumFile, err := os.CreateTemp("", "geosite_antizapret")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer geoipFileHashSumFile.Close()
	if _, err := geoipFileHashSumFile.Write([]byte(hex.EncodeToString(geoipHasher.Sum(nil)) + "  geosite.db\n")); err != nil {
		return err
	}

	if _, err := geoipFileHashSumFile.Seek(0, io.SeekStart); err != nil {
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

	if _, _, err := g.githubClient.Repositories.UploadReleaseAsset(ctx, g.githubOwner, g.githubRepo, *repositoryRelease.ID, &github.UploadOptions{
		Name: "geoip.db",
	}, geoipFile); err != nil {
		return fmt.Errorf("cannot upload release asset: %w", err)
	}

	if _, _, err := g.githubClient.Repositories.UploadReleaseAsset(ctx, g.githubOwner, g.githubRepo, *repositoryRelease.ID, &github.UploadOptions{
		Name: "geoip.db.sha256sum",
	}, geoipFileHashSumFile); err != nil {
		return fmt.Errorf("cannot upload release asset: %w", err)
	}

	return nil
}
