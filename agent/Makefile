.PHONY: build linux linux-arm64 install uninstall clean

BINDIR ?= $(HOME)/.local/bin
PLIST  := $(HOME)/Library/LaunchAgents/com.ipcheck.agent.plist
LOG    := $(HOME)/Library/Logs/ipcheck.log

build:
	go build -o bin/ipcheck .

# headless-агент для Linux (systemd + MQTT в Home Assistant)
linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ipcheck-linux-amd64 .

linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ipcheck-linux-arm64 .

install: build
	mkdir -p $(BINDIR) ~/.config/ipcheck ~/Library/LaunchAgents
	install -m 755 bin/ipcheck $(BINDIR)/ipcheck
	test -f ~/.config/ipcheck/config.yaml || cp config.example.yaml ~/.config/ipcheck/config.yaml
	sed -e 's#__BIN__#$(BINDIR)/ipcheck#' -e 's#__LOG__#$(LOG)#' com.ipcheck.agent.plist > $(PLIST)
	launchctl bootout gui/$$(id -u)/com.ipcheck.agent 2>/dev/null && sleep 2 || true
	launchctl bootstrap gui/$$(id -u) $(PLIST)

uninstall:
	launchctl bootout gui/$$(id -u)/com.ipcheck.agent 2>/dev/null || true
	rm -f $(PLIST) $(BINDIR)/ipcheck

clean:
	rm -rf bin
