//go:build windows

package main

import "golang.org/x/sys/windows"

// texts are the words of the window and its messages. Each %s takes a folder or an error.
type texts struct {
	folder, browse, note, desktop, startMenu, launch, unpack, cancel, unpacking, picker    string
	noArchive, notAbsolute, notWritable, running, failed, noShortcut, notStarted, unpacked string
}

var russianTexts = texts{
	folder:      "Папка для Poros:",
	browse:      "Обзор…",
	note:        "Данные Poros хранятся в этой же папке. Перед её удалением нажмите «Удалить следы» в «Помощи» настроек Poros.",
	desktop:     "Ярлык на рабочем столе",
	startMenu:   "Ярлык в меню «Пуск»",
	launch:      "Запустить Poros после распаковки",
	unpack:      "Распаковать",
	cancel:      "Отмена",
	unpacking:   "Распаковка…",
	picker:      "Папка для Poros",
	noArchive:   "В этом файле нет архива Poros: возможно, он скачался не полностью.\n\nСкачайте его заново со страницы выпусков Poros.",
	notAbsolute: "Укажите полный путь к папке.",
	notWritable: "Нет доступа на запись в папку:\n%s\n\nВыберите папку, в которую у вас есть права на запись.",
	running:     "Poros запущен из этой папки.\n\nЗакройте его («Выход» в меню его значка в трее) и нажмите «Распаковать» ещё раз.",
	failed:      "Не удалось распаковать Poros:\n%s",
	noShortcut:  "Не удалось создать ярлык:\n%s",
	notStarted:  "Poros распакован, но не запустился:\n%s",
	unpacked:    "Poros распакован в папку:\n%s",
}

var englishTexts = texts{
	folder:      "Folder for Poros:",
	browse:      "Browse…",
	note:        "Poros keeps its data in this folder too. Before deleting it, press Remove traces in the Help settings of Poros.",
	desktop:     "Shortcut on the desktop",
	startMenu:   "Shortcut in the Start menu",
	launch:      "Start Poros after unpacking",
	unpack:      "Unpack",
	cancel:      "Cancel",
	unpacking:   "Unpacking…",
	picker:      "Folder for Poros",
	noArchive:   "This file has no Poros archive in it: it may not have downloaded completely.\n\nDownload it again from the releases page of Poros.",
	notAbsolute: "Enter the full path of the folder.",
	notWritable: "Cannot write to the folder:\n%s\n\nChoose a folder where you can write.",
	running:     "Poros is running from this folder.\n\nQuit it first (Exit in the menu of its tray icon), then press Unpack again.",
	failed:      "Poros could not be unpacked:\n%s",
	noShortcut:  "The shortcut could not be created:\n%s",
	notStarted:  "Poros is unpacked but did not start:\n%s",
	unpacked:    "Poros is unpacked into the folder:\n%s",
}

func textsFor(russian bool) texts {
	if russian {
		return russianTexts
	}
	return englishTexts
}

var procGetUserDefaultUILanguage = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

const langRussian = 0x19

// russian tells whether the language of Windows is Russian.
func russian() bool {
	language, _, _ := procGetUserDefaultUILanguage.Call()
	return language&0x3ff == langRussian
}
