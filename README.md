# Lists of domain and IPs blocked in Russia in geosite and geoip form

It allows to create rules and unblock only necessary sites:

1. Set default outbound to `bypass`.
2. Proxify IPs with `geoip:antizapret` and sites with `geosite:antizapret`.

This project uses [zapret-info/z-i](https://github.com/zapret-info/z-i) repo
to get the latest dumps from [Roskomnadzor](https://en.wikipedia.org/wiki/Roskomnadzor).

Project currently consists from two utilities:

- `filegenerator` -- simple utility to generate `geoip.db` and `geosite.db` files from CSV dump.
- `githubreleaser` -- utility to create release and upload assets.

You can download the latest `geoip.db` and `geosite.db` here:
- https://github.com/L11R/antizapret-sing-geosite/releases/latest/download/geoip.db
- https://github.com/L11R/antizapret-sing-geosite/releases/latest/download/geosite.db
