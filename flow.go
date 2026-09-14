//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lxn/walk"
)

const countdownSeconds = 3

var autolaunchCancelled atomic.Bool

// ---------- дрібні помічники для оновлення вікна з фонової горутини ----------

func (a *App) ui(f func()) { a.mw.Synchronize(f) }

func (a *App) setStatus(s string) { a.ui(func() { a.lblStatus.SetText(s) }) }
func (a *App) setHint(s string) {
	a.ui(func() { a.lblHint.SetText(s); a.lblHint2.SetText("") })
}

func (a *App) setHint2(line1, line2 string) {
	a.ui(func() { a.lblHint.SetText(line1); a.lblHint2.SetText(line2) })
}

func (a *App) setPath() {
	a.ui(func() {
		if a.cfg.GameRoot == "" {
			a.lblPath.SetText("Теку гри ще не обрано")
			return
		}
		store := map[string]string{"steam": "Steam", "epic": "Epic Games"}[a.cfg.Store]
		if store == "" {
			store = "Невідомий магазин"
		}
		a.lblPath.SetText(store + " · " + shorten(a.cfg.GameRoot, 58))
	})
}

func (a *App) setVersion() {
	a.ui(func() {
		if a.cfg.InstalledVersion == "" {
			a.lblVersion.SetText("Переклад ще не встановлено")
			return
		}
		// Видно у вікні, щоб у Discord замість «у мене не працює» приходило
		// «у мене версія 25249397».
		a.lblVersion.SetText("Версія перекладу: " + a.cfg.InstalledVersion)
	})
}

func shorten(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "…" + string(r[len(r)-max+1:])
}

// Панель з'являється лише тоді, коли автозапуск зупинено.
func (a *App) showPanel() {
	autolaunchCancelled.Store(true)
	a.ui(func() {
		a.rowWait.SetVisible(false)
		a.rowChoose.SetVisible(false)
		a.rowPanel.SetVisible(true)
		a.pb.Hide()
	})
}

func (a *App) showChoose() {
	autolaunchCancelled.Store(true)
	a.ui(func() {
		a.rowWait.SetVisible(false)
		a.rowPanel.SetVisible(false)
		a.rowChoose.SetVisible(true)
		a.pb.Hide()
	})
}

// Помилка — автозапуску немає: людина має встигнути прочитати, що зламалося.
func (a *App) fail(status, hint string) {
	a.setStatus(status)
	a.setHint(hint)
	a.showPanel()
}

// ---------- головний хід ----------

func (a *App) Run() {
	go func() {
		defer func() {
			// Паніка у фоновій горутині не повинна прибивати вікно мовчки.
			if r := recover(); r != nil {
				a.fail("Щось пішло не так", fmt.Sprintf("%v. Напишіть у Discord, будь ласка.", r))
			}
		}()
		a.flow()
	}()
}

func (a *App) flow() {
	if !a.ensureGameRoot() {
		return
	}
	a.setPath()
	a.setVersion()

	// Завантажувач міг зникнути після оновлення гри. Не відновлюємо мовчки:
	// це могло статися тому, що гра почала його блокувати.
	if a.cfg.LoaderInstalled && !LoaderInstalled(a.cfg) {
		a.cfg.LoaderInstalled = false
		_ = a.cfg.Save()
		a.fail("Завантажувач зник із теки гри",
			"Найімовірніше його прибрало оновлення гри. Натисніть «Грати», щоб поставити знову, — або «Прибрати все», якщо не хочете ризикувати.")
		return
	}

	a.setStatus("Перевіряю оновлення…")
	m, err := FetchManifest()
	if err != nil {
		// Немає інтернету — не блокувати гру. Людина хоче грати, а не
		// діагностувати GitHub.
		a.setStatus("Не вдалося перевірити оновлення")
		a.setHint("Запускаю з наявним перекладом.")
		a.countdownAndLaunch()
		return
	}
	a.manifest = m

	// Тільки якщо на сервері справді новіша. Інакше той, хто вже поставив
	// свіжий інструмент, бачив би пораду оновитись до старішого.
	if m.Tool != "" && isNewer(m.Tool, toolVersion) {
		a.setHint("Є новіша версія інструменту (" + m.Tool + ") — посилання в Discord.")
	}

	if IsCurrent(a.cfg, m) {
		if a.cfg.InstalledVersion != m.Version {
			a.cfg.InstalledVersion = m.Version
			_ = a.cfg.Save()
			a.setVersion()
		}
		a.setStatus("Переклад актуальний")
		if !a.ensureLoader() {
			return
		}
		if a.showSetupNotice() {
			return
		}
		a.countdownAndLaunch()
		return
	}

	a.setStatus("Оновлюю переклад…")
	if !a.download(m) {
		return
	}

	a.cfg.InstalledVersion = m.Version
	_ = a.cfg.Save()
	a.setVersion()
	a.setStatus("Переклад оновлено")
	a.ui(func() { a.pb.Set(1, colGold) })

	if !a.ensureLoader() {
		return
	}
	if a.showSetupNotice() {
		return
	}
	a.countdownAndLaunch()
}

// Одна повторна спроба на зіпсованому завантаженні, потім здаємось і
// запускаємо гру зі старим паком. Зіпсований файл не ставимо.
func (a *App) download(m *Manifest) bool {
	for attempt := 0; attempt < 2; attempt++ {
		// Кожен прочитаний шматок — не привід малювати смужку заново: на
		// швидкій мережі це сотні повідомлень у чергу вікна за секунду.
		var lastPaint time.Time
		err := InstallPak(a.cfg, m, func(done, total int64) {
			if total <= 0 {
				return
			}
			if done < total && time.Since(lastPaint) < 80*time.Millisecond {
				return
			}
			lastPaint = time.Now()
			frac := float64(done) / float64(total)
			// Золота — щось змінюється; переривати зараз небезпечно.
			a.ui(func() { a.pb.Set(frac, colGold) })
		})
		if err == nil {
			return true
		}
		if err == errHashMismatch && attempt == 0 {
			a.setHint("Файл завантажився пошкодженим. Пробую ще раз…")
			continue
		}
		switch {
		case err == errHashMismatch:
			a.setStatus("Не вдалося оновити переклад")
			a.setHint("Файл двічі завантажився пошкодженим. Напишіть у Discord, будь ласка.")
			a.showPanel()
		case isFileBusy(err):
			a.fail("Гра запущена", "Закрийте її й натисніть «Грати».")
		default:
			a.fail("Не вдалося оновити переклад", explainFileError(err))
		}
		return false
	}
	return false
}

func isFileBusy(err error) bool {
	s := strings.ToLower(fmt.Sprint(err))
	return strings.Contains(s, "another process") || strings.Contains(s, "sharing violation")
}

func (a *App) ensureLoader() bool {
	if LoaderInstalled(a.cfg) {
		if !a.cfg.LoaderInstalled {
			a.cfg.LoaderInstalled = true
			_ = a.cfg.Save()
		}
		return true
	}
	// Теки loader\ немає, а завантажувач у грі вже стоїть — працюємо мовчки
	// далі. Немає ні того, ні того — це неповний архів.
	if !LoaderSourcePresent() {
		a.fail("Архів розпаковано не повністю",
			"Немає теки «loader» поруч із програмою. Завантажте архів ще раз і розпакуйте цілком.")
		return false
	}
	if err := VerifyLoaderSource(); err != nil {
		a.fail("Архів пошкоджено",
			err.Error()+". Завантажте архів ще раз. Якщо повториться — можливо, файл вирізав антивірус; напишіть у Discord.")
		return false
	}
	a.setHint("Ставлю завантажувач модів…")
	if err := InstallLoader(a.cfg); err != nil {
		a.fail("Не вдалося поставити завантажувач", explainFileError(err))
		return false
	}
	a.cfg.LoaderInstalled = true
	_ = a.cfg.Save()
	a.setHint("")
	return true
}

// Одноразова порада після першої установки.
//
// Ярлика на робочому столі інструмент не створює (рішення 13.09) — натомість
// каже людині перенести сам .exe. Після того, як завантажувач став у гру,
// тека loader\ більше не потрібна, і файл самодостатній: його можна тримати
// де завгодно, хоч на робочому столі.
//
// Повертає true, якщо пораду показано — тоді автозапуску цього разу немає:
// вікно, що зникає через три секунди, такий текст прочитати не дає.
func (a *App) showSetupNotice() bool {
	if a.cfg.SetupNoticeShown {
		return false
	}
	a.cfg.SetupNoticeShown = true
	_ = a.cfg.Save()

	name := "RivalsUA.exe"
	if exe, err := os.Executable(); err == nil {
		name = filepath.Base(exe)
	}

	a.setStatus("Переклад встановлено")
	a.setHint2(
		"Перенесіть лише цей файл «"+name+"» на робочий стіл.",
		"Запускайте гру через нього — переклад оновиться сам.")
	a.showPanel()
	return true
}

// ---------- тека гри ----------

func (a *App) ensureGameRoot() bool {
	if a.cfg.GameRoot != "" && looksLikeGameRoot(a.cfg.GameRoot) {
		return true
	}
	a.setStatus("Шукаю гру…")
	a.installs = DetectInstalls()

	switch len(a.installs) {
	case 0:
		a.setPath()
		a.fail("Не знайшов гру",
			`Не бачу MarvelGame\Marvel\Content\Paks. Натисніть «Змінити теку» й оберіть кореневу теку гри — ту, всередині якої лежить MarvelGame.`)
		return false
	case 1:
		a.adopt(a.installs[0])
		return true
	default:
		// Інструмент не вирішує, у якому магазині грати.
		a.setStatus("Знайшов дві установки")
		a.setHint("Оберіть, яку запускати. Запам'ятаю вибір.")
		a.showChoose()
		return false
	}
}

func (a *App) adopt(in Install) {
	a.cfg.GameRoot = in.Root
	a.cfg.Store = in.Store
	a.cfg.SteamAppID = in.SteamAppID
	a.cfg.EpicAppName = in.EpicAppName
	a.cfg.EpicNamespace = in.EpicNamespace
	a.cfg.EpicCatalogItem = in.EpicCatalogItem
	_ = a.cfg.Save()
	a.setPath()
}

// ---------- відлік і запуск ----------

// Якщо вікно зникає одразу, до кнопки «Прибрати все» не дістатися ніколи:
// у звичайний день переклад актуальний, вікно живе півсекунди, і єдиний шлях
// до неї закривається разом із ним. Три секунди з видимим відліком це
// розв'язують, не заважаючи тому, хто просто хоче грати.
func (a *App) countdownAndLaunch() {
	if autolaunchCancelled.Load() {
		return
	}
	const steps = countdownSeconds * 10
	for i := 0; i < steps; i++ {
		if autolaunchCancelled.Load() {
			return
		}
		left := countdownSeconds - i/10
		a.setHint(fmt.Sprintf("Запуск гри за %d с", left))
		// Синя — іде відлік до запуску, і саме для цього й дається час.
		frac := 1 - float64(i)/float64(steps)
		a.ui(func() { a.pb.Set(frac, colAccent) })
		time.Sleep(100 * time.Millisecond)
	}
	if autolaunchCancelled.Load() {
		return
	}
	a.launch()
}

func (a *App) launch() {
	a.setStatus("Запускаю гру…")
	a.ui(func() { a.pb.Set(1, colAccent) })

	form, err := Launch(a.cfg, func(secondsLeft int) {
		a.setHint(fmt.Sprintf("Чекаю на гру… %d с", secondsLeft))
	})
	if err != nil {
		a.fail("Не вдалося запустити гру",
			"Запустіть її цього разу вручну через магазин. Якщо повториться — напишіть у Discord.")
		return
	}
	// Те, що спрацювало, запам'ятовуємо: далі пробуємо одразу його.
	if a.cfg.Store == "epic" {
		if strings.Contains(form, "%3A") {
			a.cfg.EpicLaunchForm = "long"
		} else {
			a.cfg.EpicLaunchForm = "short"
		}
		_ = a.cfg.Save()
	}
	// Після запуску інструмент закривається.
	a.ui(func() { a.mw.Close() })
}

// ---------- кнопки ----------

func (a *App) onWait() {
	autolaunchCancelled.Store(true)
	a.setHint("")
	a.showPanel()
}

func (a *App) onPlay() {
	go func() {
		autolaunchCancelled.Store(false)
		defer autolaunchCancelled.Store(true)
		if a.manifest != nil && !IsCurrent(a.cfg, a.manifest) {
			a.setStatus("Оновлюю переклад…")
			if !a.download(a.manifest) {
				return
			}
			a.cfg.InstalledVersion = a.manifest.Version
			_ = a.cfg.Save()
			a.setVersion()
		}
		if !a.ensureLoader() {
			return
		}
		a.launch()
	}()
}

func (a *App) onChooseStore(store string) {
	for _, in := range a.installs {
		if in.Store == store {
			a.adopt(in)
			a.ui(func() {
				a.rowChoose.SetVisible(false)
				a.rowPanel.SetVisible(true)
			})
			// Людина щойно зробила вибір — це і є її «запускай».
			go func() {
				autolaunchCancelled.Store(false)
				a.flow()
			}()
			return
		}
	}
}

func (a *App) onChooseFolder() {
	dlg := new(walk.FileDialog)
	dlg.Title = "Оберіть кореневу теку гри — ту, всередині якої лежить MarvelGame"
	if a.cfg.GameRoot != "" {
		dlg.InitialDirPath = filepath.Dir(a.cfg.GameRoot)
	}
	ok, err := dlg.ShowBrowseFolder(a.mw)
	if err != nil || !ok || dlg.FilePath == "" {
		return
	}
	root := filepath.Clean(dlg.FilePath)
	if !looksLikeGameRoot(root) {
		// Кажемо, чого саме не знайшли, а не «неправильна тека».
		walk.MsgBox(a.mw, "Не та тека",
			"У "+root+" немає MarvelGame\\Marvel\\Content\\Paks.\n\n"+
				"Оберіть теку, всередині якої лежить MarvelGame.",
			walk.MsgBoxIconWarning)
		return
	}

	a.cfg.GameRoot = root
	// Магазин визначаємо наново: людина могла показати іншу установку.
	a.cfg.Store, a.cfg.SteamAppID = "", ""
	a.cfg.EpicAppName, a.cfg.EpicNamespace, a.cfg.EpicCatalogItem = "", "", ""
	for _, in := range DetectInstalls() {
		if strings.EqualFold(filepath.Clean(in.Root), root) {
			a.adopt(in)
			break
		}
	}
	if a.cfg.Store == "" {
		_ = a.cfg.Save()
		a.setPath()
		a.setHint("Теку бачу, але не зрозумів, з якого вона магазину. Переклад оновлю, гру запустіть вручну.")
	}
	go func() {
		autolaunchCancelled.Store(false)
		a.flow()
	}()
}

func (a *App) onRemoveAll() {
	if walk.MsgBox(a.mw, "Прибрати все",
		"Видалити пак перекладу й завантажувач модів?\n\n"+
			"Гра повернеться до англійської та до незміненого стану. "+
			"Поставити назад можна цією ж програмою.",
		walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
		return
	}
	if errs := RemoveAll(a.cfg); len(errs) > 0 {
		walk.MsgBox(a.mw, "Прибрати вдалося не все",
			"Не вдалося видалити: "+joinErrs(errs)+"\n\nНайімовірніше гра запущена — закрийте її й спробуйте ще раз.",
			walk.MsgBoxIconWarning)
		return
	}
	a.setStatus("Прибрано")
	a.setHint("Гра в незміненому стані.")
	a.setVersion()
}

func joinErrs(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, ", ")
}
