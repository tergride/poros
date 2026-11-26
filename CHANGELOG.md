# Changelog

## 0.5.0

<!-- ru -->
- **OpenVPN|BY:** профили и конечные точки для серверов OpenVPN|BY с белорусской криптографией — BTLS, belt и bash. Poros сам узнаёт такой `.ovpn`. Ключ bign под паролем открывается паролем из профиля, а при неверном пароле «Старт» останавливается и называет сервер.
- **`.ovpn` с ключами в отдельных файлах** больше не отказывает при импорте: профиль открывается в редакторе, ключи и сертификаты выбираются кнопкой «Из файла…». Конечная точка из такого файла показывает, каких данных не хватает.
- **Строка `cipher` в `.ovpn`:** конфиги с ней, а это большинство файлов, больше не падают при «Старте».
- **Секреты зашифрованы:** пароли, ключи, ссылки подписок, секреты OTP, пароль локального прокси и ключи WARP больше не лежат в данных Poros открытым текстом. Их защищает DPAPI Windows для этого компьютера.
- **Папка на другом компьютере** открывается без секретов. Такие серверы помечены ключом, а у подписки без ссылки есть кнопка «Ввести ссылку». При неудачном «Старте» Poros подсказывает, куда ввести данные: ссылку подписки, профиль в редакторе или резервную копию. Коды OTP без секрета ждут, пока его введут.
- **Резервная копия с паролем:** если в копии есть секреты, её можно целиком зашифровать паролем (AES-256-GCM, ключ из пароля получается Argon2id). При восстановлении Poros спросит пароль. Копия без пароля работает как раньше.
- **Пресеты** остаются в том порядке, в котором их создали.
- **Баннеры** в узком окне переносят кнопки под текст.

<!-- en -->
- **OpenVPN|BY:** profiles and endpoints for OpenVPN|BY servers with Belarusian cryptography — BTLS, belt and bash. Poros recognizes such an `.ovpn` by itself. A bign key under a password opens with the password from the profile, and a wrong password stops Start and names the server.
- **An `.ovpn` with its keys in separate files** no longer fails to import: the profile opens in the editor, and the keys and certificates are chosen with "From file…". An endpoint from such a file shows which data is missing.
- **The `cipher` line in `.ovpn`:** configs with it, which is most files, no longer fail at Start.
- **Secrets are encrypted:** passwords, keys, subscription links, OTP secrets, the password of the local proxy and the WARP keys no longer lie in Poros's data as plain text. Windows DPAPI protects them for this computer.
- **The folder on another computer** opens without its secrets. Such servers carry a key mark, and a subscription without its link has an "Enter the link" button. After a failed Start, Poros shows where to enter the data: the subscription link, the profile in the editor or a backup. OTP codes without their secret wait until it is entered.
- **Backups with a password:** when a backup holds secrets, it can be encrypted whole with a password (AES-256-GCM, with the key derived from the password by Argon2id). Poros asks for the password when restoring. A backup without a password works as before.
- **Presets** keep the order they were created in.
- **Banners** in a narrow window move their buttons under the text.

## 0.4.0

<!-- ru -->
- **Kill switch рядом с zapret и GoodbyeDPI:** раньше вместе с ними Kill switch закрывал весь интернет. Пока загружен их драйвер WinDivert, Kill switch работает в совместимом режиме: пропускает пакеты, которые WinDivert отправляет дальше, а остальное блокирует как обычно. В это время рядом с чипом Kill switch в заголовке стоит восклицательный знак с подсказкой.
- **Папка с кириллицей в пути:** если в названии папки Poros есть заглавные буквы не латиницей, Kill switch больше не блокирует сам Poros, а Strict Route — его DNS.
- **Первая страница без ожидания:** сразу после включения компьютера первая страница больше не ждёт около 20 секунд. Сертификаты DNS Google, Cloudflare и Quad9 Poros проверяет сам: по встроенным корням и по корням, установленным в Windows.
- **IPv6 по умолчанию выключен:** сайты, которые идут напрямую, открываются в сетях без IPv6, программы сразу переходят на IPv4. Сохранённые настройки не меняются. Серверам и сетям только с IPv6 нужен включённый IPv6.
- **Программы в правилах** совпадают по имени и по пути в любом регистре: правило для `game.exe` срабатывает и для `Game.exe`.
- **Импорт пресета под занятым именем** спрашивает: «Заменить», «Создать новый» или «Отмена». После замены пресет остаётся выбранным и на своём месте.
- **Окна:** окно, открытое последним, всегда сверху и получает клики и фокус.

<!-- en -->
- **Kill switch next to zapret and GoodbyeDPI:** the kill switch used to cut the whole internet next to them. While their WinDivert driver is loaded, the kill switch works in compatible mode: it lets out the packets WinDivert sends on and blocks the rest as usual. Meanwhile an exclamation mark with a hint stands next to the kill switch chip in the title bar.
- **A folder with Cyrillic in its path:** when the name of the Poros folder has capital letters other than Latin, the kill switch no longer blocks Poros itself, nor Strict Route its DNS.
- **The first page without a wait:** right after the computer starts, the first page no longer waits about 20 seconds. Poros verifies the certificates of the Google, Cloudflare and Quad9 DNS itself, by the built-in roots and the roots installed in Windows.
- **IPv6 is off by default:** sites that go direct open in networks without IPv6, as programs move to IPv4 at once. Saved settings stay as they are. Servers and networks with IPv6 only need IPv6 turned on.
- **Programs in rules** match by name and by path in any letter case: a rule for `game.exe` also catches `Game.exe`.
- **Importing a preset under a taken name** asks: "Replace", "Create new" or "Cancel". After a replace the preset stays chosen and in its place.
- **Dialogs:** the dialog opened last is always on top and gets the clicks and the focus.

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
