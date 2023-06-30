package geosite_antizapret

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/netip"
	"regexp"
	"strings"
)

const (
	AntizapretPACGeneratorLightUpstreamBaseURL = "https://bitbucket.org/anticensority/antizapret-pac-generator-light/raw/master/config/"

	ExcludeHostsByIPsDist = "exclude-hosts-by-ips-dist.txt"
	ExcludeHostsDist      = "exclude-hosts-dist.txt"
	ExcludeIPsDist        = "exclude-ips-dist.txt"
	ExcludeRegexpDist     = "exclude-regexp-dist.awk"

	IncludeHostsDist = "include-hosts-dist.txt"
	IncludeIPsDist   = "include-ips-dist.txt"
)

type AntizapretConfigType string

const (
	IPs        AntizapretConfigType = "ips"
	Hosts      AntizapretConfigType = "hosts"
	HostsByIPs AntizapretConfigType = "hosts_by_ips"
	Regexp     AntizapretConfigType = "regexp"
)

type AntizapretConfig struct {
	Type    AntizapretConfigType
	Exclude bool
	URL     string
}

type Configs struct {
	ExcludeHosts  []string
	ExcludeIPs    []netip.Addr
	ExcludeRegexp []*regexp.Regexp

	IncludeHosts []string
	IncludeIPs   []*net.IPNet
}

func (g *Generator) fetchAntizapretConfigs() (*Configs, error) {
	cfgs := []AntizapretConfig{
		{HostsByIPs, true, AntizapretPACGeneratorLightUpstreamBaseURL + ExcludeHostsByIPsDist},
		{Hosts, true, AntizapretPACGeneratorLightUpstreamBaseURL + ExcludeHostsDist},
		{Regexp, true, AntizapretPACGeneratorLightUpstreamBaseURL + ExcludeRegexpDist},
		{Hosts, false, AntizapretPACGeneratorLightUpstreamBaseURL + IncludeHostsDist},
	}

	configs := &Configs{}

	for _, cfg := range cfgs {
		resp, err := g.httpClient.Get(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("cannot get antizapret exclude config: %w", err)
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			switch cfg.Type {
			case Hosts:
				if cfg.Exclude {
					configs.ExcludeHosts = append(configs.ExcludeHosts, scanner.Text())
				} else {
					configs.IncludeHosts = append(configs.IncludeHosts, scanner.Text())
				}
			case HostsByIPs:
				if cfg.Exclude {
					ipStr := scanner.Text()
					ipStr = strings.ReplaceAll(ipStr, "\\", "")
					ipStr = strings.Replace(ipStr, "^", "", 1)
					ipStr = strings.Replace(ipStr, ";", "", 1)
					addr, err := netip.ParseAddr(ipStr)
					if err != nil {
						log.Println(err)
						continue
					}

					configs.ExcludeIPs = append(configs.ExcludeIPs, addr)
				}
			case Regexp:
				if cfg.Exclude {
					rxStr := scanner.Text()
					rxStr = strings.Replace(rxStr, "/) {next}", "", 1)
					rxStr = strings.Replace(rxStr, "(/", "", 1)

					rx, err := regexp.Compile(rxStr)
					if err != nil {
						log.Println(err)
						continue
					}

					configs.ExcludeRegexp = append(configs.ExcludeRegexp, rx)
				}
			}
		}
	}

	return configs, nil
}
