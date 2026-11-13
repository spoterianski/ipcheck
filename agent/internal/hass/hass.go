// Package hass publishes the agent state to Home Assistant over MQTT and
// announces the entities through MQTT discovery.
package hass

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"ipcheck/internal/agent"
	"ipcheck/internal/checker"
	"ipcheck/internal/config"
)

type Publisher struct {
	cfg    *config.MQTT
	groups []config.Group
	expire int // seconds after which HA marks a sensor unavailable
	a      *agent.Agent
	client paho.Client
}

// groupState is a check result plus ready-to-show fields for Stream Deck keys.
type groupState struct {
	checker.Result
	OK         bool   `json:"ok"`
	Flag       string `json:"flag"`
	ExpectFlag string `json:"expect_flag"`
	Label      string `json:"label"`
}

func New(c *config.Config, a *agent.Agent) *Publisher {
	return &Publisher{
		cfg:    c.MQTT,
		groups: c.Groups,
		expire: int((3*c.Interval + c.Timeout) / time.Second),
		a:      a,
	}
}

func (p *Publisher) availTopic() string { return p.cfg.Topic + "/availability" }
func (p *Publisher) stateTopic() string { return p.cfg.Topic + "/state" }
func (p *Publisher) checkTopic() string { return p.cfg.Topic + "/check" }
func (p *Publisher) groupTopic(g string) string {
	return p.cfg.Topic + "/group/" + slug(g)
}

// Start connects in the background; the connection is retried until Stop.
func (p *Publisher) Start() {
	opts := paho.NewClientOptions().
		AddBroker(p.cfg.Broker).
		SetClientID(p.cfg.ClientID).
		SetUsername(p.cfg.Username).
		SetPassword(p.cfg.Password).
		SetWill(p.availTopic(), "offline", 1, true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(10 * time.Second).
		SetOnConnectHandler(p.onConnect).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			log.Printf("mqtt: connection lost: %v", err)
		})
	p.client = paho.NewClient(opts)
	p.client.Connect()

	go func() {
		ch, _ := p.a.Subscribe()
		for range ch {
			if p.client.IsConnectionOpen() {
				p.publishState()
			}
		}
	}()
}

func (p *Publisher) Stop() {
	if p.client == nil {
		return
	}
	if p.client.IsConnectionOpen() {
		p.client.Publish(p.availTopic(), 1, true, "offline").WaitTimeout(time.Second)
	}
	p.client.Disconnect(250)
}

func (p *Publisher) onConnect(c paho.Client) {
	log.Printf("mqtt: connected to %s", p.cfg.Broker)
	c.Subscribe(p.checkTopic(), 1, func(paho.Client, paho.Message) { p.a.CheckNow() })
	// Home Assistant forgets non-retained discovery after a restart; re-announce on its birth message.
	c.Subscribe(p.cfg.DiscoveryPrefix+"/status", 1, func(_ paho.Client, m paho.Message) {
		if string(m.Payload()) == "online" {
			p.publishDiscovery()
			p.publishState()
		}
	})
	p.publishDiscovery()
	c.Publish(p.availTopic(), 1, true, "online")
	p.publishState()
}

func (p *Publisher) publishState() {
	s := p.a.Snapshot()
	if len(s.Results) == 0 {
		return
	}
	p.publishJSON(p.stateTopic(), s)
	for _, r := range s.Results {
		p.publishJSON(p.groupTopic(r.Group), makeGroupState(r))
	}
}

func makeGroupState(r checker.Result) groupState {
	g := groupState{Result: r, OK: r.Status == checker.OK, ExpectFlag: checker.Flag(r.ExpectCountry)}
	switch {
	case r.IP == "":
		g.Flag = "❔"
		g.Label = agent.Describe(r.Status)
	case r.Country == "":
		g.Flag = "❔"
		g.Label = r.IP
	default:
		g.Flag = checker.Flag(r.Country)
		g.Label = g.Flag + " " + r.Exit
	}
	return g
}

func (p *Publisher) publishDiscovery() {
	device := map[string]any{
		"identifiers":  []string{"ipcheck_" + slug(p.cfg.Topic)},
		"name":         p.cfg.DeviceName,
		"manufacturer": "ipcheck",
		"model":        "route checker",
	}
	base := func(object, name string) map[string]any {
		return map[string]any{
			"name":               name,
			"unique_id":          "ipcheck_" + slug(p.cfg.Topic) + "_" + object,
			"availability_topic": p.availTopic(),
			"device":             device,
		}
	}
	announce := func(component, object string, cfg map[string]any) {
		topic := p.cfg.DiscoveryPrefix + "/" + component + "/ipcheck_" + slug(p.cfg.Topic) + "/" + object + "/config"
		p.publishJSON(topic, cfg)
	}

	problem := base("problem", "Маршруты")
	problem["device_class"] = "problem"
	problem["state_topic"] = p.stateTopic()
	problem["value_template"] = "{{ 'OFF' if value_json.ok else 'ON' }}"
	problem["json_attributes_topic"] = p.stateTopic()
	problem["json_attributes_template"] = "{{ {'problems': value_json.problems, 'updated': value_json.updated} | tojson }}"
	problem["expire_after"] = p.expire
	announce("binary_sensor", "problem", problem)

	count := base("problems", "Проблем")
	count["state_topic"] = p.stateTopic()
	count["value_template"] = "{{ value_json.problems }}"
	count["state_class"] = "measurement"
	count["icon"] = "mdi:alert-circle-outline"
	count["expire_after"] = p.expire
	announce("sensor", "problems", count)

	updated := base("updated", "Последняя проверка")
	updated["state_topic"] = p.stateTopic()
	updated["value_template"] = "{{ value_json.updated }}"
	updated["device_class"] = "timestamp"
	updated["entity_category"] = "diagnostic"
	announce("sensor", "updated", updated)

	button := base("check", "Проверить сейчас")
	button["command_topic"] = p.checkTopic()
	button["payload_press"] = "check"
	button["icon"] = "mdi:refresh"
	announce("button", "check", button)

	for _, g := range p.groups {
		s := base("group_"+slug(g.Name), g.Name)
		s["state_topic"] = p.groupTopic(g.Name)
		s["value_template"] = "{{ value_json.status }}"
		s["json_attributes_topic"] = p.groupTopic(g.Name)
		s["json_attributes_template"] = "{{ value_json | tojson }}"
		s["icon"] = "mdi:routes"
		s["expire_after"] = p.expire
		announce("sensor", "group_"+slug(g.Name), s)
	}
}

func (p *Publisher) publishJSON(topic string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("mqtt: %s: %v", topic, err)
		return
	}
	p.client.Publish(topic, 1, true, data)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9_]+`)

func slug(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "_"), "_")
}
