.PHONY: build vps mac linux streamdeck install-mac install-streamdeck clean

build: vps mac linux

vps:
	$(MAKE) -C vps linux

mac:
	$(MAKE) -C agent build

linux:
	$(MAKE) -C agent linux

install-mac:
	$(MAKE) -C agent install

streamdeck:
	$(MAKE) -C streamdeck build

install-streamdeck:
	$(MAKE) -C streamdeck install

clean:
	$(MAKE) -C vps clean
	$(MAKE) -C agent clean
	$(MAKE) -C streamdeck clean
