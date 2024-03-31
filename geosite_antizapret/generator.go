package geosite_antizapret

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
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
	"github.com/sagernet/sing-box/common/srs"
	"github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
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

func (g *Generator) generate(in io.Reader, outSites, outIPs, outRuleSetJSON, outRuleSetBinary io.Writer) error {
	antizapretConfigs, err := g.fetchAntizapretConfigs()
	if err != nil {
		return fmt.Errorf("cannot fetch antizapret configs: %w", err)
	}

	// create csv reader with CP1251 decoder
	r := csv.NewReader(charmap.Windows1251.NewDecoder().Reader(in))
	r.Comma = ';'
	r.FieldsPerRecord = -1

	mmdb, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "sing-geoip",
		Languages:    []string{"antizapret"},
	})
	if err != nil {
		return fmt.Errorf("cannot create new mmdb: %w", err)
	}

	var domains []geosite.Item

	ruleSet := new(option.PlainRuleSetCompat)
	ruleSet.Version = 1
	ruleSet.Options.Rules = make([]option.HeadlessRule, 1)
	ruleSet.Options.Rules[0].Type = constant.RuleTypeDefault
	ruleSet.Options.Rules[0].DefaultOptions.IPCIDR = make([]string, 0)
	ruleSet.Options.Rules[0].DefaultOptions.Domain = make([]string, 0)
	ruleSet.Options.Rules[0].DefaultOptions.DomainSuffix = make([]string, 0)

	ipNetsSet := make(map[string]struct{})
	domainSuffixesSet := make(map[string]struct{})
	domainsSet := make(map[string]struct{})

	first := true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("cannot parse csv file: %w", err)
		}

		if len(rec) < 2 {
			if first {
				first = false
				continue
			}
			return errors.New("something wrong with csv")
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

				ipNetsSet[ipNet.String()] = struct{}{}
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

				// Rule Set
				domainSuffixesSet[strings.Replace(rec[1], "*", "", 1)] = struct{}{}
				domainsSet[strings.Replace(rec[1], "*.", "", 1)] = struct{}{}
			} else {
				domains = append(domains, geosite.Item{
					Type:  geosite.RuleTypeDomain,
					Value: rec[1],
				})

				// Rule Set
				domainsSet[rec[1]] = struct{}{}
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

		// Rule Set
		domainSuffixesSet["."+host] = struct{}{}
		domainsSet[host] = struct{}{}
	}

	if err := geosite.Write(outSites, map[string][]geosite.Item{
		"antizapret": domains,
	}); err != nil {
		return fmt.Errorf("cannot write into geosite file: %w", err)
	}

	if _, err := mmdb.WriteTo(outIPs); err != nil {
		return fmt.Errorf("cannot write into geoip file: %w", err)
	}

	// Rule Set
	for ipNet := range ipNetsSet {
		ruleSet.Options.Rules[0].DefaultOptions.IPCIDR = append(ruleSet.Options.Rules[0].DefaultOptions.IPCIDR, ipNet)
	}
	for domain := range domainsSet {
		ruleSet.Options.Rules[0].DefaultOptions.Domain = append(ruleSet.Options.Rules[0].DefaultOptions.Domain, domain)
	}
	for suffix := range domainSuffixesSet {
		ruleSet.Options.Rules[0].DefaultOptions.DomainSuffix = append(ruleSet.Options.Rules[0].DefaultOptions.DomainSuffix, suffix)
	}

	enc := json.NewEncoder(outRuleSetJSON)
	enc.SetIndent("", "  ")
	if err := enc.Encode(ruleSet); err != nil {
		return fmt.Errorf("cannot write into antizapret-ruleset.json file: %w", err)
	}

	plainRuleSet := ruleSet.Upgrade()
	if err := srs.Write(outRuleSetBinary, plainRuleSet); err != nil {
		return fmt.Errorf("cannot write into antizapret.srs file: %w", err)
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

	outRuleSetJSON, err := os.Create("antizapret-ruleset.json")
	if err != nil {
		return fmt.Errorf("cannot create antizapret-ruleset.json file: %w", err)
	}
	defer outRuleSetJSON.Close()

	outRuleSetBinary, err := os.Create("antizapret.srs")
	if err != nil {
		return fmt.Errorf("cannot create ruleset file: %w", err)
	}
	defer outRuleSetBinary.Close()

	if err := g.generate(resp.Body, outSites, outIPs, outRuleSetJSON, outRuleSetBinary); err != nil {
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

	ruleSetJSONFile, err := os.CreateTemp("", "geosite_antizapret")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer geoipFile.Close()

	ruleSetBinaryFile, err := os.CreateTemp("", "geosite_antizapret")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer geoipFile.Close()

	geositeHasher := sha256.New()
	geoipHasher := sha256.New()
	ruleSetJSONHasher := sha256.New()
	ruleSetBinaryHasher := sha256.New()

	if err := g.generate(
		resp.Body,
		io.MultiWriter(geositeHasher, geositeFile),
		io.MultiWriter(geoipHasher, geoipFile),
		io.MultiWriter(ruleSetJSONHasher, ruleSetJSONFile),
		io.MultiWriter(ruleSetBinaryHasher, ruleSetBinaryFile),
	); err != nil {
		return fmt.Errorf("cannot generate: %w", err)
	}

	if _, err := geositeFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}
	if _, err := geoipFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}
	if _, err := ruleSetJSONFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}
	if _, err := ruleSetBinaryFile.Seek(0, io.SeekStart); err != nil {
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

	ruleSetJSONFileHashSumFile, err := os.CreateTemp("", "geosite_antizapret")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer ruleSetJSONFileHashSumFile.Close()
	if _, err := ruleSetJSONFileHashSumFile.Write([]byte(hex.EncodeToString(geoipHasher.Sum(nil)) + "  antizapret-ruleset.json\n")); err != nil {
		return err
	}
	if _, err := ruleSetJSONFileHashSumFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	ruleSetBinaryFileHashSumFile, err := os.CreateTemp("", "geosite_antizapret")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer ruleSetBinaryFileHashSumFile.Close()
	if _, err := ruleSetBinaryFileHashSumFile.Write([]byte(hex.EncodeToString(geoipHasher.Sum(nil)) + "  ruleset.json\n")); err != nil {
		return err
	}
	if _, err := ruleSetBinaryFileHashSumFile.Seek(0, io.SeekStart); err != nil {
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

	if _, _, err := g.githubClient.Repositories.UploadReleaseAsset(ctx, g.githubOwner, g.githubRepo, *repositoryRelease.ID, &github.UploadOptions{
		Name: "antizapret-ruleset.json",
	}, ruleSetJSONFile); err != nil {
		return fmt.Errorf("cannot upload release asset: %w", err)
	}
	if _, _, err := g.githubClient.Repositories.UploadReleaseAsset(ctx, g.githubOwner, g.githubRepo, *repositoryRelease.ID, &github.UploadOptions{
		Name: "antizapret-ruleset.json.sha256sum",
	}, ruleSetJSONFileHashSumFile); err != nil {
		return fmt.Errorf("cannot upload release asset: %w", err)
	}

	if _, _, err := g.githubClient.Repositories.UploadReleaseAsset(ctx, g.githubOwner, g.githubRepo, *repositoryRelease.ID, &github.UploadOptions{
		Name: "antizapret.srs",
	}, ruleSetBinaryFile); err != nil {
		return fmt.Errorf("cannot upload release asset: %w", err)
	}
	if _, _, err := g.githubClient.Repositories.UploadReleaseAsset(ctx, g.githubOwner, g.githubRepo, *repositoryRelease.ID, &github.UploadOptions{
		Name: "antizapret.srs.sha256sum",
	}, ruleSetBinaryFileHashSumFile); err != nil {
		return fmt.Errorf("cannot upload release asset: %w", err)
	}

	return nil
}
