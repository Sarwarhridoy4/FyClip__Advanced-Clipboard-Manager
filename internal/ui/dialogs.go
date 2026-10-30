// File: internal/ui/dialogs.go
package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg" // Register decoder
	"image/png"
	_ "image/png" // Register decoder
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/internal/clipboard"
)

const (
	appVersion   = "2.7.0"
	appCopyright = "© 2024-2026"
)

func getContactEmail() string {
	if email := os.Getenv("FYCLIP_CONTACT_EMAIL"); email != "" {
		return email
	}
	return "contact@example.com"
}

// ShowAboutDialog displays a professional about dialog with enhanced styling
func ShowAboutDialog(window fyne.Window, app fyne.App) {
	aboutWindow := app.NewWindow("About FyClip")
	aboutWindow.Resize(fyne.NewSize(480, 620))
	aboutWindow.SetFixedSize(true)

	// Load custom app icon
	appIconResource, err := fyne.LoadResourceFromPath("internal/app/assets/icon.png")
	if err != nil || appIconResource == nil {
		appIconResource = theme.FyneLogo()
	}
	appIcon := widget.NewIcon(appIconResource)
	appIcon.Resize(fyne.NewSize(80, 80))

	// Title with larger font
	title := widget.NewLabel("FyClip")
	title.Alignment = fyne.TextAlignCenter
	title.TextStyle = fyne.TextStyle{Bold: true}

	// Tagline
	tagline := widget.NewLabel("Advanced Clipboard Manager")
	tagline.Alignment = fyne.TextAlignCenter
	tagline.TextStyle.Italic = true

	// Version info
	versionLabel := widget.NewLabel(fmt.Sprintf("Version %s", appVersion))
	versionLabel.Alignment = fyne.TextAlignCenter
	versionLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Main separator
	separator1 := widget.NewSeparator()

	// Description with better formatting
	descLabel := widget.NewLabel("A modular, high-performance clipboard manager\nbuilt with Go and Fyne.")
	descLabel.Alignment = fyne.TextAlignCenter
	descLabel.Wrapping = fyne.TextWrapWord

	// Features section with icons
	featuresTitle := widget.NewLabel("✨ Features")
	featuresTitle.Alignment = fyne.TextAlignCenter
	featuresTitle.TextStyle = fyne.TextStyle{Bold: true}

	// Create icon-based feature list
	featureList := container.NewVBox()
	featureItems := []struct {
		icon  fyne.Resource
		text string
	}{
		{theme.ContentCopyIcon(), "Clipboard History"},
		{theme.FileTextIcon(), "Rich Text/HTML Support"},
		{theme.FolderIcon(), "File Path History"},
		{theme.ConfirmIcon(), "Encrypted Storage"},
		{theme.DocumentCreateIcon(), "Snippets & Templates"},
		{theme.SearchIcon(), "Global Hotkey Quick Panel"},
	}

	for _, item := range featureItems {
		icon := widget.NewIcon(item.icon)
		icon.Resize(fyne.NewSize(16, 16))
		label := widget.NewLabel(item.text)
		row := container.NewHBox(icon, widget.NewLabel(" "), label)
		featureList.Add(row)
	}

	// Wrap features in a card
	featuresCard := widget.NewCard("", "", container.NewPadded(featureList))

	// Developer section
	separator2 := widget.NewSeparator()
	devTitle := widget.NewLabel("👨‍💻 Developer")
	devTitle.Alignment = fyne.TextAlignCenter
	devTitle.TextStyle = fyne.TextStyle{Bold: true}

	devName := widget.NewLabel("Sarwar Hossain")
	devName.Alignment = fyne.TextAlignCenter
	devName.TextStyle = fyne.TextStyle{Bold: true}

	// Contact links with icons
	githubIcon := widget.NewIcon(theme.InfoIcon())
	githubIcon.Resize(fyne.NewSize(16, 16))
	githubLink := widget.NewHyperlink("GitHub", nil)
	_ = githubLink.SetURLFromString("https://github.com/Sarwarhridoy4")
	githubLink.Alignment = fyne.TextAlignCenter

	emailIcon := widget.NewIcon(theme.MailSendIcon())
	emailIcon.Resize(fyne.NewSize(16, 16))
	emailLink := widget.NewHyperlink("Email", nil)
	_ = emailLink.SetURLFromString("mailto:" + getContactEmail())
	emailLink.Alignment = fyne.TextAlignCenter

	contactBox := container.NewHBox(
		layout.NewSpacer(),
		container.NewHBox(githubIcon, githubLink),
		widget.NewLabel("   |   "),
		container.NewHBox(emailIcon, emailLink),
		layout.NewSpacer(),
	)

	// License and copyright
	separator3 := widget.NewSeparator()
	licenseLabel := widget.NewLabel("📄 MIT License")
	licenseLabel.Alignment = fyne.TextAlignCenter
	licenseLabel.TextStyle = fyne.TextStyle{Italic: true}

	copyrightLabel := widget.NewLabel(fmt.Sprintf("© %s Sarwar Hossain. All rights reserved.", appCopyright))
	copyrightLabel.Alignment = fyne.TextAlignCenter

	// Technology stack with icons
	techTitle := widget.NewLabel("🔧 Built With")
	techTitle.Alignment = fyne.TextAlignCenter
	techTitle.TextStyle = fyne.TextStyle{Bold: true}

	techList := container.NewVBox()
	techItems := []struct {
		icon  fyne.Resource
		text string
	}{
		{theme.FileTextIcon(), "Go 1.21+"},
		{theme.FyneLogo(), "Fyne v2.7+"},
		{theme.ConfirmIcon(), "AES-256-GCM Encryption"},
	}

	for _, item := range techItems {
		icon := widget.NewIcon(item.icon)
		icon.Resize(fyne.NewSize(16, 16))
		label := widget.NewLabel(item.text)
		label.Alignment = fyne.TextAlignLeading
		row := container.NewHBox(icon, widget.NewLabel(" "), label)
		techList.Add(row)
	}

	// Wrap tech stack in a card
	techCard := widget.NewCard("", "", container.NewPadded(techList))

	// Close button
	closeBtn := widget.NewButton("Close", func() {
		aboutWindow.Close()
	})
	closeBtn.Importance = widget.HighImportance

	// Main content - organized with proper spacing
	content := container.NewVBox(
		container.NewHBox(layout.NewSpacer(), appIcon, layout.NewSpacer()),
		container.NewPadded(title),
		tagline,
		versionLabel,
		separator1,
		descLabel,
		container.NewPadded(featuresTitle),
		container.NewPadded(featuresCard),
		separator2,
		container.NewPadded(devTitle),
		devName,
		contactBox,
		separator3,
		container.NewPadded(techTitle),
		techCard,
		separator3,
		licenseLabel,
		copyrightLabel,
		container.NewPadded(closeBtn),
	)

	// Scroll container for smaller screens
	scrollContent := container.NewScroll(content)
	scrollContent.SetMinSize(fyne.NewSize(480, 620))

	aboutWindow.SetContent(scrollContent)
	aboutWindow.CenterOnScreen()
	aboutWindow.Show()
}

// ShowNotification shows a temporary notification
func ShowNotification(app fyne.App, message string) {
	if app != nil {
		app.SendNotification(&fyne.Notification{
			Title:   "FyClip",
			Content: message,
		})
	}
}

// ShowPopup displays a temporary popup message
func ShowPopup(window fyne.Window, message string, duration time.Duration) {
	if window == nil || window.Canvas() == nil {
		return
	}

	popupContent := widget.NewCard("", "", widget.NewLabel(message))
	popup := widget.NewPopUp(popupContent, window.Canvas())

	contentPos := window.Content().Position()
	popupPos := contentPos.Add(fyne.NewPos(10, 40))
	popup.Move(popupPos)
	popup.Resize(fyne.NewSize(200, 60))
	popup.Show()

	time.AfterFunc(duration, func() {
		fyne.Do(func() {
			popup.Hide()
		})
	})
}

// ShowImageFormatDialog shows a dialog to select the image format (PNG/JPEG)
func ShowImageFormatDialog(window fyne.Window, callback func(format string, err error)) {
	selectedFormat := "png" // Default to PNG

	radio := widget.NewRadioGroup([]string{"PNG", "JPEG"}, func(s string) {
		if s == "JPEG" {
			selectedFormat = "jpeg"
		} else {
			selectedFormat = "png"
		}
	})
	radio.SetSelected("PNG") // Set initial selection

	dialog.ShowCustomConfirm(
		"Select Image Format",
		"Save",
		"Cancel",
		radio,
		func(confirmed bool) {
			if confirmed {
				callback(selectedFormat, nil)
			} else {
				callback("", fmt.Errorf("format selection cancelled"))
			}
		},
		window,
	)
}

// SaveImage saves an image item to file
func SaveImage(item clipboard.Item, filename, format string) error {
	if item.Type != clipboard.TypeImage || item.ImageData == "" {
		return fmt.Errorf("no image data available")
	}

	// Decode base64 data
	data, err := base64.StdEncoding.DecodeString(item.ImageData)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Decode image
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to decode image format: %w", err)
	}

	// Create output file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Encode in requested format
	switch format {
	case "png":
		if err := png.Encode(file, img); err != nil {
			return fmt.Errorf("failed to encode PNG: %w", err)
		}
	case "jpeg", "jpg":
		if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 95}); err != nil {
			return fmt.Errorf("failed to encode JPEG: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	return nil
}

// ShowFeaturesDialog displays a professional dialog with feature descriptions
func ShowFeaturesDialog(window fyne.Window, app fyne.App) {
	featuresWindow := app.NewWindow("Features Guide")
	featuresWindow.Resize(fyne.NewSize(550, 650))
	featuresWindow.SetFixedSize(true)

	// Title with styling
	title := widget.NewLabel("📖 FyClip Features Guide")
	title.Alignment = fyne.TextAlignCenter
	title.TextStyle = fyne.TextStyle{Bold: true}

	// Main separator
	separator := widget.NewSeparator()

	// Features list with icons and descriptions
	features := []struct {
		icon        fyne.Resource
		name        string
		description string
	}{
		{
			icon:        theme.ContentCopyIcon(),
			name:        "📋 Clipboard History",
			description: "Automatically captures and stores all clipboard content (text, images, HTML, files). Access your history anytime with the global hotkey.",
		},
		{
			icon:        theme.ConfirmIcon(),
			name:        "📌 Pin/Unpin Items",
			description: "Pin important items to keep them at the top of your history. Pinned items are never automatically cleared.",
		},
		{
			icon:        theme.RadioButtonIcon(),
			name:        "⭐ Favorites Filter",
			description: "Toggle between showing all items or only pinned favorites. Quickly access your most important clipboard entries.",
		},
		{
			icon:        theme.MediaPauseIcon(),
			name:        "⏸️ Pause Monitoring",
			description: "Temporarily pause clipboard monitoring for 5 minutes. Useful when copying sensitive information you don't want stored.",
		},
		{
			icon:        theme.SearchIcon(),
			name:        "🔍 Search",
			description: "Search through your clipboard history by content. Quickly find specific items you copied earlier.",
		},
		{
			icon:        theme.FileIcon(),
			name:        "👁️ Preview Pane",
			description: "View detailed preview of selected items. Supports text, images, HTML, and file information with syntax highlighting.",
		},
		{
			icon:        theme.DocumentSaveIcon(),
			name:        "💾 Export",
			description: "Export selected items to files. Text items are saved as .txt files, images can be saved as PNG or JPEG.",
		},
		{
			icon:        theme.DocumentCreateIcon(),
			name:        "✂️ Snippets",
			description: "Create and manage text snippets for quick insertion. Use abbreviations for faster access.\n\nTemplate Variables:\n• {{date}} - Current date (YYYY-MM-DD)\n• {{time}} - Current time (HH:MM:SS)\n• {{datetime}} - Full date and time\n• {{year}} - Current year\n• {{month}} - Current month (01-12)\n• {{day}} - Current day (01-31)\n• {{clipboard}} - Current clipboard content\n\nTo use: Click Snippets button in toolbar, select a snippet, and click Use to copy to clipboard.",
		},
		{
			icon:        theme.SearchReplaceIcon(),
			name:        "⚡ Quick Panel",
			description: "Access clipboard history with a global hotkey (Ctrl+Shift+V). Paste items without switching windows.",
		},
		{
			icon:        theme.DeleteIcon(),
			name:        "🗑️ Clear History",
			description: "Clear all unpinned items from history. Pinned items are preserved for future use.",
		},
		{
			icon:        theme.ViewRefreshIcon(),
			name:        "🔄 Refresh",
			description: "Manually refresh the clipboard history list. Useful if items aren't updating automatically.",
		},
		{
			icon:        theme.DownloadIcon(),
			name:        "⬆️ Auto Update",
			description: "Check for and install application updates automatically. Access via Help > Check for Updates in the menu bar. You can also use terminal commands: 'fyclip --check-update' to check for updates and 'fyclip --update' to download and install the latest version.",
		},
	}

	// Create feature cards with icons
	var featureCardList []fyne.CanvasObject
	for _, f := range features {
		// Feature icon
		icon := widget.NewIcon(f.icon)
		icon.Resize(fyne.NewSize(18, 18))

		// Feature name
		nameLabel := widget.NewLabel(f.name)
		nameLabel.TextStyle = fyne.TextStyle{Bold: true}
		nameLabel.Wrapping = fyne.TextWrapOff

		// Feature description
		descLabel := widget.NewLabel(f.description)
		descLabel.Wrapping = fyne.TextWrapWord

		// Use grid layout for horizontal arrangement: icon | name | description
		gridContent := container.NewGridWithRows(1,
			icon,
			container.NewPadded(nameLabel),
			descLabel,
		)

		// Wrap in a card for professional look
		card := widget.NewCard("", "", container.NewPadded(gridContent))
		featureCardList = append(featureCardList, card)
	}

	// Close button
	closeBtn := widget.NewButton("Close", func() {
		featuresWindow.Close()
	})
	closeBtn.Importance = widget.HighImportance

	// Main content
	content := container.NewVBox(
		title,
		separator,
	)
	for _, card := range featureCardList {
		content.Add(card)
	}
	content.Add(container.NewPadded(closeBtn))

	// Scroll container for smaller screens
	scrollContent := container.NewScroll(content)
	scrollContent.SetMinSize(fyne.NewSize(550, 650))

	featuresWindow.SetContent(scrollContent)
	featuresWindow.CenterOnScreen()
	featuresWindow.Show()
}
