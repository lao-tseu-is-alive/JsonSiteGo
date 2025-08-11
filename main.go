package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	pathToTemplates     = "templates"
	AppName             = "goHttpServer"
	AppVersion          = "v0.0.1"
	initCallMsg         = "INITIAL CALL TO %s()\n"
	defaultPort         = 8888
	defaultReadTimeout  = 10 * time.Second // max time to read request from the client
	defaultWriteTimeout = 10 * time.Second // max time to write response to the client
	defaultIdleTimeout  = 2 * time.Minute  // max time for connections using TCP Keep-Alive
)

type SiteData struct {
	Title  string `json:"title"`
	Footer string `json:"footer"`
	Layout string `json:"layout"`
}

// PageData holds data passed to templates, including the current theme.
type PageData struct {
	Site  SiteData
	Title string
	Theme string // "light" or "dark"
}

// getPortFromEnvOrPanic returns a valid TCP/IP listening port based on the values of environment variable :
//
//	PORT : int value between 1 and 65535 (the parameter defaultPort will be used if env is not defined)
//	 in case the ENV variable PORT exists and contains an invalid integer the functions panics
func getPortFromEnvOrPanic(defaultPort int) int {
	srvPort := defaultPort
	var err error
	val, exist := os.LookupEnv("PORT")
	if exist {
		srvPort, err = strconv.Atoi(val)
		if err != nil {
			panic(fmt.Errorf("💥💥 ERROR: CONFIG ENV PORT should contain a valid integer. %v", err))
		}
	}
	if srvPort < 1 || srvPort > 65535 {
		panic(fmt.Errorf("💥💥 ERROR: PORT should contain an integer between 1 and 65535. Err: %v", err))
	}
	return srvPort
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
	templates := template.New(layout) //should match the name of the define

	// Add custom functions if any
	templates.Funcs(template.FuncMap{
		"replace": strings.ReplaceAll,
	})

	// Charger le layout + la page
	_, err := templates.ParseFiles(
		filepath.Join(pathToTemplates, "header.gohtml"),
		filepath.Join(pathToTemplates, "footer.gohtml"),
		filepath.Join(pathToTemplates, fmt.Sprintf("%s.gohtml", layout)),
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

// getHandler functions for each page (similar structure).
func getHandler(handlerName, page string, site *SiteData, l *log.Logger) http.HandlerFunc {
	l.Printf(initCallMsg, handlerName)
	myTemplate, err := renderLayoutTemplate(handlerName, site.Layout, page, l)
	if err != nil {
		l.Fatalf("💥💥 fatal error in renderLayoutTemplate err: %v ", err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		data := PageData{Title: handlerName, Site: *site, Theme: getThemeFromCookie(r)}
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

	mySite := SiteData{
		Title:  AppName,
		Footer: "&copy; 2025 Simple Personal Web Demo. All rights reserved.",
		Layout: "base_layout",
	}

	myServerMux := http.NewServeMux()
	listenAddress := fmt.Sprintf(":%d", getPortFromEnvOrPanic(defaultPort))

	// Define HTTP handlers for each page.
	myServerMux.Handle("GET /", getHandler("Main", "index.gohtml", &mySite, l))
	myServerMux.Handle("GET /product1", getHandler("Product 1", "product1.gohtml", &mySite, l))
	myServerMux.Handle("GET /product2", getHandler("Product 2", "product2.gohtml", &mySite, l))
	myServerMux.Handle("GET /contact", getHandler("Contact", "contact.gohtml", &mySite, l))
	myServerMux.Handle("GET /about", getHandler("About", "about.gohtml", &mySite, l))
	myServerMux.HandleFunc("POST /set-theme", handleSetTheme) // Endpoint for theme selection.

	server := http.Server{
		Addr:         listenAddress,       // configure the bind address
		Handler:      myServerMux,         // set the http mux
		ErrorLog:     l,                   // set the logger for the server
		ReadTimeout:  defaultReadTimeout,  // max time to read request from the client
		WriteTimeout: defaultWriteTimeout, // max time to write response to the client
		IdleTimeout:  defaultIdleTimeout,  // max time for connections using TCP Keep-Alive
	}

	log.Printf("Server starting on http://localhost%s", listenAddress)
	log.Fatal(server.ListenAndServe())
}
