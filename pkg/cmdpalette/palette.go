package cmdpalette

//go:generate go tool templ generate

import "strings"

// Item is one row in the palette (SSR; client filters on Keywords).
type Item struct {
	Group    string // small section label above the row (optional)
	Label    string // primary text (required)
	Href     string // navigation target (required)
	Keywords string // lowercase haystack; filled from Group+Label if empty
}

// Palette is a configured palette instance. Methods return templ components.
type Palette struct {
	// ID is the DOM id prefix (default "cmdpalette"). Button is {ID}-open, etc.
	ID string
	// Items is the SSR catalog.
	Items []Item
	// Title is the trigger button label (default "Jump").
	Title string
	// Placeholder is the search field placeholder.
	Placeholder string
	// AriaLabel labels the dialog (default "Jump to").
	AriaLabel string
	// ButtonClass is optional host styling for the trigger (e.g. daisyUI btn classes).
	ButtonClass string
	// ShowHotkey shows a Ctrl+K / ⌘K hint on the button (default true).
	ShowHotkey bool
}

// New builds a palette with defaults and normalized items.
func New(items []Item) Palette {
	p := Palette{
		ID:          "cmdpalette",
		Title:       "Jump",
		Placeholder: "Jump to…",
		AriaLabel:   "Jump to",
		ShowHotkey:  true,
	}
	return p.WithItems(items)
}

// WithItems replaces items (normalized).
func (p Palette) WithItems(items []Item) Palette {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if n, ok := Normalize(it); ok {
			out = append(out, n)
		}
	}
	p.Items = out
	return p
}

// WithID sets the DOM id prefix.
func (p Palette) WithID(id string) Palette {
	if id != "" {
		p.ID = id
	}
	return p
}

// WithTitle sets the trigger label.
func (p Palette) WithTitle(title string) Palette {
	if title != "" {
		p.Title = title
	}
	return p
}

// WithPlaceholder sets the search placeholder.
func (p Palette) WithPlaceholder(s string) Palette {
	if s != "" {
		p.Placeholder = s
	}
	return p
}

// WithButtonClass sets host CSS classes on the trigger.
func (p Palette) WithButtonClass(class string) Palette {
	p.ButtonClass = class
	return p
}

// ItemOf builds an Item; Keywords defaults to group+label (+ extras).
func ItemOf(group, label, href string, extraKeywords ...string) Item {
	parts := make([]string, 0, 2+len(extraKeywords))
	parts = append(parts, group, label)
	parts = append(parts, extraKeywords...)
	return Item{
		Group:    group,
		Label:    label,
		Href:     href,
		Keywords: Keywords(parts...),
	}
}

// Normalize fills Keywords when empty and drops unusable rows.
func Normalize(it Item) (Item, bool) {
	if it.Href == "" || it.Label == "" {
		return Item{}, false
	}
	if it.Keywords == "" {
		it.Keywords = Keywords(it.Group, it.Label)
	}
	return it, true
}

// Keywords joins parts into a lowercase filter haystack.
// Colon-separated segments are also indexed as space-separated tokens.
func Keywords(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strings.ToLower(p))
	}
	s := b.String()
	if strings.Contains(s, ":") {
		s = s + " " + strings.ReplaceAll(s, ":", " ")
	}
	return s
}

func (p Palette) rootID() string {
	if p.ID == "" {
		return "cmdpalette"
	}
	return p.ID
}

func (p Palette) openID() string   { return p.rootID() + "-open" }
func (p Palette) inputID() string  { return p.rootID() + "-input" }
func (p Palette) listID() string   { return p.rootID() + "-list" }
func (p Palette) emptyID() string  { return p.rootID() + "-empty" }
func (p Palette) hotkeyID() string { return p.rootID() + "-hotkey" }
func (p Palette) titleText() string {
	if p.Title == "" {
		return "Jump"
	}
	return p.Title
}
func (p Palette) placeholderText() string {
	if p.Placeholder == "" {
		return "Jump to…"
	}
	return p.Placeholder
}
func (p Palette) ariaLabelText() string {
	if p.AriaLabel == "" {
		return "Jump to"
	}
	return p.AriaLabel
}
func (p Palette) buttonClass() string {
	if p.ButtonClass != "" {
		return p.ButtonClass
	}
	return "cmdpalette-btn"
}
