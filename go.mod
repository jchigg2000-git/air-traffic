module github.com/jchigg2000-git/air-traffic

go 1.27

// The stdlib security fixes govulncheck flagged (net/url quadratic resolvePath,
// html/template JS-regexp context, crypto/tls post-handshake message limit,
// net/http http2 ReadHeaderTimeout) landed in go1.26.6 and are in the 1.27 line.
// Pinned to go1.27.3, the current 1.27-line release, so deploy builds
// get the patched toolchain too — not just whatever is on a dev laptop.
toolchain go1.27.3
