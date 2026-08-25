package wordpress

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/media"
)

// maxDownloadBytes bounds one attachment download; it matches the media
// service's own upload ceiling so oversized sources fail fast here.
const maxDownloadBytes = 10 << 20

const (
	downloadTimeout   = 30 * time.Second
	dialTimeout       = 5 * time.Second
	maxRedirects      = 4
	maxAttachmentURL  = 2048
	boundedChunkBytes = 64 << 10
)

// errForbiddenAddress marks SSRF policy rejections so tests can assert the
// rejection happened for the right reason (not e.g. connection refused).
var errForbiddenAddress = errors.New("attachment host resolves to a forbidden address")

// Downloader performs SSRF-hardened attachment downloads. Resolve and Dial are
// injection seams for tests; production uses the real resolver/dialer and no
// global bypass exists.
type Downloader struct {
	Resolve func(ctx context.Context, host string) ([]net.IP, error)
	Dial    func(ctx context.Context, network, addr string) (net.Conn, error)
}

func newDownloader() *Downloader {
	return &Downloader{}
}

func (d *Downloader) resolve(ctx context.Context, host string) ([]net.IP, error) {
	if d.Resolve != nil {
		return d.Resolve(ctx, host)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

func (d *Downloader) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if d.Dial != nil {
		return d.Dial(ctx, network, addr)
	}
	var nd net.Dialer
	return nd.DialContext(ctx, network, addr)
}

// validateURL enforces scheme/host policy and returns every resolved IP after
// checking that ALL of them are permitted (strict policy: one bad IP rejects).
func (d *Downloader) validateURL(ctx context.Context, raw string) (*url.URL, error) {
	if len(raw) > maxAttachmentURL {
		return nil, errors.New("attachment URL too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("attachment URL must use http or https")
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("attachment URL missing host")
	}
	if err := d.validateHost(ctx, host); err != nil {
		return nil, err
	}
	return u, nil
}

func (d *Downloader) validateHost(ctx context.Context, host string) error {
	ips, err := d.resolve(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(ips) == 0 {
		return errors.New("attachment host has no addresses")
	}
	for _, ip := range ips {
		if forbiddenIP(ip) {
			return errForbiddenAddress
		}
	}
	return nil
}

// validatedIPsKey carries per-host already-validated address sets through the
// request context so the transport dial NEVER re-resolves (no TOCTOU window).
type validatedIPsKey struct{}

func withValidatedIPs(ctx context.Context, host string, ips []net.IP) context.Context {
	m, _ := ctx.Value(validatedIPsKey{}).(map[string][]net.IP)
	next := make(map[string][]net.IP, len(m)+1)
	for k, v := range m {
		next[k] = v
	}
	next[host] = ips
	return context.WithValue(ctx, validatedIPsKey{}, next)
}

// lookupValidated returns pre-validated addresses for host when present.
func lookupValidated(ctx context.Context, host string) ([]net.IP, bool) {
	m, _ := ctx.Value(validatedIPsKey{}).(map[string][]net.IP)
	ips, ok := m[host]
	return ips, ok
}

// validateHostList enforces strict all-addresses-allowed policy.
func validateHostList(ips []net.IP) error {
	if len(ips) == 0 {
		return errors.New("attachment host has no addresses")
	}
	for _, ip := range ips {
		if forbiddenIP(ip) {
			return errForbiddenAddress
		}
	}
	return nil
}

// dialValidated dials a previously VALIDATED IP for this host. If none was
// carried in the request context it resolves+validates once itself. Either way,
// the address actually dialed is the validated one — DNS rebinding cannot swap
// in a private address between validation and connect.
func (d *Downloader) dialValidated(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, ok := lookupValidated(ctx, host)
	if !ok {
		ips, err = d.resolve(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
		if err := validateHostList(ips); err != nil {
			return nil, err
		}
	} else if err := validateHostList(ips); err != nil {
		return nil, err
	}
	validated := ips[0].String()
	return d.dial(ctx, network, net.JoinHostPort(validated, port))
}

// client builds an http.Client whose transport always routes through
// dialValidated and re-validates every redirect target before following it.
func (d *Downloader) client(ctx context.Context) *http.Client {
	tr := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return d.dialValidated(ctx, network, address)
		},
		// HTTPS: keep TLS ServerName as the ORIGINAL hostname. We dial the raw
		// TCP connection to the validated IP ourselves and let the stdlib run
		// TLS over it with correct SNI/verification semantics.
		DialTLSContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			raw, err := d.dialValidated(ctx, "tcp", net.JoinHostPort(host, port))
			if err != nil {
				return nil, err
			}
			conn := tls.Client(raw, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
			if err := conn.HandshakeContext(ctx); err != nil {
				_ = raw.Close()
				return nil, err
			}
			return conn, nil
		},
	}
	return &http.Client{
		Transport: tr,
		Timeout:   downloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			// Re-run full scheme/host/DNS/IP validation on EVERY redirect target;
			// its connection will also use dialValidated for that target.
			_, err := d.validateURL(req.Context(), req.URL.String())
			return err
		},
	}
}

// Get downloads raw, enforcing scheme/host/IP policy on the initial URL and on
// every redirect, dialing ONLY validated IPs (resolved exactly once per host),
// bounding response size, and returning an overflow-detecting reader.
func (d *Downloader) Get(ctx context.Context, raw string) (io.ReadCloser, string, error) {
	if len(raw) > maxAttachmentURL {
		return nil, "", errors.New("attachment URL too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", errors.New("attachment URL must use http or https")
	}
	host := u.Hostname()
	if host == "" {
		return nil, "", errors.New("attachment URL missing host")
	}
	// Resolve EXACTLY ONCE for this host; strict policy validates every address,
	// and the transport later dials these same IPs with NO further lookups
	// (closes the DNS-rebinding TOCTOU window completely).
	ips, err := d.resolve(ctx, host)
	if err != nil {
		return nil, "", fmt.Errorf("resolve %s: %w", host, err)
	}
	if err := validateHostList(ips); err != nil {
		return nil, "", err
	}
	ctx = withValidatedIPs(ctx, host, ips)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := d.client(ctx).Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, "", fmt.Errorf("attachment returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxDownloadBytes {
		resp.Body.Close()
		return nil, "", media.ErrTooLarge
	}
	name := path.Base(resp.Request.URL.Path)
	if name == "" || name == "/" || name == "." {
		name = "attachment"
	}
	return newBoundedBody(resp.Body, maxDownloadBytes), name, nil
}

// boundedBody fails with ErrTooLarge when the stream exceeds limit instead of
// silently truncating into a confusing parse error downstream.
type boundedBody struct {
	rc      io.ReadCloser
	read    int64
	limit   int64
	overrun bool
}

func newBoundedBody(rc io.ReadCloser, limit int64) *boundedBody {
	return &boundedBody{rc: rc, limit: limit}
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if b.read >= b.limit {
		b.overrun = true
		return 0, media.ErrTooLarge
	}
	if int64(len(p)) > b.limit-b.read+1 {
		p = p[:b.limit-b.read+1]
	}
	n, err := b.rc.Read(p)
	b.read += int64(n)
	if b.read > b.limit {
		b.overrun = true
		return n, media.ErrTooLarge
	}
	return n, err
}

func (b *boundedBody) Close() error { return b.rc.Close() }

// download keeps the previous call shape used by the importer.
func download(ctx context.Context, raw string) (io.ReadCloser, string, error) {
	return newDownloader().Get(ctx, raw)
}

func forbiddenIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		// CGNAT 100.64.0.0/10, documentation ranges, benchmarking 198.18.0.0/15, this-network 0.0.0.0/8.
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		if (ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2) || (ip4[0] == 192 && ip4[1] == 88 && ip4[2] == 99) {
			return true
		}
		if (ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100) || (ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113) {
			return true
		}
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
			return true
		}
		if ip4[0] == 0 {
			return true
		}
	} else {
		s := ip.String()
		if strings.HasPrefix(s, "2001:db8") || strings.HasPrefix(s, "64:ff9b") {
			return true
		}
	}
	return false
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
