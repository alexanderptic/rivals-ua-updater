package main

import (
	"io"
	"net/http"
	"os"
	"testing"
)

// Прогін проти справжнього релізу. Windows для цього не потрібна: саме та
// логіка, що вирішує «оновлювати чи ні», від платформи не залежить.
func TestLiveRelease(t *testing.T) {
	m, err := FetchManifest()
	if err != nil {
		t.Fatalf("не вдалося прочитати version.json: %v", err)
	}
	t.Logf("version=%s asset=%s size=%d tool=%s", m.Version, m.Asset, m.Size, m.Tool)

	if m.Version == "" || m.Asset == "" || m.SHA256 == "" || m.Size <= 0 {
		t.Fatalf("маніфест неповний: %+v", m)
	}
	if len(m.SHA256) != 64 {
		t.Fatalf("sha256 має бути 64 символи, а не %d", len(m.SHA256))
	}

	resp, err := http.Get(releaseURL(m.Asset))
	if err != nil {
		t.Fatalf("пак не завантажився: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("пак віддав %d", resp.StatusCode)
	}

	f, err := os.CreateTemp("", "pak")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if n != m.Size {
		t.Fatalf("розмір: завантажено %d, у маніфесті %d", n, m.Size)
	}
	sum, err := fileSHA256(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if sum != m.SHA256 {
		t.Fatalf("sha256 не збігся:\n  файл:     %s\n  маніфест: %s", sum, m.SHA256)
	}
	t.Logf("пак збігається з маніфестом: %d Б, %s", n, sum[:16]+"…")

	// Той самий .sha256 поруч має казати те саме.
	r2, err := http.Get(releaseURL(m.Asset + ".sha256"))
	if err == nil {
		defer r2.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(r2.Body, 128))
		side := string(b)
		if len(side) >= 64 && side[:64] != m.SHA256 {
			t.Errorf(".sha256 розходиться з version.json: %s", side[:64])
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		remote, local string
		want          bool
	}{
		{"25249397", "", true},          // ще нічого не встановлено
		{"25249397", "25249397", false}, // те саме
		{"25249398", "25249397", true},  // новіший білд
		{"25249396", "25249397", false}, // сервер відкотив реліз
		{"9999999", "25249397", false},  // менше число, хоч і більше цифр немає
		{"25249397", "9999999", true},   // навпаки
		{"season11", "season10", true},  // не числа — порівняння рядків
	}
	for _, c := range cases {
		if got := isNewer(c.remote, c.local); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, треба %v", c.remote, c.local, got, c.want)
		}
	}
}
