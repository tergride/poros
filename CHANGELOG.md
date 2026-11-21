# Changelog

## 0.3.0

<!-- ru -->
- **Подписка по ссылке из браузера:** ссылка `poros://addsub/` теперь принимает адрес подписки как есть, а не только в base64. Название после `#` работает в обоих видах. Формат ссылки для страниц провайдеров описан в README.
- **Подписка уже добавлена:** ссылка на подписку, которая уже есть в Poros, открывает окно с её именем и временем обновления. Из него подписку можно обновить или показать её группу.

<!-- en -->
- **Subscription links from the browser:** a `poros://addsub/` link now takes the subscription URL as it is, not only in base64. A name after `#` works in both forms. The README describes the link format for provider pages.
- **Subscription already added:** a link to a subscription Poros already has opens a dialog with its name and last update. From there the subscription can be updated or its group shown.

## 0.2.0

<!-- ru -->
- **Конечные точки отдельно от пресетов:** подключения OpenVPN и OpenConnect теперь в разделе «Маршрутизация → Конечные точки» расширенного режима. У каждой свои подключение, правила и выключатель, включённые работают с любым пресетом. Конечную точку можно сохранить в файл JSON с данными для входа или без них и добавить из такого файла или из `.ovpn`. Конечные точки, выбранные в пресетах 0.1.0, не переносятся: их нужно добавить заново.
- **Пресетом можно поделиться** QR-кодом в виде картинки, ссылкой или файлом JSON. Ссылки стали в несколько раз короче, прежние тоже открываются. В окно «Импорт пресета» картинку или файл можно перетащить, выбрать или вставить через Ctrl+V.
- **Редактор списков** подсказывает префиксы правил по мере ввода, а окно «Помощь» перечисляет все префиксы с примерами. Предупреждения о нераспознанных и повторных строках больше не меняют высоту поля.
- **Соединения:** пока подключения нет, экран соединений расширенного режима показывает подсказку вместо пустого графика.
- **Тексты настроек** стали точнее, у сложных настроек появились описания при наведении на значок «i».

<!-- en -->
- **Endpoints apart from presets:** OpenVPN and OpenConnect connections now live in Routing → Endpoints in advanced mode. Each one has its own connection, rules and switch, and enabled ones work with any preset. An endpoint can be saved to a JSON file with or without the data for signing in, and added from such a file or from an `.ovpn` file. Endpoints chosen in 0.1.0 presets are not carried over: add them again.
- **Share a preset** as a picture of its QR code, a link or a JSON file. Links are several times shorter, and the old ones still open. A picture or a file can be dropped into the "Import a preset" dialog, picked, or pasted with Ctrl+V.
- **The lists editor** suggests rule prefixes as you type, and its Help dialog lists every prefix with examples. Warnings about unrecognized and repeated lines no longer change the height of the field.
- **Connections:** while not connected, the connections screen of advanced mode shows a hint instead of an empty chart.
- **Setting texts** are more precise, and complex settings have notes on hover over the "i" icon.

## 0.1.0

<!-- ru -->
Первый выпуск Poros — портативного клиента VPN и прокси для Windows на sing-box и Xray-core.

- **Простой режим:** мастер первого запуска, подписки и серверы с проверкой задержки, TUN и системный прокси по отдельности, пресеты маршрутизации со своими списками, проверка IP и утечки DNS.
- **Расширенный режим:** свои группы и профили, редактор профиля, цепочки, правила маршрутизации, конечные точки OpenVPN и OpenConnect, DNS и WARP, настройки входящих, TUN, протоколов, сети и безопасности, соединения, журнал и статистика трафика.
- **Протоколы:** VLESS с REALITY и XHTTP, VMess, Trojan, Shadowsocks и ShadowTLS, Hysteria и Hysteria2, TUIC, AnyTLS, WireGuard, Naive, SSH, Snell, SOCKS и HTTP, OpenVPN, OpenConnect, MASQUE.
- **Kill switch:** при обрыве трафик VPN не уходит мимо сервера. Режимы «При подключении» (по умолчанию) и «Всегда».
- **Инструменты:** коды OTP, QR-коды, замер скорости, поиск Ctrl+K, горячие клавиши, резервные копии.
- **Система:** трей с выбором пресета, автозапуск, ссылки `poros://`, обновления с GitHub по кнопке «Перезапустить и обновить».

Нужна 64-битная Windows 10 или 11 с WebView2 Runtime (в Windows 11 он встроен). Poros не устанавливается: распакуйте архив в любую папку, где можно писать, и запустите `Poros.exe`, данные хранятся в папке `config` рядом с ним. Файлы не подписаны, поэтому при первом запуске Windows SmartScreen может предупредить: «Подробнее» → «Выполнить в любом случае». Для TUN Poros запрашивает права администратора.

<!-- en -->
The first release of Poros, a portable VPN and proxy client for Windows on sing-box and Xray-core.

- **Simple mode:** a first-run wizard, subscriptions and servers with latency tests, TUN and the system proxy each on its own, routing presets with your own lists, an IP and DNS leak check.
- **Advanced mode:** your own groups and profiles, the profile editor, chains, routing rules, OpenVPN and OpenConnect endpoints, DNS and WARP, settings for inbounds, TUN, protocols, network and security, connections, the log and traffic statistics.
- **Protocols:** VLESS with REALITY and XHTTP, VMess, Trojan, Shadowsocks and ShadowTLS, Hysteria and Hysteria2, TUIC, AnyTLS, WireGuard, Naive, SSH, Snell, SOCKS and HTTP, OpenVPN, OpenConnect, MASQUE.
- **Kill switch:** when the connection drops, VPN traffic does not go around the server. Modes "While connected" (the default) and "Always".
- **Tools:** OTP codes, QR codes, a speed test, Ctrl+K search, hotkeys, backups.
- **System:** a tray menu with the preset choice, start with Windows, `poros://` links, updates from GitHub with the "Restart and update" button.

Needs 64-bit Windows 10 or 11 with the WebView2 Runtime, which Windows 11 has built in. Poros has no installer: unpack the archive into any folder you can write to and run `Poros.exe`; its data stays in the `config` folder next to it. The files are not signed, so Windows SmartScreen may warn on the first start: More info → Run anyway. For TUN, Poros asks for administrator rights.
