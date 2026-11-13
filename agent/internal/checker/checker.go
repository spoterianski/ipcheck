// Package checker probes the echo server through each routing group and
// compares the exit IP it sees with the configured one.
package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"ipcheck/internal/config"
)

type Status string

const (
	OK        Status = "ok"
	Mismatch  Status = "mismatch"     // came through another known exit
	Unknown   Status = "unknown_exit" // came from an IP not listed in exits
	DNSBypass Status = "dns_bypass"   // domain resolved to a real IP instead of podkop FakeIP
	Error     Status = "error"        // echo unreachable or answered garbage
)

type Result struct {
	Group         string    `json:"group"`
	Host          string    `json:"host"`
	Expect        string    `json:"expect"`
	ExpectCountry string    `json:"expect_country"`
	IP            string    `json:"ip,omitempty"`
	Exit          string    `json:"exit,omitempty"`
	Country       string    `json:"country,omitempty"`
	ResolvedIP    string    `json:"resolved_ip,omitempty"`
	Status        Status    `json:"status"`
	Message       string    `json:"message,omitempty"`
	LatencyMs     int64     `json:"latency_ms"`
	CheckedAt     time.Time `json:"checked_at"`
}

type Checker struct {
	cfg    *config.Config
	client *http.Client
}

func New(cfg *config.Config) *Checker {
	return &Checker{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				// Every probe must open a fresh connection, otherwise a pooled
				// connection would hide a route change.
				DisableKeepAlives: true,
				Proxy:             nil,
			},
		},
	}
}

func (c *Checker) CheckAll(ctx context.Context) []Result {
	out := make([]Result, len(c.cfg.Groups))
	var wg sync.WaitGroup
	for i, g := range c.cfg.Groups {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = c.Check(ctx, g)
		}()
	}
	wg.Wait()
	return out
}

func (c *Checker) Check(ctx context.Context, g config.Group) Result {
	r := Result{
		Group:         g.Name,
		Expect:        g.Expect,
		ExpectCountry: c.cfg.Exits[g.Expect].Country,
		CheckedAt:     time.Now(),
	}
	u, err := url.Parse(g.URL)
	if err != nil {
		return r.fail("bad url: %v", err)
	}
	r.Host = strings.ToLower(u.Hostname())

	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	dnsOK := true
	if c.cfg.FakeIPCheck(g) {
		addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", r.Host)
		if err != nil || len(addrs) == 0 {
			return r.fail("DNS: %v", err)
		}
		r.ResolvedIP = addrs[0].Unmap().String()
		dnsOK = false
		for _, a := range addrs {
			if c.cfg.FakeIP.Contains(a.Unmap()) {
				dnsOK = true
				break
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.URL, nil)
	if err != nil {
		return r.fail("%v", err)
	}
	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return r.fail("%v", err)
	}
	defer resp.Body.Close()
	r.LatencyMs = time.Since(start).Milliseconds()
	if resp.StatusCode != http.StatusOK {
		return r.fail("HTTP %d", resp.StatusCode)
	}

	var echo struct {
		Host     string `json:"host"`
		RemoteIP string `json:"remote_ip"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&echo); err != nil {
		return r.fail("bad echo response: %v", err)
	}
	if echo.RemoteIP == "" {
		return r.fail("echo response has no remote_ip")
	}
	if !strings.EqualFold(echo.Host, r.Host) {
		return r.fail("echo saw host %q instead of %q", echo.Host, r.Host)
	}

	r.IP = echo.RemoteIP
	if name, e := c.cfg.ExitByIP(r.IP); e != nil {
		r.Exit, r.Country = name, e.Country
	}

	switch {
	case !dnsOK:
		r.Status = DNSBypass
		r.Message = fmt.Sprintf("%s резолвится в %s, а не в FakeIP %s — DNS идёт мимо podkop", r.Host, r.ResolvedIP, c.cfg.FakeIP)
	case r.Exit == g.Expect:
		r.Status = OK
	case r.Exit == "":
		r.Status = Unknown
		r.Message = fmt.Sprintf("неизвестный выход %s, ожидался %s", r.IP, g.Expect)
	default:
		r.Status = Mismatch
		r.Message = fmt.Sprintf("пришёл через %s, ожидался %s", r.Exit, g.Expect)
	}
	return r
}

func (r Result) fail(format string, args ...any) Result {
	r.Status = Error
	r.Message = fmt.Sprintf(format, args...)
	return r
}

// Flag converts an ISO 3166-1 alpha-2 code to a flag emoji.
func Flag(cc string) string {
	if len(cc) != 2 {
		return "🏳️"
	}
	cc = strings.ToUpper(cc)
	return string([]rune{0x1F1E6 + rune(cc[0]-'A'), 0x1F1E6 + rune(cc[1]-'A')})
}
