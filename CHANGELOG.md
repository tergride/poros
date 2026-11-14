# Changelog

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
