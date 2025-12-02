package ui

import "image/color"

// Theme colors - these are variables so they can be modified for dark mode
var (
	colWhite     = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	colBlack     = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	colGray      = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	colLightGray = color.NRGBA{R: 200, G: 200, B: 200, A: 255}
	colDirBlue   = color.NRGBA{R: 0, G: 0, B: 128, A: 255}
	colSelected  = color.NRGBA{R: 200, G: 220, B: 255, A: 255}
	colSidebar   = color.NRGBA{R: 245, G: 245, B: 245, A: 255}
	colDisabled  = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
	colProgress  = color.NRGBA{R: 66, G: 133, B: 244, A: 255}
	colDanger    = color.NRGBA{R: 220, G: 53, B: 69, A: 255}
	colHomeBtnBg = color.NRGBA{R: 76, G: 175, B: 80, A: 255}
	colDriveIcon = color.NRGBA{R: 96, G: 125, B: 139, A: 255}
	colSuccess   = color.NRGBA{R: 40, G: 167, B: 69, A: 255}
	colAccent    = color.NRGBA{R: 66, G: 133, B: 244, A: 255}
	colDirective = color.NRGBA{R: 103, G: 58, B: 183, A: 255}   // Purple for directives
	colDirectiveBg = color.NRGBA{R: 237, G: 231, B: 246, A: 255} // Light purple background
	// Config error banner colors
	colErrorBannerBg   = color.NRGBA{R: 220, G: 53, B: 69, A: 255}   // Red background
	colErrorBannerText = color.NRGBA{R: 139, G: 69, B: 0, A: 255}    // Dark orange text (readable on red)
	// UI Polish colors
	colShadow          = color.NRGBA{R: 0, G: 0, B: 0, A: 60}        // Modal shadow (deeper)
	colShadowOuter     = color.NRGBA{R: 0, G: 0, B: 0, A: 25}        // Outer shadow layer
	colBackdrop        = color.NRGBA{R: 0, G: 0, B: 0, A: 180}       // Modal backdrop (darker for contrast)
	colCodeBlockBg     = color.NRGBA{R: 245, G: 245, B: 245, A: 255} // Code block background
	colCodeBlockBorder = color.NRGBA{R: 220, G: 220, B: 220, A: 255} // Code block border
	colBlockquoteBg    = color.NRGBA{R: 248, G: 248, B: 248, A: 255} // Blockquote background
	colBlockquoteLine  = color.NRGBA{R: 180, G: 180, B: 180, A: 255} // Blockquote left border
	colPrimaryBtn      = color.NRGBA{R: 66, G: 133, B: 244, A: 255}  // Primary button (blue)
	colPrimaryBtnText  = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // Primary button text
	colDangerBtn       = color.NRGBA{R: 220, G: 53, B: 69, A: 255}   // Danger button (red)
	colDangerBtnText   = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // Danger button text
)
