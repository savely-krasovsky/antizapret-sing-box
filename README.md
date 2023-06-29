# List of domain blocked in Russia in geosite form

It allows to create rules and unblock only necessary sites.

This project uses [zapret-info/z-i](https://github.com/zapret-info/z-i) repo
to get the latest dumps from [Roskomnadzor](https://en.wikipedia.org/wiki/Roskomnadzor).

Project currently consists from two utilities:

- `filegenerator` -- simple utility to generate `geosite.db` files from CSV dump.
- `githubreleaser` -- utility to create release and upload assets.

You can download the latest `geosite.db` here:
- https://github.com/L11R/antizapret-sing-geosite/releases/latest/download/geosite.db
- https://cdn.jsdelivr.net/gh/L11R/antizapret-sing-geosite@release/geosite.db