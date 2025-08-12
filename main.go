package main

import (
	"encoding/json"
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
	pathToTemplates       = "templates"
	AppName               = "goHttpServer"
	AppVersion            = "v0.0.1"
	initCallMsg           = "INITIAL CALL TO %s()\n"
	defaultPort           = 8888
	defaultSiteConfigFile = "config.json"
	defaultReadTimeout    = 10 * time.Second // max time to read request from the client
	defaultWriteTimeout   = 10 * time.Second // max time to write response to the client
	defaultIdleTimeout    = 2 * time.Minute  // max time for connections using TCP Keep-Alive
)

// Route represents a parsed HTTP route.
type Route struct {
	Method string
	Path   string
}

// SiteConfig holds the overall site configuration read from the config file.
type SiteConfig struct {
	Title  string `json:"title"`
	Footer string `json:"footer"`
	Pages  []Page `json:"pages"`
}

// Page defines the structure for a single page in the website.
type Page struct {
	Route         string         `json:"route"`
	Title         string         `json:"title"`
	Content       string         `json:"content"`
	CustomContent []ContentBlock `json:"custom_content"`
	Template      string         `json:"template"`
	Layout        string         `json:"layout"`
	NeedsHandler  bool           `json:"needs_handler"`
}

// ContentBlock defines a generic block of content.
type ContentBlock struct {
	Type      string                 `json:"type"` // e.g., "AccordionCard", "AccordionFormGroup", "AccordionFormLabel"
	KeyValues map[string]interface{} `json:"keyValues"`
}

// PageData holds data passed to templates, including the current theme.
type PageData struct {
	Site  *SiteConfig
	Page  *Page
	Theme string
}

// LoadConfig reads a configuration file and decodes it into a SiteConfig struct.
func LoadConfig(filename string) (*SiteConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config SiteConfig
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
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

func renderLayoutTemplate(page *Page, l *log.Logger) (*template.Template, error) {
	l.Printf("in renderLayoutTemplate(layout:%s, page:%s)", page.Layout, page.Template)

	layoutPath := filepath.Join(pathToTemplates, fmt.Sprintf("%s.gohtml", page.Layout))
	tmpl := template.New(page.Layout).Funcs(template.FuncMap{
		"replace": strings.ReplaceAll,
		"splitFirst": func(s string) string {
			parts := strings.Split(strings.TrimSpace(s), " ")
			if len(parts) > 1 {
				return parts[1]
			}
			return ""
		},
	})

	_, err := tmpl.ParseFiles(
		layoutPath,
		filepath.Join(pathToTemplates, "header.gohtml"),
		filepath.Join(pathToTemplates, "footer.gohtml"),
	)
	if err != nil {
		return nil, fmt.Errorf("error parsing layout/partial templates: %w", err)
	}

	if page.CustomContent != nil {
		componentTemplates, err := filepath.Glob(filepath.Join(pathToTemplates, "components", "*.gohtml"))
		if err != nil {
			return nil, fmt.Errorf("error finding component templates: %w", err)
		}
		if len(componentTemplates) > 0 {
			_, err = tmpl.ParseFiles(componentTemplates...)
			if err != nil {
				return nil, fmt.Errorf("error parsing component templates: %w", err)
			}
		}

		// This is the corrected part.
		// We use `if/else if` to check the component type and call the template with a string literal.
		_, err = tmpl.Parse(`{{define "main"}}
            <main class="container">
                <h1>{{.Page.Title}}</h1>
                {{range .Page.CustomContent}}
                    {{if eq .Type "AccordionCard"}}
                        {{template "AccordionCard" .}}
                    {{else if eq .Type "AccordionFormGroup"}}
                        {{template "AccordionFormGroup" .}}
                    
                    {{else}}
                        <article>
                            <header><strong>Unsupported Component</strong></header>
                            <p>Error: The component type '{{.Type}}' is not supported.</p>
                        </article>
                    {{end}}
                {{end}}
            </main>
        {{end}}`)
		if err != nil {
			return nil, fmt.Errorf("error parsing dynamic main template for custom content: %w", err)
		}

	} else if strings.TrimSpace(page.Template) != "" {
		pageTemplatePath := filepath.Join(pathToTemplates, page.Template)
		_, err = tmpl.ParseFiles(pageTemplatePath)
		if err != nil {
			return nil, fmt.Errorf("error parsing page template %s: %w", pageTemplatePath, err)
		}
	}

	l.Println("--- Defined Templates ---")
	l.Println(tmpl.DefinedTemplates())
	l.Println("-------------------------")
	return tmpl, nil
}

// getHandler functions for each page (similar structure).
func getHandler(page *Page, site *SiteConfig, l *log.Logger) http.HandlerFunc {
	l.Printf(initCallMsg, page.Title)
	myTemplate, err := renderLayoutTemplate(page, l)
	if err != nil {
		l.Fatalf("💥💥 fatal error in renderLayoutTemplate err: %v ", err)
	}
	parts := strings.Split(strings.TrimSpace(page.Route), " ")
	// Create an instance of the Route struct
	route := Route{
		Method: parts[0],
		Path:   parts[1],
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != route.Path {
			l.Printf("💥 requested path %s is not here...", r.URL.Path)
			http.Error(w, fmt.Errorf("requested path %s not found", r.URL.Path).Error(), http.StatusBadRequest)
			return
		}
		l.Printf("in handler '%s' url: %s", page.Route, r.URL.Path)
		data := PageData{Site: site, Page: page, Theme: getThemeFromCookie(r)}
		l.Printf("data Page: %+v , site %+v", data.Page, data.Site)
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

	config, err := LoadConfig(defaultSiteConfigFile)
	if err != nil {
		l.Fatalf("💥💥 fatal error loading config file: %v", err)
	}
	myServerMux := http.NewServeMux()
	listenAddress := fmt.Sprintf(":%d", getPortFromEnvOrPanic(defaultPort))
	myServerMux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		l.Printf("in my handler 'GET /favicon.ico' url: %s", r.URL.Path)
		http.ServeFile(w, r, "./favicon.ico")
	})

	// Dynamically register handlers based on the configuration.
	for i := range config.Pages {
		page := &config.Pages[i]
		if page.NeedsHandler {
			myServerMux.Handle(page.Route, getHandler(page, config, l))
		}
	}
	myServerMux.HandleFunc("POST /set-theme", handleSetTheme) // Endpoint for theme selection.
	// Handler for the favicon.ico request

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
