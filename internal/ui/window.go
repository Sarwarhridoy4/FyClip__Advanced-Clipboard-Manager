// File: internal/ui/window.go
package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/internal/clipboard"
)

// MainWindow represents the main application window
type MainWindow struct {
	window     fyne.Window
	app        fyne.App
	manager    *clipboard.Manager
	quickPanel *QuickPanel

	list       *HistoryList
	preview    *PreviewPane
	toolbar    *Toolbar
	search     *SearchBar
	status     *StatusBar
	keyHandler *KeyHandler
}

// KeyHandler is a hidden widget that captures keyboard shortcuts
type KeyHandler struct {
	widget.BaseWidget
	mw *MainWindow
}

// FocusGained is called when the widget gains focus
func (kh *KeyHandler) FocusGained() {}

// FocusLost is called when the widget loses focus
func (kh *KeyHandler) FocusLost() {
	// Re-focus immediately to keep receiving keyboard events
	if kh.mw != nil && kh.mw.window != nil {
		kh.mw.window.Canvas().Focus(kh)
	}
}

// TypedKey handles keyboard input
func (kh *KeyHandler) TypedKey(key *fyne.KeyEvent) {
	if kh.mw.quickPanel != nil && kh.mw.quickPanel.IsVisible() {
		handled := kh.mw.quickPanel.HandleKeyEvent(key)
		if handled {
			return
		}
	}

	switch key.Name {
	case fyne.KeyDown:
		kh.mw.moveSelection(1)
	case fyne.KeyUp:
		kh.mw.moveSelection(-1)
	case fyne.KeyEnter:
		if kh.mw.toolbar != nil {
			kh.mw.toolbar.onCopy()
		}
	case fyne.KeyDelete:
		if kh.mw.toolbar != nil {
			kh.mw.toolbar.onDelete()
		}
	case fyne.KeyEscape:
		if kh.mw.list != nil && kh.mw.list.IsSelectionMode() {
			kh.mw.list.SetSelectionMode(false)
			if kh.mw.toolbar != nil {
				kh.mw.toolbar.SetSelectionModeActive(false)
			}
		}
	case fyne.KeySpace:
		if kh.mw.list != nil && kh.mw.list.IsSelectionMode() {
			idx := kh.mw.manager.GetSelectedIndex()
			if idx >= 0 {
				kh.mw.list.ToggleSelection(idx)
			}
		} else if kh.mw.toolbar != nil {
			kh.mw.toolbar.onCopy()
		}
	case fyne.KeyHome:
		kh.mw.moveToTop()
	case fyne.KeyEnd:
		kh.mw.moveToBottom()
	case fyne.KeyF1:
		if kh.mw.quickPanel != nil {
			kh.mw.quickPanel.Show()
		}
	}
}

// TypedRune handles character input
func (kh *KeyHandler) TypedRune(r rune) {
	// Not used
}

// Tapped is required by Tappable interface but we don't use it
func (kh *KeyHandler) Tapped(_ *fyne.PointEvent) {}

// CreateRenderer is required by Widget interface
func (kh *KeyHandler) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(fyne.NewContainer())
}

// NewMainWindow creates a new main window
func NewMainWindow(window fyne.Window, app fyne.App, manager *clipboard.Manager) *MainWindow {
	mw := &MainWindow{
		window:  window,
		app:     app,
		manager: manager,
	}

	// Create components
	mw.list = NewHistoryList(manager, mw.onItemSelected, app, window)
	mw.preview = NewPreviewPane(manager)
	mw.toolbar = NewToolbar(window, app, manager, mw.list)
	mw.search = NewSearchBar(manager, mw.list)
	mw.status = NewStatusBar(manager)

	// Create quick panel
	mw.quickPanel = NewQuickPanel(manager, window, mw.onQuickPaste)

	return mw
}

// Build constructs the UI layout
func (mw *MainWindow) Build() fyne.CanvasObject {
	// Split between list and preview
	split := container.NewHSplit(
		mw.list.Build(),
		mw.preview.Build(),
	)
	split.SetOffset(0.5)

	// Main layout
	content := container.NewBorder(
		mw.search.Build(),
		container.NewVBox(
			mw.toolbar.Build(),
			mw.status.Build(),
		),
		nil,
		nil,
		split,
	)

	// Setup keyboard shortcuts
	mw.setupShortcuts()

	// Setup key handler for keyboard shortcuts
	mw.setupKeyHandler()

	return content
}

// setupKeyHandler creates and focuses the key handler widget
func (mw *MainWindow) setupKeyHandler() {
	kh := &KeyHandler{mw: mw}
	kh.ExtendBaseWidget(kh)
	mw.keyHandler = kh
	
	// Focus the key handler so it receives keyboard events
	mw.window.Canvas().Focus(kh)
	
	// Start a goroutine to periodically ensure focus is maintained
	// This ensures keyboard shortcuts work even after clicking on other elements
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if mw.window == nil {
				return
			}
			canvas := mw.window.Canvas()
			if canvas == nil {
				return
			}
			// Check if our key handler has focus, if not, refocus it
			currentFocused := canvas.Focused()
			if currentFocused != kh {
				defer func() { recover() }() // Ignore focus errors
				canvas.Focus(kh)
			}
		}
	}()
}

// setupShortcuts sets up the menu and keyboard shortcuts
func (mw *MainWindow) setupShortcuts() {
	// Edit menu items
	copyItem := fyne.NewMenuItem("Copy", func() {
		if mw.toolbar != nil {
			mw.toolbar.onCopy()
		}
	})

	deleteItem := fyne.NewMenuItem("Delete", func() {
		if mw.toolbar != nil {
			mw.toolbar.onDelete()
		}
	})

	searchItem := fyne.NewMenuItem("Search", func() {
		if mw.search != nil {
			mw.search.Focus(mw.window)
		}
	})

	backupItem := fyne.NewMenuItem("Backup", func() {
		if mw.toolbar != nil {
			mw.toolbar.onBackup()
		}
	})

	restoreItem := fyne.NewMenuItem("Restore", func() {
		if mw.toolbar != nil {
			mw.toolbar.onRestore()
		}
	})

	editMenu := fyne.NewMenu("Edit", copyItem, deleteItem, searchItem, backupItem, restoreItem)

	// View menu items
	quickPanelItem := fyne.NewMenuItem("Quick Paste", func() {
		if mw.quickPanel != nil {
			mw.quickPanel.Toggle()
		}
	})
	sortByDateItem := fyne.NewMenuItem("Sort by Date", func() {
		if mw.toolbar != nil {
			mw.toolbar.onToggleSortDate()
		}
	})
	viewMenu := fyne.NewMenu("View", quickPanelItem, sortByDateItem)

	// Help menu items
	featuresItem := fyne.NewMenuItem("Features", func() {
		ShowFeaturesDialog(mw.window, mw.app)
	})

	updateItem := fyne.NewMenuItem("Check for Updates", func() {
		ShowUpdateDialog(mw.window, mw.app, "2.7.0")
	})

	aboutItem := fyne.NewMenuItem("About", func() {
		ShowAboutDialog(mw.window, mw.app)
	})

	helpMenu := fyne.NewMenu("Help", featuresItem, updateItem, aboutItem)

	mainMenu := fyne.NewMainMenu(editMenu, viewMenu, helpMenu)
	mw.window.SetMainMenu(mainMenu)

	// Register keyboard shortcuts
	// Copy: Ctrl+C
	copyShortcut := &desktop.CustomShortcut{KeyName: fyne.KeyC, Modifier: fyne.KeyModifierShortcutDefault}
	mw.window.Canvas().AddShortcut(copyShortcut, func(shortcut fyne.Shortcut) {
		if mw.toolbar != nil {
			mw.toolbar.onCopy()
		}
	})

	// Delete: Delete key
	deleteShortcut := &desktop.CustomShortcut{KeyName: fyne.KeyDelete}
	mw.window.Canvas().AddShortcut(deleteShortcut, func(shortcut fyne.Shortcut) {
		if mw.toolbar != nil {
			mw.toolbar.onDelete()
		}
	})

	// Search: Ctrl+F
	searchShortcut := &desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault}
	mw.window.Canvas().AddShortcut(searchShortcut, func(shortcut fyne.Shortcut) {
		if mw.search != nil {
			mw.search.Focus(mw.window)
		}
	})

	// Backup: Ctrl+B
	backupShortcut := &desktop.CustomShortcut{KeyName: fyne.KeyB, Modifier: fyne.KeyModifierShortcutDefault}
	mw.window.Canvas().AddShortcut(backupShortcut, func(shortcut fyne.Shortcut) {
		if mw.toolbar != nil {
			mw.toolbar.onBackup()
		}
	})

	// Restore: Ctrl+R
	restoreShortcut := &desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierShortcutDefault}
	mw.window.Canvas().AddShortcut(restoreShortcut, func(shortcut fyne.Shortcut) {
		if mw.toolbar != nil {
			mw.toolbar.onRestore()
		}
	})

	// Quick Paste: F1
	quickPasteShortcut := &desktop.CustomShortcut{KeyName: fyne.KeyF1}
	mw.window.Canvas().AddShortcut(quickPasteShortcut, func(shortcut fyne.Shortcut) {
		if mw.quickPanel != nil {
			mw.quickPanel.Show()
		}
	})

	// Select All: Ctrl+A
	selectAllShortcut := &desktop.CustomShortcut{KeyName: fyne.KeyA, Modifier: fyne.KeyModifierShortcutDefault}
	mw.window.Canvas().AddShortcut(selectAllShortcut, func(shortcut fyne.Shortcut) {
		if mw.list != nil {
			mw.list.SelectAll()
			count := mw.list.GetSelectedCount()
			ShowNotification(mw.app, fmt.Sprintf("%d items selected", count))
		}
	})
}

// moveSelection moves the selection up or down
func (mw *MainWindow) moveSelection(direction int) {
	current := mw.manager.GetSelectedIndex()
	count := mw.manager.GetFilteredCount()
	if count == 0 {
		return
	}

	newIndex := current + direction
	if newIndex < 0 {
		newIndex = 0
	} else if newIndex >= count {
		newIndex = count - 1
	}

	mw.manager.SetSelected(newIndex)
	if mw.list != nil {
		mw.list.Refresh()
	}
	if mw.preview != nil {
		mw.preview.Refresh()
	}
}

// moveToTop moves selection to the first item
func (mw *MainWindow) moveToTop() {
	mw.manager.SetSelected(0)
	if mw.list != nil {
		mw.list.Refresh()
	}
	if mw.preview != nil {
		mw.preview.Refresh()
	}
}

// moveToBottom moves selection to the last item
func (mw *MainWindow) moveToBottom() {
	count := mw.manager.GetFilteredCount()
	if count > 0 {
		mw.manager.SetSelected(count - 1)
		if mw.list != nil {
			mw.list.Refresh()
		}
		if mw.preview != nil {
			mw.preview.Refresh()
		}
	}
}

// Refresh updates all UI components
func (mw *MainWindow) Refresh() {
	if mw.list != nil {
		mw.list.Refresh()
	}
	if mw.preview != nil {
		mw.preview.Refresh()
	}
	if mw.toolbar != nil {
		mw.toolbar.Refresh()
	}
	if mw.status != nil {
		mw.status.Refresh()
	}
}

// onItemSelected handles item selection
func (mw *MainWindow) onItemSelected(index int) {
	mw.manager.SetSelected(index)
	if mw.preview != nil {
		mw.preview.Refresh()
	}
}

// onQuickPaste handles quick paste from the panel
func (mw *MainWindow) onQuickPaste(item clipboard.Item) {
	// Find the item in the manager and copy it
	items := mw.manager.GetFiltered()
	for i, it := range items {
		if it.ID == item.ID {
			mw.manager.CopyToClipboard(i)
			break
		}
	}
}

// ShowQuickPanel shows the quick paste panel
func (mw *MainWindow) ShowQuickPanel() {
	if mw.quickPanel != nil {
		mw.quickPanel.Show()
	}
}
