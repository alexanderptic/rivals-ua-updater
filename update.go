//go:build windows

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Перевіряти файл, а не запис про нього.
//
// Оновлення гри або «перевірка цілісності файлів» у Steam чи Epic може
// прибрати вміст ~mods. Тоді в конфізі значиться «встановлено 25249397», а
// паку фізично немає — і інструмент відрапортував би «усе актуально»,
// запустивши англійську гру. Тому всі три умови, а не одна.
func IsCurrent(cfg *Config, m *Manifest) bool {
	pak := filepath.Join(cfg.ModsDir(), m.Asset)
	st, err := os.Stat(pak)
	if err != nil || st.Size() != m.Size {
		return false
	}
	sum, err := fileSHA256(pak)
	if err != nil || !strings.EqualFold(sum, m.SHA256) {
		return false
	}
	return !isNewer(m.Version, cfg.InstalledVersion)
}

type ProgressFn func(done, total int64)

// Завантажує пак і ставить його на місце.
//
// Відхилення від первинної специфікації, свідоме: тимчасовий файл лежить у
// тій самій теці ~mods, а не в %TEMP%. Перейменування атомарне лише в межах
// одного тому, а %TEMP% майже завжди на C:, тоді як гра часто на D:. Через
// %TEMP% це було б копіювання, і обрив живлення посеред нього лишив би
// напівзаписаний пак. Розширення .part гра ігнорує — вантажить лише *.pak.
func InstallPak(cfg *Config, m *Manifest, progress ProgressFn) error {
	mods := cfg.ModsDir()
	if err := os.MkdirAll(mods, 0o755); err != nil {
		return fmt.Errorf("не вдалося створити теку ~mods: %w", err)
	}
	cleanupPartials(mods)

	dst := filepath.Join(mods, m.Asset)
	tmp := dst + ".part"

	if err := downloadTo(tmp, releaseURL(m.Asset), m.Size, progress); err != nil {
		os.Remove(tmp)
		return err
	}
	sum, err := fileSHA256(tmp)
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if !strings.EqualFold(sum, m.SHA256) {
		os.Remove(tmp)
		return errHashMismatch
	}
	if err := replaceFile(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

var errHashMismatch = fmt.Errorf("хеш не збігся")

func cleanupPartials(dir string) {
	parts, _ := filepath.Glob(filepath.Join(dir, "*.part"))
	for _, p := range parts {
		os.Remove(p)
	}
}

func downloadTo(path, url string, expect int64, progress ProgressFn) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "RivalsUA/"+toolVersion)
	// Завантаження великого файлу не має впиратися в таймаут клієнта.
	dl := &http.Client{Timeout: 15 * time.Minute}
	resp, err := dl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("сервер відповів %d", resp.StatusCode)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	if total <= 0 {
		total = expect
	}
	var done int64
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	// Спорожнити буфери до перейменування: інакше «атомарність» лише на папері.
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if expect > 0 && done != expect {
		return fmt.Errorf("розмір не збігся: %d замість %d", done, expect)
	}
	return nil
}

// os.Rename у Windows падає, якщо ціль існує. MoveFileEx з REPLACE_EXISTING
// підміняє за один крок; WRITE_THROUGH не дає операції «завершитися» раніше,
// ніж вона справді ляже на диск.
func replaceFile(from, to string) error {
	f, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	t, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(f, t,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
