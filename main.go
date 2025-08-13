package main

import (
	"encoding/json"
	"fmt"
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

	"github.com/xeipuuv/gojsonschema"
)

const (
	pathToTemplates       = "templates"
	AppName               = "goMagicWebServer"
	AppGitHub             = "https://github.com/lao-tseu-is-alive/JsonSiteGo.git"
	AppVersion            = "v0.2.0"
	initCallMsg           = "INITIAL CALL TO %s()\n"
	defaultPort           = 8888
	defaultSiteConfigFile = "config.json"
	defaultSchemaFile     = "https://raw.githubusercontent.com/lao-tseu-is-alive/JsonSiteGo/refs/heads/main/config.schema.json"
	defaultReadTimeout    = 10 * time.Second // max time to read request from the client
	defaultWriteTimeout   = 10 * time.Second // max time to write response to the client
	defaultIdleTimeout    = 2 * time.Minute  // max time for connections using TCP Keep-Alive
)

var (
	//create our cache to hold the final, assembled templates for each route.
	templateCache = make(map[string]*template.Template)
	error404Tmpl  *template.Template
	error500Tmpl  *template.Template
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
	Route         string         `json:"route"`                 // the http Mux router like GET /page
	Title         string         `json:"title"`                 // Page-specific title
	Description   string         `json:"description,omitempty"` // Page-specific description
	Draft         bool           `json:"draft,omitempty"`       // Don't render if true
	CreateHandler bool           `json:"create_handler"`        // Should we register an handler
	ShowInMenu    bool           `json:"showInMenu"`            // Control visibility in nav
	MenuOrder     int            `json:"menuOrder,omitempty"`   // Control nav order
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

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func loadErrorTemplates() error {
	var err error
	error404Tmpl, err = template.ParseFiles(
		filepath.Join(pathToTemplates, "errors", "error_404.gohtml"),
		filepath.Join(pathToTemplates, "base_layout.gohtml"),
		filepath.Join(pathToTemplates, "header.gohtml"),
		filepath.Join(pathToTemplates, "footer.gohtml"),
	)
	if err != nil {
		return err
	}
	error500Tmpl, err = template.ParseFiles(
		filepath.Join(pathToTemplates, "errors", "error_500.gohtml"),
		filepath.Join(pathToTemplates, "base_layout.gohtml"),
		filepath.Join(pathToTemplates, "header.gohtml"),
		filepath.Join(pathToTemplates, "footer.gohtml"),
	)
	return err
}

func renderError404(w http.ResponseWriter, r *http.Request) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"not found"}`)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	error404Tmpl.ExecuteTemplate(w, "base_layout", nil)
}

func renderError500(w http.ResponseWriter, r *http.Request, err error) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()))
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	error500Tmpl.ExecuteTemplate(w, "base_layout", map[string]interface{}{
		"Error": err.Error(),
	})
}

// LoadConfig now validates the config file against the schema before decoding.
// configPath is the path to the config file to load
// schemaPath is a local or remote schemas to validate
func LoadConfig(configPath, schemaPath string) (*SiteConfig, error) {
	var schemaLoader gojsonschema.JSONLoader

	// Determine if the schema path is a remote URL or a local file
	if strings.HasPrefix(schemaPath, "https://") || strings.HasPrefix(schemaPath, "https://") {
		log.Printf("Attempting to load remote JSON schema from: %s", schemaPath)
		schemaLoader = gojsonschema.NewReferenceLoader(schemaPath)
	} else {
		// It's a local file, check if it exists
		if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
			log.Printf("WARNING: Local JSON schema file not found at '%s'. Skipping validation.", schemaPath)
			// If schema is not found, we skip validation and proceed
			data, err := os.ReadFile(configPath)
			if err != nil {
				return nil, err
			}
			var config SiteConfig
			err = json.Unmarshal(data, &config)
			return &config, err // Return here, no validation to perform
		}
		// It's a local file that exists, get its absolute path
		absSchemaPath, err := filepath.Abs(schemaPath)
		if err != nil {
			return nil, fmt.Errorf("could not get absolute path for schema: %w", err)
		}
		log.Printf("Loading local JSON schema from: %s", absSchemaPath)
		schemaLoader = gojsonschema.NewReferenceLoader("file://" + absSchemaPath)
	}

	// The document to validate is always a local file, so we still need its absolute path
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path for config: %w", err)
	}
	documentLoader := gojsonschema.NewReferenceLoader("file://" + absConfigPath)

	// Perform validation
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
		return nil, fmt.Errorf(strings.Join(errorStrings, "\n"))
	}
	log.Println("✅ Configuration file validated successfully against schema.")

	// If valid, unmarshal the config file into the struct
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config SiteConfig
	err = json.Unmarshal(data, &config)
	return &config, err
}

// getPortFromEnvOrPanic returns a valid TCP/IP listening port based on the values of environment variable :
// PORT : int value between 1 and 65535 (the parameter defaultPort will be used if env is not defined)
// in case the ENV variable PORT exists and contains an invalid integer the functions panics
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
		theme = getThemeFromCookie(r)
	}
	if theme == "light" {
		theme = "dark"
	} else {
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

func parseTemplates(page *Page, l *log.Logger) (*template.Template, error) {
	l.Printf("in parseTemplates(layout:%s, page:%s)", page.Layout, page.Template)

	// Define FuncMap with custom functions
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
	layoutPath := filepath.Join(pathToTemplates, fmt.Sprintf("%s.gohtml", page.Layout))
	tmpl := template.New(page.Layout).Funcs(funcMap)

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

// getHandler allow to register http handler in a generic way
func getHandler(page *Page, site *SiteConfig, l *log.Logger) http.HandlerFunc {
	l.Printf(initCallMsg, page.Title)
	myTemplate, err := parseTemplates(page, l)
	if err != nil {
		l.Fatalf("💥💥 fatal error in parseTemplates err: %v ", err)
	}
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
		if r.URL.Path != route.Path {
			l.Printf("💥 requested path %s is not here...", r.URL.Path)
			renderError404(w, r)
			return
		}
		l.Printf("in handler '%s' url: %s", page.Route, r.URL.Path)
		data := PageData{
			Site:      site,
			Page:      page,
			Theme:     getThemeFromCookie(r),
			MenuPages: menuPages,
		}

		l.Printf("data Page: %+v , site %+v", data.Page, data.Site)
		err = myTemplate.Execute(w, data)
		if err != nil {
			l.Printf("💥💥 fatal error in template execution err: %v ", err)
			renderError500(w, r, err)
		}
	}
}

func main() {
	l := log.New(os.Stderr, AppName, log.Ldate|log.Ltime|log.Lshortfile)
	l.Printf("🚀🚀 Starting App: %s, version: %s, from: %s", AppName, AppVersion, AppGitHub)

	config, err := LoadConfig(defaultSiteConfigFile, defaultSchemaFile)
	if err != nil {
		l.Fatalf("💥💥 fatal error loading config file: %v", err)
	}
	myServerMux := http.NewServeMux()
	listenAddress := fmt.Sprintf(":%d", getPortFromEnvOrPanic(defaultPort))

	err = loadErrorTemplates()
	if err != nil {
		l.Fatalf("💥💥 fatal error loading templates files: %v", err)
	}

	myServerMux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		l.Printf("in my handler 'GET /favicon.ico' url: %s", r.URL.Path)
		http.ServeFile(w, r, "./favicon.ico")
	})

	// Dynamically register handlers based on the configuration.
	for i := range config.Pages {
		page := &config.Pages[i]
		if page.CreateHandler && !page.Draft {
			myServerMux.Handle(page.Route, getHandler(page, config, l))
		}
	}
	myServerMux.HandleFunc("POST /set-theme", handleSetTheme)
	myServerMux.HandleFunc("GET /set-theme", handleSetTheme)

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
