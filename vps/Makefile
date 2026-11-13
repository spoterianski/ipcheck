.PHONY: build linux linux-arm64 nginx certbot check clean

# Хосты echo: свои значения в hosts.local.mk (не в git), иначе пример.
include $(if $(wildcard hosts.local.mk),hosts.local.mk,hosts.example.mk)
FQDNS := $(addsuffix .$(ZONE),$(HOSTS))

build:
	go build -o bin/ipcheck-echo .

linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ipcheck-echo-linux-amd64 .

linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ipcheck-echo-linux-arm64 .

# nginx-конфиг с подставленными хостами: bin/nginx-ipcheck.conf
nginx:
	mkdir -p bin
	sed -e 's/__SERVER_NAMES__/$(FQDNS)/' -e 's/__ZONE__/$(ZONE)/g' nginx-ipcheck.conf > bin/nginx-ipcheck.conf

# команда certbot для VPS: один сертификат на все хосты
certbot:
	@echo certbot certonly --nginx --cert-name $(ZONE) $(addprefix -d ,$(FQDNS))

# проверить все echo-хосты отсюда
check:
	@for h in $(FQDNS); do printf '%s: ' $$h; curl -s -m 5 https://$$h/ || echo FAIL; done

clean:
	rm -rf bin
