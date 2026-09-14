//go:build windows

package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Імена процесів, поява яких означає «гра стартувала». Список навмисне
// ширший за один рядок: назва шипінг-бінарника змінюється між патчами.
var gameProcessNames = []string{
	"marvel-win64-shipping.exe",
	"marvelrivals.exe",
	"marvelrivals_launcher.exe",
}

// Запуск через магазин, а не прямим викликом .exe. Прямий запуск оминає
// лаунчер: не ініціалізується його бібліотека, ламається оверлей, а античит
// може відмовитися стартувати без батьківського процесу.
func LaunchURLs(cfg *Config) []string {
	switch cfg.Store {
	case "steam":
		if cfg.SteamAppID == "" {
			return nil
		}
		return []string{"steam://rungameid/" + cfg.SteamAppID}

	case "epic":
		if cfg.EpicAppName == "" {
			return nil
		}
		// Довга форма — три поля маніфесту через закодовану двокрапку.
		// Формат узято з робочої реалізації (BoilR), не з документації Epic:
		// їхня сторінка про протокол вмісту не віддає. Тому нижче є запасний.
		long := ""
		if cfg.EpicNamespace != "" && cfg.EpicCatalogItem != "" {
			long = fmt.Sprintf(
				"com.epicgames.launcher://apps/%s%%3A%s%%3A%s?action=launch&silent=true",
				url.PathEscape(cfg.EpicNamespace),
				url.PathEscape(cfg.EpicCatalogItem),
				url.PathEscape(cfg.EpicAppName))
		}
		short := "com.epicgames.launcher://apps/" +
			url.PathEscape(cfg.EpicAppName) + "?action=launch&silent=true"

		// Те, що спрацювало минулого разу, пробуємо першим: невизначеність
		// коштує двадцять секунд один раз у житті, а не щоразу.
		switch {
		case cfg.EpicLaunchForm == "short":
			return []string{short, long}
		case long == "":
			return []string{short}
		default:
			return []string{long, short}
		}
	}
	return nil
}

func openURL(u string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	target, err := windows.UTF16PtrFromString(u)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
}

// OpenExternal — для посилання на Discord у вікні.
func OpenExternal(u string) { _ = openURL(u) }

func gameRunning() bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)

	var e windows.ProcessEntry32
	e.Size = uint32(unsafeSizeofProcessEntry32)
	if windows.Process32First(snap, &e) != nil {
		return false
	}
	for {
		name := strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
		for _, want := range gameProcessNames {
			if name == want {
				return true
			}
		}
		if windows.Process32Next(snap, &e) != nil {
			return false
		}
	}
}

// Запускає гру й чекає, чи вона справді з'явилася. Запасний шлях замість
// перевірки формату: Windows у нас немає, тож інструмент не покладається на
// посилання наосліп. Цей же механізм ловить і решту причин, чому гра могла
// не стартувати.
//
// Повертає форму посилання, що спрацювала, або помилку.
func Launch(cfg *Config, onWait func(secondsLeft int)) (string, error) {
	urls := LaunchURLs(cfg)
	if len(urls) == 0 {
		return "", fmt.Errorf("магазин не визначено")
	}
	for i, u := range urls {
		if err := openURL(u); err != nil {
			continue
		}
		// Останню форму не чекаємо: якщо й вона не спрацювала, чекати нічого.
		wait := 20
		if i == len(urls)-1 {
			wait = 8
		}
		for s := wait; s > 0; s-- {
			if onWait != nil {
				onWait(s)
			}
			time.Sleep(time.Second)
			if gameRunning() {
				return u, nil
			}
		}
	}
	return "", fmt.Errorf("гра не з'явилася")
}
