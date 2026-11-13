// Command echo is the VPS side of ipcheck: it answers every request with the
// hostname the client asked for and the IP address the request came from.
package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

type response struct {
	Host     string    `json:"host"`
	SNI      string    `json:"sni,omitempty"`
	RemoteIP string    `json:"remote_ip"`
	TLS      bool      `json:"tls"`
	Time     time.Time `json:"time"`
}

func main() {
	var (
		hosts      = flag.String("hosts", "", "comma-separated hostnames to obtain Let's Encrypt certificates for; empty = plain HTTP only")
		cacheDir   = flag.String("cache", "/var/lib/ipcheck-echo", "directory for ACME certificates")
		email      = flag.String("email", "", "contact email for Let's Encrypt (optional)")
		httpAddr   = flag.String("http", ":80", "HTTP listen address (also serves ACME challenges)")
		httpsAddr  = flag.String("https", ":443", "HTTPS listen address (used only with -hosts)")
		trustProxy = flag.Bool("trust-proxy", false, "take client IP from X-Real-IP / X-Forwarded-For (only behind your own reverse proxy)")
	)
	flag.Parse()

	h := handler(*trustProxy)

	if *hosts == "" {
		log.Printf("echo: plain HTTP on %s", *httpAddr)
		log.Fatal(newServer(*httpAddr, h, nil).ListenAndServe())
	}

	list := strings.Split(*hosts, ",")
	for i := range list {
		list[i] = strings.TrimSpace(strings.ToLower(list[i]))
	}
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(list...),
		Cache:      autocert.DirCache(*cacheDir),
		Email:      *email,
	}

	go func() {
		log.Printf("echo: HTTP + ACME challenges on %s", *httpAddr)
		log.Fatal(newServer(*httpAddr, m.HTTPHandler(h), nil).ListenAndServe())
	}()
	log.Printf("echo: HTTPS on %s for %v", *httpsAddr, list)
	log.Fatal(newServer(*httpsAddr, h, m.TLSConfig()).ListenAndServeTLS("", ""))
}

func newServer(addr string, h http.Handler, tlsCfg *tls.Config) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
}

func handler(trustProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if trustProxy {
			if v := r.Header.Get("X-Real-IP"); v != "" {
				ip = v
			} else if v := r.Header.Get("X-Forwarded-For"); v != "" {
				ip = strings.TrimSpace(strings.Split(v, ",")[0])
			}
		}
		if a, err := netip.ParseAddr(ip); err == nil {
			ip = a.Unmap().String()
		}

		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		resp := response{
			Host:     strings.ToLower(host),
			RemoteIP: ip,
			Time:     time.Now().UTC(),
		}
		if r.TLS != nil {
			resp.TLS = true
			resp.SNI = r.TLS.ServerName
		} else if trustProxy && r.Header.Get("X-Forwarded-Proto") == "https" {
			resp.TLS = true
			resp.SNI = r.Header.Get("X-Forwarded-SNI")
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
