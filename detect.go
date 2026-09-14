//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Знайдена установка гри. Назва кореневої теки ознакою не є:
//
//	MarvelRivals        — Steam
//	MarvelRivalsjKtnW   — Epic (суфікс не константа, у сусіда інший)
//
// Тому визначаємо за доказами на диску, а не за іменем.
type Install struct {
	Root            string
	Store           string // "steam" | "epic"
	SteamAppID      string
	EpicAppName     string
	EpicNamespace   string
	EpicCatalogItem string
}

// Єдиний доказ, що це справді корінь Marvel Rivals, а не схожа тека.
func looksLikeGameRoot(root string) bool {
	st, err := os.Stat(filepath.Join(root, "MarvelGame", "Marvel", "Content", "Paks"))
	return err == nil && st.IsDir()
}

func DetectInstalls() []Install {
	var out []Install
	out = append(out, detectSteam()...)
	out = append(out, detectEpic()...)
	return out
}

// ---------- Steam ----------

var vdfPath = regexp.MustCompile(`(?i)"path"\s+"((?:[^"\\]|\\.)*)"`)
var acfKey = regexp.MustCompile(`(?i)"(appid|installdir)"\s+"((?:[^"\\]|\\.)*)"`)

// У VDF зворотні скісні подвоєні.
func unescapeVDF(s string) string {
	return strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(s)
}

func steamRoot() string {
	for _, k := range []struct {
		hive registry.Key
		path string
		name string
	}{
		{registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath"},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"},
		{registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, "InstallPath"},
	} {
		key, err := registry.OpenKey(k.hive, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := key.GetStringValue(k.name)
		key.Close()
		if err == nil && v != "" {
			return filepath.FromSlash(v)
		}
	}
	return ""
}

func detectSteam() []Install {
	root := steamRoot()
	if root == "" {
		return nil
	}

	// Бібліотек може бути кілька — гра рідко лежить там само, де клієнт.
	libs := []string{root}
	if b, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil {
		for _, m := range vdfPath.FindAllStringSubmatch(string(b), -1) {
			if p := unescapeVDF(m[1]); p != "" {
				libs = append(libs, p)
			}
		}
	}

	seen := map[string]bool{}
	var out []Install
	for _, lib := range libs {
		apps := filepath.Join(lib, "steamapps")
		manifests, _ := filepath.Glob(filepath.Join(apps, "appmanifest_*.acf"))
		for _, mf := range manifests {
			b, err := os.ReadFile(mf)
			if err != nil {
				continue
			}
			var appid, installdir string
			for _, m := range acfKey.FindAllStringSubmatch(string(b), -1) {
				switch strings.ToLower(m[1]) {
				case "appid":
					appid = m[2]
				case "installdir":
					installdir = unescapeVDF(m[2])
				}
			}
			if appid == "" || installdir == "" {
				continue
			}
			candidate := filepath.Join(apps, "common", installdir)
			if !looksLikeGameRoot(candidate) || seen[strings.ToLower(candidate)] {
				continue
			}
			seen[strings.ToLower(candidate)] = true
			out = append(out, Install{Root: candidate, Store: "steam", SteamAppID: appid})
		}
	}
	return out
}

// ---------- Epic ----------

type epicManifest struct {
	InstallLocation  string `json:"InstallLocation"`
	AppName          string `json:"AppName"`
	CatalogNamespace string `json:"CatalogNamespace"`
	CatalogItemId    string `json:"CatalogItemId"`
}

func detectEpic() []Install {
	dir := filepath.Join(os.Getenv("ProgramData"), "Epic", "EpicGamesLauncher", "Data", "Manifests")
	items, _ := filepath.Glob(filepath.Join(dir, "*.item"))

	var out []Install
	for _, it := range items {
		b, err := os.ReadFile(it)
		if err != nil {
			continue
		}
		var m epicManifest
		if json.Unmarshal(b, &m) != nil || m.InstallLocation == "" {
			continue
		}
		root := filepath.Clean(m.InstallLocation)
		if !looksLikeGameRoot(root) {
			continue
		}
		out = append(out, Install{
			Root:            root,
			Store:           "epic",
			EpicAppName:     m.AppName,
			EpicNamespace:   m.CatalogNamespace,
			EpicCatalogItem: m.CatalogItemId,
		})
	}
	return out
}
