//go:build windows

package main

import (
	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

const discordURL = "https://discord.gg/HDPdYC7q9F"

// Попередження без екрана згоди з галочкою — рішення від 13.09. Замість
// одноразового кліку текст видно у вікні постійно.
// Двома рядками, а не одним із \n: STATIC-контроль Windows переносить рядки
// непередбачувано, а обрізане попередження гірше за жодне.
const (
	warnLine1 = "Інструмент вимикає перевірку підпису файлів гри й підмінює її вміст."
	warnLine2 = "Правила Marvel Rivals це забороняють — ви робите це на власний ризик."
)

type App struct {
	mw    *walk.MainWindow
	cfg   *Config
	fonts *fonts

	manifest *Manifest
	installs []Install

	lblStatus  *walk.Label
	lblPath    *walk.Label
	lblVersion *walk.Label
	lblHint    *walk.Label
	lblHint2   *walk.Label

	pb *bar

	rowWait   *walk.Composite
	rowPanel  *walk.Composite
	rowChoose *walk.Composite

	btnWait    *flatButton
	btnPlay    *flatButton
	btnFolder  *flatButton
	btnRemove  *flatButton
	btnSteam   *flatButton
	btnEpic    *flatButton
	btnDiscord *flatButton

	waitPressed chan struct{}
}

func NewApp(cfg *Config) *App {
	a := &App{cfg: cfg, fonts: loadFonts(), pb: &bar{}, waitPressed: make(chan struct{}, 1)}

	a.btnWait = &flatButton{text: "Зачекати", fill: colPanel2, fillDown: colLine,
		textColor: colInk, border: true, font: a.fonts.button, onClick: a.onWait}
	a.btnPlay = &flatButton{text: "ГРАТИ", fill: colAccent, fillDown: colAccentDim,
		textColor: colInk, font: a.fonts.button, onClick: a.onPlay}
	a.btnFolder = &flatButton{text: "Змінити теку", fill: colPanel2, fillDown: colLine,
		textColor: colInk, border: true, font: a.fonts.body, onClick: a.onChooseFolder}
	a.btnRemove = &flatButton{text: "Прибрати все", fill: colPanel2, fillDown: colLine,
		textColor: colInk, border: true, font: a.fonts.body, onClick: a.onRemoveAll}
	a.btnSteam = &flatButton{text: "Steam", fill: colAccent, fillDown: colAccentDim,
		textColor: colInk, font: a.fonts.button, onClick: func() { a.onChooseStore("steam") }}
	a.btnEpic = &flatButton{text: "Epic Games", fill: colAccent, fillDown: colAccentDim,
		textColor: colInk, font: a.fonts.button, onClick: func() { a.onChooseStore("epic") }}
	a.btnDiscord = &flatButton{text: "discord.gg/HDPdYC7q9F", fill: colPanel, fillDown: colPanel,
		textColor: colAccent, font: a.fonts.small, onClick: func() { OpenExternal(discordURL) }}

	return a
}

func (a *App) Build() error {
	err := d.MainWindow{
		AssignTo:   &a.mw,
		Title:      "Marvel Rivals українською",
		Background: d.SolidColorBrush{Color: colVoid},
		Size:       d.Size{Width: 540, Height: 430},
		MinSize:    d.Size{Width: 540, Height: 430},
		MaxSize:    d.Size{Width: 540, Height: 430},
		Layout:     d.VBox{MarginsZero: true, SpacingZero: true},
		Children: []d.Widget{
			a.header(),
			a.body(),
			a.footer(),
		},
	}.Create()
	if err != nil {
		return err
	}
	lockWindowSize(a.mw)
	return nil
}

// MinSize/MaxSize у walk — це обмеження розкладки, а не рамки вікна: розтягнути
// за край або розгорнути на весь екран вони не заважають. Перший прогін на
// справжній Windows саме так і виглядав — вікно на 1900 px завширшки.
// Тому знімаємо саму можливість: без WS_THICKFRAME немає за що тягнути,
// без WS_MAXIMIZEBOX нічого розгортати.
func lockWindowSize(mw *walk.MainWindow) {
	h := mw.Handle()
	style := win.GetWindowLong(h, win.GWL_STYLE)
	style &^= win.WS_THICKFRAME | win.WS_MAXIMIZEBOX
	win.SetWindowLong(h, win.GWL_STYLE, style)
	// Без SWP_FRAMECHANGED Windows перемалює рамку лише колись потім.
	win.SetWindowPos(h, 0, 0, 0, 0, 0,
		win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
}

func (a *App) header() d.Widget {
	return d.Composite{
		Background: d.SolidColorBrush{Color: colPanel},
		MinSize:    d.Size{Height: 52},
		MaxSize:    d.Size{Height: 52},
		Layout:     d.HBox{Margins: d.Margins{Left: 0, Top: 0, Right: 16, Bottom: 0}, Spacing: 12},
		Children: []d.Widget{
			// Жовтий язичок із сайту — єдине, що з його оформлення переноситься
			// в рідні віджети майже без втрат.
			d.Composite{
				Background: d.SolidColorBrush{Color: colGold},
				MinSize:    d.Size{Width: 6},
				MaxSize:    d.Size{Width: 6},
				Layout:     d.VBox{MarginsZero: true},
			},
			d.Label{
				Text:      "MARVEL RIVALS · УКРАЇНСЬКОЮ",
				Font:      fontOf(a.fonts.title),
				TextColor: colInk,
			},
			d.HSpacer{},
			d.Label{
				Text:      "v" + toolVersion,
				Font:      fontOf(a.fonts.small),
				TextColor: colInkDim,
			},
		},
	}
}

func (a *App) body() d.Widget {
	return d.Composite{
		Background:    d.SolidColorBrush{Color: colVoid},
		StretchFactor: 1,
		Layout:        d.VBox{Margins: d.Margins{Left: 20, Top: 18, Right: 20, Bottom: 12}, Spacing: 8},
		Children: []d.Widget{
			d.Label{AssignTo: &a.lblStatus, Text: "Перевіряю…",
				Font: fontOf(a.fonts.status), TextColor: colInk},
			d.Label{AssignTo: &a.lblPath, Text: "",
				Font: fontOf(a.fonts.body), TextColor: colInkDim},
			d.Label{AssignTo: &a.lblVersion, Text: "",
				Font: fontOf(a.fonts.body), TextColor: colInkDim},
			d.VSpacer{Size: 2},
			a.pb.widget(6),
			d.Label{AssignTo: &a.lblHint, Text: "", MinSize: d.Size{Height: 17},
				Font: fontOf(a.fonts.body), TextColor: colInkDim},
			d.Label{AssignTo: &a.lblHint2, Text: "", MinSize: d.Size{Height: 17},
				Font: fontOf(a.fonts.body), TextColor: colInkDim},
			d.VSpacer{},

			// Транзитний стан — те, що бачать у 95 % запусків.
			d.Composite{
				AssignTo:   &a.rowWait,
				Background: d.SolidColorBrush{Color: colVoid},
				Layout:     d.HBox{MarginsZero: true, Spacing: 8},
				Children: []d.Widget{
					d.HSpacer{},
					a.btnWait.widget(150, 34, 0),
				},
			},

			// Вибір магазину — лише коли знайдено дві установки.
			d.Composite{
				AssignTo:   &a.rowChoose,
				Visible:    false,
				Background: d.SolidColorBrush{Color: colVoid},
				Layout:     d.HBox{MarginsZero: true, Spacing: 8},
				Children: []d.Widget{
					a.btnSteam.widget(120, 34, 1),
					a.btnEpic.widget(120, 34, 1),
				},
			},

			// Панель — повний набір. З'являється тільки коли автозапуск
			// зупинено, сама собою ніколи.
			d.Composite{
				AssignTo:   &a.rowPanel,
				Visible:    false,
				Background: d.SolidColorBrush{Color: colVoid},
				Layout:     d.VBox{MarginsZero: true, Spacing: 8},
				Children: []d.Widget{
					a.btnPlay.widget(0, 40, 0),
					d.Composite{
						Background: d.SolidColorBrush{Color: colVoid},
						Layout:     d.HBox{MarginsZero: true, Spacing: 8},
						Children: []d.Widget{
							a.btnFolder.widget(0, 30, 1),
							a.btnRemove.widget(0, 30, 1),
						},
					},
				},
			},
		},
	}
}

func (a *App) footer() d.Widget {
	return d.Composite{
		Background: d.SolidColorBrush{Color: colPanel},
		MinSize:    d.Size{Height: 92},
		MaxSize:    d.Size{Height: 92},
		Layout:     d.VBox{Margins: d.Margins{Left: 20, Top: 10, Right: 20, Bottom: 10}, Spacing: 3},
		Children: []d.Widget{
			d.Label{Text: warnLine1, MinSize: d.Size{Height: 16}, Font: fontOf(a.fonts.small), TextColor: colInkDim},
			d.Label{Text: warnLine2, MinSize: d.Size{Height: 16}, Font: fontOf(a.fonts.small), TextColor: colInkDim},
			d.Composite{
				Background: d.SolidColorBrush{Color: colPanel},
				Layout:     d.HBox{MarginsZero: true, Spacing: 0},
				Children: []d.Widget{
					a.btnDiscord.widget(180, 18, 0),
					d.HSpacer{},
				},
			},
		},
	}
}

func fontOf(f *walk.Font) d.Font {
	if f == nil {
		return d.Font{}
	}
	return d.Font{Family: f.Family(), PointSize: f.PointSize(), Bold: f.Bold()}
}
