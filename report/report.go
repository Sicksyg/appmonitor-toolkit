package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
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
	AnalysisDate        string
	AppStoreDescription string
	AppStoreIconPath    string
	AppStoreURL         string
	OutPath             string
	SDKMap              map[string][]string
	SDKDetails          map[string]models.IosSDKDetail
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
		WithLeftMargin(22).
		WithTopMargin(16).
		WithRightMargin(22).
		WithBottomMargin(18).
		WithCustomFonts(reportFonts()).
		WithDefaultFont(&props.Font{Family: fontSans, Style: fontstyle.Normal, Size: 11, Color: &darkCharcoal}).
		WithPageNumber(props.PageNumber{Pattern: "Page {current} of {total}", Place: props.RightBottom, Size: 9, Color: &mediumGray}).
		Build()

	// Create maroto
	mrt := maroto.New(cfg)

	// Build documents

	// Front page
	rm.buildHeader(mrt, input.ApplicationName, input.ApplicationBundleID, len(sdkMap))
	rm.buildFrontPage(mrt, input.ApplicationName, input.ApplicationBundleID, input.AnalysisDate, input.AppStoreDescription, input.AppStoreIconPath, input.AppStoreURL, sdkMap, permissionItems)
	mrt.AddPages(page.New())

	// Details pages
	rm.buildSDKSection(mrt, sdkMap, input.SDKDetails)
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

// Report palette: restrained navy, blue, and neutral grays.
var (
	darkCharcoal     = props.Color{Red: 35, Green: 49, Blue: 63}
	mediumGray       = props.Color{Red: 89, Green: 101, Blue: 112}
	veryLightGray    = props.Color{Red: 244, Green: 247, Blue: 249}
	professionalBlue = props.Color{Red: 36, Green: 73, Blue: 108}
	paleBlue         = props.Color{Red: 224, Green: 232, Blue: 241}
	slateBlue        = props.Color{Red: 79, Green: 111, Blue: 139}
	charcoalDivider  = props.Color{Red: 190, Green: 201, Blue: 210}
	panelOffWhite    = props.Color{Red: 249, Green: 250, Blue: 251}
	linkBlue         = props.Color{Red: 42, Green: 91, Blue: 136}
)

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

// hardWrap breaks text at a fixed rune count; used for unbroken strings such as class names and regexes.
func hardWrap(text string, width int) string {
	runes := []rune(text)
	if width <= 0 || len(runes) <= width {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(runes); i += width {
		if i > 0 {
			b.WriteString("\n")
		}
		end := i + width
		if end > len(runes) {
			end = len(runes)
		}
		b.WriteString(string(runes[i:end]))
	}
	return b.String()
}

// multiLine renders pre-wrapped text as one text component per line. Maroto does not honour
// "\n" inside a string, so each line is placed explicitly.
func multiLine(value string, base props.Text) []core.Component {
	lineHeight := base.Size
	if lineHeight == 0 {
		lineHeight = 9
	}
	lineHeight *= 0.48
	parts := strings.Split(value, "\n")
	components := make([]core.Component, 0, len(parts))
	for i, part := range parts {
		lineProps := base
		lineProps.Top = base.Top + float64(i)*lineHeight
		components = append(components, text.New(part, lineProps))
	}
	return components
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
		row.New(16).Add(
			col.New(12).Add(
				text.New(fmt.Sprintf("AppMonitor Analysis Report: %s", applicationName), props.Text{
					Top:    5.5,
					Size:   15,
					Family: fontSerif,
					Style:  fontstyle.Bold,
					Left:   4,
					Color:  &props.WhiteColor,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: &professionalBlue}),
		row.New(9).Add(
			col.New(6).Add(
				text.New(fmt.Sprintf("%d SDKs detected", sdkCount), props.Text{
					Top:   2.5,
					Left:  4,
					Size:  10,
					Color: &mediumGray,
				}),
			),
			col.New(6).Add(
				text.New(fmt.Sprintf("Bundle ID: %s", bundleID), props.Text{
					Top:   2.5,
					Right: 4,
					Size:  10,
					Align: align.Right,
					Color: &mediumGray,
				}),
			),
		),
		row.New(3).WithStyle(&props.Cell{BackgroundColor: &professionalBlue}),
	); err != nil {
		rm.logger(fmt.Sprintf("unable to register report header: %v", err), "report.Manager.buildHeader")
	}
}

func (rm *Manager) buildFrontPage(m core.Maroto, applicationName, bundleID, analysisDate string, appStoreDescription string, appStoreIconPath string, appStoreURL string, sdkMap map[string][]string, permissions []PermissionItem) {
	iconCol := col.New(3).Add(
		text.New("No icon", props.Text{
			Top:   16,
			Size:  11,
			Align: align.Center,
			Color: &mediumGray,
		}),
	)
	if strings.TrimSpace(appStoreIconPath) != "" {
		iconCol = image.NewFromFileCol(3, appStoreIconPath, props.Rect{
			Center:  true,
			Percent: 72,
		})
	}

	// Long names wrap onto at most two lines; the description starts below however many were used.
	const nameSize = 22.0
	nameLines := multiLine(wrapTextLong(applicationName, 28, 56), props.Text{
		Top:    8,
		Size:   nameSize,
		Family: fontSerif,
		Left:   2,
		Style:  fontstyle.Bold,
		Color:  &darkCharcoal,
	})
	descriptionTop := 8 + float64(len(nameLines))*nameSize*0.48 + 3

	// Icon left and text about the app on the right
	m.AddRows(
		row.New(70).Add(
			iconCol,
			col.New(9).Add(append(nameLines, multiLine(wrapTextLong(appStoreDescription, 54, 300), props.Text{
				Top:   descriptionTop,
				Left:  2,
				Right: 4,
				Size:  11,
				Color: &mediumGray,
			})...)...),
		).WithStyle(&props.Cell{BackgroundColor: &panelOffWhite}),

		row.New(10).Add(
			col.New(7).Add(
				text.New(appStoreURL, props.Text{
					Top:       2.5,
					Size:      10,
					Left:      4,
					Color:     &linkBlue,
					Hyperlink: &appStoreURL,
				}),
			),
			col.New(5).Add(
				text.New(bundleID, props.Text{
					Top:   2.5,
					Right: 4,
					Size:  10,
					Align: align.Right,
					Color: &mediumGray,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: &panelOffWhite}),
		row.New(10).Add(
			col.New(12).Add(
				text.New("ANALYSIS TIME  "+formatAnalysisDate(analysisDate), props.Text{
					Top:   1.5,
					Size:  10,
					Style: fontstyle.Bold,
					Left:  4,
					Color: &professionalBlue,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: &panelOffWhite}),

		row.New(10),
	)
	m.AddRows(
		row.New(22).Add(
			col.New(6).Add(
				text.New(fmt.Sprintf("%d", len(sdkMap)), props.Text{
					Top:    3,
					Left:   4,
					Size:   22,
					Family: fontSerif,
					Style:  fontstyle.Bold,
					Color:  &professionalBlue,
				}),
				text.New("Detected SDKs", props.Text{
					Top:   12.5,
					Left:  4,
					Size:  10,
					Color: &mediumGray,
				}),
			),
			col.New(6).Add(
				text.New(fmt.Sprintf("%d", len(permissions)), props.Text{
					Top:    3,
					Left:   4,
					Size:   22,
					Family: fontSerif,
					Style:  fontstyle.Bold,
					Color:  &professionalBlue,
				}),
				text.New("Declared permissions", props.Text{
					Top:   12.5,
					Left:  4,
					Size:  10,
					Color: &mediumGray,
				}),
			),
		).WithStyle(&props.Cell{BackgroundColor: &veryLightGray}),
		row.New(10),
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

	m.AddRow(9,
		col.New(6).Add(
			text.New("DETECTED SDKs", props.Text{Top: 1.5, Left: 4, Size: 11, Style: fontstyle.Bold, Color: &professionalBlue}),
		),
		col.New(6).Add(
			text.New("DECLARED PERMISSIONS", props.Text{Top: 1.5, Left: 4, Size: 11, Style: fontstyle.Bold, Color: &professionalBlue}),
		),
	)
	m.AddRow(1).WithStyle(&props.Cell{BackgroundColor: &charcoalDivider})

	if maxRows == 0 {
		m.AddRow(8,
			col.New(6).Add(
				text.New("None", props.Text{Top: 2, Left: 4, Size: 11, Color: &mediumGray}),
			),
			col.New(6).Add(
				text.New("None", props.Text{Top: 2, Left: 4, Size: 11, Color: &mediumGray}),
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

		rowStyle := &props.Cell{BackgroundColor: &props.WhiteColor}
		if i%2 == 0 {
			rowStyle = &props.Cell{BackgroundColor: &veryLightGray}
		}
		m.AddRow(8,
			col.New(6).Add(
				text.New(sdkText, props.Text{Top: 2, Left: 4, Size: 11, Color: &darkCharcoal}),
			),
			col.New(6).Add(
				text.New(permissionText, props.Text{Top: 2, Left: 4, Size: 11, Color: &darkCharcoal}),
			),
		).WithStyle(rowStyle)
	}
}

func (rm *Manager) buildSDKSection(m core.Maroto, sdkMap map[string][]string, sdkDetails map[string]models.IosSDKDetail) {
	rm.addSectionTitle(m, "SDK analysis", "Matched signatures and class-level evidence")
	if len(sdkMap) == 0 {
		m.AddRow(15,
			col.New(12).Add(
				text.New("No SDKs detected", props.Text{
					Size:   14,
					Family: fontSerif,
					Align:  align.Center,
					Color:  &mediumGray,
				}),
			),
		)
		return
	}

	sdkNames := make([]string, 0, len(sdkMap))
	for name := range sdkMap {
		sdkNames = append(sdkNames, name)
	}
	sort.Strings(sdkNames)
	for _, sdkname := range sdkNames {
		detail := sdkDetails[sdkname]
		m.AddRows(
			row.New(11).Add(
				col.New(9).Add(
					text.New(sdkname, props.Text{
						Top:    2.5,
						Size:   14,
						Family: fontSerif,
						Style:  fontstyle.Bold,
						Left:   4,
						Color:  &professionalBlue,
					}),
				),
				col.New(3).Add(
					text.New(fmt.Sprintf("%d classes", len(sdkMap[sdkname])), props.Text{
						Top:   3.5,
						Size:  10,
						Align: align.Right,
						Right: 4,
						Color: &mediumGray,
					}),
				),
			).WithStyle(&props.Cell{BackgroundColor: &paleBlue}),
			row.New(0.8).WithStyle(&props.Cell{BackgroundColor: &professionalBlue}),
			row.New(3),
		)

		rm.addSDKMetadata(m, "Description", detail.Comment)
		rm.addSDKMetadata(m, "Ownership", detail.Detail)
		rm.addSDKLink(m, "Website", detail.Website)
		rm.addSDKLink(m, "Documentation", detail.Link)
		rm.addSDKMetadata(m, "Class signature", detail.Regex)
		rm.addSDKMetadata(m, "Domain signature", detail.DomainRegex)
		if detail.ID != 0 {
			rm.addSDKMetadata(m, "Signature ID", fmt.Sprintf("%d", detail.ID))
		}

		m.AddRow(3)
		m.AddRow(9,
			col.New(12).Add(
				text.New("MATCHING CLASSES", props.Text{
					Top:   3,
					Size:  10,
					Style: fontstyle.Bold,
					Left:  4,
					Color: &slateBlue,
				}),
			),
		)
		for i, className := range sdkMap[sdkname] {
			wrappedClass := hardWrap(className, 64)
			classRow := row.New(dynamicRowHeight(wrappedClass, 8, 5)).Add(
				col.New(1).Add(
					text.New("•", props.Text{Top: 1.5, Size: 11, Style: fontstyle.Bold, Align: align.Right, Right: 1, Color: &professionalBlue}),
				),
				col.New(11).Add(
					multiLine(wrappedClass, props.Text{Top: 1.5, Left: 1, Family: fontMono, Size: 10, Color: &darkCharcoal})...,
				),
			)
			if i%2 == 0 {
				classRow.WithStyle(&props.Cell{BackgroundColor: &veryLightGray})
			}
			m.AddRows(classRow)
		}
		m.AddRow(12)
	}
}

func (rm *Manager) addSectionTitle(m core.Maroto, title, subtitle string) {
	m.AddRows(
		row.New(6),
		row.New(19).Add(
			col.New(12).Add(
				text.New(title, props.Text{
					Size:   19,
					Family: fontSerif,
					Style:  fontstyle.Bold,
					Color:  &professionalBlue,
				}),
				text.New(subtitle, props.Text{
					Top:   8.5,
					Size:  10,
					Color: &mediumGray,
				}),
			),
		),
		row.New(1).WithStyle(&props.Cell{BackgroundColor: &slateBlue}),
		row.New(5),
	)
}

func (rm *Manager) addSDKMetadata(m core.Maroto, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	valueProps := props.Text{Top: 1.5, Size: 11, Color: &darkCharcoal}
	wrapped := wrapText(value, 54)
	if strings.Contains(strings.ToLower(label), "signature") {
		wrapped = hardWrap(value, 52)
		valueProps.Family = fontMono
		valueProps.Size = 10
	}
	m.AddRow(dynamicRowHeight(wrapped, 8, 6)+3,
		col.New(3).Add(
			text.New(label, props.Text{
				Top:   2,
				Size:  9,
				Style: fontstyle.Bold,
				Left:  4,
				Color: &slateBlue,
			}),
		),
		col.New(9).Add(
			multiLine(wrapped, valueProps)...,
		),
	).WithStyle(&props.Cell{BackgroundColor: &panelOffWhite})
}

func (rm *Manager) addSDKLink(m core.Maroto, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	hyperlink := value
	if !strings.HasPrefix(hyperlink, "https://") && !strings.HasPrefix(hyperlink, "http://") {
		hyperlink = "https://" + hyperlink
	}
	if strings.Contains(value, ",") {
		hyperlink = ""
	}
	wrapped := wrapText(value, 54)
	linkText := props.Text{
		Top:   1.5,
		Size:  10,
		Color: &linkBlue,
	}
	if hyperlink != "" {
		linkText.Hyperlink = &hyperlink
	}
	m.AddRow(dynamicRowHeight(wrapped, 8, 6)+3,
		col.New(3).Add(
			text.New(label, props.Text{
				Top:   2,
				Size:  9,
				Style: fontstyle.Bold,
				Left:  4,
				Color: &slateBlue,
			}),
		),
		col.New(9).Add(
			multiLine(wrapped, linkText)...,
		),
	).WithStyle(&props.Cell{BackgroundColor: &panelOffWhite})
}

func formatAnalysisDate(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format("02 Jan 2006, 15:04:05 UTC")
}

func (rm *Manager) buildPermissionsSection(m core.Maroto, permissions []PermissionItem) {
	rm.addSectionTitle(m, "Permissions analysis", "Declared app capabilities and their descriptions")

	if len(permissions) == 0 {
		m.AddRow(15,
			col.New(12).Add(
				text.New("No permissions detected", props.Text{
					Size:   14,
					Family: fontSerif,
					Align:  align.Center,
					Color:  &mediumGray,
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
		wrappedTitle := wrapText(fmt.Sprintf(" %s", title), 60)

		m.AddRow(dynamicRowHeight(wrappedTitle, 10, 6)+3,
			col.New(12).Add(
				multiLine(wrappedTitle, props.Text{
					Top:    2.5,
					Size:   13,
					Family: fontSerif,
					Style:  fontstyle.Bold,
					Color:  &professionalBlue,
					Left:   4,
				})...,
			),
		).WithStyle(&props.Cell{BackgroundColor: bgColor})

		if permission.SystemDescription != "" {
			label := strings.TrimSpace(permission.SourceLabel)
			if label == "" {
				label = "System"
			}
			systemText := wrapText(fmt.Sprintf("%s: %s", label, permission.SystemDescription), 76)
			m.AddRow(dynamicRowHeight(systemText, 8, 6)+2,
				col.New(12).Add(
					multiLine(systemText, props.Text{
						Top:   1,
						Size:  11,
						Left:  6,
						Color: &darkCharcoal,
					})...,
				),
			)
		}

		if permission.DeveloperDescription != "" {
			developerText := wrapText(fmt.Sprintf("Developer: %s", permission.DeveloperDescription), 76)
			m.AddRow(dynamicRowHeight(developerText, 8, 6)+2,
				col.New(12).Add(
					multiLine(developerText, props.Text{
						Top:   1,
						Size:  11,
						Left:  6,
						Color: &mediumGray,
					})...,
				),
			)
		}

		if permission.Category != "" {
			categoryText := wrapText(fmt.Sprintf("Category: %s", permission.Category), 76)
			m.AddRow(dynamicRowHeight(categoryText, 7, 5)+2,
				col.New(12).Add(
					multiLine(categoryText, props.Text{
						Top:   1,
						Size:  10,
						Left:  6,
						Style: fontstyle.Italic,
						Color: &slateBlue,
					})...,
				),
			)
		}

		m.AddRow(5)
	}
}

func (rm *Manager) buildFooter(m core.Maroto) {
	m.AddRows(
		row.New(10),
		row.New(1).WithStyle(&props.Cell{BackgroundColor: &charcoalDivider}),
		row.New(9).Add(
			col.New(6).Add(
				text.New("AppMonitor  |  Analysis report", props.Text{
					Top:   3,
					Size:  9,
					Left:  4,
					Color: &mediumGray,
				}),
			),
			col.New(6).Add(
				text.New("Generated for research and review", props.Text{
					Top:   3,
					Size:  9,
					Align: align.Right,
					Right: 4,
					Color: &mediumGray,
				}),
			),
		),
	)
}
