module github.com/jchigg2000-git/air-traffic

go 1.26

// Build with >= go1.26.6 to pick up the stdlib security fixes govulncheck
// flagged (net/url quadratic resolvePath, html/template JS-regexp context,
// crypto/tls post-handshake message limit, net/http http2 ReadHeaderTimeout).
// Pinned to go1.26.7, the current 1.26-line release, so deploy builds
// get the patched toolchain too — not just whatever is on a dev laptop.
toolchain go1.26.7
