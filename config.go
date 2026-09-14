package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Стан інструменту живе в %APPDATA%\RivalsUA\config.json, а не поруч із .exe:
// той може лежати в теці гри й зникнути при переустановці гри.
type Config struct {
	GameRoot         string `json:"gameRoot"`
	Store            string `json:"store"` // "steam" | "epic"
	SteamAppID       string `json:"steamAppId,omitempty"`
	EpicAppName      string `json:"epicAppName,omitempty"`
	EpicNamespace    string `json:"epicNamespace,omitempty"`
	EpicCatalogItem  string `json:"epicCatalogItemId,omitempty"`
	EpicLaunchForm   string `json:"epicLaunchForm,omitempty"` // "long" | "short" — що спрацювало
	InstalledVersion string `json:"installedVersion,omitempty"`
	LoaderInstalled  bool   `json:"loaderInstalled"`
	SetupNoticeShown bool   `json:"setupNoticeShown"`
}

func configDir() string {
	return filepath.Join(os.Getenv("APPDATA"), "RivalsUA")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

func LoadConfig() *Config {
	c := &Config{}
	b, err := os.ReadFile(configPath())
	if err != nil {
		return c // першого запуску ще не було — це не помилка
	}
	// Понівечений конфіг не повинен блокувати запуск: вважаємо, що його немає,
	// і проходимо шлях першого запуску наново.
	if json.Unmarshal(b, c) != nil {
		return &Config{}
	}
	return c
}

func (c *Config) Save() error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	// Той самий прийом, що й із паком: пишемо поруч, потім підміняємо.
	// Вимкнене живлення посеред запису не має лишати порожній конфіг.
	tmp := configPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, configPath())
}

// Теки всередині кореня гри однакові для Steam і Epic.
func (c *Config) PaksDir() string {
	return filepath.Join(c.GameRoot, "MarvelGame", "Marvel", "Content", "Paks")
}

// Тильда на початку обов'язкова: від неї залежить порядок монтування паків.
func (c *Config) ModsDir() string {
	return filepath.Join(c.PaksDir(), "~mods")
}

func (c *Config) BinariesDir() string {
	return filepath.Join(c.GameRoot, "MarvelGame", "Marvel", "Binaries", "Win64")
}
