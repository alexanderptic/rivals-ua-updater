//go:build windows

package main

import "github.com/lxn/walk"

// Кольори зняті з marvel-rivals-ua.netlify.app 13.09.
// walk дає рідні віджети Windows, тож це наближення, а не копія сайту:
// ні жорсткої тіні без розмиття, ні Oswald тут не буде. Свідомий обмін на
// відсутність залежності від WebView2.
var (
	colVoid      = walk.RGB(0x0a, 0x0a, 0x10) // тло вікна
	colPanel     = walk.RGB(0x15, 0x13, 0x19) // картка
	colPanel2    = walk.RGB(0x1c, 0x19, 0x22) // поля
	colLine      = walk.RGB(0x31, 0x2c, 0x39) // межі
	colAccent    = walk.RGB(0x2f, 0x6f, 0xed) // «Грати», посилання
	colAccentDim = walk.RGB(0x17, 0x3d, 0x94) // натиснутий стан
	colGold      = walk.RGB(0xff, 0xcc, 0x00) // акцент, індикатор оновлення
	colInk       = walk.RGB(0xf3, 0xef, 0xe6) // текст
	colInkDim    = walk.RGB(0xa9, 0xa3, 0xb4) // другорядний текст
)

// Шрифти сайту — Oswald і Exo 2. Постачати TTF і реєструвати через
// AddFontResourceEx заради двох написів не варто, тому системні відповідники:
// Bahnschrift (є у Windows 10+, вузький гротеск, близький до Oswald) для
// заголовків, Segoe UI для решти.
const (
	fontHead = "Bahnschrift"
	fontBody = "Segoe UI"
)

type fonts struct {
	title  *walk.Font
	status *walk.Font
	body   *walk.Font
	small  *walk.Font
	button *walk.Font
}

func loadFonts() *fonts {
	mk := func(family string, size int, style walk.FontStyle) *walk.Font {
		f, err := walk.NewFont(family, size, style)
		if err != nil {
			// Bahnschrift немає на Windows 8 і старіших — не привід падати.
			f, err = walk.NewFont(fontBody, size, style)
			if err != nil {
				return nil
			}
		}
		return f
	}
	return &fonts{
		title:  mk(fontHead, 14, walk.FontBold),
		status: mk(fontHead, 13, 0),
		body:   mk(fontBody, 9, 0),
		small:  mk(fontBody, 8, 0),
		button: mk(fontHead, 11, 0),
	}
}
