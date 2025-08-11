package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	pathToTemplates = "templates"
	AppName         = "goHttpServer"
	AppVersion      = "v0.0.1"
	initCallMsg     = "INITIAL CALL TO %s()\n"
)

// PageData holds data passed to templates, including the current theme.
type PageData struct {
	Title     string
	Theme     string // "light" or "dark"
	MainBlock string
}

// getThemeFromCookie retrieves the theme from the cookie or defaults to "light".
func getThemeFromCookie(r *http.Request) string {
	cookie, err := r.Cookie("theme")
	if err != nil || (cookie.Value != "light" && cookie.Value != "dark") {
		return "light"
	}
	return cookie.Value
}

// handleSetTheme sets the theme cookie and redirects back to the referrer.
func handleSetTheme(w http.ResponseWriter, r *http.Request) {
	theme := r.FormValue("theme")
	if theme != "light" && theme != "dark" {
		theme = "light"
	}
	http.SetCookie(w, &http.Cookie{Name: "theme", Value: theme, Path: "/"})
	// Redirect back to the page the user came from.
	referer := r.Referer()
	if referer == "" {
		referer = "/"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

func renderLayoutTemplate(name, layout, page string, l *log.Logger) (*template.Template, error) {
	l.Printf("in renderLayoutTemplate(layout:%s, page:%s)", layout, page)
	// Initialize templates
	templates := template.New("base_layout") //should match the name of the define

	// Add custom functions if any
	templates.Funcs(template.FuncMap{
		"replace": strings.ReplaceAll,
	})

	// Charger le layout + la page
	_, err := templates.ParseFiles(
		filepath.Join(pathToTemplates, "header.gohtml"),
		filepath.Join(pathToTemplates, "footer.gohtml"),
		filepath.Join(pathToTemplates, layout),
		filepath.Join(pathToTemplates, page),
	)
	if err != nil {
		l.Printf(" renderLayoutTemplate parse error : %v", err)
		return nil, err
	}
	// Log all defined templates
	l.Println(templates.DefinedTemplates())
	return templates, err
}

// getIndexHandler functions for each page (similar structure).
func getHandler(handlerName, layout, page string, l *log.Logger) http.HandlerFunc {
	l.Printf(initCallMsg, handlerName)
	myTemplate, err := renderLayoutTemplate(handlerName, layout, page, l)
	if err != nil {
		l.Fatalf("💥💥 fatal error in renderLayoutTemplate err: %v ", err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		data := PageData{Title: handlerName, Theme: getThemeFromCookie(r)}
		err = myTemplate.Execute(w, data)
		if err != nil {
			l.Printf("💥💥 fatal error in template execution err: %v ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func main() {
	l := log.New(os.Stderr, AppName, log.Ldate|log.Ltime|log.Lshortfile)
	l.Printf("🚀🚀 Starting App: %s, versio: %s", AppName, AppVersion)

	// Define HTTP handlers for each page.
	http.Handle("/", getHandler("Main", "base_layout.gohtml", "index.gohtml", l))
	http.HandleFunc("/product1", getHandler("Product 1", "base_layout.gohtml", "product1.gohtml", l))
	http.HandleFunc("/product2", getHandler("Product 2", "base_layout.gohtml", "product2.gohtml", l))
	http.HandleFunc("/contact", getHandler("Contact", "base_layout.gohtml", "contact.gohtml", l))
	http.HandleFunc("/about", getHandler("About", "base_layout.gohtml", "about.gohtml", l))
	http.HandleFunc("/set-theme", handleSetTheme) // Endpoint for theme selection.

	log.Println("Server starting on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
