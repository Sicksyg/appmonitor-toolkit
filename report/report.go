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

// Manager handles PDF report generation
type Manager struct {
	logger     func(message, function string)
	outputPath string
	tempPath   string
	reportPath string
}

type Paths struct {
	OutputPath string
	TempPath   string
	ReportPath string
}

// Input contains all data needed to generate a report.
type Input struct {
	ApplicationName     string
	ApplicationBundleID string
	AppStoreDescription string
	AppStoreIconPath    string
	AppStoreURL         string
	OutPath             string
	SDKMap              map[string][]string
	Permissions         []PermissionItem
}

// PermissionItem is a platform-agnostic permission representation for report rendering.
type PermissionItem struct {
	Key                  string
	DisplayName          string
	SystemDescription    string
	DeveloperDescription string
	Category             string
	SourceLabel          string
}

type newItemMap struct {
	Name  string
	Count int
}

// NewManager creates a new report Manager
func NewManager(logger func(message, function string), paths Paths) *Manager {
	return &Manager{
		logger:     logger,
		outputPath: paths.OutputPath,
		tempPath:   paths.TempPath,
		reportPath: paths.ReportPath,
	}
}

// --- Main function to generate a PDF report using Maroto v2 ---
// MakeMarotoReport generates a professional PDF report using Maroto v2
func (rm *Manager) MakeMarotoReport(input Input) error {
	sdkMap := input.SDKMap
	permissionItems := input.Permissions

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
	rm.buildHeader(mrt, input.ApplicationName, input.ApplicationBundleID, len(sdkMap))
	rm.buildFrontPage(mrt, input.ApplicationName, input.ApplicationBundleID, input.AppStoreDescription, input.AppStoreIconPath, input.AppStoreURL, sdkMap, permissionItems)
	mrt.AddPages(page.New())

	// Details pages
	rm.buildSDKSection(mrt, sdkMap)
	rm.buildPermissionsSection(mrt, permissionItems)
	rm.buildFooter(mrt)

	// Generate
	document, err := mrt.Generate()
	if err != nil {
		return fmt.Errorf("generate pdf: %w", err)
	}

	// Save
	if err := document.Save(input.OutPath); err != nil {
		return fmt.Errorf("save pdf: %w", err)
	}

	rm.logger(fmt.Sprintf("PDF written to: %s", input.OutPath), "report.Manager.MakeMarotoReport")
	return nil
}

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

func cutText(text string, maxLength int) string {
	runes := []rune(text)
	if len(runes) <= maxLength {
		return text
	}
	return string(runes[:maxLength])
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
				result.WriteString(line)
				result.WriteString("\n")
				line = word
			} else {
				result.WriteString(word)
				result.WriteString("\n")
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
				result.WriteString(line)
				result.WriteString("\n")
				line = word
			} else {
				result.WriteString(word)
				result.WriteString("\n")
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

func dynamicRowHeight(wrappedText string, minHeight int, perLine int) float64 {
	lineCount := 1
	trimmed := strings.TrimSpace(wrappedText)
	if trimmed != "" {
		lineCount = strings.Count(trimmed, "\n") + 1
	}

	height := lineCount * perLine
	if height < minHeight {
		return float64(minHeight)
	}

	return float64(height)
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

	rows, err := list.Build(items)
	if err != nil {
		return nil, names, err
	}

	return rows, names, nil
}

// IosPermissionItems converts iOS permissions into report PermissionItem entries.
func IosPermissionItems(permissionMap map[string]models.IosPermissionDetail) []PermissionItem {
	items := make([]PermissionItem, 0, len(permissionMap))
	for key, detail := range permissionMap {
		displayName := strings.TrimSpace(detail.CommonName)
		if displayName == "" {
			displayName = key
		}

		items = append(items, PermissionItem{
			Key:                  key,
			DisplayName:          displayName,
			SystemDescription:    strings.TrimSpace(detail.AppleDescription),
			DeveloperDescription: strings.TrimSpace(detail.DeveloperDescription),
			Category:             strings.TrimSpace(detail.Category),
			SourceLabel:          "Apple",
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].DisplayName) < strings.ToLower(items[j].DisplayName)
	})

	return items
}

// AndroidPermissionItems converts Android permissions into report PermissionItem entries.
func AndroidPermissionItems(permissionMap map[string]models.AndroidPermissionDetail) []PermissionItem {
	items := make([]PermissionItem, 0, len(permissionMap))
	for key, detail := range permissionMap {
		displayName := strings.TrimSpace(detail.CommonName)
		if displayName == "" {
			displayName = key
		}

		items = append(items, PermissionItem{
			Key:                  key,
			DisplayName:          displayName,
			SystemDescription:    strings.TrimSpace(detail.DescriptionSimple),
			DeveloperDescription: strings.TrimSpace(detail.DescriptionDetailed),
			Category:             strings.TrimSpace(detail.ProtectionLevel),
			SourceLabel:          "Android",
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].DisplayName) < strings.ToLower(items[j].DisplayName)
	})

	return items
}

// buildHeader registers a consistent header for each page with the application name and report title
func (rm *Manager) buildHeader(m core.Maroto, applicationName string, bundleID string, sdkCount int) {
	if err := m.RegisterHeader(
		row.New(15).Add(
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
	); err != nil {
		rm.logger(fmt.Sprintf("unable to register report header: %v", err), "report.Manager.buildHeader")
	}
}

func (rm *Manager) buildFrontPage(m core.Maroto, applicationName, bundleID string, appStoreDescription string, appStoreIconPath string, appStoreURL string, sdkMap map[string][]string, permissions []PermissionItem) {
	// Developer mod - Add border around every element for easier debugging

	//fmt.Printf("App Store Icon Path: %s\n", appStoreIconPath)

	//icon := "tmp/com.netflix.mediaclient_icon.png" // Placeholder icon path - replace with actual app icon if available

	iconCol := col.New(3).Add(
		text.New("No icon", props.Text{
			Top:   16,
			Size:  9,
			Align: align.Center,
			Color: &mediumGray,
		}),
	)
	if strings.TrimSpace(appStoreIconPath) != "" {
		iconCol = image.NewFromFileCol(3, appStoreIconPath, props.Rect{
			Center:  true,
			Percent: 80,
		})
	}

	// Icon left and text about the app on the right
	m.AddRows(
		row.New(60).Add(
			// First Column with icon
			iconCol,
			// Second Column with appStoreDescription text
			col.New(9).Add(
				text.New(applicationName, props.Text{
					Top:   8,
					Size:  18,
					Style: fontstyle.Bold,
					Color: &darkCharcoal,
				}),
				text.New(wrapTextLong(appStoreDescription, 60, 400), props.Text{
					Top:   20,
					Size:  10,
					Color: &mediumGray,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: &panelOffWhite}),

		row.New(12).Add(
			col.New(12).Add(
				text.New(appStoreURL, props.Text{
					Size:  9,
					Left:  2,
					Color: &professionalBlue,
				}),
				text.New(bundleID, props.Text{
					Top:   8,
					Size:  9,
					Left:  2,
					Color: &professionalBlue,
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
			),
			col.New(6).Add(
				text.New(fmt.Sprintf("Total Permissions: %d", len(permissions)), props.Text{
					Size:  14,
					Style: fontstyle.Bold,
					Color: &professionalBlue,
				}),
			),
		),
	)

	sdkNames := make([]string, 0, len(sdkMap))
	for sdk := range sdkMap {
		sdkNames = append(sdkNames, sdk)
	}
	sort.Strings(sdkNames)

	permissionNames := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		displayName := strings.TrimSpace(permission.DisplayName)
		if displayName == "" {
			displayName = strings.TrimSpace(permission.Key)
		}
		permissionNames = append(permissionNames, displayName)
	}
	sort.Strings(permissionNames)

	maxRows := len(sdkNames)
	if len(permissionNames) > maxRows {
		maxRows = len(permissionNames)
	}

	m.AddRow(7,
		col.New(6).Add(
			text.New("SDKs Detected:", props.Text{Size: 10, Style: fontstyle.Bold, Color: &darkCharcoal}),
		),
		col.New(6).Add(
			text.New("Permissions Detected:", props.Text{Size: 10, Style: fontstyle.Bold, Color: &darkCharcoal}),
		),
	)

	if maxRows == 0 {
		m.AddRow(7,
			col.New(6).Add(
				text.New("None", props.Text{Size: 10, Color: &mediumGray}),
			),
			col.New(6).Add(
				text.New("None", props.Text{Size: 10, Color: &mediumGray}),
			),
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
			permissionText = fmt.Sprintf("• %s", permissionNames[i])
		}

		m.AddRow(7,
			col.New(6).Add(
				text.New(sdkText, props.Text{Size: 10, Color: &mediumGray}),
			),
			col.New(6).Add(
				text.New(permissionText, props.Text{Size: 10, Color: &mediumGray}),
			),
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

	for _, sdkname := range sdkNames {
		if len(sdkMap[sdkname]) == 0 {
			continue
		}

		// Add SDK name as a header for the list of classes
		m.AddRow(6,
			col.New(12).Add(
				text.New(fmt.Sprintf("%s classes:", sdkname), props.Text{
					Size:  10,
					Style: fontstyle.Bold,
					Color: &slateBlue,
				}),
			),
		)
		// Add info on the SDK (comment, website, detail, link to documentation) from the sdkMap.
		// Should be in this format:
		// type SDKSignature struct {
		// 	Regex       string `json:"regex"`
		// 	DomainRegex string `json:"domain_regex"`
		// 	Name        string `json:"name"`
		// 	Comment     string `json:"comment"`
		// 	Detail      string `json:"detail"`
		// 	Website     string `json:"website"`
		// 	Link        string `json:"link"`
		// 	ID          int    `json:"id"`
		// }

		// Assuming the first class name can be used to retrieve SDK details if needed
		// Add each class name under the SDK name
		for _, className := range sdkMap[sdkname] {
			m.AddRow(5,
				col.New(12).Add(
					text.New(fmt.Sprintf("  - %s", cutText(className, 40)), props.Text{
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

func (rm *Manager) buildPermissionsSection(m core.Maroto, permissions []PermissionItem) {
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

	if len(permissions) == 0 {
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

	for i, permission := range permissions {
		bgColor := &props.WhiteColor
		if i%2 == 0 {
			bgColor = &veryLightGray
		}

		title := strings.TrimSpace(permission.DisplayName)
		if title == "" {
			title = strings.TrimSpace(permission.Key)
		}
		wrappedTitle := wrapText(fmt.Sprintf(" %s", title), 80)

		m.AddRow(dynamicRowHeight(wrappedTitle, 8, 5),
			col.New(12).Add(
				text.New(wrappedTitle, props.Text{
					Size:  11,
					Style: fontstyle.Bold,
					Color: &mediumGray,
					Left:  2,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: bgColor})

		if permission.SystemDescription != "" {
			label := strings.TrimSpace(permission.SourceLabel)
			if label == "" {
				label = "System"
			}
			systemText := wrapText(fmt.Sprintf("%s: %s", label, permission.SystemDescription), 100)
			m.AddRow(dynamicRowHeight(systemText, 6, 4),
				col.New(12).Add(
					text.New(systemText, props.Text{
						Size: 9,
						Left: 5,
					}),
				),
			)
		}

		if permission.DeveloperDescription != "" {
			developerText := wrapText(fmt.Sprintf("Developer: %s", permission.DeveloperDescription), 100)
			m.AddRow(dynamicRowHeight(developerText, 6, 4),
				col.New(12).Add(
					text.New(developerText, props.Text{
						Size: 9,
						Left: 5,
					}),
				),
			)
		}

		if permission.Category != "" {
			categoryText := wrapText(fmt.Sprintf("Category: %s", permission.Category), 100)
			m.AddRow(dynamicRowHeight(categoryText, 4, 4),
				col.New(12).Add(
					text.New(categoryText, props.Text{
						Size: 9,
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
