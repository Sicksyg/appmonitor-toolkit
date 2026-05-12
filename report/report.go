package report

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/list"
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

type newItemMap struct {
	Name  string
	Count int
}

func (s newItemMap) GetHeader() core.Row {
	return row.New(8).Add(
		text.NewCol(9, "SDK", props.Text{Style: fontstyle.Bold, Color: &darkCharcoal}),
		text.NewCol(3, "Matches", props.Text{Style: fontstyle.Bold, Align: align.Right, Color: &darkCharcoal}),
	).WithStyle(&props.Cell{BackgroundColor: &lightGray})
}

func (s newItemMap) GetContent(i int) core.Row {
	r := row.New(7).Add(
		text.NewCol(9, s.Name, props.Text{Size: 10, Color: &mediumGray}),
		text.NewCol(3, fmt.Sprintf("%d", s.Count), props.Text{Size: 10, Align: align.Right, Color: &mediumGray}),
	)

	if i%2 == 0 {
		r.WithStyle(&props.Cell{BackgroundColor: &veryLightGray})
	}

	return r
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

// Function to build SDK and Permissions list for front page summary and details section. Returns rows, sorted names, and error if any.
func (rm *Manager) rowBuilder(itemMap map[string][]string) ([]core.Row, []string, error) {
	if len(itemMap) == 0 {
		return []core.Row{
			row.New(7).Add(
				text.NewCol(12, "None", props.Text{Size: 11, Color: &mediumGray}),
			),
		}, nil, nil
	}

	names := make([]string, 0, len(itemMap))
	for item := range itemMap {
		names = append(names, item)
	}
	sort.Strings(names)

	items := make([]newItemMap, 0, len(names))
	for _, item := range names {
		items = append(items, newItemMap{
			Name:  item,
			Count: len(itemMap[item]),
		})
	}

	rows, err := list.Build[newItemMap](items)
	if err != nil {
		return nil, names, err
	}

	return rows, names, nil
}

// MakeMarotoReport generates a professional PDF report using Maroto v2
func (rm *Manager) MakeMarotoReport(applicationName, applicationBundleID, appStoreDescription, appStoreIcon, appStoreURL, outPath string, sdkMap map[string][]string, permissionMap map[string]models.PermissionDetail) error {
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
	rm.buildHeader(mrt, applicationName, applicationBundleID, len(sdkMap))
	rm.buildFrontPage(mrt, applicationName, applicationBundleID, appStoreDescription, appStoreIcon, appStoreURL, sdkMap, permissionMap)
	mrt.AddPages(page.New())

	// Details pages
	rm.buildHeader(mrt, applicationName, applicationBundleID, len(sdkMap))
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

// buildHeader creates a consistent header for each page with the application name and report title
func (rm *Manager) buildHeader(m core.Maroto, applicationName string, bundleID string, sdkCount int) {
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
	)
}

func (rm *Manager) buildFrontPage(m core.Maroto, applicationName, bundleID string, appStoreDescription string, appStoreIcon string, appStoreURL string, sdkMap map[string][]string, permissionMap map[string]models.PermissionDetail) {
	// Developer mod - Add border around every element for easier debugging

	//icon := "./testicon.png" // Placeholder icon path - replace with actual app icon if available

	// Icon left and text about the app on the right
	m.AddRows(
		row.New(40).Add(
			// First Column with icon
			image.NewFromFileCol(3, appStoreIcon, props.Rect{
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
	// Summary row with total SDKs and permissions detected, followed by a list of detected SDKs with counts
	m.AddRows(
		row.New(10).Add(
			col.New(6).Add(
				text.New(fmt.Sprintf("Total SDKs: %d", len(sdkMap)), props.Text{
					Size:  14,
					Style: fontstyle.Bold,
					Color: &professionalBlue,
				}),
			).WithStyle(&props.Cell{BackgroundColor: &cardBeige}),
			col.New(6).Add(
				text.New(fmt.Sprintf("Total Permissions: %d", len(permissionMap)), props.Text{
					Size:  14,
					Style: fontstyle.Bold,
					Color: &professionalBlue,
				}),
			).WithStyle(&props.Cell{BackgroundColor: &cardBeige}),
		),
	)

	sdkNames := make([]string, 0, len(sdkMap))
	for sdk := range sdkMap {
		sdkNames = append(sdkNames, sdk)
	}
	sort.Strings(sdkNames)

	permissionNames := make([]string, 0, len(permissionMap))
	for permission := range permissionMap {
		permissionNames = append(permissionNames, permission)
	}
	sort.Strings(permissionNames)

	maxRows := len(sdkNames)
	if len(permissionNames) > maxRows {
		maxRows = len(permissionNames)
	}

	m.AddRow(7,
		col.New(6).Add(
			text.New("SDKs Detected:", props.Text{Size: 10, Style: fontstyle.Bold, Color: &darkCharcoal}),
		).WithStyle(&props.Cell{BackgroundColor: &cardBeige}),
		col.New(6).Add(
			text.New("Permissions Detected:", props.Text{Size: 10, Style: fontstyle.Bold, Color: &darkCharcoal}),
		).WithStyle(&props.Cell{BackgroundColor: &cardBeige}),
	)

	if maxRows == 0 {
		m.AddRow(7,
			col.New(6).Add(
				text.New("None", props.Text{Size: 10, Color: &mediumGray}),
			).WithStyle(&props.Cell{BackgroundColor: &cardBeige}),
			col.New(6).Add(
				text.New("None", props.Text{Size: 10, Color: &mediumGray}),
			).WithStyle(&props.Cell{BackgroundColor: &cardBeige}),
		)
		return
	}

	for i := 0; i < maxRows; i++ {
		sdkText := ""
		if i < len(sdkNames) {
			sdkName := sdkNames[i]
			sdkText = fmt.Sprintf("• %s (%d matches)", sdkName, len(sdkMap[sdkName]))
		}

		permissionText := ""
		if i < len(permissionNames) {
			permissionName := permissionNames[i]
			commonName := strings.TrimSpace(permissionMap[permissionName].CommonName)
			if commonName == "" {
				commonName = permissionName
			}
			permissionText = fmt.Sprintf("• %s", commonName)
		}

		m.AddRow(7,
			col.New(6).Add(
				text.New(sdkText, props.Text{Size: 10, Color: &mediumGray}),
			).WithStyle(&props.Cell{BackgroundColor: &cardBeige}),
			col.New(6).Add(
				text.New(permissionText, props.Text{Size: 10, Color: &mediumGray}),
			).WithStyle(&props.Cell{BackgroundColor: &cardBeige}),
		)
	}
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

	rows, sdkNames, err := rm.rowBuilder(sdkMap)
	if err != nil {
		rm.logger(fmt.Sprintf("unable to build SDK list rows: %v", err), "report.Manager.buildSDKSection")
		for _, sdk := range sdkNames {
			m.AddRow(7,
				col.New(12).Add(
					text.New(fmt.Sprintf("• %s (%d matches)", sdk, len(sdkMap[sdk])), props.Text{Size: 11, Color: &mediumGray}),
				),
			)
		}
	} else {
		m.AddRows(rows...)
	}

	m.AddRow(3)

	for _, sdk := range sdkNames {
		if len(sdkMap[sdk]) == 0 {
			continue
		}

		m.AddRow(6,
			col.New(12).Add(
				text.New(fmt.Sprintf("%s classes:", sdk), props.Text{
					Size:  10,
					Style: fontstyle.Bold,
					Color: &slateBlue,
				}),
			),
		)

		for _, className := range sdkMap[sdk] {
			m.AddRow(5,
				col.New(12).Add(
					text.New(fmt.Sprintf("  - %s", className), props.Text{
						Size:  8,
						Left:  4,
						Color: &mediumGray,
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
