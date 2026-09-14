//go:build windows

package main

import (
	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
)

// Рідні кнопки Windows на майже чорному тлі виглядають сірими латками, тому
// всі кнопки малюємо самі. Наведення миші не відстежуємо: walk не віддає
// WM_MOUSELEAVE, а «залипла» підсвітка гірша за її відсутність.
type flatButton struct {
	cw        *walk.CustomWidget
	text      string
	fill      walk.Color
	fillDown  walk.Color
	textColor walk.Color
	border    bool
	font      *walk.Font
	pressed   bool
	disabled  bool
	onClick   func()
}

func (b *flatButton) SetText(s string) {
	b.text = s
	if b.cw != nil {
		b.cw.Invalidate()
	}
}

func (b *flatButton) SetDisabled(v bool) {
	b.disabled = v
	if b.cw != nil {
		b.cw.Invalidate()
	}
}

func (b *flatButton) SetVisible(v bool) {
	if b.cw != nil {
		b.cw.SetVisible(v)
	}
}

func (b *flatButton) paint(canvas *walk.Canvas, _ walk.Rectangle) error {
	r := widgetBounds(b.cw)

	fill := b.fill
	if b.pressed && !b.disabled {
		fill = b.fillDown
	}
	br, err := walk.NewSolidColorBrush(fill)
	if err != nil {
		return err
	}
	defer br.Dispose()
	if err := canvas.FillRectangle(br, r); err != nil {
		return err
	}

	if b.border {
		pen, err := walk.NewCosmeticPen(walk.PenSolid, colLine)
		if err != nil {
			return err
		}
		defer pen.Dispose()
		if err := canvas.DrawRectangle(pen, walk.Rectangle{
			X: r.X, Y: r.Y, Width: r.Width - 1, Height: r.Height - 1,
		}); err != nil {
			return err
		}
	}

	col := b.textColor
	if b.disabled {
		col = colInkDim
	}
	return canvas.DrawText(b.text, b.font, col, r,
		walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextNoPrefix)
}

func (b *flatButton) widget(minW, minH int, stretch int) d.CustomWidget {
	return d.CustomWidget{
		AssignTo:            &b.cw,
		ToolTipText:         b.text,
		MinSize:             d.Size{Width: minW, Height: minH},
		StretchFactor:       stretch,
		ClearsBackground:    false,
		InvalidatesOnResize: true,
		PaintMode:           d.PaintNoErase,
		Paint:               b.paint,
		OnMouseDown: func(x, y int, button walk.MouseButton) {
			if b.disabled || button != walk.LeftButton {
				return
			}
			b.pressed = true
			b.cw.Invalidate()
		},
		OnMouseUp: func(x, y int, button walk.MouseButton) {
			if !b.pressed {
				return
			}
			b.pressed = false
			b.cw.Invalidate()
			if b.disabled {
				return
			}
			// Натиснув і відвів курсор геть — це скасування, не клік.
			cb := b.cw.ClientBoundsPixels()
			if x < 0 || y < 0 || x >= cb.Width || y >= cb.Height {
				return
			}
			if b.onClick != nil {
				b.onClick()
			}
		},
	}
}

// Межі самого віджета в тих самих одиницях (1/96"), у яких walk віддає
// координати в Paint.
//
// НЕ canvas.Bounds(): walk рахує їх через GetDeviceCaps(HORZRES/VERTRES),
// а це розмір екрана, а не віджета — і для DC вікна, і для буфера. Заливка
// такої помилки не показує, бо зайве обрізається по віджету. Текст показує
// одразу: TextCenter центрує його по екрану, тобто за межами віджета, і
// кнопка виглядає порожньою.
func widgetBounds(cw *walk.CustomWidget) walk.Rectangle {
	if cw == nil {
		return walk.Rectangle{}
	}
	b := cw.ClientBounds()
	b.X, b.Y = 0, 0
	return b
}

// ---------- смужка ----------

// Золота — щось змінюється (завантаження, заміна файлу).
// Синя — іде відлік до запуску.
// Плутати два стани одним кольором не можна: у першому переривати
// небезпечно, у другому саме для цього й дається час.
type bar struct {
	cw    *walk.CustomWidget
	value float64 // 0..1
	color walk.Color
	shown bool
}

func (b *bar) Set(value float64, color walk.Color) {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	b.value, b.color, b.shown = value, color, true
	if b.cw != nil {
		b.cw.Invalidate()
	}
}

func (b *bar) Hide() {
	b.shown = false
	if b.cw != nil {
		b.cw.Invalidate()
	}
}

func (b *bar) paint(canvas *walk.Canvas, _ walk.Rectangle) error {
	r := widgetBounds(b.cw)

	track, err := walk.NewSolidColorBrush(colPanel2)
	if err != nil {
		return err
	}
	defer track.Dispose()
	if err := canvas.FillRectangle(track, r); err != nil {
		return err
	}
	if !b.shown || b.value <= 0 {
		return nil
	}

	fill, err := walk.NewSolidColorBrush(b.color)
	if err != nil {
		return err
	}
	defer fill.Dispose()
	w := int(float64(r.Width) * b.value)
	if w < 1 {
		w = 1
	}
	return canvas.FillRectangle(fill, walk.Rectangle{X: r.X, Y: r.Y, Width: w, Height: r.Height})
}

func (b *bar) widget(height int) d.CustomWidget {
	return d.CustomWidget{
		AssignTo:            &b.cw,
		MinSize:             d.Size{Height: height},
		MaxSize:             d.Size{Height: height},
		ClearsBackground:    false,
		InvalidatesOnResize: true,
		PaintMode:           d.PaintNoErase,
		Paint:               b.paint,
	}
}
