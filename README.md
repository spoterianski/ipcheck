# IP Check

```
vps/        — ipcheck-echo: echo-сервер для VPS (отдельный Go-модуль)
streamdeck/ — плагин Stream Deck: сводная кнопка и кнопки маршрутов
agent/      — ipcheck: агент для Mac (трей, уведомления) и Linux (headless + MQTT в Home Assistant), дашборд, API, CLI
```

Проверяет, что трафик к каждой группе доменов уходит через свой выход
(podkop → каскад 3x-ui), и сразу сигнализирует, если это не так.

```
Mac: ipcheck ──► yt.check.example.com ──► podkop: секция youtube ──► выход DE ──┐
             ──► ai.check.example.com ──► podkop: секция ai      ──► выход US ──┼──► VPS: ipcheck-echo
             ──► direct.check...      ──► напрямую               ──► провайдер ─┘     {"host", "remote_ip"}
```

Агент сравнивает `remote_ip`, который увидел echo, с ожидаемым выходом группы.

Статусы:
- `ok`: всё совпадает;
- `mismatch`: трафик пришёл через другой известный выход;
- `unknown_exit`: трафик пришёл с IP, которого нет в `exits`;
- `dns_bypass`: домен резолвится не в FakeIP podkop, значит Mac использует чужой DNS (DoH, Private Relay, ручной DNS);
- `error`: echo недоступен.

## VPS

На каждую группу маршрутизации нужен свой echo-хост в общей зоне, например `yt.check.example.com`,
`ai.check.example.com`, `direct.check.example.com`. TLS держит nginx, echo слушает `127.0.0.1:8887`.

Свои хосты и зону впиши в `vps/hosts.local.mk` (он в `.gitignore`):

```sh
cp vps/hosts.example.mk vps/hosts.local.mk && vim vps/hosts.local.mk
```

1. DNS: заведи A-записи всех хостов на IP VPS.
2. Собери бинарник командой `make -C vps linux` (для ARM: `make -C vps linux-arm64`) и скопируй `vps/bin/ipcheck-echo-linux-amd64` на VPS в `/usr/local/bin/ipcheck-echo`.
3. Запусти echo:
   ```sh
   cp ipcheck-echo.service /etc/systemd/system/
   systemctl daemon-reload && systemctl enable --now ipcheck-echo
   curl -s http://127.0.0.1:8887/ -H 'Host: test'
   ```
4. Собери nginx-конфиг с твоими хостами командой `make -C vps nginx`, получится `vps/bin/nginx-ipcheck.conf`.
   Подключи его на VPS, пока только с HTTP-частью: закомментируй блок `listen 443`, потому что сертификата ещё нет:
   ```sh
   cp nginx-ipcheck.conf /etc/nginx/sites-available/ipcheck
   ln -s /etc/nginx/sites-available/ipcheck /etc/nginx/sites-enabled/
   nginx -t && systemctl reload nginx
   ```
5. Выпусти один сертификат на все хосты. Продление потом работает автоматически.
   `make -C vps certbot` печатает готовую команду, её нужно выполнить на VPS:
   ```sh
   certbot certonly --nginx --cert-name check.example.com \
     -d yt.check.example.com -d ai.check.example.com -d direct.check.example.com
   ```
6. Раскомментируй блок `listen 443`, выполни `nginx -t && systemctl reload nginx` и проверь все хосты: `make -C vps check`.
   С самого VPS `remote_ip` будет IP этого VPS. Реальные маршруты проверяй с Mac через `ipcheck status`.

Чтобы добавить группу:
1. Заведи DNS-запись нового хоста.
2. Добавь хост в `HOSTS` в `vps/hosts.local.mk` и заново собери конфиг командой `make -C vps nginx`.
3. Повтори шаг 5 с новой командой из `make -C vps certbot`.

Без nginx echo умеет сам выпускать сертификаты Let's Encrypt (`-hosts ... -https :443`), но для этого нужны свободные порты 80 и 443. Этот вариант описан в unit-файле в комментарии.

Echo-VPS не должен быть узлом каскада, иначе запрос может закончиться на нём самом.

## Роутер

- **podkop**: добавь каждый тестовый домен в user domain list нужной секции.
- **3x-ui**: если группа разветвляется на втором уровне, добавь домен в routing rules. Sniffing (`http`, `tls`) должен быть включён.

## Mac

```sh
make install-mac          # = make -C agent install: ~/.local/bin/ipcheck, конфиг, автозапуск (без sudo)
vim ~/.config/ipcheck/config.yaml
launchctl kickstart -k gui/$(id -u)/com.ipcheck.agent
```

- Иконка в меню-баре: зелёная, когда всё в порядке; красная с числом проблем, если есть проблемы. В меню показан статус каждой группы.
- Уведомления macOS приходят только при смене статуса.
- Дашборд: http://127.0.0.1:8787/
- Лог: `~/Library/Logs/ipcheck.log`; удалить: `make -C agent uninstall`

Консоль:

```sh
ipcheck status          # спрашивает запущенный агент, без него проверяет сам
ipcheck status -local   # всегда проверять напрямую
ipcheck status -json    # код возврата 1, если есть проблемы
```

## Stream Deck

Плагин рисует кнопки сам и берёт состояние напрямую у агента на Mac (`/api/events`), без Home Assistant.
Интеграция с HA остаётся как есть, это параллельный способ показа.

```sh
make install-streamdeck   # npm install, сборка, симлинк в плагины Stream Deck, перезапуск приложения
```

Нужны Node.js (для сборки) и Stream Deck 7.1+. Действия появляются в категории **IP Check**:

| Действие | Как выглядит |
|---|---|
| Сводка | зелёная с ✓ и `4/4 ok`; красная (мигает) с числом проблем и именами групп |
| Маршрут | группа сверху, флаг страны фактического выхода (из `flag-icons`), снизу задержка или выход; цвет по статусу |

Цвета: зелёный `ok`; красный `mismatch` / `unknown_exit`; оранжевый `dns_bypass`; тёмно-красный `error`;
серый, если агент не отвечает; жёлто-коричневый, если последняя проверка старше трёх интервалов.
Нажатие на любую кнопку запускает проверку. В настройках кнопки задаются адрес агента (общий, по умолчанию
`http://127.0.0.1:8787`), маршрут и мигание сводки.

После правок в `streamdeck/src`: `make -C streamdeck build restart`.

## API

| Метод | Путь          | Что делает                            |
|-------|---------------|---------------------------------------|
| GET   | `/api/status` | текущее состояние (JSON)              |
| GET   | `/api/events` | SSE-поток состояний после каждой проверки |
| POST  | `/api/check`  | проверить немедленно                  |


## Linux + Home Assistant

На постоянно включённой Linux-машине в той же сети агент работает без трея и публикует статус в Home Assistant через MQTT.
На Mac раздел `mqtt:` не указывается, и агент просто показывает статус в меню-баре.

Linux-машина должна ходить через тот же роутер и использовать его DNS. Если это не так, агент покажет `dns_bypass`.

```sh
make -C agent linux            # или linux-arm64
scp agent/bin/ipcheck-linux-amd64 host:/usr/local/bin/ipcheck
scp agent/ipcheck-agent.service host:/etc/systemd/system/
# на Linux:
mkdir -p /etc/ipcheck && vim /etc/ipcheck/config.yaml   # тот же конфиг + раздел mqtt:
chmod 644 /etc/ipcheck/config.yaml
systemctl daemon-reload && systemctl enable --now ipcheck-agent
journalctl -u ipcheck-agent -f   # ждём "mqtt: connected"
```

Сущности появляются в HA автоматически, в устройстве **IP Check**:

| Сущность | Что показывает |
|---|---|
| `binary_sensor.ip_check_marshruty` | `on`, если есть проблема (device_class `problem`) |
| `sensor.ip_check_problem` | число проблем |
| `sensor.ip_check_<маршрут>` | `ok` / `mismatch` / `unknown_exit` / `dns_bypass` / `error`; в атрибутах `ip`, `exit`, `flag`, `label` («🇩🇪 de-vpn»), `message` |
| `button.ip_check_proverit_seichas` | запустить проверку немедленно |
| `sensor.ip_check_posledniaia_proverka` | время последней проверки |

Точные `entity_id` HA формирует сам из названий, их можно посмотреть в карточке устройства.
Если агент не присылает данные дольше трёх интервалов или выключен, сенсоры становятся `unavailable`.

MQTT-топики: `ipcheck/state` (всё состояние), `ipcheck/group/<маршрут>`, `ipcheck/availability`, `ipcheck/check` (команда).

## Лицензия

MIT, см. [LICENSE](LICENSE).
