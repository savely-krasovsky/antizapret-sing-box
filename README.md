# Lists of domain and IPs blocked in Russia in Rule Set form

Legacy geosite and geoip are also supported for now.

It allows to create rules and unblock only necessary sites:

1. Set default outbound to `bypass`.
2. Proxify IPs with [Rule Set](https://sing-box.sagernet.org/configuration/rule-set/) rule
   (or use legacy `geoip:antizapret` and `geosite:antizapret`).

This project uses [zapret-info/z-i](https://github.com/zapret-info/z-i) repo
to get the latest dumps from [Roskomnadzor](https://en.wikipedia.org/wiki/Roskomnadzor).

Project currently consists from two utilities:

- `filegenerator` -- simple utility to generate `antizapret.srs`, `antizapret-ruleset.json`, `geoip.db`
  and `geosite.db` files from CSV dump.
- `githubreleaser` -- utility to create release and upload assets.

You can download the latest `antizapret.srs`, `geoip.db` and `geosite.db` here:
- https://github.com/L11R/antizapret-sing-geosite/releases/latest/download/antizapret.srs
- https://github.com/L11R/antizapret-sing-geosite/releases/latest/download/geoip.db
- https://github.com/L11R/antizapret-sing-geosite/releases/latest/download/geosite.db

## Example

Below is the example of configuration using WireGuard outbound
(you can easily switch it to Shadowsocks or everything else sing-box supports) and encrypted AdGuard DNS
(which will work over WireGuard to block ads and trackers while connected).

```json
{
  "log": {
    "level": "warn"
  },
  "dns": {
    "servers": [
      {
        "tag": "adguard-dns",
        "address": "tls://dns.adguard-dns.com",
        "address_resolver": "local-dns",
        "detour": "wireguard-out"
      },
      {
        "tag": "local-dns",
        "address": "local",
        "detour": "direct-out"
      }
    ],
    "rules": [
      {
        "outbound": "any",
        "server": "local-dns"
      }
    ]
  },
  "inbounds": [
    {
      "type": "tun",
      "inet4_address": "172.16.0.1/30",
      "auto_route": true,
      "strict_route": true,
      "sniff": true
    }
  ],
  "outbounds": [
    {
      "type": "direct",
      "tag": "direct-out"
    },
    {
      "type": "wireguard",
      "tag": "wireguard-out",
      "server": "REDACTED",
      "server_port": 51820,
      "system_interface": true,
      "local_address": [
        "10.252.0.1/32",
        "2600:xxxx:xxxx:cafe::1/128"
      ],
      "private_key": "REDACTED",
      "peer_public_key": "REDACTED",
      "pre_shared_key": "REDACTED"
    },
    {
      "type": "dns",
      "tag": "dns-out"
    }
  ],
  "route": {
    "rules": [
      {
        "rule_set": "antizapret",
        "outbound": "wireguard-out"
      },
      {
        "protocol": "dns",
        "outbound": "dns-out"
      }
    ],
    "rule_set": [
      {
        "tag": "antizapret",
        "type": "remote",
        "format": "binary",
        "url": "https://github.com/L11R/antizapret-sing-geosite/releases/latest/download/antizapret.srs",
        "download_detour": "wireguard-out"
      }
    ],
    "auto_detect_interface": true
  },
  "experimental": {
    "cache_file": {
      "enabled": true
    }
  }
}
```