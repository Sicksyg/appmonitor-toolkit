package report

import (
	_ "embed"

	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core/entity"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
)

// Embedded Go fonts: a clean humanist sans for text and a monospace for code-like evidence.
const (
	fontSans = "gosans"
	fontMono = "gomono"
	// PT Serif (SIL OFL, see fonts/OFL.txt) gives headings a formal, document-like tone.
	fontSerif = "ptserif"
)

//go:embed fonts/PT_Serif-Web-Regular.ttf
var ptSerifRegular []byte

//go:embed fonts/PT_Serif-Web-Bold.ttf
var ptSerifBold []byte

//go:embed fonts/PT_Serif-Web-Italic.ttf
var ptSerifItalic []byte

func reportFonts() []*entity.CustomFont {
	return []*entity.CustomFont{
		{Family: fontSans, Style: fontstyle.Normal, Bytes: goregular.TTF},
		{Family: fontSans, Style: fontstyle.Bold, Bytes: gobold.TTF},
		{Family: fontSans, Style: fontstyle.Italic, Bytes: goitalic.TTF},
		{Family: fontSans, Style: fontstyle.BoldItalic, Bytes: gobolditalic.TTF},
		{Family: fontSerif, Style: fontstyle.Normal, Bytes: ptSerifRegular},
		{Family: fontSerif, Style: fontstyle.Bold, Bytes: ptSerifBold},
		{Family: fontSerif, Style: fontstyle.Italic, Bytes: ptSerifItalic},
		{Family: fontMono, Style: fontstyle.Normal, Bytes: gomono.TTF},
		{Family: fontMono, Style: fontstyle.Bold, Bytes: gomonobold.TTF},
	}
}
