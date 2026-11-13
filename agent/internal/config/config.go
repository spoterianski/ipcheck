// Package config loads the ipcheck agent configuration.
package config

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
	// Listen is the address of the dashboard / API. Use 0.0.0.0:8787 to reach it from other devices (Steam Deck).
	Listen string `yaml:"listen"`
	// FakeIPCIDR is the range podkop (sing-box) answers with for routed domains.
	FakeIPCIDR  string `yaml:"fakeip_cidr"`
	CheckFakeIP *bool  `yaml:"check_fakeip"`
	Notify      *bool  `yaml:"notify"`

	Exits  map[string]*Exit `yaml:"exits"`
	Groups []Group          `yaml:"groups"`

	// MQTT publishes the state to Home Assistant; omit it to disable.
	MQTT *MQTT `yaml:"mqtt"`

	FakeIP netip.Prefix `yaml:"-"`
}

type Exit struct {
	IP      string   `yaml:"ip"`
	IPs     []string `yaml:"ips"`
	Country string   `yaml:"country"`
}

type MQTT struct {
	Broker          string `yaml:"broker"` // tcp://homeassistant.local:1883
	Username        string `yaml:"username"`
	Password        string `yaml:"password"`
	ClientID        string `yaml:"client_id"`
	Topic           string `yaml:"topic"`            // base topic, default "ipcheck"
	DiscoveryPrefix string `yaml:"discovery_prefix"` // default "homeassistant"
	DeviceName      string `yaml:"device_name"`      // default "IP Check"
}

type Group struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	Expect string `yaml:"expect"`
	// CheckFakeIP overrides the global setting, e.g. false for a "direct" group whose domain is not in podkop lists.
	CheckFakeIP *bool `yaml:"check_fakeip"`
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ipcheck", "config.yaml")
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := c.normalize(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) normalize() error {
	if c.Interval <= 0 {
		c.Interval = 30 * time.Second
	}
	if c.Timeout <= 0 {
		c.Timeout = 10 * time.Second
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8787"
	}
	if c.FakeIPCIDR == "" {
		c.FakeIPCIDR = "198.18.0.0/15"
	}
	p, err := netip.ParsePrefix(c.FakeIPCIDR)
	if err != nil {
		return fmt.Errorf("fakeip_cidr: %w", err)
	}
	c.FakeIP = p

	owner := map[string]string{}
	for name, e := range c.Exits {
		if e == nil {
			return fmt.Errorf("exit %q is empty", name)
		}
		if e.IP != "" {
			e.IPs = append([]string{e.IP}, e.IPs...)
		}
		if len(e.IPs) == 0 {
			return fmt.Errorf("exit %q has no ip", name)
		}
		for _, ip := range e.IPs {
			if _, err := netip.ParseAddr(ip); err != nil {
				return fmt.Errorf("exit %q: %w", name, err)
			}
			if other, dup := owner[ip]; dup {
				return fmt.Errorf("ip %s belongs to both exits %q and %q; use one exit for both groups", ip, other, name)
			}
			owner[ip] = name
		}
	}

	if m := c.MQTT; m != nil {
		if m.Broker == "" {
			return fmt.Errorf("mqtt: broker is required")
		}
		if m.ClientID == "" {
			host, _ := os.Hostname()
			m.ClientID = "ipcheck-" + host
		}
		if m.Topic == "" {
			m.Topic = "ipcheck"
		}
		if m.DiscoveryPrefix == "" {
			m.DiscoveryPrefix = "homeassistant"
		}
		if m.DeviceName == "" {
			m.DeviceName = "IP Check"
		}
	}

	if len(c.Groups) == 0 {
		return fmt.Errorf("no groups configured")
	}
	seen := map[string]bool{}
	for _, g := range c.Groups {
		if g.Name == "" || g.URL == "" {
			return fmt.Errorf("group needs name and url: %+v", g)
		}
		if seen[g.Name] {
			return fmt.Errorf("duplicate group %q", g.Name)
		}
		seen[g.Name] = true
		if _, ok := c.Exits[g.Expect]; !ok {
			return fmt.Errorf("group %q expects unknown exit %q", g.Name, g.Expect)
		}
	}
	return nil
}

func (c *Config) NotifyEnabled() bool { return c.Notify == nil || *c.Notify }

func (c *Config) FakeIPCheck(g Group) bool {
	if g.CheckFakeIP != nil {
		return *g.CheckFakeIP
	}
	return c.CheckFakeIP == nil || *c.CheckFakeIP
}

// ExitByIP returns the name of the exit that owns ip, or "" if none.
func (c *Config) ExitByIP(ip string) (string, *Exit) {
	for name, e := range c.Exits {
		for _, v := range e.IPs {
			if v == ip {
				return name, e
			}
		}
	}
	return "", nil
}
