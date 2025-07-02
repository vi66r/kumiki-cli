package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/AlecAivazis/survey/v2"
)

// ------------------------------- embedded templates --------------------------

//go:embed templates/project.yml.tmpl
var projectYML string

//go:embed templates/Info.plist.tmpl
var infoPlist string

//go:embed templates/SwiftUIApp.swift.tmpl
var swiftuiAppFile string

//go:embed templates/UIKitAppDelegate.swift.tmpl
var uikitAppFile string

// ------------------------------- capabilities map ---------------------------

var capabilities = map[string]string{
	"Location":             "com.apple.location.wheninuse",
	"Camera":               "com.apple.developer.avfoundation.camera",
	"Microphone":           "com.apple.developer.avfoundation.microphone",
	"Photos Library":       "com.apple.developer.photos.add",
	"Push Notifications":   "aps-environment",
	"HealthKit":            "com.apple.developer.healthkit",
	"App Groups":           "com.apple.security.application-groups",
	"iCloud":               "com.apple.developer.icloud-services",
	"In-App Purchase":      "com.apple.InAppPurchase",
	"Sign in with Apple":   "com.apple.developer.applesignin",
}

// ------------------------------- answers model ------------------------------

type answers struct {
	ProjName        string
	BundleID        string
	DisplayName     string
	UIStack         string
	Capabilities    []string
	UsePulse        bool
	UseShuttle      bool
	DeployTarget    string
	RemoteURL       string
	DefaultBranch   string
	CreateSwiftData bool
	TeamID          string
}

// ------------------------------- main entry ---------------------------------

func main() {
	ans, err := ask()
	if err != nil {
		log.Fatal(err)
	}

	if err := scaffold(ans); err != nil {
		log.Fatalf("❌  %v", err)
	}

	fmt.Println("🚀  Done! Opening Xcode…")
	exec.Command("open", ans.ProjName+".xcodeproj").Run()
}

// ------------------------------- survey wizard ------------------------------

func ask() (*answers, error) {
	var a answers
	u, _ := user.Current()

	qs := []*survey.Question{
		{
			Name:     "ProjName",
			Prompt:   &survey.Input{Message: "Project name:"},
			Validate: survey.Required,
		},
	}

	if err := survey.Ask(qs, &a); err != nil {
		return nil, err
	}

	defBundle := fmt.Sprintf("com.%s.%s",
		strings.ToLower(strings.ReplaceAll(u.Username, " ", "")),
		strings.ToLower(a.ProjName))

	qs = []*survey.Question{
		{
			Name:   "BundleID",
			Prompt: &survey.Input{Message: "Bundle identifier:", Default: defBundle},
		},
		{
			Name:   "DisplayName",
			Prompt: &survey.Input{Message: "Display name:", Default: a.ProjName},
		},
		{
			Name: "UIStack",
			Prompt: &survey.Select{
				Message: "Choose UI stack:",
				Options: []string{"SwiftUI", "UIKit"},
				Default: "SwiftUI",
			},
		},
		{
			Name: "Capabilities",
			Prompt: &survey.MultiSelect{
				Message: "Capabilities (space = toggle, arrows = move):",
				Options: mapKeys(capabilities),
			},
		},
		{
			Name:   "UsePulse",
			Prompt: &survey.Confirm{Message: "Add Kumiki Networking (Pulse-powered)?", Default: true},
		},
	}

	if err := survey.Ask(qs, &a); err != nil {
		return nil, err
	}

	if !a.UsePulse {
		survey.AskOne(&survey.Confirm{
			Message: "Add Kumiki Concurrency helpers (Shuttle)?",
			Default: true,
		}, &a.UseShuttle)
	} else {
		a.UseShuttle = true
	}

	qs = []*survey.Question{
		{
			Name:   "DeployTarget",
			Prompt: &survey.Input{Message: "Deployment target:", Default: "17.0"},
		},
		{
			Name:   "RemoteURL",
			Prompt: &survey.Input{Message: "Remote git repo URL (optional):"},
		},
		{
			Name:   "DefaultBranch",
			Prompt: &survey.Input{Message: "Default git branch:", Default: "main"},
		},
		{
			Name:   "CreateSwiftData",
			Prompt: &survey.Confirm{Message: "Create SwiftData model template?", Default: false},
		},
		{
			Name:   "TeamID",
			Prompt: &survey.Input{Message: "Apple Team ID (optional):"},
		},
	}

	if err := survey.Ask(qs, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// ------------------------------- scaffold -----------------------------------

func scaffold(a *answers) error {
	// create project dir
	if err := os.MkdirAll(a.ProjName+"/App", 0o755); err != nil {
		return err
	}
	if err := os.Chdir(a.ProjName); err != nil {
		return err
	}

	// render templates
	ctx := map[string]any{
		"ProjName":     a.ProjName,
		"BundleID":     a.BundleID,
		"DisplayName":  a.DisplayName,
		"DeployTarget": a.DeployTarget,
		"TeamID":       a.TeamID,
		"UsePulse":     a.UsePulse,
		"UseShuttle":   a.UseShuttle,
	}

	if err := render("project.yml", projectYML, ctx); err != nil {
		return err
	}
	if err := render("Info.plist", infoPlist, ctx); err != nil {
		return err
	}

	// write entitlements
	if err := writeEntitlements(a); err != nil {
		return err
	}

	// main source file
	var src string
	if a.UIStack == "SwiftUI" {
		src = swiftuiAppFile
	} else {
		src = uikitAppFile
	}
	if err := os.WriteFile(filepath.Join("App", a.ProjName+".swift"),
		[]byte(replaceVars(src, ctx)), 0o644); err != nil {
		return err
	}

	// git init
	if err := initGit(a); err != nil {
		return err
	}

	// xcodegen
	if _, err := exec.LookPath("xcodegen"); err != nil {
		return fmt.Errorf("xcodegen not installed (run install.sh first)")
	}
	if out, err := exec.Command("xcodegen").CombinedOutput(); err != nil {
		return fmt.Errorf("xcodegen: %v\n%s", err, out)
	}

	return nil
}

// ------------------------------- helpers ------------------------------------

func render(outPath, tmpl string, data any) error {
	out := bytes.Buffer{}
	if err := template.Must(template.New("").Parse(tmpl)).Execute(&out, data); err != nil {
		return err
	}
	return os.WriteFile(outPath, out.Bytes(), 0o644)
}

func writeEntitlements(a *answers) error {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
`)
	for _, cap := range a.Capabilities {
		buf.WriteString("  <key>" + capabilities[cap] + "</key><true/>\n")
	}
	buf.WriteString("</dict></plist>")
	return os.WriteFile(a.ProjName+".entitlements", buf.Bytes(), 0o644)
}

func initGit(a *answers) error {
	if _, err := exec.Command("git", "init").Output(); err != nil {
		return err
	}

	// .gitignore
	ignoreURL := "https://raw.githubusercontent.com/github/gitignore/main/Swift.gitignore"
	ignore, _ := exec.Command("curl", "-sSL", ignoreURL).Output()
	_ = os.WriteFile(".gitignore", ignore, 0o644)

	if err := exec.Command("git", "checkout", "-b", a.DefaultBranch).Run(); err != nil {
		return err
	}
	if err := exec.Command("git", "add", ".").Run(); err != nil {
		return err
	}
	msg := fmt.Sprintf("chore: initial scaffold %s (%s)", a.ProjName, time.Now().Format("2006-01-02"))
	if err := exec.Command("git", "commit", "-m", msg).Run(); err != nil {
		return err
	}
	if a.RemoteURL != "" {
		_ = exec.Command("git", "remote", "add", "origin", a.RemoteURL).Run()
	}
	return nil
}

func mapKeys[M ~map[string]string](m M) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func replaceVars(t string, ctx map[string]any) string {
	for k, v := range ctx {
		t = strings.ReplaceAll(t, "{{."+k+"}}", fmt.Sprint(v))
	}
	return t
}
