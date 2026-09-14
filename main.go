//go:build windows

package main

import (
	"github.com/lxn/walk"
)

func main() {
	cfg := LoadConfig()

	app := NewApp(cfg)
	if err := app.Build(); err != nil {
		walk.MsgBox(nil, "Marvel Rivals українською",
			"Не вдалося створити вікно. Напишіть у Discord, будь ласка:\n"+discordURL,
			walk.MsgBoxIconError)
		return
	}

	app.Run()
	app.mw.Run()
}
