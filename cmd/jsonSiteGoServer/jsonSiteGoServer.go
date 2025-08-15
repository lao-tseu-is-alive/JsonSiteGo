package main

import (
	"encoding/json"
	"fmt"
	"github.com/lao-tseu-is-alive/JsonSiteGo/pkg/version"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xeipuuv/gojsonschema"
)

const (
	pathToTemplates       = "templates"
	initCallMsg           = "INITIAL CALL TO %s()\n"
	defaultPort           = 8888
	defaultLogName        = "stderr"
	defaultSiteConfigFile = "config.json"
	defaultSchemaFile     = "https://raw.githubusercontent.com/lao-tseu-is-alive/JsonSiteGo/refs/heads/main/config.schema.json"
	defaultReadTimeout    = 10 * time.Second // max time to read request from the client
	defaultWriteTimeout   = 10 * time.Second // max time to write response to the client
	defaultIdleTimeout    = 2 * time.Minute  // max time for connections using TCP Keep-Alive
	customContentTemplate = `
        {{define "main"}}
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
        {{end}}`
)

var (
	// templateCache holds all final, assembled templates, including error pages.
	templateCache = make(map[string]*template.Template)
)

// Route represents a parsed HTTP route.
type Route struct {
	Method string
	Path   string
}

// Author contains author information
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SiteConfig holds the overall site configuration read from the config file.
type SiteConfig struct {
	Title       string            `json:"title"`
	BaseURL     string            `json:"baseURL"`
	Language    string            `json:"language"`
	Description string            `json:"description"`
	Author      Author            `json:"author"`
	Social      map[string]string `json:"social"` // e.g., "github": "https://..."
	Footer      string            `json:"footer"`
	Pages       []Page            `json:"pages"`
}

// Page defines the structure for a single page in the website.
type Page struct {
	Route         string         `json:"route"`                   // the http Mux router like GET /page
	Title         string         `json:"title"`                   // Page-specific title
	Description   string         `json:"description,omitempty"`   // Page-specific description
	Draft         bool           `json:"draft,omitempty"`         // Don't render if true
	ErrorHttpCode string         `json:"ErrorHttpCode,omitempty"` // the actual http error template
	ErrorMsg      string         `json:"ErrorMsg,omitempty"`      // the actual http error msg
	CreateHandler bool           `json:"create_handler"`          // Should we register an handler
	ShowInMenu    bool           `json:"showInMenu"`              // Control visibility in nav
	MenuOrder     int            `json:"menuOrder,omitempty"`     // Control nav order
	Content       string         `json:"content,omitempty"`
	CustomContent []ContentBlock `json:"custom_content"`
	Template      string         `json:"template"`
	Layout        string         `json:"layout"`
}

// ContentBlock defines a generic block of content.
type ContentBlock struct {
	Type      string                 `json:"type"` // e.g., "AccordionCard", "AccordionFormGroup", "AccordionFormLabel"
	KeyValues map[string]interface{} `json:"keyValues"`
}

// PageData holds data passed to templates, including the current theme.
type PageData struct {
	Site      *SiteConfig
	Page      *Page
	Theme     string
	MenuPages []Page
}

// wantsJSON checks if the client wants a JSON response.
func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

// renderError404 serves the 404 Not Found error page using the cached template.
func renderError404(w http.ResponseWriter, r *http.Request, data PageData, l *log.Logger) {
	l.Printf("renderError404: in handler '%s' this path was not found: %v", data.Page.Route, r.URL.Path)
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"not found"}`)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	data.Page.ErrorHttpCode = "error_404"
	data.Page.ErrorMsg = fmt.Sprintf("the resource '%s' was not found.", r.URL.Path)
	tmpl, ok := templateCache["error_404"]
	if !ok {
		// Fallback in case the template is somehow missing from the cache
		http.Error(w, "Critical Error: 404 Not Found template is missing", http.StatusInternalServerError)
		return
	}
	// The menu isn't available on error pages, so we pass nil.
	err := tmpl.ExecuteTemplate(w, "base_layout", data)
	if err != nil {
		l.Printf("error in %s renderError404 doing ExecuteTemplate: %v", data.Page.Route, err)
		return
	}
}

// renderError500 serves the 500 Internal Server Error page using the cached template.
func renderError500(w http.ResponseWriter, r *http.Request, err error, data PageData, l *log.Logger) {
	l.Printf("error in %s was: %v", data.Page.Route, err)
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()))
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	data.Page.ErrorHttpCode = "error_500"
	data.Page.ErrorMsg = fmt.Sprintf("error in server %s", err.Error())
	tmpl, ok := templateCache["error_500"]
	if !ok {
		// Fallback in case the template is somehow missing from the cache
		http.Error(w, "Critical Error: 500 Internal Server Error template is missing", http.StatusInternalServerError)
		return
	}
	err = tmpl.ExecuteTemplate(w, "base_layout", data)
	if err != nil {
		l.Printf("error in %s renderError500 doing ExecuteTemplate: %v", data.Page.Route, err)
		return
	}
}

// LoadConfig validates the config file against the schema before decoding.
func LoadConfig(configPath, schemaPath string, l *log.Logger) (*SiteConfig, error) {
	var schemaLoader gojsonschema.JSONLoader
	if strings.HasPrefix(schemaPath, "https://") || strings.HasPrefix(schemaPath, "https://") {
		l.Printf("Attempting to load remote JSON schema from: %s", schemaPath)
		schemaLoader = gojsonschema.NewReferenceLoader(schemaPath)
	} else {
		if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
			l.Printf("WARNING: Local JSON schema file not found at '%s'. Skipping validation.", schemaPath)
			data, err := os.ReadFile(configPath)
			if err != nil {
				return nil, err
			}
			var config SiteConfig
			err = json.Unmarshal(data, &config)
			return &config, err
		}
		absSchemaPath, err := filepath.Abs(schemaPath)
		if err != nil {
			return nil, fmt.Errorf("could not get absolute path for schema: %w", err)
		}
		l.Printf("Loading local JSON schema from: %s", absSchemaPath)
		schemaLoader = gojsonschema.NewReferenceLoader("file://" + absSchemaPath)
	}

	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path for config: %w", err)
	}
	documentLoader := gojsonschema.NewReferenceLoader("file://" + absConfigPath)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return nil, fmt.Errorf("error during JSON schema validation: %w", err)
	}
	if !result.Valid() {
		var errorStrings []string
		errorStrings = append(errorStrings, "Configuration file is invalid. Please fix the following errors:")
		for _, desc := range result.Errors() {
			errorStrings = append(errorStrings, fmt.Sprintf("- %s: %s ", desc.Field(), desc.Description()))
		}
		l.Printf("💥💥 errors in configuration file %v", strings.Join(errorStrings, "\n"))
		return nil, fmt.Errorf("💥💥 errors in configuration file")
	}
	l.Println("✅ Configuration file validated successfully against schema.")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var config SiteConfig
	err = json.Unmarshal(data, &config)
	return &config, err
}

// getPortFromEnvOrPanic returns a valid TCP/IP port from the environment or a default.
func getPortFromEnvOrPanic(defaultPort int) int {
	srvPort := defaultPort
	if val, exist := os.LookupEnv("PORT"); exist {
		if p, err := strconv.Atoi(val); err == nil {
			srvPort = p
		} else {
			panic(fmt.Errorf("💥💥 ERROR: CONFIG ENV PORT should contain a valid integer. %v", err))
		}
	}
	if srvPort < 1 || srvPort > 65535 {
		panic(fmt.Errorf("💥💥 ERROR: PORT should contain an integer between 1 and 65535"))
	}
	return srvPort
}

// GetLogWriterFromEnvOrPanic returns the name of the filename to use for LOG from the content of the env variable :
// LOG_FILE : string containing the filename to use for LOG, use DISCARD for no log, default is STDERR
func GetLogWriterFromEnvOrPanic(defaultLogName string) io.Writer {
	logFileName := defaultLogName
	val, exist := os.LookupEnv("LOG_FILE")
	if exist {
		logFileName = val
	}
	if utf8.RuneCountInString(logFileName) < 5 {
		panic(fmt.Sprintf("💥💥 error env LOG_FILE filename should contain at least %d characters (got %d).",
			5, utf8.RuneCountInString(val)))
	}
	switch logFileName {
	case "stdout":
		return os.Stdout
	case "stderr":
		return os.Stderr
	case "DISCARD":
		return io.Discard
	default:
		// Open the file with append, create, and write permissions.
		// The 0644 permission allows the owner to read/write and others to read.
		file, err := os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			// Return an error if the file cannot be opened (e.g., due to permissions).
			panic(fmt.Sprintf("💥💥 ERROR: LOG_FILE %q could not be open : %v", logFileName, err))
		}
		return file
	}
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
	theme := "light"
	if getThemeFromCookie(r) == "light" {
		theme = "dark"
	}
	http.SetCookie(w, &http.Cookie{Name: "theme", Value: theme, Path: "/"})
	referer := r.Referer()
	if referer == "" {
		referer = "/"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

// parseTemplates creates the template cache at startup for all pages and error types.
func parseTemplates(config *SiteConfig, l *log.Logger) error {
	l.Println("🚀 Caching templates...")
	funcMap := template.FuncMap{
		"replace": strings.ReplaceAll,
		"splitFirst": func(s string) string {
			parts := strings.Split(strings.TrimSpace(s), " ")
			if len(parts) > 1 {
				return parts[1]
			}
			return ""
		},
		"default": func(fallback, value string) string {
			if value == "" {
				return fallback
			}
			return value
		},
	}

	// 1. Parse all base and component files into a master template set.
	baseTemplate, err := template.New("base").Funcs(funcMap).ParseFiles(
		filepath.Join(pathToTemplates, "base_layout.gohtml"),
		filepath.Join(pathToTemplates, "header.gohtml"),
		filepath.Join(pathToTemplates, "footer.gohtml"),
		filepath.Join(pathToTemplates, "errors", "error_500.gohtml"),
		filepath.Join(pathToTemplates, "errors", "error_404.gohtml"),
	)
	if err != nil {
		return fmt.Errorf("error parsing base templates: %w", err)
	}

	_, err = baseTemplate.ParseGlob(filepath.Join(pathToTemplates, "components", "*.gohtml"))
	if err != nil {
		return fmt.Errorf("error parsing component templates: %w", err)
	}

	// 2. Iterate through pages to build and cache a specific template for each route.
	for _, page := range config.Pages {
		if !page.CreateHandler || page.Draft {
			continue
		}
		tmpl, err := baseTemplate.Clone()
		if err != nil {
			return fmt.Errorf("error cloning base template for route %s: %w", page.Route, err)
		}

		if page.CustomContent != nil {
			/* maybe : build the template based on available components ?
			var sb strings.Builder
			sb.WriteString(`{{define "main"}}<main class="container"><h1>{{.Page.Title}}</h1>`)
			for _, block := range page.CustomContent {
				sb.WriteString(fmt.Sprintf(`{{template "%s" .}}`, block.Type))
			}
			sb.WriteString(`</main>{{end}}`)
			_, err = tmpl.Parse(sb.String())

			*/
			_, err = tmpl.Parse(customContentTemplate)
			if err != nil {
				return fmt.Errorf("error parsing custom content template for route %s: %w", page.Route, err)
			}
		} else if strings.TrimSpace(page.Template) != "" {
			pageTemplatePath := filepath.Join(pathToTemplates, page.Template)
			_, err = tmpl.ParseFiles(pageTemplatePath)
			if err != nil {
				return fmt.Errorf("error parsing page template %s for route %s: %w", pageTemplatePath, page.Route, err)
			}
		}
		templateCache[page.Route] = tmpl
		l.Printf("✅ Template cached for route: %s", page.Route)
	}
	// Cache the error pages.
	// Cache 404
	tmpl404, err := baseTemplate.Clone()
	if err != nil {
		return fmt.Errorf("error cloning base template for 404 page: %w", err)
	}
	_, err = tmpl404.ParseFiles(filepath.Join(pathToTemplates, "errors", "error_404.gohtml"))
	if err != nil {
		return fmt.Errorf("error parsing 404 template: %w", err)
	}
	templateCache["error_404"] = tmpl404
	l.Printf("✅ Template cached for: error_404")
	// Cache 500
	tmpl500, err := baseTemplate.Clone()
	if err != nil {
		return fmt.Errorf("error cloning base template for 500 page: %w", err)
	}
	_, err = tmpl500.ParseFiles(filepath.Join(pathToTemplates, "errors", "error_500.gohtml"))
	if err != nil {
		return fmt.Errorf("error parsing 500 template: %w", err)
	}
	templateCache["error_500"] = tmpl500
	l.Printf("✅ Template cached for: error_500")

	return nil
}

// getHandler creates a generic HTTP handler for a given page.
func getHandler(page *Page, site *SiteConfig, l *log.Logger) http.HandlerFunc {
	l.Printf(initCallMsg, page.Title)
	parts := strings.Split(strings.TrimSpace(page.Route), " ")
	route := Route{
		Method: parts[0],
		Path:   parts[1],
	}
	var menuPages []Page
	for _, p := range site.Pages {
		if !p.Draft && p.ShowInMenu {
			menuPages = append(menuPages, p)
		}
	}
	sort.Slice(menuPages, func(i, j int) bool {
		return menuPages[i].MenuOrder < menuPages[j].MenuOrder
	})

	return func(w http.ResponseWriter, r *http.Request) {
		l.Printf("in handler '%s' url: %s", page.Route, r.URL.Path)
		data := PageData{
			Site:      site,
			Page:      page,
			Theme:     getThemeFromCookie(r),
			MenuPages: menuPages,
		}
		if r.URL.Path != route.Path {
			l.Printf("💥 requested path %s is not here...", r.URL.Path)
			renderError404(w, r, data, l)
			return
		}
		myTemplate, ok := templateCache[page.Route]
		if !ok {
			err := fmt.Errorf("template for route '%s' not found in cache", page.Route)
			renderError500(w, r, err, data, l)
			return
		}
		err := myTemplate.ExecuteTemplate(w, "base_layout", data)
		if err != nil {
			l.Printf("💥💥 error in template execution err: %v ", err)
			renderError500(w, r, fmt.Errorf("template execution failed for %s: %w", page.Route, err), data, l)
		}
	}
}

func main() {
	l := log.New(GetLogWriterFromEnvOrPanic(defaultLogName), fmt.Sprintf("%s, ", version.APP), log.Ldate|log.Ltime|log.Lshortfile)
	l.Printf("🚀🚀 Starting App: %s, version: %s, build: %s", version.APP, version.VERSION, version.BuildStamp)

	config, err := LoadConfig(defaultSiteConfigFile, defaultSchemaFile, l)
	if err != nil {
		l.Fatalf("💥💥 fatal error loading config file: %v", err)
	}

	// A single call to parse and cache all templates.
	if err := parseTemplates(config, l); err != nil {
		l.Fatalf("💥💥 fatal error caching templates: %v", err)
	}

	myServerMux := http.NewServeMux()
	listenAddress := fmt.Sprintf(":%d", getPortFromEnvOrPanic(defaultPort))

	myServerMux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./favicon.ico")
	})

	for i := range config.Pages {
		page := &config.Pages[i]
		if page.CreateHandler && !page.Draft {
			myServerMux.Handle(page.Route, getHandler(page, config, l))
		}
	}
	myServerMux.HandleFunc("GET /set-theme", handleSetTheme)

	server := http.Server{
		Addr:         listenAddress,
		Handler:      myServerMux,
		ErrorLog:     l,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
		IdleTimeout:  defaultIdleTimeout,
	}

	l.Printf("Server starting on http://localhost%s", listenAddress)
	if err := server.ListenAndServe(); err != nil {
		l.Fatalf("💥💥 Server failed to start: %v", err)
	}
}
