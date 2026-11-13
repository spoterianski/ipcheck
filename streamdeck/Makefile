.PHONY: build install uninstall restart clean

PLUGIN  := com.ipcheck.streamdeck.sdPlugin
PLUGINS := $(HOME)/Library/Application Support/com.elgato.StreamDeck/Plugins

build: node_modules
	npm run build

node_modules: package.json
	npm install
	touch node_modules

# Плагин подключается симлинком, так что после make build достаточно make restart.
install: build
	ln -sfn "$(CURDIR)/$(PLUGIN)" "$(PLUGINS)/$(PLUGIN)"
	$(MAKE) restart

uninstall:
	rm -f "$(PLUGINS)/$(PLUGIN)"
	$(MAKE) restart

restart:
	-osascript -e 'quit app "Elgato Stream Deck"'
	while pgrep -x "Stream Deck" >/dev/null; do sleep 0.5; done
	open -a "Elgato Stream Deck"

clean:
	rm -rf $(PLUGIN)/bin node_modules
