package main

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
)

// PageData holds data passed to templates, including the current theme.
type PageData struct {
	Title     string
	Theme     string // "light" or "dark"
	MainBlock string
}

var templates *template.Template

func main() {
	// Initialize templates
	templates = template.New("main_template")

	// Add custom functions if any
	// templates.Funcs(template.FuncMap{})

	// Parse all template files
	_, err := templates.ParseGlob(filepath.Join("templates", "*.gohtml"))
	if err != nil {
		log.Fatalf("Error parsing templates: %v", err)
	}
	log.Println(templates.DefinedTemplates()) // Log all defined templates

	// Define HTTP handlers for each page.
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/product1", handleProduct1)
	http.HandleFunc("/product2", handleProduct2)
	http.HandleFunc("/contact", handleContact)
	http.HandleFunc("/about", handleAbout)
	http.HandleFunc("/set-theme", handleSetTheme) // Endpoint for theme selection.

	log.Println("Server starting on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
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

// renderTemplate is a helper function to render templates.
func renderTemplate(w http.ResponseWriter, tmpl string, data PageData) {
	// We execute the base layout, which will in turn call the correct "main" template.
	// The key is that we are executing a specific template file that defines the content.
	err := templates.ExecuteTemplate(w, tmpl, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Handler functions for each page (similar structure).
func handleIndex(w http.ResponseWriter, r *http.Request) {
	data := PageData{Title: "Main Page", Theme: getThemeFromCookie(r), MainBlock: "index_main"}
	renderTemplate(w, "index.gohtml", data)
}

func handleProduct1(w http.ResponseWriter, r *http.Request) {
	data := PageData{Title: "Product 1", Theme: getThemeFromCookie(r), MainBlock: "product1_main"}
	renderTemplate(w, "product1.gohtml", data)
}

func handleProduct2(w http.ResponseWriter, r *http.Request) {
	data := PageData{Title: "Product 2", Theme: getThemeFromCookie(r), MainBlock: "product2_main"}
	renderTemplate(w, "product2.gohtml", data)
}

func handleContact(w http.ResponseWriter, r *http.Request) {
	data := PageData{Title: "Contact", Theme: getThemeFromCookie(r), MainBlock: "contact_main"}
	renderTemplate(w, "contact.gohtml", data)
}

func handleAbout(w http.ResponseWriter, r *http.Request) {
	data := PageData{Title: "About Us", Theme: getThemeFromCookie(r), MainBlock: "about_main"}
	renderTemplate(w, "about.gohtml", data)
}
