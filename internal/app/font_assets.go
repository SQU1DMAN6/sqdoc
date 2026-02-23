package app

import _ "embed"

var (
	//go:embed assets/fonts/LiberationSans-Regular.ttf
	fontSansRegular []byte
	//go:embed assets/fonts/LiberationSans-Bold.ttf
	fontSansBold []byte
	//go:embed assets/fonts/LiberationSans-Italic.ttf
	fontSansItalic []byte
	//go:embed assets/fonts/LiberationSans-BoldItalic.ttf
	fontSansBoldItalic []byte

	//go:embed assets/fonts/LiberationSerif-Regular.ttf
	fontSerifRegular []byte
	//go:embed assets/fonts/LiberationSerif-Bold.ttf
	fontSerifBold []byte
	//go:embed assets/fonts/LiberationSerif-Italic.ttf
	fontSerifItalic []byte
	//go:embed assets/fonts/LiberationSerif-BoldItalic.ttf
	fontSerifBoldItalic []byte

	//go:embed assets/fonts/LiberationMono-Regular.ttf
	fontMonoRegular []byte
	//go:embed assets/fonts/LiberationMono-Bold.ttf
	fontMonoBold []byte
	//go:embed assets/fonts/LiberationMono-Italic.ttf
	fontMonoItalic []byte
	//go:embed assets/fonts/LiberationMono-BoldItalic.ttf
	fontMonoBoldItalic []byte
)
