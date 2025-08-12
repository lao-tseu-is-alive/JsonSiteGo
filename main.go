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
	Type      string            `json:"type"` // e.g., "AccordionCard", "AccordionFormGroup", "AccordionFormLabel"
	KeyValues map[string]string `json:"keyValues"`
}

var CustomContentTemplates = map[string]string{
	"AccordionCard": `
{{define "main"}}
    <main class="container">
    <h1>{{- /*gotype: github.com/lao-tseu-is-alive/go-simple-http-static-server.PageData*/ -}}
        {{.Page.Title}}</h1>
    {{ range .Page.CustomContent }}
        {{if eq .Type "AccordionCard"}}
			{{ with .KeyValues }}
            <details name="AccordionCard">
                <summary>{{.SummaryContent}}</summary>
                <div class="grid">
                    <div>
                        <article>
                            <header><strong>{{.Article1Title}}</strong></header>
                            {{.Article1Text}}
                        </article>
                    </div>
                    <div>
                        <article>
                            <header><strong>{{.Article2Title}}</strong></header>
                            {{.Article2Text}}
                        </article>
                    </div>
                </div>
            </details>
            {{ end }}
        {{ else }}
            <article>
                <header><strong>Error unhandled custom Type : {{.Type}} </strong></header>
                Sorry but this typw of custom Content :{{.Type}}, is not supported.
            </article>
        {{ end }}
    {{end}}
    </main>
{{end}}
`,
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
	// Initialize templates
	templates := template.New(page.Layout) //should match the name of the define

	// Add custom functions if any
	templates.Funcs(template.FuncMap{
		"replace": strings.ReplaceAll,
		"splitFirst": func(s string) string {
			parts := strings.Split(strings.TrimSpace(s), " ")
			if len(parts) > 0 {
				return parts[1]
			}
			return ""
		},
	})

	// Charger le layout + la page
	if strings.TrimSpace(page.Template) != "" && page.CustomContent == nil {
		_, err := templates.ParseFiles(
			filepath.Join(pathToTemplates, "header.gohtml"),
			filepath.Join(pathToTemplates, "footer.gohtml"),
			filepath.Join(pathToTemplates, fmt.Sprintf("%s.gohtml", page.Layout)),
			filepath.Join(pathToTemplates, page.Template),
		)
		if err != nil {
			l.Printf(" renderLayoutTemplate parse error : %v", err)
			return nil, err
		}
	} else {
		_, err := templates.ParseFiles(
			filepath.Join(pathToTemplates, "header.gohtml"),
			filepath.Join(pathToTemplates, "footer.gohtml"),
			filepath.Join(pathToTemplates, fmt.Sprintf("%s.gohtml", page.Layout)),
		)
		if err != nil {
			l.Printf(" renderLayoutTemplate parse error : %v", err)
			return nil, err
		}
		if page.CustomContent != nil {
			customContent := page.CustomContent
			for i := 0; i < len(customContent); i++ {
				if customContent[i].Type == "AccordionCard" {
					ccTemplate := CustomContentTemplates["AccordionCard"]
					_, err := templates.Parse(ccTemplate)
					if err != nil {
						return nil, err
					}
				}
			}
		} else {
			return nil, fmt.Errorf("error in page %#v, err: template and customcontent cannot be both nil", page)
		}

	}
	// Log all defined templates
	l.Println(templates.DefinedTemplates())
	return templates, nil
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
