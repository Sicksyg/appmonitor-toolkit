package report

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/page"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"AppMonitor/models"
)

// Color definitions for the report - Professional office theme
var (
	// Neutral grays and professional blue
	darkCharcoal     = props.Color{Red: 60, Green: 60, Blue: 60}    // Main text and emphasis
	mediumGray       = props.Color{Red: 100, Green: 100, Blue: 100} // Secondary elements
	lightGray        = props.Color{Red: 220, Green: 220, Blue: 220} // Subtle backgrounds
	veryLightGray    = props.Color{Red: 240, Green: 240, Blue: 240} // Row alternation
	professionalBlue = props.Color{Red: 70, Green: 110, Blue: 160}  // Headers and accents
	slateBlue        = props.Color{Red: 100, Green: 130, Blue: 170} // Secondary accents
	charcoalDivider  = props.Color{Red: 80, Green: 80, Blue: 80}    // Dividers
	cardBeige        = props.Color{Red: 208, Green: 178, Blue: 145} // Front-page stat card
	panelOffWhite    = props.Color{Red: 246, Green: 243, Blue: 239} // Front-page content panel
)

// Manager handles PDF report generation
type Manager struct {
	logger func(message, function string)
}

// NewManager creates a new report Manager
func NewManager(logger func(message, function string)) *Manager {
	return &Manager{
		logger: logger,
	}
}

// wrapText wraps text to fit within a specified width (character limit)
func wrapText(text string, maxCharsPerLine int) string {
	if len(text) <= maxCharsPerLine {
		return text
	}

	var result strings.Builder
	words := strings.Fields(text)
	var line string

	for _, word := range words {
		if len(line)+len(word)+1 > maxCharsPerLine {
			if line != "" {
				result.WriteString(line + "\n")
				line = word
			} else {
				result.WriteString(word + "\n")
				line = ""
			}
		} else {
			if line == "" {
				line = word
			} else {
				line += " " + word
			}
		}
	}

	if line != "" {
		result.WriteString(line)
	}

	return result.String()
}

// wrapTextLong wraps text and truncates it if it exceeds a maximum length, adding "..." at the end. Maybe change to lines in future if we want to preserve more of the text instead of just truncating at a character limit.
func wrapTextLong(text string, maxCharsPerLine int, maxLength int) string {
	if len(text) <= maxCharsPerLine {
		return text
	}

	if maxLength > 0 {
		runes := []rune(text)
		if len(runes) > maxLength {
			cut := maxLength

			// Move cut back to previous whitespace so we do not split a word.
			for cut > 0 && !unicode.IsSpace(runes[cut-1]) {
				cut--
			}

			// Fallback: if no whitespace found, hard-cut at maxLength.
			if cut == 0 {
				cut = maxLength
			}

			text = strings.TrimSpace(string(runes[:cut])) + "..."
		}
	}

	var result strings.Builder
	words := strings.Fields(text)
	var line string

	for _, word := range words {
		if len(line)+len(word)+1 > maxCharsPerLine {
			if line != "" {
				result.WriteString(line + "\n")
				line = word
			} else {
				result.WriteString(word + "\n")
				line = ""
			}
		} else {
			if line == "" {
				line = word
			} else {
				line += " " + word
			}
		}
	}

	if line != "" {
		result.WriteString(line)
	}

	return result.String()
}

// MakeMarotoReport generates a professional PDF report using Maroto v2
func (rm *Manager) MakeMarotoReport(applicationName, applicationBundleID, appStoreDescription string, outPath string, sdkMap map[string][]string, permissionMap map[string]models.PermissionDetail) error {
	// Create config
	cfg := config.NewBuilder().
		WithDimensions(210, 297).
		WithLeftMargin(20).
		WithTopMargin(15).
		WithRightMargin(20).
		Build()

	// Create maroto
	mrt := maroto.New(cfg)

	// Build documents

	// Front page
	rm.buildHeader(mrt, applicationName)
	rm.buildFrontPage(mrt, applicationName, applicationBundleID, appStoreDescription, len(sdkMap), len(permissionMap))
	mrt.AddPages(page.New())

	// Details pages
	rm.buildHeader(mrt, applicationName)
	rm.buildInfo(mrt, applicationBundleID, len(sdkMap))
	rm.buildSDKSection(mrt, sdkMap)
	rm.buildPermissionsSection(mrt, permissionMap)
	rm.buildFooter(mrt)

	// Generate
	document, err := mrt.Generate()
	if err != nil {
		return fmt.Errorf("generate pdf: %w", err)
	}

	// Save
	if err := document.Save(outPath); err != nil {
		return fmt.Errorf("save pdf: %w", err)
	}

	rm.logger(fmt.Sprintf("PDF written to: %s", outPath), "report.Manager.MakeMarotoReport")
	return nil
}

func (rm *Manager) buildFrontPage(m core.Maroto, applicationName, bundleID string, appStoreDescription string, sdkCount, permissionCount int) {
	// Developer mod - Add border around every element for easier debugging

	icon := "./testicon.png" // Placeholder icon path - replace with actual app icon if available

	// Icon left and text about the app on the right
	m.AddRows(
		row.New(40).Add(
			// First Column with icon
			image.NewFromFileCol(3, icon, props.Rect{
				Center:  true,
				Percent: 80,
			}),
			// Second Column with appStoreDescription text
			col.New(9).Add(
				text.New(applicationName, props.Text{
					Top:   8,
					Size:  18,
					Style: fontstyle.Bold,
					Color: &darkCharcoal,
				}),
				text.New(wrapTextLong(appStoreDescription, 60, 300), props.Text{
					Top:   20,
					Size:  10,
					Color: &mediumGray,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: &panelOffWhite}),

		row.New(5),
		row.New(1).WithStyle(&props.Cell{BackgroundColor: &charcoalDivider}),
		row.New(5),
	)
	// Two columns with stats - total sdks as int and then a list of the sdks, and then total permissions as int and a list of the permissions.
	m.AddRows(
		row.New(20).Add(
			// SDK Column
			col.New(6).Add(
				text.New(fmt.Sprintf("Total SDKs: %d", sdkCount), props.Text{
					Size:  14,
					Style: fontstyle.Bold,
					Color: &professionalBlue,
				}),
				text.New("SDKs Detected:", props.Text{
					Size:  12,
					Style: fontstyle.Bold,
					Color: &mediumGray,
					Top:   10,
				}),
				// List SDKs with bullet points and without classnames

			),
			// Permissions Column
			col.New(6).Add(
				text.New(fmt.Sprintf("Total Permissions: %d", permissionCount), props.Text{
					Size:  14,
					Style: fontstyle.Bold,
					Color: &professionalBlue,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: &cardBeige}),
	)
}

func (rm *Manager) buildHeader(m core.Maroto, applicationName string) {
	m.AddRows(
		row.New(20).Add(
			col.New(12).Add(
				text.New(fmt.Sprintf("AppMonitor Analysis Report: %s", applicationName), props.Text{
					Top:   6,
					Size:  16,
					Style: fontstyle.Bold,
					Align: align.Center,
					Color: &props.WhiteColor,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: &professionalBlue}),
		row.New(2),
	)
}

func (rm *Manager) buildInfo(m core.Maroto, bundleID string, sdkCount int) {
	m.AddRows(
		row.New(8).Add(
			col.New(6).Add(
				text.New(fmt.Sprintf("Total SDKs: %d", sdkCount), props.Text{
					Size:  11,
					Style: fontstyle.Bold,
				}),
			),
			col.New(6).Add(
				text.New(fmt.Sprintf("Bundle ID: %s", bundleID), props.Text{
					Size:  10,
					Align: align.Right,
					Style: fontstyle.Bold,
				}),
			),
		),
		row.New(2),
		row.New(1).WithStyle(&props.Cell{BackgroundColor: &slateBlue}),
		row.New(3),
	)
}

func (rm *Manager) buildSDKSection(m core.Maroto, sdkMap map[string][]string) {
	if len(sdkMap) == 0 {
		m.AddRow(15,
			col.New(12).Add(
				text.New("No SDKs detected", props.Text{
					Size:  12,
					Align: align.Center,
					Color: &mediumGray,
				}),
			),
		)
		return
	}

	sdkNames := make([]string, 0, len(sdkMap))
	for sdk := range sdkMap {
		sdkNames = append(sdkNames, sdk)
	}
	sort.Strings(sdkNames)

	for i, sdk := range sdkNames {
		bgColor := &props.WhiteColor
		if i%2 == 0 {
			bgColor = &veryLightGray
		}

		m.AddRow(8,
			col.New(12).Add(
				text.New(fmt.Sprintf("● %s", sdk), props.Text{
					Size:  12,
					Style: fontstyle.Bold,
					Color: &mediumGray,
					Left:  2,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: bgColor})

		for _, className := range sdkMap[sdk] {
			m.AddRow(5,
				col.New(12).Add(
					text.New(fmt.Sprintf("   → %s", className), props.Text{
						Size: 9,
						Left: 5,
					}),
				),
			)
		}

		m.AddRow(2)
	}
}

func (rm *Manager) buildPermissionsSection(m core.Maroto, permissionMap map[string]models.PermissionDetail) {
	// Add section header
	m.AddRows(
		row.New(5),
		row.New(12).Add(
			col.New(12).Add(
				text.New("Permissions Analysis", props.Text{
					Size:  14,
					Style: fontstyle.Bold,
					Color: &professionalBlue,
				}),
			),
		),
		row.New(1).WithStyle(&props.Cell{BackgroundColor: &slateBlue}),
		row.New(2),
	)

	if len(permissionMap) == 0 {
		m.AddRow(15,
			col.New(12).Add(
				text.New("No permissions detected", props.Text{
					Size:  12,
					Align: align.Center,
					Color: &mediumGray,
				}),
			),
		)
		return
	}

	permissionNames := make([]string, 0, len(permissionMap))
	for permission := range permissionMap {
		permissionNames = append(permissionNames, permission)
	}
	sort.Strings(permissionNames)

	for i, permission := range permissionNames {
		detail := permissionMap[permission]
		bgColor := &props.WhiteColor
		if i%2 == 0 {
			bgColor = &veryLightGray
		}

		m.AddRow(8,
			col.New(12).Add(
				text.New(fmt.Sprintf(" %s", detail.CommonName), props.Text{
					Size:  11,
					Style: fontstyle.Bold,
					Color: &mediumGray,
					Left:  2,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: bgColor})

		m.AddRows(
			row.New(6).Add(
				col.New(12).Add(
					text.New(wrapText(fmt.Sprintf("Apple: %s", detail.AppleDescription), 100), props.Text{
						Size: 9,
						Left: 5,
					}),
				),
			),
			row.New(6).Add(
				col.New(12).Add(
					text.New(wrapText(fmt.Sprintf("Developer: %s", detail.DeveloperDescription), 100), props.Text{
						Size: 9,
						Left: 5,
					}),
				),
			),
		)

		if detail.Category != "" {
			m.AddRow(4,
				col.New(12).Add(
					text.New(fmt.Sprintf("Category: %s", detail.Category), props.Text{
						Size: 8,
						Left: 5,
					}),
				),
			)
		}

		m.AddRow(2)
	}
}

func (rm *Manager) buildFooter(m core.Maroto) {
	m.AddRows(
		row.New(5),
		row.New(1).WithStyle(&props.Cell{BackgroundColor: &charcoalDivider}),
		row.New(8).Add(
			col.New(12).Add(
				text.New("Generated by AppMonitor © 2026", props.Text{
					Size:  8,
					Align: align.Center,
				}),
			),
		),
	)
}
