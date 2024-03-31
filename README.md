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
