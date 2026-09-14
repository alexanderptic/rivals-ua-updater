package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const repoPublic = "alexanderptic/rivals-ua"

// Стала адреса релізу. Свідомо НЕ через GitHub API: неавтентифіковані запити
// обмежені 60 на годину на IP-адресу, і люди за спільним NAT упиралися б у
// ліміт. Завантаження ассета таким лімітом не обмежене.
func releaseURL(name string) string {
	return "https://github.com/" + repoPublic + "/releases/latest/download/" + name
}

type Manifest struct {
	Version   string `json:"version"`
	Asset     string `json:"asset"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Published string `json:"published"`
	Tool      string `json:"tool"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func FetchManifest() (*Manifest, error) {
	req, _ := http.NewRequest("GET", releaseURL("version.json"), nil)
	req.Header.Set("User-Agent", "RivalsUA/"+toolVersion)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("сервер відповів %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Asset == "" || m.SHA256 == "" || m.Version == "" {
		return nil, fmt.Errorf("неповний version.json")
	}
	// Ім'я приходить із сервера і йде у шлях файлу — не пускаємо в нього
	// роздільники, інакше зіпсований маніфест писав би куди завгодно.
	if strings.ContainsAny(m.Asset, `\/:*?"<>|`) {
		return nil, fmt.Errorf("недопустиме ім'я файлу в version.json")
	}
	return &m, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Порівняння версій. Тут два різні формати: версія перекладу — це номер
// білда Steam (одне велике число), версія інструменту — «1.0.2». Один розбір
// покриває обидва: ділимо на сегменти по крапці й порівнюємо числами там, де
// це числа.
//
// Просте порівняння рядків тут не годиться: «1.0.10» лексично менше за
// «1.0.9», і оновлення почало б їздити по колу.
func isNewer(remote, local string) bool {
	if local == "" {
		return true
	}
	if remote == local {
		return false
	}
	rs, ls := strings.Split(remote, "."), strings.Split(local, ".")
	for i := 0; i < len(rs) || i < len(ls); i++ {
		r, l := "0", "0"
		if i < len(rs) {
			r = rs[i]
		}
		if i < len(ls) {
			l = ls[i]
		}
		rn, err1 := strconv.ParseInt(r, 10, 64)
		ln, err2 := strconv.ParseInt(l, 10, 64)
		switch {
		case err1 == nil && err2 == nil:
			if rn != ln {
				return rn > ln
			}
		case r != l:
			return r > l
		}
	}
	return false
}
