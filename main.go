// File: main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/internal/app"
	"github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/internal/update"
	"github.com/Sarwarhridoy4/FyClip---Advanced-Clipboard-Manager/internal/version"
)

// Version and BuildTime are set from internal/version package
var Version = version.Version
var BuildTime = version.BuildTime

// GitHub repository info for update checking
const (
	githubOwner = "Sarwarhridoy4"
	githubRepo  = "FyClip---Advanced-Clipboard-Manager"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Print version info if requested
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		log.Printf("FyClip version %s (built: %s)", Version, BuildTime)
		os.Exit(0)
	}

	// Print help
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		log.Printf("FyClip - Advanced Clipboard Manager")
		log.Printf("Version: %s", Version)
		log.Printf("")
		log.Printf("Usage: fyclip [options]")
		log.Printf("  --version, -v            Show version")
		log.Printf("  --check-update          Check for updates")
		log.Printf("  --update                Download and install latest update")
		log.Printf("  --help, -h              Show help")
		os.Exit(0)
	}

	// Handle update commands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--check-update":
			handleCheckUpdate()
			os.Exit(0)
		case "--update":
			handleUpdate()
			os.Exit(0)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Fatal panic: %v", r)
		}
	}()

	// Check for single instance - prevents multiple instances from running
	lock, err := app.NewSingleInstanceLock()
	if err != nil {
		fmt.Fprintln(os.Stderr, "FyClip could not start due to single instance lock:", err)
		log.Printf("Single instance lock error: %v", err)
		if strings.Contains(err.Error(), "stale") || strings.Contains(err.Error(), "lock") {
			log.Println("This appears to be a stale lockfile issue. Try manually removing the lock file.")
		}

		// Try to show a notification using platform-specific methods
		time.Sleep(500 * time.Millisecond) // Small delay to ensure environment is ready
		showNotification("FyClip could not start: " + err.Error())

		os.Exit(1)
	}
	defer lock.Release()

	application := app.New()
	if application == nil {
		fmt.Fprintln(os.Stderr, "FyClip could not create the application window.")
		log.Fatal("Failed to create application")
		os.Exit(1)
	}

	if err := application.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "FyClip exited with an application error:", err)
		log.Fatalf("Application error: %v", err)
		os.Exit(1)
	}
}

// handleCheckUpdate checks for updates and prints information
func handleCheckUpdate() {
	log.Printf("FyClip version %s (built: %s)", Version, BuildTime)
	log.Println("Checking for updates...")

	checker := update.NewChecker(githubOwner, githubRepo, Version)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	updateInfo, err := checker.CheckForUpdate(ctx)
	if err != nil {
		log.Printf("Error checking for updates: %v", err)
		log.Println("Make sure you have an internet connection.")
		os.Exit(1)
	}

	// Compare versions exactly
	if updateInfo.LatestVersion == Version {
		log.Printf("You are using the latest version: %s", Version)
	} else {
		log.Printf("Update available: %s -> %s", Version, updateInfo.LatestVersion)
		log.Printf("Release notes: %s", updateInfo.ReleaseURL)
		log.Printf("Download: %s", updateInfo.DownloadURL)
		if updateInfo.IsPrerelease {
			log.Println("Note: This is a pre-release version")
		}
		log.Println("\nTo update, run: fyclip --update")
	}
}

// handleUpdate downloads and installs the latest update
func handleUpdate() {
	log.Printf("FyClip version %s (built: %s)", Version, BuildTime)
	log.Println("Starting update process...")

	updater := update.NewAutoUpdater(githubOwner, githubRepo, Version)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second) // 5 min timeout
	defer cancel()

	// Check and download
	log.Println("Checking for updates...")
	_, downloader, err := updater.CheckAndDownload(ctx)
	if err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}

	if downloader == nil {
		log.Printf("You are already using the latest version: %s", Version)
		os.Exit(0)
	}

	log.Printf("Downloaded: %s", downloader.GetDownloadPath())

	// Install
	log.Println("Installing update...")
	installer := update.NewInstaller(downloader.GetDownloadPath(), "FyClip", downloader.ExpectedHash())
	if err := installer.Install(); err != nil {
		log.Printf("Installation error: %v", err)
		output := installer.GetOutput()
		if output != "" {
			log.Printf("Installation output:\n%s", output)
		}
		log.Println("You may need to install manually.")
		os.Exit(1)
	}

	output := installer.GetOutput()
	if output != "" {
		log.Printf("Installation output:\n%s", output)
	}
	log.Println("Update installed successfully!")
	log.Println("Please restart FyClip to use the new version.")
}

// sanitizeShellInput removes potentially dangerous characters from shell input
func sanitizeShellInput(input string) string {
	// Remove or escape shell metacharacters
	// Keep only alphanumeric, spaces, and basic punctuation
	reg := regexp.MustCompile(`[^\w\s\.,!?\-:]`)
	return reg.ReplaceAllString(input, "")
}

// validateCommandArgs validates command arguments for security
func validateCommandArgs(args []string) error {
	for _, arg := range args {
		// Check for directory traversal attempts
		if strings.Contains(arg, "..") || strings.Contains(arg, "/") && strings.Contains(arg, "..") {
			return fmt.Errorf("invalid command argument: %s", arg)
		}
		// Check for shell injection attempts
		if strings.Contains(arg, ";") || strings.Contains(arg, "|") || strings.Contains(arg, "&") ||
			strings.Contains(arg, "`") || strings.Contains(arg, "$(") {
			return fmt.Errorf("potentially dangerous command argument: %s", arg)
		}
	}
	return nil
}

// showNotification shows a notification to the user
// This is used when another instance is already running
func showNotification(message string) {
	// Sanitize the message to prevent command injection
	safeMessage := sanitizeShellInput(message)
	if safeMessage == "" {
		safeMessage = "FyClip is already running"
	}

	switch runtime.GOOS {
	case "windows":
		// Use PowerShell to show a toast notification on Windows
		script := fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$template = '<visual><binding template="ToastText02"><text id="1">FyClip</text><text id="2">%s</text></binding></visual>'
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("FyClip").Show($toast)`, safeMessage)

		tmpFile, err := os.CreateTemp("", "fyclip-notify-*.ps1")
		if err != nil {
			log.Printf("Failed to create temp script: %v", err)
			return
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(script); err != nil {
			log.Printf("Failed to write notification script: %v", err)
			return
		}
		if err := tmpFile.Close(); err != nil {
			log.Printf("Failed to close notification script: %v", err)
			return
		}

		cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", tmpFile.Name())
		cmd.Run()

	case "darwin":
		// Use osascript to show a notification on macOS
		cmd := exec.Command("osascript", "-e",
			fmt.Sprintf(`display notification "%s" with title "FyClip"`, safeMessage))
		if err := validateCommandArgs(cmd.Args); err != nil {
			log.Printf("Command validation failed: %v", err)
			return
		}
		cmd.Run()

	default: // linux
		// Try shownotification first
		cmd := exec.Command("shownotification", "-u", "critical", "-t", "3000", "FyClip", safeMessage)
		if err := validateCommandArgs(cmd.Args); err != nil {
			log.Printf("Command validation failed: %v", err)
			return
		}

		// Set a timeout to prevent hanging
		timer := time.AfterFunc(2*time.Second, func() {
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
		})
		defer timer.Stop()
		if err := cmd.Run(); err != nil {
			log.Printf("shownotification failed: %v, trying zenity", err)
			// Try zenity as fallback
			cmd := exec.Command("zenity", "--info", "--text", safeMessage, "--title=FyClip")
			if err := validateCommandArgs(cmd.Args); err != nil {
				log.Printf("Command validation failed: %v", err)
				return
			}
			if err := cmd.Run(); err != nil {
				log.Printf("zenity failed: %v", err)
			}
		}
	}
}
