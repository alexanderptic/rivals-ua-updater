//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Хеші того, що їде в архіві. Потрібні не для безпеки, а щоб відрізнити
// «наш завантажувач стоїть» від «людина має власні ASI-моди».
const (
	sha256DSound = "bb8767f918c52a2ad055d2de9baffd2478598643b9894f09abd20d1f1ffd170c"
	sha256Bypass = "2f8b183149f30c94319a5f6636491ff39aa22afbbf3f5cbc5fb8f7ecc287a62d"
	pluginName   = "MarvelRivalsUTOCSignatureBypass.asi"
)

// Завантажувач лежить поруч із .exe, а не всередині нього. Програма, що
// дістає з себе DLL і кладе її біля чужої гри, — поведінковий портрет
// дроппера; копіювання файлу, що лежить поруч, сигнал значно слабший.
func loaderSourceDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "loader"
	}
	return filepath.Join(filepath.Dir(exe), "loader")
}

func LoaderSourcePresent() bool {
	st, err := os.Stat(filepath.Join(loaderSourceDir(), "dsound.dll"))
	return err == nil && !st.IsDir()
}

// Чи стоїть завантажувач у грі.
func LoaderInstalled(cfg *Config) bool {
	st, err := os.Stat(filepath.Join(cfg.BinariesDir(), "dsound.dll"))
	return err == nil && !st.IsDir()
}

// Перевіряємо, що в архіві саме те, що ми туди клали. Ловить не зловмисника,
// а звичайніше: недокачаний архів, розпакований «поверх» старий файл,
// антивірус, що вирізав .asi й лишив нульовий файл на його місці.
func VerifyLoaderSource() error {
	src := loaderSourceDir()
	for _, f := range []struct {
		path, want string
	}{
		{filepath.Join(src, "dsound.dll"), sha256DSound},
		{filepath.Join(src, "plugins", pluginName), sha256Bypass},
	} {
		got, err := fileSHA256(f.path)
		if err != nil {
			return fmt.Errorf("немає %s", filepath.Base(f.path))
		}
		if !strings.EqualFold(got, f.want) {
			return fmt.Errorf("%s пошкоджено", filepath.Base(f.path))
		}
	}
	return nil
}

func InstallLoader(cfg *Config) error {
	src := loaderSourceDir()
	bin := cfg.BinariesDir()
	if st, err := os.Stat(bin); err != nil || !st.IsDir() {
		return fmt.Errorf("не бачу теки %s", bin)
	}
	if err := VerifyLoaderSource(); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(src, "dsound.dll"), filepath.Join(bin, "dsound.dll")); err != nil {
		return err
	}
	plugins := filepath.Join(bin, "plugins")
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		return err
	}
	return copyFile(filepath.Join(src, "plugins", pluginName), filepath.Join(plugins, pluginName))
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	// Пишемо поруч і підміняємо: якщо гра тримає стару DLL, помилка прийде
	// на підміні, а не лишить напівзаписаний файл.
	tmp := to + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := replaceFile(tmp, to); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// «Прибрати все» — найважливіша функція безпеки в інструменті. Якщо з'явиться
// новина про блокування, людина має відкотитися за секунди.
//
// Чуже не чіпаємо: сторонній .asi-мод у plugins\ лишається, теку видаляємо
// лише якщо після нас вона порожня.
func RemoveAll(cfg *Config) []error {
	var errs []error
	rm := func(p string) {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(p), err))
		}
	}

	paks, _ := filepath.Glob(filepath.Join(cfg.ModsDir(), "UAMarvelRivals_*.pak"))
	for _, p := range paks {
		rm(p)
	}
	cleanupPartials(cfg.ModsDir())

	bin := cfg.BinariesDir()
	rm(filepath.Join(bin, "dsound.dll"))
	rm(filepath.Join(bin, "plugins", pluginName))

	plugins := filepath.Join(bin, "plugins")
	if entries, err := os.ReadDir(plugins); err == nil && len(entries) == 0 {
		os.Remove(plugins)
	}

	cfg.InstalledVersion = ""
	cfg.LoaderInstalled = false
	if err := cfg.Save(); err != nil {
		errs = append(errs, err)
	}
	return errs
}

// Помилки файлових операцій людською мовою. Системних кодів не показуємо.
func explainFileError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "being used by another process"),
		strings.Contains(s, "используется"),
		strings.Contains(s, "sharing violation"):
		return "Гра запущена. Закрийте її й натисніть ще раз."
	case errors.Is(err, fs.ErrPermission), strings.Contains(s, "access is denied"):
		return "Немає доступу до теки гри. Запустіть інструмент від імені адміністратора."
	}
	return "Не вдалося записати файл у теку гри. Перевірте, що гра закрита."
}
